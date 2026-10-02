// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
)

func TestCacheSyncUpstream_WithoutUpstreamConfigIsSkipped(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterCacheAPI(srv.API(), application)

	resp, err := http.Post(base+"/api/v1/cache/sync/upstream", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "skipped: upstream not configured" {
		t.Fatalf("unexpected status %q", body.Status)
	}
}

func TestCacheDelete_InvalidTypeReturns400(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterCacheAPI(srv.API(), application)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, base+"/api/v1/cache/not-valid", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCacheSyncGit_EmptyConfigReturns400(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterCacheAPI(srv.API(), application)

	resp, err := http.Post(base+"/api/v1/cache/sync/git", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCacheSyncBugs_EmptyConfigReturnsZeroSynced(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterCacheAPI(srv.API(), application)

	resp, err := http.Post(base+"/api/v1/cache/sync/bugs", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCacheSyncReleases_WithConfiguredPublicationReturns200(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterCacheAPI(srv.API(), application)

	resp, err := http.Post(base+"/api/v1/cache/sync/releases", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Status     string   `json:"status"`
		Discovered int      `json:"discovered"`
		Synced     int      `json:"synced"`
		Skipped    int      `json:"skipped"`
		Warnings   []string `json:"warnings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Discovered != 0 || body.Synced != 0 || body.Skipped != 0 {
		t.Fatalf("body = %+v, want zero-count ok result", body)
	}
}

func TestCacheSyncReviews_EmptyConfigReturnsZeroCounts(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterCacheAPI(srv.API(), application)

	resp, err := http.Post(base+"/api/v1/cache/sync/reviews", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		ProjectsSynced  int      `json:"projects_synced"`
		SummariesSynced int      `json:"summaries_synced"`
		DetailsSynced   int      `json:"details_synced"`
		Warnings        []string `json:"warnings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ProjectsSynced != 0 || body.SummariesSynced != 0 || body.DetailsSynced != 0 {
		t.Fatalf("body = %+v, want zero-count review sync result", body)
	}
}

func TestCacheSyncReviews_AcceptsMultipleProjects(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{})
	RegisterCacheAPI(srv.API(), application)

	resp, err := http.Post(base+"/api/v1/cache/sync/reviews", "application/json", bytes.NewBufferString(`{"projects":["snap-openstack","sunbeam-charms"]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCacheDeleteGit_AcceptsRepeatedProjectQuery(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, &config.Config{
		Projects: []config.ProjectConfig{
			{Name: "keystone", Code: config.CodeConfig{Forge: "github", Owner: "canonical", Project: "keystone"}},
			{Name: "glance", Code: config.CodeConfig{Forge: "github", Owner: "canonical", Project: "glance"}},
		},
	})
	RegisterCacheAPI(srv.API(), application)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, base+"/api/v1/cache/git?project=keystone&project=glance", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200/204, got %d", resp.StatusCode)
	}
}
