// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package authflowstore

import (
	"context"
	"errors"
	"testing"
	"time"

	gh "github.com/canonical/sunbeam-watchtower/pkg/github/v1"
)

func TestMemoryGitHubFlowStore_PutGetDeleteRoundTrip(t *testing.T) {
	store := NewMemoryGitHubFlowStore()
	flow := &gh.PendingAuthFlow{
		ID:         "flow-123",
		DeviceCode: "device",
		UserCode:   "ABCD-EFGH",
		CreatedAt:  time.Now().Add(-time.Minute),
		ExpiresAt:  time.Now().Add(time.Minute),
	}

	if err := store.Put(context.Background(), flow); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, err := store.Get(context.Background(), flow.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.DeviceCode != flow.DeviceCode || got.UserCode != flow.UserCode {
		t.Fatalf("Get() = %+v, want %+v", got, flow)
	}
	if err := store.Delete(context.Background(), flow.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(context.Background(), flow.ID); !errors.Is(err, gh.ErrPendingAuthFlowNotFound) {
		t.Fatalf("Get() after delete error = %v, want %v", err, gh.ErrPendingAuthFlowNotFound)
	}
}

func TestBoltGitHubFlowStoreRoundTrip(t *testing.T) {
	store, err := NewBoltGitHubFlowStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewBoltGitHubFlowStore() error = %v", err)
	}
	defer store.Close()

	flow := &gh.PendingAuthFlow{
		ID:         "flow-123",
		DeviceCode: "device",
		UserCode:   "ABCD-EFGH",
		CreatedAt:  time.Now().Add(-time.Minute),
		ExpiresAt:  time.Now().Add(time.Minute),
	}
	if err := store.Put(context.Background(), flow); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, err := store.Get(context.Background(), flow.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.DeviceCode != flow.DeviceCode || got.UserCode != flow.UserCode {
		t.Fatalf("Get() = %+v, want %+v", got, flow)
	}
}

func TestMemoryGitHubFlowStoreReturnsExpiredFlowError(t *testing.T) {
	store := NewMemoryGitHubFlowStore()
	flow := &gh.PendingAuthFlow{
		ID:        "expired",
		CreatedAt: time.Now().Add(-2 * time.Minute),
		ExpiresAt: time.Now().Add(-time.Minute),
	}

	if err := store.Put(context.Background(), flow); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, err := store.Get(context.Background(), flow.ID); !errors.Is(err, gh.ErrPendingAuthFlowExpired) {
		t.Fatalf("Get() error = %v, want %v", err, gh.ErrPendingAuthFlowExpired)
	}
}

func TestBoltGitHubFlowStoreDeleteAndExpired(t *testing.T) {
	store, err := NewBoltGitHubFlowStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewBoltGitHubFlowStore() error = %v", err)
	}
	defer store.Close()

	active := &gh.PendingAuthFlow{
		ID:         "active",
		DeviceCode: "device",
		CreatedAt:  time.Now().Add(-time.Minute),
		ExpiresAt:  time.Now().Add(time.Minute),
	}
	expired := &gh.PendingAuthFlow{
		ID:         "expired",
		DeviceCode: "device",
		CreatedAt:  time.Now().Add(-2 * time.Minute),
		ExpiresAt:  time.Now().Add(-time.Minute),
	}
	if err := store.Put(context.Background(), active); err != nil {
		t.Fatalf("Put(active) error = %v", err)
	}
	if err := store.Put(context.Background(), expired); err != nil {
		t.Fatalf("Put(expired) error = %v", err)
	}

	if _, err := store.Get(context.Background(), expired.ID); !errors.Is(err, gh.ErrPendingAuthFlowExpired) {
		t.Fatalf("Get(expired) error = %v, want %v", err, gh.ErrPendingAuthFlowExpired)
	}
	if err := store.Delete(context.Background(), active.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(context.Background(), active.ID); !errors.Is(err, gh.ErrPendingAuthFlowNotFound) {
		t.Fatalf("Get(active) after delete error = %v, want %v", err, gh.ErrPendingAuthFlowNotFound)
	}
}
