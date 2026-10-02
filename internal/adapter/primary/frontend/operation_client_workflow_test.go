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

func TestOperationClientWorkflowListGetEventsCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/operations":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jobs": []dto.OperationJob{{
					ID:    "op-1",
					Kind:  dto.OperationKindProjectSync,
					State: dto.OperationStateRunning,
				}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/operations/op-1":
			_ = json.NewEncoder(w).Encode(dto.OperationJob{
				ID:    "op-1",
				Kind:  dto.OperationKindProjectSync,
				State: dto.OperationStateRunning,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/operations/op-1/events":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"events": []dto.OperationEvent{{Type: "started", Message: "operation started"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/operations/op-1/cancel":
			_ = json.NewEncoder(w).Encode(dto.OperationJob{
				ID:    "op-1",
				Kind:  dto.OperationKindProjectSync,
				State: dto.OperationStateCancelled,
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ts.Close()

	workflow := NewOperationClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	jobs, err := workflow.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "op-1" {
		t.Fatalf("List() = %+v, want op-1", jobs)
	}

	job, err := workflow.Get(context.Background(), "op-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if job == nil || job.ID != "op-1" {
		t.Fatalf("Get() = %+v, want op-1", job)
	}

	events, err := workflow.Events(context.Background(), "op-1")
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != "started" {
		t.Fatalf("Events() = %+v, want started event", events)
	}

	job, err = workflow.Cancel(context.Background(), "op-1")
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if job == nil || job.State != dto.OperationStateCancelled {
		t.Fatalf("Cancel() = %+v, want cancelled job", job)
	}
}

func TestOperationClientWorkflowWaitForTerminalState(t *testing.T) {
	responses := []dto.OperationJob{
		{ID: "op-1", Kind: dto.OperationKindProjectSync, State: dto.OperationStateQueued},
		{ID: "op-1", Kind: dto.OperationKindProjectSync, State: dto.OperationStateRunning},
		{ID: "op-1", Kind: dto.OperationKindProjectSync, State: dto.OperationStateSucceeded, Summary: "done"},
	}
	index := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/operations/op-1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		response := responses[index]
		if index < len(responses)-1 {
			index++
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	workflow := NewOperationClientWorkflow(NewClientTransport(client.NewClient(ts.URL)))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	job, err := workflow.WaitForTerminalState(ctx, "op-1", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForTerminalState() error = %v", err)
	}
	if job == nil || job.State != dto.OperationStateSucceeded || job.Summary != "done" {
		t.Fatalf("WaitForTerminalState() = %+v, want succeeded done", job)
	}
}
