// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gboutry/sunbeam-watchtower/pkg/client"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
)

func TestCacheSyncReleasesRendersCountsAndWarnings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/cache/sync/releases" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(dto.ReleaseSyncResult{
			Status:     "ok",
			Discovered: 4,
			Synced:     3,
			Skipped:    1,
			Warnings:   []string{"sunbeam: skipped (no series, release.tracks, or release.branches configured)"},
		})
	}))
	defer server.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &errOut,
		Output: "table",
		Client: client.NewClient(server.URL),
		Logger: discardTestLogger(),
	}

	cmd := newCacheCmd(opts)
	cmd.SetArgs([]string{"sync", "releases"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "discovered 4, synced 3, skipped 1") {
		t.Fatalf("stdout = %q, want counted release sync summary", got)
	}
	if got := errOut.String(); !strings.Contains(got, "warning: sunbeam: skipped") {
		t.Fatalf("stderr = %q, want release skip warning", got)
	}
}

func TestCacheSyncSRUUsesCacheEndpointAndReportsTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/cache/sync/sru" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(dto.SRUSnapshot{Rows: []dto.SRURow{{BugID: "2167438"}, {BugID: "2167438"}}})
	}))
	defer server.Close()
	var out bytes.Buffer
	opts := &Options{Out: &out, ErrOut: &bytes.Buffer{}, Output: "table", Client: client.NewClient(server.URL), Logger: discardTestLogger()}
	cmd := newCacheCmd(opts)
	cmd.SetArgs([]string{"sync", "sru"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "2 targets") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCacheSyncReviewsRendersCountsAndWarnings(t *testing.T) {
	var gotBody client.CacheSyncReviewsOptions
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			Warnings:        []string{"snap-openstack:#42: detail fetch failed"},
		})
	}))
	defer server.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &errOut,
		Output: "table",
		Client: client.NewClient(server.URL),
		Logger: discardTestLogger(),
	}

	cmd := newCacheCmd(opts)
	cmd.SetArgs([]string{"sync", "reviews", "--project", "snap-openstack", "--project", "sunbeam-charms"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(gotBody.Projects) != 2 || gotBody.Projects[0] != "snap-openstack" || gotBody.Projects[1] != "sunbeam-charms" {
		t.Fatalf("request body = %+v, want both projects", gotBody)
	}
	if got := out.String(); !strings.Contains(got, "1 projects, 5 summaries, 3 details") {
		t.Fatalf("stdout = %q, want counted review sync summary", got)
	}
	if got := errOut.String(); !strings.Contains(got, "warning: snap-openstack:#42") {
		t.Fatalf("stderr = %q, want review warning", got)
	}
}

func TestCacheClearGitSupportsRepeatedProjectFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/cache/git" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query()["project"]; len(got) != 2 || got[0] != "keystone" || got[1] != "glance" {
			t.Fatalf("project query = %+v, want repeated projects", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "table",
		Client: client.NewClient(server.URL),
		Logger: discardTestLogger(),
	}

	cmd := newCacheCmd(opts)
	cmd.SetArgs([]string{"clear", "git", "--project", "keystone", "--project", "glance"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "git cache") {
		t.Fatalf("stdout = %q, want git cache clear output", got)
	}
}
