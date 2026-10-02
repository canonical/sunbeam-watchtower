// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package operationstore

import (
	"context"
	"fmt"
	"sort"
	"sync"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// MemoryStore keeps operation snapshots and events in process memory.
type MemoryStore struct {
	mu     sync.RWMutex
	jobs   map[string]dto.OperationJob
	events map[string][]dto.OperationEvent
}

// NewMemoryStore creates an in-memory operation store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		jobs:   make(map[string]dto.OperationJob),
		events: make(map[string][]dto.OperationEvent),
	}
}

// Create stores a new job snapshot.
func (s *MemoryStore) Create(_ context.Context, job dto.OperationJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[job.ID]; exists {
		return fmt.Errorf("operation job %q already exists", job.ID)
	}

	s.jobs[job.ID] = cloneJob(job)
	return nil
}

// Get returns a job snapshot by ID.
func (s *MemoryStore) Get(_ context.Context, id string) (*dto.OperationJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, ok := s.jobs[id]
	if !ok {
		return nil, nil
	}

	cloned := cloneJob(job)
	return &cloned, nil
}

// List returns all known jobs ordered by creation time descending.
func (s *MemoryStore) List(_ context.Context) ([]dto.OperationJob, error) {
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

// Update replaces a previously created job snapshot.
func (s *MemoryStore) Update(_ context.Context, job dto.OperationJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[job.ID]; !exists {
		return fmt.Errorf("operation job %q not found", job.ID)
	}

	s.jobs[job.ID] = cloneJob(job)
	return nil
}

// AppendEvent adds an event to a job's event stream.
func (s *MemoryStore) AppendEvent(_ context.Context, id string, event dto.OperationEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[id]; !exists {
		return fmt.Errorf("operation job %q not found", id)
	}

	s.events[id] = append(s.events[id], cloneEvent(event))
	return nil
}

// Events returns the full event history for a job.
func (s *MemoryStore) Events(_ context.Context, id string) ([]dto.OperationEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.jobs[id]; !exists {
		return nil, nil
	}

	events := s.events[id]
	out := make([]dto.OperationEvent, 0, len(events))
	for _, event := range events {
		out = append(out, cloneEvent(event))
	}
	return out, nil
}

func cloneJob(job dto.OperationJob) dto.OperationJob {
	job.Progress = cloneProgress(job.Progress)
	job.Attributes = cloneAttributes(job.Attributes)
	return job
}

func cloneEvent(event dto.OperationEvent) dto.OperationEvent {
	event.Progress = cloneProgress(event.Progress)
	return event
}

func cloneProgress(progress *dto.OperationProgress) *dto.OperationProgress {
	if progress == nil {
		return nil
	}

	cloned := *progress
	return &cloned
}

func cloneAttributes(attributes map[string]string) map[string]string {
	if len(attributes) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(attributes))
	for key, value := range attributes {
		cloned[key] = value
	}
	return cloned
}
