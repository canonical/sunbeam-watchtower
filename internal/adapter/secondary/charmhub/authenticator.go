// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package charmhub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	sa "github.com/canonical/sunbeam-watchtower/pkg/storeauth/v1"
)

const (
	defaultTokensEndpoint   = "https://api.charmhub.io/v1/tokens"
	defaultExchangeEndpoint = "https://api.charmhub.io/v1/tokens/exchange"
	defaultFlowTTL          = 10 * time.Minute
)

// tokensRequest is the JSON body for the Charmhub tokens endpoint.
type tokensRequest struct {
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// tokensResponse is the JSON body returned by the Charmhub tokens endpoint.
type tokensResponse struct {
	Macaroon string `json:"macaroon"`
}

// Authenticator performs Charmhub authentication via httpbakery macaroon discharge.
type Authenticator struct {
	tokensEndpoint   string
	exchangeEndpoint string
	logger           *slog.Logger
	httpClient       *http.Client
}

// NewAuthenticator creates a Charmhub authenticator adapter.
func NewAuthenticator(logger *slog.Logger, httpClient *http.Client) *Authenticator {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Authenticator{
		tokensEndpoint:   defaultTokensEndpoint,
		exchangeEndpoint: defaultExchangeEndpoint,
		logger:           logger,
		httpClient:       httpClient,
	}
}

// BeginAuth requests a root macaroon from Charmhub and returns a pending flow.
// The actual discharge (browser interaction) happens in PollAuth.
func (a *Authenticator) BeginAuth(ctx context.Context) (*sa.PendingAuthFlow, error) {
	a.logger.Info("requesting root macaroon from charmhub")

	rootMacaroon, err := a.requestRootMacaroon(ctx)
	if err != nil {
		return nil, fmt.Errorf("requesting charmhub root macaroon: %w", err)
	}

	now := time.Now().UTC()
	return &sa.PendingAuthFlow{
		RootMacaroon: rootMacaroon,
		CreatedAt:    now,
		ExpiresAt:    now.Add(defaultFlowTTL),
	}, nil
}

func (a *Authenticator) requestRootMacaroon(ctx context.Context) (string, error) {
	body, err := json.Marshal(tokensRequest{
		Description: "sunbeam-watchtower",
		Permissions: []string{
			"account-view-packages",
			"package-manage-acl",
			"package-view",
			"package-view-acl",
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshaling tokens request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.tokensEndpoint, strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("creating tokens request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing tokens request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("charmhub tokens request failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var tokensResp tokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokensResp); err != nil {
		return "", fmt.Errorf("decoding tokens response: %w", err)
	}
	if tokensResp.Macaroon == "" {
		return "", fmt.Errorf("charmhub returned empty macaroon")
	}

	return tokensResp.Macaroon, nil
}

// ExchangeToken exchanges a client-discharged macaroon bundle for the
// short-lived token that Charmhub's publisher endpoints actually accept on
// Authorization: Macaroon <token>.
//
// dischargedBundle is the space-separated base64 macaroon slice produced by
// storeauth/v1.SerializeMacaroonSlice. Charmhub expects the slice re-encoded
// as a base64 JSON array in the `Macaroons` request header (craft-store's
// CandidClient.exchange_macaroons convention).
func (a *Authenticator) ExchangeToken(ctx context.Context, dischargedBundle string) (string, error) {
	if strings.TrimSpace(dischargedBundle) == "" {
		return "", fmt.Errorf("empty discharged macaroon bundle")
	}

	slice, err := sa.DecodeMacaroonSlice(dischargedBundle)
	if err != nil {
		return "", fmt.Errorf("decoding discharged bundle: %w", err)
	}

	sliceJSON, err := json.Marshal(slice)
	if err != nil {
		return "", fmt.Errorf("marshaling discharged macaroons: %w", err)
	}
	header := base64.StdEncoding.EncodeToString(sliceJSON)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.exchangeEndpoint, strings.NewReader("{}"))
	if err != nil {
		return "", fmt.Errorf("creating exchange request: %w", err)
	}
	req.Header.Set("Macaroons", header)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	a.logger.Info("exchanging discharged macaroon for charmhub token")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing exchange request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("charmhub exchange failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out tokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decoding exchange response: %w", err)
	}
	if out.Macaroon == "" {
		return "", fmt.Errorf("charmhub returned empty exchanged macaroon")
	}
	return out.Macaroon, nil
}
