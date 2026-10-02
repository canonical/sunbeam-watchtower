// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package authflowstore

import (
	"context"
	"errors"
	"testing"
	"time"

	sa "github.com/canonical/sunbeam-watchtower/pkg/storeauth/v1"
)

func TestMemoryStoreFlowStorePutAndGet(t *testing.T) {
	store := NewMemoryStoreFlowStore()

	flow := &sa.PendingAuthFlow{
		ID:           "flow-1",
		RootMacaroon: "root-mac",
		CaveatID:     "caveat",
		VisitURL:     "https://example.com/visit",
		WaitURL:      "https://example.com/wait",
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(10 * time.Minute),
	}

	if err := store.Put(context.Background(), flow); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, err := store.Get(context.Background(), "flow-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != "flow-1" || got.RootMacaroon != "root-mac" {
		t.Fatalf("Get() = %+v, want flow-1 with root-mac", got)
	}
}

func TestMemoryStoreFlowStoreGetNotFound(t *testing.T) {
	store := NewMemoryStoreFlowStore()

	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, sa.ErrPendingAuthFlowNotFound) {
		t.Fatalf("Get() error = %v, want %v", err, sa.ErrPendingAuthFlowNotFound)
	}
}

func TestMemoryStoreFlowStoreGetExpired(t *testing.T) {
	store := NewMemoryStoreFlowStore()

	flow := &sa.PendingAuthFlow{
		ID:        "expired-flow",
		ExpiresAt: time.Now().UTC().Add(-1 * time.Minute),
	}
	if err := store.Put(context.Background(), flow); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	_, err := store.Get(context.Background(), "expired-flow")
	if !errors.Is(err, sa.ErrPendingAuthFlowExpired) {
		t.Fatalf("Get() error = %v, want %v", err, sa.ErrPendingAuthFlowExpired)
	}
}

func TestMemoryStoreFlowStoreDelete(t *testing.T) {
	store := NewMemoryStoreFlowStore()

	flow := &sa.PendingAuthFlow{
		ID:        "flow-to-delete",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}
	if err := store.Put(context.Background(), flow); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	if err := store.Delete(context.Background(), "flow-to-delete"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err := store.Get(context.Background(), "flow-to-delete")
	if !errors.Is(err, sa.ErrPendingAuthFlowNotFound) {
		t.Fatalf("Get() after delete error = %v, want %v", err, sa.ErrPendingAuthFlowNotFound)
	}
}
