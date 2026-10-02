// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package snapstore

import (
	"context"
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
	defaultTokensEndpoint = "https://dashboard.snapcraft.io/api/v2/tokens"
	defaultFlowTTL        = 10 * time.Minute
)

// tokensRequest is the JSON body for the Snap Store v2 tokens endpoint.
type tokensRequest struct {
	Permissions []string `json:"permissions"`
	Description string   `json:"description,omitempty"`
}

// tokensResponse is the JSON body returned by the Snap Store tokens endpoint.
type tokensResponse struct {
	Macaroon string `json:"macaroon"`
}

// Authenticator performs Snap Store authentication via httpbakery macaroon discharge.
type Authenticator struct {
	tokensEndpoint string
	permissions    []string
	logger         *slog.Logger
	httpClient     *http.Client
}

// NewAuthenticator creates a Snap Store authenticator adapter.
func NewAuthenticator(logger *slog.Logger, httpClient *http.Client) *Authenticator {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Authenticator{
		tokensEndpoint: defaultTokensEndpoint,
		permissions:    []string{"package_access", "package_manage"},
		logger:         logger,
		httpClient:     httpClient,
	}
}

// BeginAuth requests a root macaroon from the Snap Store and returns a pending flow.
func (a *Authenticator) BeginAuth(ctx context.Context) (*sa.PendingAuthFlow, error) {
	a.logger.Info("requesting root macaroon from snap store")

	rootMacaroon, err := a.requestRootMacaroon(ctx)
	if err != nil {
		return nil, fmt.Errorf("requesting snap store root macaroon: %w", err)
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
		Permissions: a.permissions,
		Description: "sunbeam-watchtower",
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
		return "", fmt.Errorf("snap store tokens request failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var tokensResp tokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokensResp); err != nil {
		return "", fmt.Errorf("decoding tokens response: %w", err)
	}
	if tokensResp.Macaroon == "" {
		return "", fmt.Errorf("snap store returned empty macaroon")
	}

	return tokensResp.Macaroon, nil
}
