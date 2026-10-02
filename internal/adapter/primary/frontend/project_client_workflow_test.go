// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestProjectClientWorkflowSync(t *testing.T) {
	var gotBody client.ProjectsSyncOptions
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects/sync" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_ = json.NewEncoder(w).Encode(client.ProjectsSyncResult{
			Actions: []dto.ProjectSyncAction{{
				Project:    "demo",
				Series:     "2025.1",
				ActionType: dto.ProjectSyncActionSetDevFocus,
			}},
			Errors: []string{"non-fatal warning"},
		})
	}))
	defer ts.Close()

	workflow := NewProjectClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	got, err := workflow.Sync(context.Background(), ProjectSyncRequest{
		Projects: []string{"demo"},
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(got.Actions) != 1 || got.Actions[0].Project != "demo" {
		t.Fatalf("Sync() = %+v, want one demo action", got)
	}
	if len(got.Errors) != 1 || got.Errors[0] != "non-fatal warning" {
		t.Fatalf("Sync() errors = %+v, want warning", got.Errors)
	}
	if len(gotBody.Projects) != 1 || gotBody.Projects[0] != "demo" || !gotBody.DryRun {
		t.Fatalf("request body = %+v, want demo dry-run request", gotBody)
	}
}

func TestProjectClientWorkflowStartAndWaitForSyncCompletion(t *testing.T) {
	responses := []dto.OperationJob{
		{ID: "op-project-1", Kind: dto.OperationKindProjectSync, State: dto.OperationStateRunning},
		{ID: "op-project-1", Kind: dto.OperationKindProjectSync, State: dto.OperationStateSucceeded, Summary: "done"},
	}
	index := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/sync/async":
			_ = json.NewEncoder(w).Encode(dto.OperationJob{
				ID:    "op-project-1",
				Kind:  dto.OperationKindProjectSync,
				State: dto.OperationStateQueued,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/operations/op-project-1":
			response := responses[index]
			if index < len(responses)-1 {
				index++
			}
			_ = json.NewEncoder(w).Encode(response)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ts.Close()

	workflow := NewProjectClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	job, err := workflow.StartSync(context.Background(), ProjectSyncRequest{
		Projects: []string{"demo"},
	})
	if err != nil {
		t.Fatalf("StartSync() error = %v", err)
	}
	if job.ID != "op-project-1" || job.Kind != dto.OperationKindProjectSync {
		t.Fatalf("StartSync() = %+v, want project sync operation", job)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	job, err = workflow.WaitForSyncCompletion(ctx, "op-project-1", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForSyncCompletion() error = %v", err)
	}
	if job.State != dto.OperationStateSucceeded || job.Summary != "done" {
		t.Fatalf("WaitForSyncCompletion() = %+v, want succeeded done", job)
	}
}
