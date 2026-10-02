// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	opsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/operation"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

var _ port.OperationStore = (*fakeOperationStore)(nil)

type fakeOperationStore struct {
	mu     sync.RWMutex
	jobs   map[string]dto.OperationJob
	events map[string][]dto.OperationEvent
}

func newFakeOperationStore() *fakeOperationStore {
	return &fakeOperationStore{
		jobs:   make(map[string]dto.OperationJob),
		events: make(map[string][]dto.OperationEvent),
	}
}

func (s *fakeOperationStore) Create(_ context.Context, job dto.OperationJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return fmt.Errorf("operation job %q already exists", job.ID)
	}
	s.jobs[job.ID] = job
	return nil
}

func (s *fakeOperationStore) Get(_ context.Context, id string) (*dto.OperationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, nil
	}
	return &job, nil
}

func (s *fakeOperationStore) List(_ context.Context) ([]dto.OperationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]dto.OperationJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	return jobs, nil
}

func (s *fakeOperationStore) Update(_ context.Context, job dto.OperationJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; !exists {
		return fmt.Errorf("operation job %q not found", job.ID)
	}
	s.jobs[job.ID] = job
	return nil
}

func (s *fakeOperationStore) AppendEvent(_ context.Context, id string, event dto.OperationEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[id]; !exists {
		return fmt.Errorf("operation job %q not found", id)
	}
	s.events[id] = append(s.events[id], event)
	return nil
}

func (s *fakeOperationStore) Events(_ context.Context, id string) ([]dto.OperationEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]dto.OperationEvent(nil), s.events[id]...), nil
}

func TestOperationWorkflowStartListGetCancel(t *testing.T) {
	service := opsvc.NewService(newFakeOperationStore(), discardFrontendLogger())
	workflow := NewOperationWorkflowFromService(service)

	blocked := make(chan struct{})
	job, err := workflow.Start(context.Background(), dto.OperationKindProjectSync, map[string]string{"scope": "test"}, func(ctx context.Context, reporter *opsvc.Reporter) (string, error) {
		reporter.Event("entered runner")
		reporter.Progress(dto.OperationProgress{Phase: "running", Message: "working"})
		close(blocked)
		<-ctx.Done()
		return "", ctx.Err()
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	<-blocked

	jobs, err := workflow.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != job.ID {
		t.Fatalf("List() = %+v, want one job %q", jobs, job.ID)
	}

	got, err := workflow.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.ID != job.ID {
		t.Fatalf("Get() = %+v, want job %q", got, job.ID)
	}

	events, err := workflow.Events(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(events) == 0 {
		t.Fatal("Events() returned no events")
	}

	if err := workflow.Cancel(context.Background(), job.ID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err = service.Wait(waitCtx, job.ID)
	if err != nil {
		t.Fatalf("Wait() after cancel error = %v", err)
	}
	if got == nil || got.State != dto.OperationStateCancelled {
		t.Fatalf("job state after cancel = %+v, want cancelled", got)
	}
	if got.Error != context.Canceled.Error() {
		t.Fatalf("cancelled job error = %q, want %q", got.Error, context.Canceled.Error())
	}

	events, err = workflow.Events(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Events() after cancel error = %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("Events() after cancel = %+v, want recorded lifecycle events", events)
	}
}
