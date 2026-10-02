package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestAuthStatusCmd_PrintsAuthenticatedUser(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/auth/status" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(dto.AuthStatus{
			Launchpad: dto.LaunchpadAuthStatus{
				Authenticated: true,
				Username:      "jdoe",
				DisplayName:   "Jane Doe",
				Source:        "file",
			},
		})
	}))
	defer ts.Close()

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "table",
		Client: client.NewClient(ts.URL),
	}

	cmd := newAuthCmd(opts)
	cmd.SetArgs([]string{"status"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Authenticated as: Jane Doe (jdoe)") || !strings.Contains(output, "Source: file") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestAuthLoginCmd_CompletesFlow(t *testing.T) {
	credsPath := filepath.Join(t.TempDir(), "launchpad-creds")
	var finalizePayload map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/launchpad/begin":
			_ = json.NewEncoder(w).Encode(dto.LaunchpadAuthBeginResult{
				FlowID:       "flow-123",
				AuthorizeURL: "https://launchpad.net/+authorize-token?oauth_token=req-token",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/launchpad/finalize":
			if err := json.NewDecoder(r.Body).Decode(&finalizePayload); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			_ = json.NewEncoder(w).Encode(dto.LaunchpadAuthFinalizeResult{
				Launchpad: dto.LaunchpadAuthStatus{
					Authenticated:   true,
					Username:        "jdoe",
					DisplayName:     "Jane Doe",
					CredentialsPath: credsPath,
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ts.Close()

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "table",
		Client: client.NewClient(ts.URL),
	}

	cmd := newAuthCmd(opts)
	cmd.SetIn(strings.NewReader("\n"))
	cmd.SetArgs([]string{"launchpad", "login"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if finalizePayload["flow_id"] != "flow-123" {
		t.Fatalf("flow_id = %q, want flow-123", finalizePayload["flow_id"])
	}
	output := out.String()
	if !strings.Contains(output, "Starting Launchpad OAuth flow") ||
		!strings.Contains(output, "https://launchpad.net/+authorize-token?oauth_token=req-token") ||
		!strings.Contains(output, "Authenticated as: Jane Doe (jdoe)") ||
		!strings.Contains(output, "Credentials saved to "+credsPath) {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestAuthLogoutCmd_PrintsRemovedPath(t *testing.T) {
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

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "table",
		Client: client.NewClient(ts.URL),
	}

	cmd := newAuthCmd(opts)
	cmd.SetArgs([]string{"launchpad", "logout"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out.String(), "Removed Launchpad credentials from "+credsPath) {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestAuthGitHubLoginCmd_CompletesFlow(t *testing.T) {
	credsPath := filepath.Join(t.TempDir(), "github-creds")
	var finalizePayload map[string]string
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
			if err := json.NewDecoder(r.Body).Decode(&finalizePayload); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			_ = json.NewEncoder(w).Encode(dto.GitHubAuthFinalizeResult{
				GitHub: dto.GitHubAuthStatus{
					Authenticated:   true,
					Username:        "jdoe",
					DisplayName:     "Jane Doe",
					CredentialsPath: credsPath,
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ts.Close()

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "table",
		Client: client.NewClient(ts.URL),
	}

	cmd := newAuthCmd(opts)
	cmd.SetArgs([]string{"github", "login"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if finalizePayload["flow_id"] != "flow-123" {
		t.Fatalf("flow_id = %q, want flow-123", finalizePayload["flow_id"])
	}
	output := out.String()
	if !strings.Contains(output, "Starting GitHub device flow") ||
		!strings.Contains(output, "ABCD-EFGH") ||
		!strings.Contains(output, "Authenticated as: Jane Doe (jdoe)") ||
		!strings.Contains(output, "Credentials saved to "+credsPath) {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestBuildListCmd_RendersBuildsFromAPI(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/builds" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query().Get("state"); got != "building" {
			t.Fatalf("state = %q, want building", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"builds": []dto.Build{{
				Project:      "demo",
				Recipe:       "my-rock",
				ArtifactType: dto.ArtifactRock,
				State:        dto.BuildBuilding,
				Arch:         "amd64",
				WebLink:      "https://launchpad.net/build/1",
			}},
		})
	}))
	defer ts.Close()

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "table",
		Client: client.NewClient(ts.URL),
	}

	cmd := newBuildCmd(opts)
	cmd.SetArgs([]string{"list", "--state", "building"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "PROJECT") || !strings.Contains(output, "demo") || !strings.Contains(output, "my-rock") || !strings.Contains(output, "building") {
		t.Fatalf("unexpected output: %q", output)
	}
}
