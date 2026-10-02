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

func TestReviewClientWorkflowList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/reviews" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		query := r.URL.Query()
		if got := query["project"]; len(got) != 1 || got[0] != "keystone" {
			t.Fatalf("project query = %+v, want keystone", got)
		}
		if query.Get("since") != "2025-01-01T00:00:00Z" {
			t.Fatalf("since = %q, want 2025-01-01T00:00:00Z", query.Get("since"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"merge_requests": []forge.MergeRequest{{
				ID:    "42",
				Repo:  "keystone",
				Title: "Refactor auth",
			}},
			"warnings": []string{"forge timeout"},
		})
	}))
	defer ts.Close()

	workflow := NewReviewClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.List(context.Background(), ReviewListRequest{
		Projects: []string{"keystone"},
		Since:    "2025-01-01",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got.MergeRequests) != 1 || got.MergeRequests[0].ID != "42" {
		t.Fatalf("List() = %+v, want MR 42", got)
	}
	if len(got.Warnings) != 1 || got.Warnings[0] != "forge timeout" {
		t.Fatalf("warnings = %+v, want forge timeout", got.Warnings)
	}
}

func TestReviewClientWorkflowShow(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/reviews/keystone/42" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(forge.MergeRequest{
			ID:    "42",
			Repo:  "keystone",
			Title: "Refactor auth",
		})
	}))
	defer ts.Close()

	workflow := NewReviewClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.Show(context.Background(), "keystone", "42")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if got.ID != "42" {
		t.Fatalf("Show() = %+v, want 42", got)
	}
}
