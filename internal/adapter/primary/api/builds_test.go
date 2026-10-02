package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// startBuildsTestServer boots an API server with the builds endpoints
// registered against an ephemeral app. Used by retry_count contract tests.
func startBuildsTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	srv, base := startTestServer(t)
	application := newEphemeralTestApp(t, &config.Config{})
	RegisterBuildsAPI(srv.API(), application)
	return srv, base
}

func readBodyString(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(raw)
}

// Confirm Huma treats retry_count as optional: a request body omitting the
// field must NOT be rejected with a validation 422. The downstream service
// may fail later (500 / other) because the ephemeral app has no configured
// projects, but that's irrelevant for the Huma optional-field contract.
func TestBuildsTriggerEndpoint_RetryCountOptional(t *testing.T) {
	srv, base := startBuildsTestServer(t)
	defer srv.Shutdown(context.Background())

	resp := postJSON(t, base, "/api/v1/builds/trigger", map[string]any{
		"project": "nonexistent",
	})
	body := readBodyString(t, resp)

	if resp.StatusCode == http.StatusUnprocessableEntity {
		t.Fatalf("retry_count should be optional; got 422 validation error: %s", body)
	}
}

func TestBuildsTriggerEndpoint_RetryWithoutWait422(t *testing.T) {
	srv, base := startBuildsTestServer(t)
	defer srv.Shutdown(context.Background())

	resp := postJSON(t, base, "/api/v1/builds/trigger", map[string]any{
		"project":     "nonexistent",
		"retry_count": 3,
	})
	body := readBodyString(t, resp)

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte("retry_count > 1 requires wait=true")) {
		t.Errorf("expected retry_count > 1 requires wait error, got: %s", body)
	}
}

func TestBuildsTriggerEndpoint_RetryNegative422(t *testing.T) {
	srv, base := startBuildsTestServer(t)
	defer srv.Shutdown(context.Background())

	resp := postJSON(t, base, "/api/v1/builds/trigger", map[string]any{
		"project":     "nonexistent",
		"retry_count": -1,
		"wait":        true,
	})
	body := readBodyString(t, resp)

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte("retry_count must be >= 0")) {
		t.Errorf("expected retry_count must be >= 0 error, got: %s", body)
	}
}

func TestBuildsTriggerAsyncEndpoint_RetryRejected422(t *testing.T) {
	srv, base := startBuildsTestServer(t)
	defer srv.Shutdown(context.Background())

	resp := postJSON(t, base, "/api/v1/builds/trigger/async", map[string]any{
		"project":     "nonexistent",
		"retry_count": 3,
	})
	body := readBodyString(t, resp)

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte("retry_count is not supported on the async trigger endpoint")) {
		t.Errorf("expected async retry rejection error, got: %s", body)
	}
}

func TestBuildsDownloadEndpoint_RetryCountOptional(t *testing.T) {
	srv, base := startBuildsTestServer(t)
	defer srv.Shutdown(context.Background())

	resp := postJSON(t, base, "/api/v1/builds/download", map[string]any{
		"project": "nonexistent",
	})
	body := readBodyString(t, resp)

	if resp.StatusCode == http.StatusUnprocessableEntity {
		t.Fatalf("retry_count should be optional; got 422 validation error: %s", body)
	}
}

func TestBuildsDownloadEndpoint_RetryNegative422(t *testing.T) {
	srv, base := startBuildsTestServer(t)
	defer srv.Shutdown(context.Background())

	resp := postJSON(t, base, "/api/v1/builds/download", map[string]any{
		"project":     "nonexistent",
		"retry_count": -1,
	})
	body := readBodyString(t, resp)

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte("retry_count must be >= 0")) {
		t.Errorf("expected retry_count must be >= 0 error, got: %s", body)
	}
}

func TestBuildTriggerOptionsFromInput_PreparedSource(t *testing.T) {
	input := &BuildsTriggerInput{}
	input.Body.Project = "demo"
	input.Body.Wait = true
	input.Body.Timeout = "45m"
	input.Body.Owner = "lp-user"
	input.Body.Prefix = "tmp-build"
	input.Body.TargetRef = "remote-ref"
	input.Body.Prepared = &dto.PreparedBuildSource{
		Backend:       dto.PreparedBuildBackendLaunchpad,
		TargetRef:     "prepared-ref",
		RepositoryRef: "/repo/demo",
		Recipes: map[string]dto.PreparedBuildRecipe{
			"tmp-build-01234567-keystone": {SourceRef: "/ref/tmp", BuildPath: "rocks/keystone"},
		},
	}

	got, err := buildTriggerOptionsFromInput(input)
	if err != nil {
		t.Fatalf("buildTriggerOptionsFromInput() error = %v", err)
	}

	if !got.Wait {
		t.Fatal("Wait = false, want true")
	}
	if got.Timeout != 45*time.Minute {
		t.Fatalf("Timeout = %s, want 45m", got.Timeout)
	}
	if got.Owner != "lp-user" || got.Prefix != "tmp-build" || got.TargetRef != "remote-ref" {
		t.Fatalf("unexpected trigger options: %+v", got)
	}
	if got.Prepared == nil {
		t.Fatal("Prepared = nil, want value")
	}
	normalized := got.Prepared.Normalize()
	if normalized.TargetRef != "prepared-ref" || normalized.RepositoryRef != "/repo/demo" {
		t.Fatalf("unexpected prepared source: %+v", got.Prepared)
	}
	if normalized.Recipes["tmp-build-01234567-keystone"].BuildPath != "rocks/keystone" {
		t.Fatalf("unexpected prepared recipes: %+v", normalized.Recipes)
	}
}
