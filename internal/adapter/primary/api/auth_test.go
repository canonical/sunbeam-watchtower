// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/testsupport"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withDefaultTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = orig
	})
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newAuthTestApp() *app.App {
	return app.NewAppWithOptions(&config.Config{}, discardLogger(), app.Options{
		RuntimeMode: app.RuntimeModeEphemeral,
	})
}

func postJSON(t *testing.T, baseURL, path string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+path, reader)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	return resp
}

func TestAuthStatusEndpoint_NotAuthenticated(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newAuthTestApp()
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	RegisterAuthAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/auth/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var status dto.AuthStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if status.Launchpad.Authenticated {
		t.Fatal("expected unauthenticated status")
	}
}

func TestAuthLaunchpadBeginAndFinalizeEndpoints(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())

	origTransport := http.DefaultTransport
	withDefaultTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "launchpad.net" && req.URL.Path == "/+request-token":
			return jsonResponse(http.StatusOK, "oauth_token=req-token&oauth_token_secret=req-secret"), nil
		case req.URL.Host == "launchpad.net" && req.URL.Path == "/+access-token":
			return jsonResponse(http.StatusOK, "oauth_token=access-token&oauth_token_secret=access-secret"), nil
		case req.URL.Host == "api.launchpad.net" && req.URL.Path == "/devel/people/+me":
			return jsonResponse(http.StatusOK, `{"name":"jdoe","display_name":"Jane Doe"}`), nil
		default:
			return origTransport.RoundTrip(req)
		}
	}))

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newAuthTestApp()
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	RegisterAuthAPI(srv.API(), application)

	beginResp := postJSON(t, base, "/api/v1/auth/launchpad/begin", nil)
	defer beginResp.Body.Close()
	if beginResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(beginResp.Body)
		t.Fatalf("begin expected 200, got %d: %s", beginResp.StatusCode, body)
	}

	var begin dto.LaunchpadAuthBeginResult
	if err := json.NewDecoder(beginResp.Body).Decode(&begin); err != nil {
		t.Fatalf("Decode(begin) error = %v", err)
	}
	if begin.FlowID == "" || begin.AuthorizeURL == "" {
		t.Fatalf("unexpected begin result: %+v", begin)
	}

	finalizeResp := postJSON(t, base, "/api/v1/auth/launchpad/finalize", map[string]string{"flow_id": begin.FlowID})
	defer finalizeResp.Body.Close()
	if finalizeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(finalizeResp.Body)
		t.Fatalf("finalize expected 200, got %d: %s", finalizeResp.StatusCode, body)
	}

	var finalized dto.LaunchpadAuthFinalizeResult
	if err := json.NewDecoder(finalizeResp.Body).Decode(&finalized); err != nil {
		t.Fatalf("Decode(finalize) error = %v", err)
	}
	if !finalized.Launchpad.Authenticated {
		t.Fatalf("expected authenticated finalize result, got %+v", finalized)
	}
	if finalized.Launchpad.Username != "jdoe" || finalized.Launchpad.DisplayName != "Jane Doe" {
		t.Fatalf("unexpected finalized identity: %+v", finalized.Launchpad)
	}
	if finalized.Launchpad.CredentialsPath == "" {
		t.Fatal("expected credentials path after finalize")
	}
	if _, err := os.Stat(finalized.Launchpad.CredentialsPath); err != nil {
		t.Fatalf("expected credentials file to exist: %v", err)
	}
}

func TestAuthLaunchpadFinalizeEndpoint_UnknownFlow(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newAuthTestApp()
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	RegisterAuthAPI(srv.API(), application)

	resp := postJSON(t, base, "/api/v1/auth/launchpad/finalize", map[string]string{"flow_id": "missing"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, body)
	}
}

func TestAuthLaunchpadLogoutEndpoint_EnvironmentCredentials(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LP_ACCESS_TOKEN", "env-token")
	t.Setenv("LP_ACCESS_TOKEN_SECRET", "env-secret")

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newAuthTestApp()
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	RegisterAuthAPI(srv.API(), application)

	resp := postJSON(t, base, "/api/v1/auth/launchpad/logout", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, body)
	}
}

func TestAuthGitHubBeginAndFinalizeEndpoints(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("WATCHTOWER_GITHUB_CLIENT_ID", "client-id")

	origTransport := http.DefaultTransport
	withDefaultTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "github.com" && req.URL.Path == "/login/device/code":
			return jsonResponse(http.StatusOK, `{"device_code":"device","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","expires_in":900,"interval":1}`), nil
		case req.URL.Host == "github.com" && req.URL.Path == "/login/oauth/access_token":
			return jsonResponse(http.StatusOK, `{"access_token":"token","token_type":"bearer"}`), nil
		case req.URL.Host == "api.github.com" && req.URL.Path == "/user":
			return jsonResponse(http.StatusOK, `{"login":"jdoe","name":"Jane Doe"}`), nil
		default:
			return origTransport.RoundTrip(req)
		}
	}))

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newAuthTestApp()
	t.Cleanup(func() {
		if err := application.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	RegisterAuthAPI(srv.API(), application)

	beginResp := postJSON(t, base, "/api/v1/auth/github/begin", nil)
	defer beginResp.Body.Close()
	if beginResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(beginResp.Body)
		t.Fatalf("begin expected 200, got %d: %s", beginResp.StatusCode, body)
	}

	var begin dto.GitHubAuthBeginResult
	if err := json.NewDecoder(beginResp.Body).Decode(&begin); err != nil {
		t.Fatalf("Decode(begin) error = %v", err)
	}
	finalizeResp := postJSON(t, base, "/api/v1/auth/github/finalize", map[string]string{"flow_id": begin.FlowID})
	defer finalizeResp.Body.Close()
	if finalizeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(finalizeResp.Body)
		t.Fatalf("finalize expected 200, got %d: %s", finalizeResp.StatusCode, body)
	}

	var finalized dto.GitHubAuthFinalizeResult
	if err := json.NewDecoder(finalizeResp.Body).Decode(&finalized); err != nil {
		t.Fatalf("Decode(finalize) error = %v", err)
	}
	if !finalized.GitHub.Authenticated || finalized.GitHub.Username != "jdoe" {
		t.Fatalf("unexpected finalized identity: %+v", finalized.GitHub)
	}
}
