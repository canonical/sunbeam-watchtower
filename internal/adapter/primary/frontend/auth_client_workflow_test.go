// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestAuthClientWorkflowStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/auth/status" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(dto.AuthStatus{
			Launchpad: dto.LaunchpadAuthStatus{
				Authenticated: true,
				Username:      "jdoe",
			},
		})
	}))
	defer ts.Close()

	workflow := NewAuthClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if got.Launchpad.Username != "jdoe" {
		t.Fatalf("Status() = %+v, want jdoe", got)
	}
}

func TestAuthClientWorkflowLoginLaunchpad(t *testing.T) {
	var finalizedFlowID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/launchpad/begin":
			_ = json.NewEncoder(w).Encode(dto.LaunchpadAuthBeginResult{
				FlowID:       "flow-123",
				AuthorizeURL: "https://launchpad.net/+authorize-token?oauth_token=req-token",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/launchpad/finalize":
			var body struct {
				FlowID string `json:"flow_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("Decode(finalize) error = %v", err)
			}
			finalizedFlowID = body.FlowID
			_ = json.NewEncoder(w).Encode(dto.LaunchpadAuthFinalizeResult{
				Launchpad: dto.LaunchpadAuthStatus{
					Authenticated: true,
					Username:      "jdoe",
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ts.Close()

	workflow := NewAuthClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	handlerCalled := false
	got, err := workflow.LoginLaunchpad(context.Background(), func(ctx context.Context, begin *dto.LaunchpadAuthBeginResult) error {
		handlerCalled = true
		if begin.FlowID != "flow-123" {
			t.Fatalf("begin.FlowID = %q, want flow-123", begin.FlowID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("LoginLaunchpad() error = %v", err)
	}
	if !handlerCalled {
		t.Fatal("authorization handler was not called")
	}
	if finalizedFlowID != "flow-123" {
		t.Fatalf("finalized flow_id = %q, want flow-123", finalizedFlowID)
	}
	if got.Finalized == nil || !got.Finalized.Launchpad.Authenticated {
		t.Fatalf("LoginLaunchpad() = %+v, want authenticated result", got)
	}
}

func TestAuthClientWorkflowLogoutLaunchpad(t *testing.T) {
	credsPath := filepath.Join(t.TempDir(), "launchpad-creds")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth/launchpad/logout" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(dto.LaunchpadAuthLogoutResult{
			Cleared:         true,
			CredentialsPath: credsPath,
		})
	}))
	defer ts.Close()

	workflow := NewAuthClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.LogoutLaunchpad(context.Background())
	if err != nil {
		t.Fatalf("LogoutLaunchpad() error = %v", err)
	}
	if !got.Cleared || got.CredentialsPath != credsPath {
		t.Fatalf("LogoutLaunchpad() = %+v, want cleared credentials at %q", got, credsPath)
	}
}

func TestAuthClientWorkflowLoginGitHub(t *testing.T) {
	var finalizedFlowID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/github/begin":
			_ = json.NewEncoder(w).Encode(dto.GitHubAuthBeginResult{
				FlowID:          "flow-123",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://github.com/login/device",
				IntervalSeconds: 5,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/github/finalize":
			var body struct {
				FlowID string `json:"flow_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("Decode(finalize) error = %v", err)
			}
			finalizedFlowID = body.FlowID
			_ = json.NewEncoder(w).Encode(dto.GitHubAuthFinalizeResult{
				GitHub: dto.GitHubAuthStatus{
					Authenticated: true,
					Username:      "jdoe",
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ts.Close()

	workflow := NewAuthClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.LoginGitHub(context.Background(), func(ctx context.Context, begin *dto.GitHubAuthBeginResult) error {
		if begin.UserCode != "ABCD-EFGH" {
			t.Fatalf("begin.UserCode = %q, want ABCD-EFGH", begin.UserCode)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("LoginGitHub() error = %v", err)
	}
	if finalizedFlowID != "flow-123" {
		t.Fatalf("finalized flow_id = %q, want flow-123", finalizedFlowID)
	}
	if got.Finalized == nil || !got.Finalized.GitHub.Authenticated {
		t.Fatalf("LoginGitHub() = %+v, want authenticated result", got)
	}
}
