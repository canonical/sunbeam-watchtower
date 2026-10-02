// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

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
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func TestConfigShowCmd_RendersYAML(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(dto.Config{
			Build: dto.BuildConfig{DefaultPrefix: "tmp-build"},
		})
	}))
	defer ts.Close()

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "yaml",
		Client: client.NewClient(ts.URL),
	}

	cmd := newConfigCmd(opts)
	cmd.SetArgs([]string{"show"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out.String(), "default_prefix: tmp-build") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestCacheStatusCmd_RendersCacheStatus(t *testing.T) {
	baseDir := t.TempDir()
	gitDir := filepath.Join(baseDir, "git")
	releasesDir := filepath.Join(baseDir, "releases")
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
			Releases: struct {
				Directory string                   `json:"directory"`
				Entries   []dto.ReleaseCacheStatus `json:"entries"`
				Error     string                   `json:"error,omitempty"`
			}{
				Directory: releasesDir,
				Entries:   []dto.ReleaseCacheStatus{{Project: "sunbeam", Name: "snap-openstack", ArtifactType: dto.ArtifactSnap, TrackCount: 1, ChannelCount: 2}},
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
		Logger: discardTestLogger(),
	}

	cmd := newCacheCmd(opts)
	cmd.SetArgs([]string{"status"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out.String(), gitDir) || !strings.Contains(out.String(), "keystone") || !strings.Contains(out.String(), "snap-openstack") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestBugListCmd_RendersWarningsAndTasks(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/bugs" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tasks": []forge.BugTask{{
				Project: "keystone",
				BugID:   "12345",
				Title:   "Fix auth flow",
				Status:  "Triaged",
			}},
			"warnings": []string{"partial tracker failure"},
		})
	}))
	defer ts.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &errOut,
		Output: "table",
		Client: client.NewClient(ts.URL),
	}

	cmd := newBugCmd(opts)
	cmd.SetArgs([]string{"list", "--merge", "--limit", "7"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out.String(), "12345") || !strings.Contains(errOut.String(), "partial tracker failure") {
		t.Fatalf("unexpected output: out=%q err=%q", out.String(), errOut.String())
	}
	if !strings.Contains(gotQuery, "merge=true") {
		t.Fatalf("query = %q, want merge=true", gotQuery)
	}
	if !strings.Contains(gotQuery, "limit=7") {
		t.Fatalf("query = %q, want limit=7", gotQuery)
	}
}

func TestBugSearchCmdMapsFlagsAndRendersEvidence(t *testing.T) {
	var got dto.BugSearchRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/bugs/search" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(dto.BugSearchResponse{
			Outcome: dto.BugSearchOutcomeMatches,
			Applied: dto.BugSearchApplied{
				Mode: "text", Fields: []string{"title"}, Fuzzy: false,
				Closed: "include", Merge: true, Sort: "relevance",
				DocumentCount: 2,
			},
			Results: []dto.BugSearchResult{{
				Reference:      "launchpad:12345",
				Title:          "Complete untruncated Cinder title",
				Classification: dto.BugSearchDirect,
				Score:          12.5,
				URL:            "https://bugs.launchpad.net/sunbeam/+bug/12345",
				Evidence: []dto.BugMatchEvidence{{
					Field:   "title",
					Excerpt: "Complete untruncated Cinder title",
					Matches: []dto.BugEvidenceMatch{{
						MatchType:    "exact",
						QueryConcept: "cinder",
						MatchedText:  "Cinder",
					}},
				}},
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
	cmd := newBugCmd(opts)
	cmd.SetArgs([]string{
		"search", "cinder", "snapshot",
		"--field", "title",
		"--fuzzy=false",
		"--require", "restart",
		"--limit", "7",
		"--evidence-limit", "3",
	})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got.Query != "cinder snapshot" || got.Limit != 7 || got.EvidenceLimit != 3 ||
		got.Fuzzy == nil || *got.Fuzzy || len(got.RequiredTerms) != 1 {
		t.Fatalf("request = %+v", got)
	}
	if !strings.Contains(out.String(), "Complete untruncated Cinder title") ||
		!strings.Contains(out.String(), `Why: title [cinder/exact="Cinder"]`) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestReviewListCmd_RendersWarningsAndMergeRequests(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/reviews" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"merge_requests": []forge.MergeRequest{{
				ID:    "42",
				Repo:  "keystone",
				Title: "Refactor auth",
				URL:   "https://example.invalid/42",
			}},
			"warnings": []string{"forge timeout"},
		})
	}))
	defer ts.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &errOut,
		Output: "table",
		Client: client.NewClient(ts.URL),
		Logger: discardTestLogger(),
	}

	cmd := newReviewCmd(opts)
	cmd.SetArgs([]string{"list"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out.String(), "Refactor auth") || !strings.Contains(errOut.String(), "forge timeout") {
		t.Fatalf("unexpected output: out=%q err=%q", out.String(), errOut.String())
	}
}

func TestCommitTrackCmd_RendersWarningsAndCommits(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/commits/track" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"commits": []forge.Commit{{
				SHA:     "deadbeef",
				Repo:    "keystone",
				Message: "LP: #12345 fix auth workflow",
				URL:     "https://example.invalid/deadbeef",
			}},
			"warnings": []string{"partial history"},
		})
	}))
	defer ts.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &errOut,
		Output: "table",
		Client: client.NewClient(ts.URL),
		Logger: discardTestLogger(),
	}

	cmd := newCommitCmd(opts)
	cmd.SetArgs([]string{"track", "--bug-id", "12345"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out.String(), "deadbeef") || !strings.Contains(errOut.String(), "partial history") {
		t.Fatalf("unexpected output: out=%q err=%q", out.String(), errOut.String())
	}
}
