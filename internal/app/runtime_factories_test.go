// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/authflowstore"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/operationstore"
)

func TestNewLaunchpadPendingAuthFlowStoreUsesMemoryInEphemeralMode(t *testing.T) {
	store := newLaunchpadPendingAuthFlowStore(testAppLogger(), RuntimeModeEphemeral, func() (string, error) {
		t.Fatal("stateDir should not be called for ephemeral auth flow store")
		return "", nil
	})

	if _, ok := store.(*authflowstore.MemoryLaunchpadFlowStore); !ok {
		t.Fatalf("store = %T, want *authflowstore.MemoryLaunchpadFlowStore", store)
	}
}

func TestNewLaunchpadPendingAuthFlowStoreUsesBoltInPersistentMode(t *testing.T) {
	dir := t.TempDir()
	store := newLaunchpadPendingAuthFlowStore(testAppLogger(), RuntimeModePersistent, func() (string, error) {
		return dir, nil
	})
	defer store.(interface{ Close() error }).Close()

	if _, ok := store.(*authflowstore.BoltLaunchpadFlowStore); !ok {
		t.Fatalf("store = %T, want *authflowstore.BoltLaunchpadFlowStore", store)
	}
}

func TestNewLaunchpadPendingAuthFlowStoreFallsBackToMemory(t *testing.T) {
	store := newLaunchpadPendingAuthFlowStore(testAppLogger(), RuntimeModePersistent, func() (string, error) {
		return "", errors.New("boom")
	})

	if _, ok := store.(*authflowstore.MemoryLaunchpadFlowStore); !ok {
		t.Fatalf("store = %T, want *authflowstore.MemoryLaunchpadFlowStore", store)
	}
}

func TestNewOperationStoreUsesMemoryInEphemeralMode(t *testing.T) {
	store := newOperationStore(testAppLogger(), RuntimeModeEphemeral, func() (string, error) {
		t.Fatal("stateDir should not be called for ephemeral operation store")
		return "", nil
	})

	if _, ok := store.(*operationstore.MemoryStore); !ok {
		t.Fatalf("store = %T, want *operationstore.MemoryStore", store)
	}
}

func TestNewOperationStoreUsesBoltInPersistentMode(t *testing.T) {
	dir := t.TempDir()
	store := newOperationStore(testAppLogger(), RuntimeModePersistent, func() (string, error) {
		return dir, nil
	})
	defer store.(interface{ Close() error }).Close()

	if _, ok := store.(*operationstore.BoltStore); !ok {
		t.Fatalf("store = %T, want *operationstore.BoltStore", store)
	}
}

func TestNewOperationStoreFallsBackToMemory(t *testing.T) {
	store := newOperationStore(testAppLogger(), RuntimeModePersistent, func() (string, error) {
		return "", errors.New("boom")
	})

	if _, ok := store.(*operationstore.MemoryStore); !ok {
		t.Fatalf("store = %T, want *operationstore.MemoryStore", store)
	}
}
