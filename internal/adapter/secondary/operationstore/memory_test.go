// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package operationstore

import (
	"context"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestMemoryStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	now := time.Now().UTC()
	job := dto.OperationJob{
		ID:        "job-1",
		Kind:      dto.OperationKindBuildTrigger,
		State:     dto.OperationStateQueued,
		CreatedAt: now,
		Attributes: map[string]string{
			"project": "demo",
		},
	}

	if err := store.Create(context.Background(), job); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.Attributes["project"] != "demo" {
		t.Fatalf("Get() = %+v, want stored job", got)
	}

	job.State = dto.OperationStateRunning
	job.Progress = &dto.OperationProgress{Phase: "syncing", Message: "in progress", Current: 1, Total: 2}
	if err := store.Update(context.Background(), job); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if err := store.AppendEvent(context.Background(), job.ID, dto.OperationEvent{
		Time:    now.Add(time.Second),
		Type:    "progress",
		Message: "step complete",
	}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	events, err := store.Events(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(events) != 1 || events[0].Message != "step complete" {
		t.Fatalf("Events() = %+v, want appended event", events)
	}

	list, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].State != dto.OperationStateRunning {
		t.Fatalf("List() = %+v, want updated job", list)
	}
}

func TestMemoryStoreReturnsCopies(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	job := dto.OperationJob{
		ID:        "job-1",
		Kind:      dto.OperationKindProjectSync,
		State:     dto.OperationStateQueued,
		CreatedAt: time.Now().UTC(),
		Attributes: map[string]string{
			"project": "demo",
		},
	}

	if err := store.Create(context.Background(), job); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	got.Attributes["project"] = "mutated"

	again, err := store.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if again.Attributes["project"] != "demo" {
		t.Fatalf("Get() returned shared map, got %+v", again.Attributes)
	}
}
