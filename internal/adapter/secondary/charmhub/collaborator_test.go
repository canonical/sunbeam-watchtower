// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package charmhub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
)

// staticProvider serves a fixed token and counts Refresh calls. When
// refreshToken is non-empty, Refresh swaps Token to that value.
type staticProvider struct {
	mu           sync.Mutex
	token        string
	refreshToken string
	refreshErr   error
	refreshCalls int
}

func (p *staticProvider) Token(context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.token, nil
}

func (p *staticProvider) Refresh(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refreshCalls++
	if p.refreshErr != nil {
		return p.refreshErr
	}
	if p.refreshToken != "" {
		p.token = p.refreshToken
	}
	return nil
}

func newProvider(token string) *staticProvider { return &staticProvider{token: token} }

func TestCollaboratorManager_ListCollaborators(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/charm/my-charm/collaborators" {
			t.Fatalf("path = %q, want /v1/charm/my-charm/collaborators", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Macaroon test-auth" {
			t.Fatalf("Authorization = %q, want Macaroon test-auth", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"collaborators": []map[string]any{
				{"email": "alice@example.com", "username": "alice", "display-name": "Alice Smith", "status": "accepted"},
				{"email": "bob@example.com", "username": "bob", "display-name": "Bob Jones", "status": "pending"},
			},
		})
	}))
	defer server.Close()

	mgr := NewCollaboratorManager(newProvider("test-auth"), server.Client())
	mgr.baseURL = server.URL

	collaborators, err := mgr.ListCollaborators(context.Background(), "my-charm")
	if err != nil {
		t.Fatalf("ListCollaborators() error = %v", err)
	}
	if len(collaborators) != 2 {
		t.Fatalf("ListCollaborators() = %d collaborators, want 2", len(collaborators))
	}
	if collaborators[0].Email != "alice@example.com" || collaborators[0].Username != "alice" || collaborators[0].Status != "accepted" {
		t.Fatalf("collaborators[0] = %+v, want alice/accepted", collaborators[0])
	}
	if collaborators[1].Email != "bob@example.com" || collaborators[1].Username != "bob" || collaborators[1].Status != "pending" {
		t.Fatalf("collaborators[1] = %+v, want bob/pending", collaborators[1])
	}
}

func TestCollaboratorManager_InviteCollaborator(t *testing.T) {
	var receivedBody struct {
		Invites []struct {
			Email string `json:"email"`
		} `json:"invites"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/charm/my-charm/collaborators/invites" {
			t.Fatalf("path = %q, want /v1/charm/my-charm/collaborators/invites", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Macaroon test-auth" {
			t.Fatalf("Authorization = %q, want Macaroon test-auth", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	mgr := NewCollaboratorManager(newProvider("test-auth"), server.Client())
	mgr.baseURL = server.URL

	err := mgr.InviteCollaborator(context.Background(), "my-charm", "carol@example.com")
	if err != nil {
		t.Fatalf("InviteCollaborator() error = %v", err)
	}
	if len(receivedBody.Invites) != 1 || receivedBody.Invites[0].Email != "carol@example.com" {
		t.Fatalf("request invites = %+v, want single invite for carol@example.com", receivedBody.Invites)
	}
}

func TestCollaboratorManager_InviteCollaborator_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	mgr := NewCollaboratorManager(newProvider("bad-auth"), server.Client())
	mgr.baseURL = server.URL

	err := mgr.InviteCollaborator(context.Background(), "my-charm", "carol@example.com")
	if err == nil {
		t.Fatal("InviteCollaborator() expected error for HTTP 403, got nil")
	}
}

// TestCollaboratorManager_ListCollaborators_DecodesErrorList: a 400 with
// the documented error-list body must surface both the status and the
// decoded code/message in the wrapped error string. Auth-class codes
// trigger exactly one refresh+retry — if the refresh also returns
// re-login required the error propagates unchanged.
func TestCollaboratorManager_ListCollaborators_DecodesErrorList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error-list":[{"code":"macaroon-needs-refresh","message":"macaroon has expired"}]}`))
	}))
	defer server.Close()

	provider := &staticProvider{token: "expired-auth", refreshErr: port.ErrCharmhubReloginRequired}
	mgr := NewCollaboratorManager(provider, server.Client())
	mgr.baseURL = server.URL

	_, err := mgr.ListCollaborators(context.Background(), "my-charm")
	if err == nil {
		t.Fatal("ListCollaborators() expected error, got nil")
	}
	if !errors.Is(err, port.ErrCharmhubReloginRequired) {
		t.Errorf("errors.Is(err, ErrCharmhubReloginRequired) = false, want true (err=%v)", err)
	}
	if !errors.Is(err, port.ErrStoreAuthExpired) {
		t.Errorf("errors.Is(err, ErrStoreAuthExpired) = false, want true")
	}
	if provider.refreshCalls != 1 {
		t.Errorf("refreshCalls = %d, want 1", provider.refreshCalls)
	}
}

