package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
)

func TestBuildsList_EmptyConfigReturnsEmptyList(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterBuildsAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/builds")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Builds []any `json:"builds"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Builds) != 0 {
		t.Fatalf("expected no builds, got %d", len(body.Builds))
	}
}

func TestBuildsTrigger_InvalidTimeoutReturns422(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterBuildsAPI(srv.API(), application)

	payload := []byte(`{"project":"demo","timeout":"definitely-not-a-duration"}`)
	resp, err := http.Post(base+"/api/v1/builds/trigger", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body["detail"].(string), "invalid timeout duration") {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestCacheSyncExcuses_InvalidTrackerReturns400(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterCacheAPI(srv.API(), application)

	payload := []byte(`{"trackers":["not-a-tracker"]}`)
	resp, err := http.Post(base+"/api/v1/cache/sync/excuses", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCacheStatus_IncludesExcusesSection(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterCacheAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/cache/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Excuses struct {
			Directory string `json:"directory"`
		} `json:"excuses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Excuses.Directory == "" {
		t.Fatal("expected excuses directory to be populated")
	}
	if !strings.HasPrefix(body.Excuses.Directory, filepath.Join(os.Getenv("XDG_CACHE_HOME"), "sunbeam-watchtower")) {
		t.Fatalf("unexpected excuses directory: %q", body.Excuses.Directory)
	}
}

func TestProjectsSync_AuthRequiredReturns401(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LP_ACCESS_TOKEN", "")
	t.Setenv("LP_ACCESS_TOKEN_SECRET", "")

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{
		Projects: []config.ProjectConfig{{
			Name: "demo",
			Code: config.CodeConfig{Forge: "github", Owner: "canonical", Project: "demo"},
			Bugs: []config.BugTrackerConfig{{Forge: "launchpad", Project: "demo-lp"}},
		}},
	})
	RegisterProjectsAPI(srv.API(), application)

	resp, err := http.Post(base+"/api/v1/projects/sync", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
