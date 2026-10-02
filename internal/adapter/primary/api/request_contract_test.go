package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func TestCommitsList_OmittedOptionalQueryParamsReturns200(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterCommitsAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/commits")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Commits []any `json:"commits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Commits) != 0 {
		t.Fatalf("expected no commits, got %d", len(body.Commits))
	}
}

func TestReviewsList_OmittedOptionalQueryParamsReturns200(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterReviewsAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/reviews")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		MergeRequests []any `json:"merge_requests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.MergeRequests) != 0 {
		t.Fatalf("expected no merge requests, got %d", len(body.MergeRequests))
	}
}

func TestBugsList_OmittedOptionalQueryParamsReturns200(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterBugsAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/bugs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Tasks []any `json:"tasks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tasks) != 0 {
		t.Fatalf("expected no bug tasks, got %d", len(body.Tasks))
	}
}

func TestBugsSearch_AcceptsOmittedOptionalFields(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	cache, err := application.BugCache()
	if err != nil {
		t.Fatal(err)
	}
	bug := &forge.Bug{
		Forge:           forge.ForgeLaunchpad,
		ID:              "123",
		Title:           "Cinder volume attach failure",
		Description:     "Attaching a volume times out.",
		VisibilityKnown: true,
		UpdatedAt:       time.Now().UTC(),
	}
	task := forge.BugTask{
		Forge:           forge.ForgeLaunchpad,
		BugID:           bug.ID,
		Title:           bug.Title,
		Project:         "cinder",
		TargetName:      "cinder",
		Status:          "New",
		VisibilityKnown: true,
	}
	if err := cache.ReplaceProject(
		context.Background(),
		forge.ForgeLaunchpad,
		"cinder",
		[]*forge.Bug{bug},
		[]forge.BugTask{task},
		time.Now().UTC(),
		dto.BugCacheSchemaVersion,
	); err != nil {
		t.Fatal(err)
	}
	RegisterBugsAPI(srv.API(), application)

	resp, err := http.Post(base+"/api/v1/bugs/search", "application/json", bytes.NewBufferString(`{"query":"volume attach"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body dto.BugSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != dto.BugSearchOutcomeMatches || len(body.Results) != 1 || body.Results[0].Reference != "launchpad:123" {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestBugsSearch_LegacyCacheReturns409(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())
	application := newEphemeralTestApp(t, &config.Config{})
	cache, err := application.BugCache()
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.StoreBugs(context.Background(), []*forge.Bug{{
		Forge: forge.ForgeLaunchpad, ID: "123", Title: "Legacy volume failure",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := cache.StoreBugTasks(
		context.Background(),
		forge.ForgeLaunchpad,
		"cinder",
		[]forge.BugTask{{Forge: forge.ForgeLaunchpad, BugID: "123", TargetName: "cinder"}},
	); err != nil {
		t.Fatal(err)
	}
	RegisterBugsAPI(srv.API(), application)

	resp, err := http.Post(
		base+"/api/v1/bugs/search",
		"application/json",
		bytes.NewBufferString(`{"query":"volume"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestBugsSearch_EmptyCacheReturns409(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())
	application := newEphemeralTestApp(t, &config.Config{})
	RegisterBugsAPI(srv.API(), application)

	resp, err := http.Post(base+"/api/v1/bugs/search", "application/json", bytes.NewBufferString(`{"query":"volume"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestBuildsList_OmittedOptionalQueryParamsReturns200(t *testing.T) {
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
