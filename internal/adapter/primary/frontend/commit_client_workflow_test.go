// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canonical/sunbeam-watchtower/pkg/client"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func TestCommitClientWorkflowLog(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/commits" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		query := r.URL.Query()
		if query.Get("branch") != "main" || query.Get("author") != "jdoe" || query.Get("include_mrs") != "true" {
			t.Fatalf("query = %s, want branch/author/include_mrs", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"commits": []forge.Commit{{
				SHA:     "deadbeef",
				Repo:    "keystone",
				Message: "Refactor auth workflow",
			}},
			"warnings": []string{"partial history"},
		})
	}))
	defer ts.Close()

	workflow := NewCommitClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.Log(context.Background(), CommitLogRequest{
		Projects:   []string{"keystone"},
		Branch:     "main",
		Author:     "jdoe",
		IncludeMRs: true,
	})
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	if len(got.Commits) != 1 || got.Commits[0].SHA != "deadbeef" {
		t.Fatalf("Log() = %+v, want deadbeef", got)
	}
	if len(got.Warnings) != 1 || got.Warnings[0] != "partial history" {
		t.Fatalf("warnings = %+v, want partial history", got.Warnings)
	}
}

func TestCommitClientWorkflowTrack(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/commits/track" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("bug_id") != "12345" {
			t.Fatalf("bug_id = %q, want 12345", r.URL.Query().Get("bug_id"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"commits": []forge.Commit{{
				SHA:     "cafebabe",
				Repo:    "keystone",
				Message: "LP: #12345 fix auth workflow",
			}},
		})
	}))
	defer ts.Close()

	workflow := NewCommitClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.Track(context.Background(), CommitTrackRequest{BugID: "12345"})
	if err != nil {
		t.Fatalf("Track() error = %v", err)
	}
	if len(got.Commits) != 1 || got.Commits[0].SHA != "cafebabe" {
		t.Fatalf("Track() = %+v, want cafebabe", got)
	}
}
