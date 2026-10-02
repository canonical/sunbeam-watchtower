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

func TestBoltLaunchpadFlowStoreRoundTrip(t *testing.T) {
	store, err := NewBoltLaunchpadFlowStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewBoltLaunchpadFlowStore() error = %v", err)
	}
	defer store.Close()

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

	got, err := store.Get(context.Background(), flow.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != flow.ID || got.RequestToken != flow.RequestToken || got.RequestTokenSecret != flow.RequestTokenSecret {
		t.Fatalf("Get() = %+v, want %+v", got, flow)
	}

	if err := store.Delete(context.Background(), flow.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(context.Background(), flow.ID); !errors.Is(err, lp.ErrPendingAuthFlowNotFound) {
		t.Fatalf("Get() after delete error = %v, want %v", err, lp.ErrPendingAuthFlowNotFound)
	}
}

func TestBoltLaunchpadFlowStoreGetExpiredDeletesFlow(t *testing.T) {
	store, err := NewBoltLaunchpadFlowStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewBoltLaunchpadFlowStore() error = %v", err)
	}
	defer store.Close()

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
