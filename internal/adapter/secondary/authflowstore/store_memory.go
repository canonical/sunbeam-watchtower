// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package authflowstore

import (
	"context"
	"sync"
	"time"

	sa "github.com/canonical/sunbeam-watchtower/pkg/storeauth/v1"
)

// MemoryStoreFlowStore stores pending store auth flows in memory.
// It implements both port.SnapStorePendingAuthFlowStore and port.CharmhubPendingAuthFlowStore.
type MemoryStoreFlowStore struct {
	mu    sync.Mutex
	flows map[string]sa.PendingAuthFlow
}

// NewMemoryStoreFlowStore creates an in-memory pending store auth flow store.
func NewMemoryStoreFlowStore() *MemoryStoreFlowStore {
	return &MemoryStoreFlowStore{
		flows: make(map[string]sa.PendingAuthFlow),
	}
}

// Put stores or replaces a pending auth flow.
func (s *MemoryStoreFlowStore) Put(_ context.Context, flow *sa.PendingAuthFlow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpiredLocked(time.Now())
	s.flows[flow.ID] = *flow
	return nil
}

// Get returns a pending auth flow if it exists and has not expired.
func (s *MemoryStoreFlowStore) Get(_ context.Context, id string) (*sa.PendingAuthFlow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.pruneExpiredExceptLocked(now, id)

	flow, ok := s.flows[id]
	if !ok {
		return nil, sa.ErrPendingAuthFlowNotFound
	}
	if !flow.ExpiresAt.IsZero() && now.After(flow.ExpiresAt) {
		delete(s.flows, id)
		return nil, sa.ErrPendingAuthFlowExpired
	}

	flowCopy := flow
	return &flowCopy, nil
}

// Delete removes a pending auth flow.
func (s *MemoryStoreFlowStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.flows, id)
	return nil
}

func (s *MemoryStoreFlowStore) pruneExpiredLocked(now time.Time) {
	s.pruneExpiredExceptLocked(now, "")
}

func (s *MemoryStoreFlowStore) pruneExpiredExceptLocked(now time.Time, keepID string) {
	for id, flow := range s.flows {
		if id == keepID {
			continue
		}
		if !flow.ExpiresAt.IsZero() && now.After(flow.ExpiresAt) {
			delete(s.flows, id)
		}
	}
}