// TestCollaboratorManager_ListCollaborators_401MapsToUnauthorized: plain
// 401 without a JSON body triggers the refresh path. When the refresh
// cannot recover, the re-login sentinel bubbles up.
func TestCollaboratorManager_ListCollaborators_401MapsToUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := &staticProvider{token: "bad-auth", refreshErr: port.ErrCharmhubReloginRequired}
	mgr := NewCollaboratorManager(provider, server.Client())
	mgr.baseURL = server.URL

	_, err := mgr.ListCollaborators(context.Background(), "my-charm")
	if err == nil {
		t.Fatal("ListCollaborators() expected error")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("errors.Is(err, ErrUnauthorized) = false for HTTP 401 (err=%v)", err)
	}
}

// TestCollaboratorManager_ListCollaborators_MalformedBodyFallsBack: when
// the response body isn't either documented JSON shape, the raw text must
// be included verbatim in the error. 500 is not auth-class so no refresh.
func TestCollaboratorManager_ListCollaborators_MalformedBodyFallsBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream exploded: backend timeout"))
	}))
	defer server.Close()

	provider := newProvider("ok-auth")
	mgr := NewCollaboratorManager(provider, server.Client())
	mgr.baseURL = server.URL

	_, err := mgr.ListCollaborators(context.Background(), "my-charm")
	if err == nil {
		t.Fatal("ListCollaborators() expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "HTTP 500") || !strings.Contains(msg, "upstream exploded") {
		t.Errorf("error %q should contain status + raw body fallback", msg)
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Errorf("plain 500 must not map to ErrUnauthorized (err=%v)", err)
	}
	if provider.refreshCalls != 0 {
		t.Errorf("refreshCalls = %d, want 0 for non-auth error", provider.refreshCalls)
	}
}

