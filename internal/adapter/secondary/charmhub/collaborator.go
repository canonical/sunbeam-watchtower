// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package charmhub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

var charmhubBaseURL = "https://api.charmhub.io"

// Compile-time interface compliance check.
var _ port.StoreCollaboratorManager = (*CollaboratorManager)(nil)

// CollaboratorManager implements port.StoreCollaboratorManager for Charmhub.
//
// Each publisher request pulls the current macaroon from the credential
// provider, and any auth-class failure triggers a single silent re-exchange
// via provider.Refresh before retrying the original request. A second
// auth-class failure (or a refresh failure) is surfaced to the caller so the
// user gets an actionable re-login hint instead of an infinite retry loop.
type CollaboratorManager struct {
	baseURL  string
	provider port.CharmhubCredentialProvider
	client   *http.Client
}

// NewCollaboratorManager creates a CollaboratorManager for Charmhub. The
// provider is consulted on every request so a refreshed token is picked up
// without re-creating the manager. An optional *http.Client may be
// supplied; if omitted a default client is used.
func NewCollaboratorManager(provider port.CharmhubCredentialProvider, clients ...*http.Client) *CollaboratorManager {
	var client *http.Client
	if len(clients) > 0 && clients[0] != nil {
		client = clients[0]
	} else {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &CollaboratorManager{
		baseURL:  charmhubBaseURL,
		provider: provider,
		client:   client,
	}
}

// requestFactory rebuilds an HTTP request so the retry path can send it
// again (including a fresh body reader) after a refresh.
type requestFactory func(ctx context.Context, auth string) (*http.Request, error)

// do executes a request factory with retry-once semantics on auth-class
// failures. Returns the final HTTP response (never both a response and a
// non-nil error) or a wrapped error. The caller owns the response body on
// success and is responsible for closing it.
func (m *CollaboratorManager) do(ctx context.Context, factory requestFactory) (*http.Response, error) {
	auth, err := m.provider.Token(ctx)
	if err != nil {
		return nil, err
	}

	resp, authExpired, err := m.sendOnce(ctx, factory, auth)
	if err != nil {
		return nil, err
	}
	if !authExpired {
		return resp, nil
	}

	// Drain and close the expired-auth response before refreshing so the
	// HTTP connection can be reused for the retry.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if refreshErr := m.provider.Refresh(ctx); refreshErr != nil {
		return nil, refreshErr
	}

	auth, err = m.provider.Token(ctx)
	if err != nil {
		return nil, err
	}

	resp, authExpired, err = m.sendOnce(ctx, factory, auth)
	if err != nil {
		return nil, err
	}
	if authExpired {
		// The freshly-exchanged token was also rejected — fold into the
		// re-login sentinel rather than loop.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return nil, port.ErrCharmhubReloginRequired
	}
	return resp, nil
}

// sendOnce issues one request and signals whether the non-2xx response was
// an auth-class failure. On transport errors it returns (nil, false, err).
func (m *CollaboratorManager) sendOnce(ctx context.Context, factory requestFactory, auth string) (*http.Response, bool, error) {
	req, err := factory(ctx, auth)
	if err != nil {
		return nil, false, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, false, nil
	}
	decoded := decodeHTTPError(resp)
	if errors.Is(decoded, port.ErrStoreAuthExpired) {
		return resp, true, nil
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return nil, false, decoded
}

// ListCollaborators returns all collaborators for the named charm.
func (m *CollaboratorManager) ListCollaborators(ctx context.Context, storeName string) ([]dto.StoreCollaborator, error) {
	endpoint := m.baseURL + "/v1/charm/" + url.PathEscape(storeName) + "/collaborators"

	factory := func(ctx context.Context, auth string) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("creating list collaborators request: %w", err)
		}
		req.Header.Set("Authorization", "Macaroon "+auth)
		req.Header.Set("Accept", "application/json")
		return req, nil
	}

	resp, err := m.do(ctx, factory)
	if err != nil {
		return nil, fmt.Errorf("listing collaborators: %w", err)
	}
	defer resp.Body.Close()

	var payload charmCollaboratorsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding collaborators response: %w", err)
	}

	result := make([]dto.StoreCollaborator, 0, len(payload.Collaborators))
	for _, c := range payload.Collaborators {
		result = append(result, dto.StoreCollaborator{
			Username:    c.Username,
			Email:       c.Email,
			DisplayName: c.DisplayName,
			Status:      c.Status,
		})
	}
	return result, nil
}

// InviteCollaborator sends a collaborator invitation for the named charm.
//
// The documented Charmhub publisher endpoint accepts a batch of invites under
// the `invites` key and POSTs to `/v1/charm/{name}/collaborators/invites`.
func (m *CollaboratorManager) InviteCollaborator(ctx context.Context, storeName string, email string) error {
	endpoint := m.baseURL + "/v1/charm/" + url.PathEscape(storeName) + "/collaborators/invites"

	body, err := json.Marshal(charmInvitesRequest{
		Invites: []charmInviteRequest{{Email: email}},
	})
	if err != nil {
		return fmt.Errorf("encoding invite request: %w", err)
	}

	factory := func(ctx context.Context, auth string) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("creating invite collaborator request: %w", err)
		}
		req.Header.Set("Authorization", "Macaroon "+auth)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		return req, nil
	}

	resp, err := m.do(ctx, factory)
	if err != nil {
		return fmt.Errorf("inviting collaborator: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

type charmInvitesRequest struct {
	Invites []charmInviteRequest `json:"invites"`
}

type charmInviteRequest struct {
	Email string `json:"email"`
}

type charmCollaboratorsResponse struct {
	Collaborators []charmCollaborator `json:"collaborators"`
}

type charmCollaborator struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	DisplayName string `json:"display-name"`
	Status      string `json:"status"`
}
