// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package authflowstore

import (
	"context"
	"errors"
	"testing"
	"time"

	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

func TestMemoryLaunchpadFlowStore_PutGetDeleteRoundTrip(t *testing.T) {
	store := NewMemoryLaunchpadFlowStore()
	flow := &lp.PendingAuthFlow{
		ID:                 "flow-123",
		RequestToken:       "token",
		RequestTokenSecret: "secret",
		CreatedAt:          time.Now().Add(-time.Minute),
		ExpiresAt:          time.Now().Add(time.Minute),
	}

	if err := store.Put(context.Background(), flow); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, err := store.Get(context.Background(), "flow-123")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != flow.ID || got.RequestToken != flow.RequestToken || got.RequestTokenSecret != flow.RequestTokenSecret {
		t.Fatalf("Get() = %+v, want %+v", got, flow)
	}

	if err := store.Delete(context.Background(), "flow-123"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(context.Background(), "flow-123"); !errors.Is(err, lp.ErrPendingAuthFlowNotFound) {
		t.Fatalf("Get() after delete error = %v, want %v", err, lp.ErrPendingAuthFlowNotFound)
	}
}

func TestMemoryLaunchpadFlowStore_GetExpiredDeletesFlow(t *testing.T) {
	store := NewMemoryLaunchpadFlowStore()
	if err := store.Put(context.Background(), &lp.PendingAuthFlow{
		ID:                 "expired-flow",
		RequestToken:       "token",
		RequestTokenSecret: "secret",
		CreatedAt:          time.Now().Add(-2 * time.Minute),
		ExpiresAt:          time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	if _, err := store.Get(context.Background(), "expired-flow"); !errors.Is(err, lp.ErrPendingAuthFlowExpired) {
		t.Fatalf("Get() error = %v, want %v", err, lp.ErrPendingAuthFlowExpired)
	}
	if _, err := store.Get(context.Background(), "expired-flow"); !errors.Is(err, lp.ErrPendingAuthFlowNotFound) {
		t.Fatalf("Get() second error = %v, want %v", err, lp.ErrPendingAuthFlowNotFound)
	}
}

func TestMemoryLaunchpadFlowStore_PutPrunesOtherExpiredFlows(t *testing.T) {
	store := NewMemoryLaunchpadFlowStore()
	if err := store.Put(context.Background(), &lp.PendingAuthFlow{
		ID:        "old-flow",
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Put(old) error = %v", err)
	}

	if err := store.Put(context.Background(), &lp.PendingAuthFlow{
		ID:        "new-flow",
		ExpiresAt: time.Now().Add(time.Minute),
	}); err != nil {
		t.Fatalf("Put(new) error = %v", err)
	}

	if _, err := store.Get(context.Background(), "old-flow"); !errors.Is(err, lp.ErrPendingAuthFlowNotFound) {
		t.Fatalf("Get(old) error = %v, want %v", err, lp.ErrPendingAuthFlowNotFound)
	}
	if _, err := store.Get(context.Background(), "new-flow"); err != nil {
		t.Fatalf("Get(new) error = %v", err)
	}
}
