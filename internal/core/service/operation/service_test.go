// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestServiceStartCompletes(t *testing.T) {
	t.Parallel()

	service := NewService(newTestStore(), slog.Default())
	job, err := service.Start(context.Background(), dto.OperationKindProjectSync, map[string]string{"scope": "test"}, func(_ context.Context, reporter *Reporter) (string, error) {
		reporter.Event("entered test runner")
		reporter.Progress(dto.OperationProgress{Phase: "step", Message: "halfway", Current: 1, Total: 2})
		return "done", nil
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	finalJob := waitForState(t, service, job.ID, dto.OperationStateSucceeded)
	if finalJob.Summary != "done" {
		t.Fatalf("Summary = %q, want done", finalJob.Summary)
	}
	if finalJob.Progress == nil || finalJob.Progress.Message != "halfway" {
		t.Fatalf("Progress = %+v, want latest progress snapshot", finalJob.Progress)
	}

	events, err := service.Events(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(events) < 4 {
		t.Fatalf("len(Events()) = %d, want at least 4", len(events))
	}
	if events[len(events)-1].Type != "succeeded" {
		t.Fatalf("last event = %+v, want succeeded", events[len(events)-1])
	}
}

func TestServiceCancel(t *testing.T) {
	t.Parallel()

	service := NewService(newTestStore(), slog.Default())
	job, err := service.Start(context.Background(), dto.OperationKindBuildTrigger, nil, func(ctx context.Context, _ *Reporter) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := service.Cancel(context.Background(), job.ID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	finalJob := waitForState(t, service, job.ID, dto.OperationStateCancelled)
	if !errors.Is(errors.New(finalJob.Error), context.Canceled) && finalJob.Error != context.Canceled.Error() {
		t.Fatalf("Error = %q, want context canceled", finalJob.Error)
	}

	events, err := service.Events(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}

	foundCancelRequested := false
	for _, event := range events {
		if event.Type == "cancel_requested" {
			foundCancelRequested = true
			break
		}
	}
	if !foundCancelRequested {
		t.Fatalf("Events() = %+v, want cancel_requested event", events)
	}
}

func TestNewServiceRecoversInFlightJobsAsInterrupted(t *testing.T) {
	t.Parallel()

	store := newTestStore()
	queued := dto.OperationJob{
		ID:          "queued-job",
		Kind:        dto.OperationKindProjectSync,
		State:       dto.OperationStateQueued,
		CreatedAt:   time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC),
		Cancellable: true,
	}
	running := dto.OperationJob{
		ID:          "running-job",
		Kind:        dto.OperationKindBuildTrigger,
		State:       dto.OperationStateRunning,
		CreatedAt:   time.Date(2026, 3, 7, 10, 1, 0, 0, time.UTC),
		StartedAt:   time.Date(2026, 3, 7, 10, 2, 0, 0, time.UTC),
		Cancellable: true,
	}
	for _, job := range []dto.OperationJob{queued, running} {
		if err := store.Create(context.Background(), job); err != nil {
			t.Fatalf("Create(%s) error = %v", job.ID, err)
		}
	}

	service := NewService(store, slog.Default())

	for _, jobID := range []string{queued.ID, running.ID} {
		job, err := service.Get(context.Background(), jobID)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", jobID, err)
		}
		if job.State != dto.OperationStateInterrupted {
			t.Fatalf("job %s state = %q, want interrupted", jobID, job.State)
		}
		if job.Cancellable {
			t.Fatalf("job %s remained cancellable after recovery", jobID)
		}
		if job.FinishedAt.IsZero() {
			t.Fatalf("job %s FinishedAt is zero after recovery", jobID)
		}
		if job.Error != interruptedMessage {
			t.Fatalf("job %s Error = %q, want %q", jobID, job.Error, interruptedMessage)
		}

		events, err := service.Events(context.Background(), jobID)
		if err != nil {
			t.Fatalf("Events(%s) error = %v", jobID, err)
		}
		if len(events) != 1 || events[0].Type != "interrupted" {
			t.Fatalf("Events(%s) = %+v, want one interrupted event", jobID, events)
		}
	}
}

func TestNewServiceLeavesTerminalJobsUntouched(t *testing.T) {
	t.Parallel()

	store := newTestStore()
	terminal := dto.OperationJob{
		ID:          "done-job",
		Kind:        dto.OperationKindProjectSync,
		State:       dto.OperationStateSucceeded,
		CreatedAt:   time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC),
		FinishedAt:  time.Date(2026, 3, 7, 10, 5, 0, 0, time.UTC),
		Cancellable: false,
		Summary:     "done",
	}
	if err := store.Create(context.Background(), terminal); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	service := NewService(store, slog.Default())
	job, err := service.Get(context.Background(), terminal.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if job.State != dto.OperationStateSucceeded || job.Summary != "done" {
		t.Fatalf("terminal job mutated during recovery: %+v", job)
	}
	events, err := service.Events(context.Background(), terminal.ID)
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("terminal job recovery events = %+v, want none", events)
	}
}

func waitForState(t *testing.T, service *Service, jobID string, want dto.OperationState) dto.OperationJob {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	job, err := service.Wait(ctx, jobID)
	if err != nil {
		t.Fatalf("Wait(%s) error = %v", jobID, err)
	}
	if job == nil {
		t.Fatalf("Wait(%s) = nil, want job in state %q", jobID, want)
	}
	if job.State != want {
		t.Fatalf("job %q final state = %q, want %q (job=%+v)", jobID, job.State, want, job)
	}
	return *job
}

type testStore struct {
	mu     sync.RWMutex
	jobs   map[string]dto.OperationJob
	events map[string][]dto.OperationEvent
}

func newTestStore() *testStore {
	return &testStore{
		jobs:   make(map[string]dto.OperationJob),
		events: make(map[string][]dto.OperationEvent),
	}
}

func (s *testStore) Create(_ context.Context, job dto.OperationJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *testStore) Get(_ context.Context, id string) (*dto.OperationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, nil
	}
	cloned := cloneJob(job)
	return &cloned, nil
}

func (s *testStore) List(_ context.Context) ([]dto.OperationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]dto.OperationJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, cloneJob(job))
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	return jobs, nil
}

func (s *testStore) Update(_ context.Context, job dto.OperationJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *testStore) AppendEvent(_ context.Context, id string, event dto.OperationEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[id] = append(s.events[id], cloneEvent(event))
	return nil
}

func (s *testStore) Events(_ context.Context, id string) ([]dto.OperationEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := s.events[id]
	out := make([]dto.OperationEvent, 0, len(events))
	for _, event := range events {
		out = append(out, cloneEvent(event))
	}
	return out, nil
}

func cloneJob(job dto.OperationJob) dto.OperationJob {
	job.Progress = cloneProgress(job.Progress)
	if len(job.Attributes) == 0 {
		job.Attributes = nil
		return job
	}

	attributes := make(map[string]string, len(job.Attributes))
	for key, value := range job.Attributes {
		attributes[key] = value
	}
	job.Attributes = attributes
	return job
}

func cloneEvent(event dto.OperationEvent) dto.OperationEvent {
	event.Progress = cloneProgress(event.Progress)
	return event
}
