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

func TestCacheClientWorkflowSyncGit(t *testing.T) {
	var gotBody client.CacheSyncGitOptions

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/cache/sync/git" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_ = json.NewEncoder(w).Encode(client.CacheSyncGitResult{
			Synced:   2,
			Warnings: []string{"partial"},
		})
	}))
	defer ts.Close()

	workflow := NewCacheClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.SyncGit(context.Background(), CacheSyncGitRequest{Projects: []string{"keystone", "glance"}})
	if err != nil {
		t.Fatalf("SyncGit() error = %v", err)
	}
	if got.Synced != 2 || len(got.Warnings) != 1 || got.Warnings[0] != "partial" {
		t.Fatalf("SyncGit() = %+v, want synced result", got)
	}
	if len(gotBody.Projects) != 2 || gotBody.Projects[0] != "keystone" || gotBody.Projects[1] != "glance" {
		t.Fatalf("request body = %+v, want projects keystone/glance", gotBody)
	}
}

func TestCacheClientWorkflowSyncPackagesIndex(t *testing.T) {
	var gotBody client.PackagesCacheSyncOptions

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/packages/cache/sync" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	workflow := NewCacheClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	err := workflow.SyncPackagesIndex(context.Background(), CacheSyncPackagesIndexRequest{
		Distros:   []string{"ubuntu"},
		Releases:  []string{"noble"},
		Backports: []string{"none"},
	})
	if err != nil {
		t.Fatalf("SyncPackagesIndex() error = %v", err)
	}
	if len(gotBody.Distros) != 1 || gotBody.Distros[0] != "ubuntu" {
		t.Fatalf("distros = %+v, want ubuntu", gotBody.Distros)
	}
	if len(gotBody.Releases) != 1 || gotBody.Releases[0] != "noble" {
		t.Fatalf("releases = %+v, want noble", gotBody.Releases)
	}
	if len(gotBody.Backports) != 1 || gotBody.Backports[0] != "none" {
		t.Fatalf("backports = %+v, want none", gotBody.Backports)
	}
}

func TestCacheClientWorkflowSyncBugs(t *testing.T) {
	var gotBody client.CacheSyncBugsOptions

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/cache/sync/bugs" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_ = json.NewEncoder(w).Encode(client.CacheSyncBugsResult{Synced: 3})
	}))
	defer ts.Close()

	workflow := NewCacheClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.SyncBugs(context.Background(), CacheSyncBugsRequest{Projects: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("SyncBugs() error = %v", err)
	}
	if got.Synced != 3 {
		t.Fatalf("SyncBugs() = %+v, want synced result", got)
	}
	if len(gotBody.Projects) != 2 || gotBody.Projects[0] != "a" || gotBody.Projects[1] != "b" {
		t.Fatalf("request body = %+v, want projects a/b", gotBody)
	}
}

