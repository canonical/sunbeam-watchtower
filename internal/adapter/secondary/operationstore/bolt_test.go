// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package operationstore

import (
	"context"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestBoltStoreRoundTrip(t *testing.T) {
	store, err := NewBoltStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewBoltStore() error = %v", err)
	}
	defer store.Close()

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

func TestBoltStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()

	store, err := NewBoltStore(dir)
	if err != nil {
		t.Fatalf("NewBoltStore() error = %v", err)
	}

	job := dto.OperationJob{
		ID:        "job-1",
		Kind:      dto.OperationKindProjectSync,
		State:     dto.OperationStateSucceeded,
		CreatedAt: time.Now().UTC(),
		Summary:   "done",
	}
	if err := store.Create(context.Background(), job); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := NewBoltStore(dir)
	if err != nil {
		t.Fatalf("NewBoltStore(reopen) error = %v", err)
	}
	defer reopened.Close()

	got, err := reopened.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.Summary != "done" {
		t.Fatalf("Get() = %+v, want persisted job", got)
	}
}
