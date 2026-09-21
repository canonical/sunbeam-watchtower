// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gboutry/sunbeam-watchtower/pkg/client"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/gboutry/sunbeam-watchtower/pkg/forge/v1"
)

func TestBugClientWorkflowList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/bugs" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		query := r.URL.Query()
		if got := query["project"]; len(got) != 1 || got[0] != "keystone" {
			t.Fatalf("project query = %+v, want keystone", got)
		}
		if query.Get("since") != "2025-01-01T00:00:00Z" {
			t.Fatalf("since = %q, want 2025-01-01T00:00:00Z", query.Get("since"))
		}
		if query.Get("merge") != "true" {
			t.Fatalf("merge = %q, want true", query.Get("merge"))
		}
		if query.Get("limit") != "5" {
			t.Fatalf("limit = %q, want 5", query.Get("limit"))
		}
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

	workflow := NewBugClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.List(context.Background(), BugListRequest{
		Projects: []string{"keystone"},
		Since:    "2025-01-01",
		Merge:    true,
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got.Tasks) != 1 || got.Tasks[0].BugID != "12345" {
		t.Fatalf("List() = %+v, want bug task 12345", got)
	}
	if len(got.Warnings) != 1 || got.Warnings[0] != "partial tracker failure" {
		t.Fatalf("warnings = %+v, want partial tracker failure", got.Warnings)
	}
}

func TestBugClientWorkflowSearch(t *testing.T) {
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
			Results: []dto.BugSearchResult{{Reference: "launchpad:12345"}},
		})
	}))
	defer ts.Close()

	workflow := NewBugClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	result, err := workflow.Search(context.Background(), dto.BugSearchRequest{
		Query:         "snapshot restart",
		ModifiedAfter: "2025-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Query != "snapshot restart" || got.ModifiedAfter != "2025-01-01T00:00:00Z" {
		t.Fatalf("request = %+v", got)
	}
	if len(result.Results) != 1 || result.Results[0].Reference != "launchpad:12345" {
		t.Fatalf("result = %+v", result)
	}
}

func TestBugClientWorkflowSync(t *testing.T) {
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/bugs/sync" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"actions": []dto.BugSyncAction{{
				BugID:      "12345",
				ActionType: dto.BugSyncActionStatusUpdate,
			}},
			"skipped": 2,
			"errors":  []string{"series missing"},
		})
	}))
	defer ts.Close()

	workflow := NewBugClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.Sync(context.Background(), BugSyncRequest{
		Projects: []string{"keystone"},
		BugIDs:   []string{"#12345", "12345", "23456"},
		DryRun:   true,
		Since:    "2025-01-01",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if gotBody["since"] != "2025-01-01T00:00:00Z" {
		t.Fatalf("since body = %v, want 2025-01-01T00:00:00Z", gotBody["since"])
	}
	if gotBody["dry_run"] != true {
		t.Fatalf("dry_run = %v, want true", gotBody["dry_run"])
	}
	bugIDs, ok := gotBody["bug_ids"].([]any)
	if !ok || len(bugIDs) != 2 || bugIDs[0] != "12345" || bugIDs[1] != "23456" {
		t.Fatalf("bug_ids = %#v, want normalized IDs", gotBody["bug_ids"])
	}
	if got.Result == nil || got.Result.Skipped != 2 || len(got.Result.Actions) != 1 {
		t.Fatalf("Sync() result = %+v, want actions and skipped", got.Result)
	}
	if len(got.Warnings) != 1 || got.Warnings[0] != "series missing" {
		t.Fatalf("warnings = %+v, want series missing", got.Warnings)
	}
}

func TestBugClientWorkflowShow(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/bugs/12345" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(forge.Bug{
			ID:    "12345",
			Title: "Fix auth flow",
		})
	}))
	defer ts.Close()

	workflow := NewBugClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.Show(context.Background(), "12345")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if got.ID != "12345" {
		t.Fatalf("Show() = %+v, want 12345", got)
	}
}