func TestCacheClientWorkflowClearExcuses(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/cache/excuses" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query()["tracker"]; len(got) != 2 || got[0] != "ubuntu-devel" || got[1] != "ubuntu-updates" {
			t.Fatalf("tracker query = %+v, want both trackers", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	workflow := NewCacheClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	if err := workflow.Clear(context.Background(), CacheClearRequest{
		Type:     "excuses",
		Trackers: []string{"ubuntu-devel", "ubuntu-updates"},
	}); err != nil {
		t.Fatalf("ClearExcuses() error = %v", err)
	}
}

func TestCacheClientWorkflowClearGitWithMultipleProjects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/cache/git" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query()["project"]; len(got) != 2 || got[0] != "keystone" || got[1] != "glance" {
			t.Fatalf("project query = %+v, want both projects", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	workflow := NewCacheClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	if err := workflow.Clear(context.Background(), CacheClearRequest{
		Type:     "git",
		Projects: []string{"keystone", "glance"},
	}); err != nil {
		t.Fatalf("ClearGit() error = %v", err)
	}
}

func TestCacheClientWorkflowStatus(t *testing.T) {
	baseDir := t.TempDir()
	gitDir := filepath.Join(baseDir, "git")
	packagesDir := filepath.Join(baseDir, "packages")
	releasesDir := filepath.Join(baseDir, "releases")
	reviewsDir := filepath.Join(baseDir, "reviews")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/cache/status" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(client.CacheStatusResult{
			Git: struct {
				Directory string              `json:"directory"`
				Repos     []client.CacheEntry `json:"repos"`
			}{
				Directory: gitDir,
				Repos:     []client.CacheEntry{{Name: "keystone", Size: "4.0 MiB"}},
			},
			Packages: struct {
				Directory string            `json:"directory"`
				Sources   []dto.CacheStatus `json:"sources"`
				Error     string            `json:"error,omitempty"`
			}{
				Directory: packagesDir,
			},
			Releases: struct {
				Directory string                   `json:"directory"`
				Entries   []dto.ReleaseCacheStatus `json:"entries"`
				Error     string                   `json:"error,omitempty"`
			}{
				Directory: releasesDir,
				Entries:   []dto.ReleaseCacheStatus{{Project: "sunbeam", Name: "snap-openstack", ArtifactType: dto.ArtifactSnap, TrackCount: 1, ChannelCount: 2}},
			},
			Reviews: struct {
				Directory string                  `json:"directory"`
				Entries   []dto.ReviewCacheStatus `json:"entries"`
				Error     string                  `json:"error,omitempty"`
			}{
				Directory: reviewsDir,
				Entries:   []dto.ReviewCacheStatus{{Project: "snap-openstack", ForgeType: "GitHub", SummaryCount: 4, DetailCount: 2}},
			},
		})
	}))
	defer ts.Close()

	workflow := NewCacheClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if got.Git.Directory != gitDir || len(got.Git.Repos) != 1 || got.Git.Repos[0].Name != "keystone" {
		t.Fatalf("Status() = %+v, want git cache snapshot", got)
	}
	if got.Releases.Directory != releasesDir || len(got.Releases.Entries) != 1 || got.Releases.Entries[0].Name != "snap-openstack" {
		t.Fatalf("Status() releases = %+v, want releases snapshot", got.Releases)
	}
	if got.Reviews.Directory != reviewsDir || len(got.Reviews.Entries) != 1 || got.Reviews.Entries[0].Project != "snap-openstack" {
		t.Fatalf("Status() reviews = %+v, want reviews snapshot", got.Reviews)
	}
}

func TestCacheClientWorkflowSyncReleases(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/cache/sync/releases" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(client.CacheSyncReleasesResult{
			Status:     "ok",
			Discovered: 4,
			Synced:     3,
			Skipped:    1,
			Warnings:   []string{"sunbeam: skipped (no series, release.tracks, or release.branches configured)"},
		})
	}))
	defer ts.Close()

	workflow := NewCacheClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.SyncReleases(context.Background())
	if err != nil {
		t.Fatalf("SyncReleases() error = %v", err)
	}
	if got.Status != "ok" || got.Discovered != 4 || got.Synced != 3 || got.Skipped != 1 || len(got.Warnings) != 1 {
		t.Fatalf("SyncReleases() = %+v, want counted result", got)
	}
}

func TestCacheClientWorkflowSyncReviews(t *testing.T) {
	var gotBody client.CacheSyncReviewsOptions

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/cache/sync/reviews" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_ = json.NewEncoder(w).Encode(client.CacheSyncReviewsResult{
			ProjectsSynced:  1,
			SummariesSynced: 5,
			DetailsSynced:   3,
			Warnings:        []string{"snap-openstack:#43: rate limited"},
		})
	}))
	defer ts.Close()

	workflow := NewCacheClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.SyncReviews(context.Background(), CacheSyncReviewsRequest{
		Projects: []string{"snap-openstack", "sunbeam-charms"},
		Since:    "2025-01-01",
	})
	if err != nil {
		t.Fatalf("SyncReviews() error = %v", err)
	}
	if len(gotBody.Projects) != 2 || gotBody.Projects[0] != "snap-openstack" || gotBody.Projects[1] != "sunbeam-charms" || gotBody.Since != "2025-01-01T00:00:00Z" {
		t.Fatalf("request body = %+v, want projects and resolved since", gotBody)
	}
	if got.ProjectsSynced != 1 || got.SummariesSynced != 5 || got.DetailsSynced != 3 || len(got.Warnings) != 1 {
		t.Fatalf("SyncReviews() = %+v, want counted result", got)
	}
}