// TestCollaboratorManager_InviteCollaborator_DecodesSingleErrorShape:
// exercise the `{"error":{...}}` shape on the invite path. This is an
// auth-class code, so the refresh path is invoked.
func TestCollaboratorManager_InviteCollaborator_DecodesSingleErrorShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"permission-required","message":"missing package-manage-collaborators"}}`))
	}))
	defer server.Close()

	provider := &staticProvider{token: "ok-auth", refreshErr: port.ErrCharmhubReloginRequired}
	mgr := NewCollaboratorManager(provider, server.Client())
	mgr.baseURL = server.URL

	err := mgr.InviteCollaborator(context.Background(), "my-charm", "x@example.com")
	if err == nil {
		t.Fatal("InviteCollaborator() expected error")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("permission-required code should map to ErrUnauthorized (err=%v)", err)
	}
}

// TestCollaboratorManager_ListCollaborators_RefreshesAndRetries:
// first call returns macaroon-needs-refresh; the provider swaps the
// token on Refresh; the retried call uses the new token and succeeds.
func TestCollaboratorManager_ListCollaborators_RefreshesAndRetries(t *testing.T) {
	var attempts atomic.Int32
	var lastAuth atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		lastAuth.Store(r.Header.Get("Authorization"))
		if attempts.Load() == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error-list":[{"code":"macaroon-needs-refresh","message":"expired"}]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"collaborators": []map[string]any{
				{"email": "a@example.com", "username": "a", "display-name": "A", "status": "accepted"},
			},
		})
	}))
	defer server.Close()

	provider := &staticProvider{token: "stale-token", refreshToken: "fresh-token"}
	mgr := NewCollaboratorManager(provider, server.Client())
	mgr.baseURL = server.URL

	collabs, err := mgr.ListCollaborators(context.Background(), "my-charm")
	if err != nil {
		t.Fatalf("ListCollaborators() error = %v", err)
	}
	if len(collabs) != 1 {
		t.Fatalf("ListCollaborators() = %d, want 1", len(collabs))
	}
	if provider.refreshCalls != 1 {
		t.Errorf("refreshCalls = %d, want 1", provider.refreshCalls)
	}
	if attempts.Load() != 2 {
		t.Errorf("server attempts = %d, want 2", attempts.Load())
	}
	if got, _ := lastAuth.Load().(string); got != "Macaroon fresh-token" {
		t.Errorf("last Authorization = %q, want %q", got, "Macaroon fresh-token")
	}
}

// TestCollaboratorManager_InviteCollaborator_RefreshesAndRetries: the
// invite path rebuilds the POST body on retry, so a refreshed call must
// send the same invite payload again and succeed.
func TestCollaboratorManager_InviteCollaborator_RefreshesAndRetries(t *testing.T) {
	var attempts atomic.Int32
	var bodies []string
	var bodiesMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		bodiesMu.Lock()
		bodies = append(bodies, string(buf))
		bodiesMu.Unlock()

		attempts.Add(1)
		if attempts.Load() == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := &staticProvider{token: "stale-token", refreshToken: "fresh-token"}
	mgr := NewCollaboratorManager(provider, server.Client())
	mgr.baseURL = server.URL

	if err := mgr.InviteCollaborator(context.Background(), "my-charm", "c@example.com"); err != nil {
		t.Fatalf("InviteCollaborator() error = %v", err)
	}
	if provider.refreshCalls != 1 {
		t.Errorf("refreshCalls = %d, want 1", provider.refreshCalls)
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2", attempts.Load())
	}
	bodiesMu.Lock()
	defer bodiesMu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("len(bodies) = %d, want 2", len(bodies))
	}
	if !strings.Contains(bodies[0], "c@example.com") || bodies[0] != bodies[1] {
		t.Errorf("retry body differs from first: %q vs %q", bodies[0], bodies[1])
	}
}

// TestCollaboratorManager_ListCollaborators_RefreshFailurePropagates:
// the discharge is gone too — the retry does not happen and the
// re-login sentinel surfaces directly.
func TestCollaboratorManager_ListCollaborators_RefreshFailurePropagates(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := &staticProvider{token: "stale-token", refreshErr: port.ErrCharmhubReloginRequired}
	mgr := NewCollaboratorManager(provider, server.Client())
	mgr.baseURL = server.URL

	_, err := mgr.ListCollaborators(context.Background(), "my-charm")
	if err == nil {
		t.Fatal("ListCollaborators() expected error")
	}
	if !errors.Is(err, port.ErrCharmhubReloginRequired) {
		t.Errorf("errors.Is(err, ErrCharmhubReloginRequired) = false (err=%v)", err)
	}
	if provider.refreshCalls != 1 {
		t.Errorf("refreshCalls = %d, want 1", provider.refreshCalls)
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1 (no retry after refresh failure)", attempts.Load())
	}
	if !strings.Contains(err.Error(), "watchtower auth charmhub login") {
		t.Errorf("error %q must surface actionable re-login hint", err.Error())
	}
}

// TestCollaboratorManager_ListCollaborators_SecondFailureDoesNotLoop:
// refresh succeeds but the server still returns 401 — the second failure
// must surface re-login required (not loop forever).
func TestCollaboratorManager_ListCollaborators_SecondFailureDoesNotLoop(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := &staticProvider{token: "stale-token", refreshToken: "also-bad-token"}
	mgr := NewCollaboratorManager(provider, server.Client())
	mgr.baseURL = server.URL

	_, err := mgr.ListCollaborators(context.Background(), "my-charm")
	if err == nil {
		t.Fatal("ListCollaborators() expected error")
	}
	if !errors.Is(err, port.ErrCharmhubReloginRequired) {
		t.Errorf("errors.Is(err, ErrCharmhubReloginRequired) = false (err=%v)", err)
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2 (exactly one retry)", attempts.Load())
	}
}
