// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package githubauth

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	gh "github.com/canonical/sunbeam-watchtower/pkg/github/v1"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func authJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAuthenticatorBeginDeviceFlow(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != deviceCodeURL {
			t.Fatalf("unexpected URL: %s", req.URL.String())
		}
		if err := req.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := req.FormValue("client_id"); got != "client-id" {
			t.Fatalf("client_id = %q, want client-id", got)
		}
		return authJSONResponse(http.StatusOK, `{"device_code":"device","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","expires_in":900,"interval":5}`), nil
	})}

	flow, err := NewAuthenticator("client-id", discardLogger(), client).BeginDeviceFlow(context.Background())
	if err != nil {
		t.Fatalf("BeginDeviceFlow() error = %v", err)
	}
	if flow.DeviceCode != "device" || flow.UserCode != "ABCD-EFGH" {
		t.Fatalf("unexpected flow: %+v", flow)
	}
}

func TestAuthenticatorPollAccessToken(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return authJSONResponse(http.StatusOK, `{"access_token":"token","token_type":"bearer","scope":""}`), nil
	})}

	creds, err := NewAuthenticator("client-id", discardLogger(), client).PollAccessToken(context.Background(), &gh.PendingAuthFlow{
		DeviceCode:      "device",
		IntervalSeconds: 1,
	})
	if err != nil {
		t.Fatalf("PollAccessToken() error = %v", err)
	}
	if creds.AccessToken != "token" {
		t.Fatalf("unexpected credentials: %+v", creds)
	}
}
