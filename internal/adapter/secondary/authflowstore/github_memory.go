// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package authflowstore

import (
	"context"
	"sync"
	"time"

	gh "github.com/canonical/sunbeam-watchtower/pkg/github/v1"
)

// MemoryGitHubFlowStore stores pending GitHub auth flows in memory.
type MemoryGitHubFlowStore struct {
	mu    sync.Mutex
	flows map[string]gh.PendingAuthFlow
}

// NewMemoryGitHubFlowStore creates an in-memory pending auth flow store.
func NewMemoryGitHubFlowStore() *MemoryGitHubFlowStore {
	return &MemoryGitHubFlowStore{
		flows: make(map[string]gh.PendingAuthFlow),
	}
}

// Put stores or replaces a pending auth flow.
func (s *MemoryGitHubFlowStore) Put(_ context.Context, flow *gh.PendingAuthFlow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpiredLocked(time.Now())
	s.flows[flow.ID] = *flow
	return nil
}

// Get returns a pending auth flow if it exists and has not expired.
func (s *MemoryGitHubFlowStore) Get(_ context.Context, id string) (*gh.PendingAuthFlow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.pruneExpiredExceptLocked(now, id)

	flow, ok := s.flows[id]
	if !ok {
		return nil, gh.ErrPendingAuthFlowNotFound
	}
	if !flow.ExpiresAt.IsZero() && now.After(flow.ExpiresAt) {
		delete(s.flows, id)
		return nil, gh.ErrPendingAuthFlowExpired
	}

	flowCopy := flow
	return &flowCopy, nil
}

// Delete removes a pending auth flow.
func (s *MemoryGitHubFlowStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.flows, id)
	return nil
}

func (s *MemoryGitHubFlowStore) pruneExpiredLocked(now time.Time) {
	s.pruneExpiredExceptLocked(now, "")
}

func (s *MemoryGitHubFlowStore) pruneExpiredExceptLocked(now time.Time, keepID string) {
	for id, flow := range s.flows {
		if id == keepID {
			continue
		}
		if !flow.ExpiresAt.IsZero() && now.After(flow.ExpiresAt) {
			delete(s.flows, id)
		}
	}
}
