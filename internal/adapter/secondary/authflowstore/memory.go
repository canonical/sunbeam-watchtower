// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package authflowstore

import (
	"context"
	"sync"
	"time"

	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

// MemoryLaunchpadFlowStore stores pending Launchpad auth flows in memory.
type MemoryLaunchpadFlowStore struct {
	mu    sync.Mutex
	flows map[string]lp.PendingAuthFlow
}

// NewMemoryLaunchpadFlowStore creates an in-memory pending auth flow store.
func NewMemoryLaunchpadFlowStore() *MemoryLaunchpadFlowStore {
	return &MemoryLaunchpadFlowStore{
		flows: make(map[string]lp.PendingAuthFlow),
	}
}

// Put stores or replaces a pending auth flow.
func (s *MemoryLaunchpadFlowStore) Put(_ context.Context, flow *lp.PendingAuthFlow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpiredLocked(time.Now())
	s.flows[flow.ID] = *flow
	return nil
}

// Get returns a pending auth flow if it exists and has not expired.
func (s *MemoryLaunchpadFlowStore) Get(_ context.Context, id string) (*lp.PendingAuthFlow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.pruneExpiredExceptLocked(now, id)

	flow, ok := s.flows[id]
	if !ok {
		return nil, lp.ErrPendingAuthFlowNotFound
	}
	if !flow.ExpiresAt.IsZero() && now.After(flow.ExpiresAt) {
		delete(s.flows, id)
		return nil, lp.ErrPendingAuthFlowExpired
	}

	flowCopy := flow
	return &flowCopy, nil
}

// Delete removes a pending auth flow.
func (s *MemoryLaunchpadFlowStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.flows, id)
	return nil
}

func (s *MemoryLaunchpadFlowStore) pruneExpiredLocked(now time.Time) {
	s.pruneExpiredExceptLocked(now, "")
}

func (s *MemoryLaunchpadFlowStore) pruneExpiredExceptLocked(now time.Time, keepID string) {
	for id, flow := range s.flows {
		if id == keepID {
			continue
		}
		if !flow.ExpiresAt.IsZero() && now.After(flow.ExpiresAt) {
			delete(s.flows, id)
		}
	}
}
