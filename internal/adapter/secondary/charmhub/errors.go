// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package charmhub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
)

// ErrUnauthorized signals that Charmhub rejected the request because the
// macaroon credentials are missing, expired, or lack the required
// permissions. It wraps port.ErrStoreAuthExpired so callers that don't
// import this package can still branch on the cross-store sentinel.
var ErrUnauthorized = port.ErrStoreAuthExpired

// errorBodyCap bounds how much of a non-2xx body we will read to avoid
// unbounded allocations on a misbehaving server.
const errorBodyCap = 64 * 1024

// HTTPError is the typed error produced by decodeHTTPError for any non-2xx
// response from Charmhub. It carries the original status code plus the
// decoded per-error code/message pairs when the body matched one of the
// known shapes.
type HTTPError struct {
	StatusCode int
	Codes      []string
	Messages   []string
	// Raw holds the (bounded) response body text when neither known JSON
	// shape matched; empty otherwise.
	Raw string
	// authExpired is true when the status code or any decoded code maps to
	// an authentication problem, so Unwrap can expose ErrUnauthorized.
	authExpired bool
}

func (e *HTTPError) Error() string {
	var detail string
	switch {
	case len(e.Messages) > 0:
		parts := make([]string, 0, len(e.Messages))
		for i, msg := range e.Messages {
			if i < len(e.Codes) && e.Codes[i] != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", e.Codes[i], msg))
			} else {
				parts = append(parts, msg)
			}
		}
		detail = strings.Join(parts, "; ")
	case len(e.Codes) > 0:
		detail = strings.Join(e.Codes, "; ")
	case e.Raw != "":
		detail = e.Raw
	}
	if detail == "" {
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, detail)
}

// Unwrap lets errors.Is(err, ErrUnauthorized) / port.ErrStoreAuthExpired
// succeed for authentication-class failures without losing the rich
// status/code/message context on the wrapping HTTPError.
func (e *HTTPError) Unwrap() error {
	if e.authExpired {
		return port.ErrStoreAuthExpired
	}
	return nil
}

// charmhubErrorList and charmhubErrorSingle model the two documented error
// body shapes the Canonical store family returns.
type charmhubErrorList struct {
	ErrorList []charmhubErrorItem `json:"error-list"`
}

type charmhubErrorSingle struct {
	Error charmhubErrorItem `json:"error"`
}

type charmhubErrorItem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// authErrorCodes are the documented codes Charmhub/Snap Store return when
// the macaroon is missing, expired, or otherwise rejected. We match on
// substrings to tolerate minor naming variations across endpoints.
var authErrorCodes = []string{
	"macaroon-authorization-required",
	"macaroon-needs-refresh",
	"macaroon-expired",
	"macaroon-invalid",
	"permission-required",
	"authorization-required",
	"unauthorized",
}

func isAuthErrorCode(code string) bool {
	c := strings.ToLower(code)
	for _, want := range authErrorCodes {
		if c == want || strings.Contains(c, want) {
			return true
		}
	}
	return false
}

// decodeHTTPError reads the (bounded) body of a non-2xx response and
// returns a typed *HTTPError. Callers wrap the returned error with their
// own operation context; the returned error already carries status +
// decoded codes/messages and is Unwrap()-able to ErrUnauthorized when the
// failure is an authentication problem.
func decodeHTTPError(resp *http.Response) *HTTPError {
	out := &HTTPError{StatusCode: resp.StatusCode}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyCap))
	trimmed := strings.TrimSpace(string(body))

	if trimmed != "" {
		var list charmhubErrorList
		if err := json.Unmarshal([]byte(trimmed), &list); err == nil && len(list.ErrorList) > 0 {
			for _, item := range list.ErrorList {
				out.Codes = append(out.Codes, item.Code)
				out.Messages = append(out.Messages, item.Message)
			}
		} else {
			var single charmhubErrorSingle
			if err := json.Unmarshal([]byte(trimmed), &single); err == nil && (single.Error.Code != "" || single.Error.Message != "") {
				out.Codes = append(out.Codes, single.Error.Code)
				out.Messages = append(out.Messages, single.Error.Message)
			} else {
				out.Raw = trimmed
			}
		}
	}

	if resp.StatusCode == http.StatusUnauthorized {
		out.authExpired = true
	}
	for _, code := range out.Codes {
		if isAuthErrorCode(code) {
			out.authExpired = true
			break
		}
	}

	return out
}

var (
	_ error                  = (*HTTPError)(nil)
	_ func(*HTTPError) error = (*HTTPError).Unwrap
)
