// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLaunchpadPendingAuthFlowStorePersistsAcrossApps(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	first := NewApp(&config.Config{}, discardLogger())
	flow := &lp.PendingAuthFlow{
		ID:                 "flow-1",
		RequestToken:       "token",
		RequestTokenSecret: "secret",
		CreatedAt:          time.Now().UTC(),
		ExpiresAt:          time.Now().UTC().Add(time.Minute),
	}
	if err := first.LaunchpadPendingAuthFlowStore().Put(context.Background(), flow); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second := NewApp(&config.Config{}, discardLogger())
	defer second.Close()

	got, err := second.LaunchpadPendingAuthFlowStore().Get(context.Background(), flow.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.RequestToken != flow.RequestToken {
		t.Fatalf("Get() = %+v, want persisted flow", got)
	}
}

func TestOperationStorePersistsAcrossApps(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	first := NewApp(&config.Config{}, discardLogger())
	job := dto.OperationJob{
		ID:        "job-1",
		Kind:      dto.OperationKindBuildTrigger,
		State:     dto.OperationStateSucceeded,
		CreatedAt: time.Now().UTC(),
		Summary:   "done",
	}
	if err := first.OperationStore().Create(context.Background(), job); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second := NewApp(&config.Config{}, discardLogger())
	defer second.Close()

	got, err := second.OperationStore().Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.Summary != "done" {
		t.Fatalf("Get() = %+v, want persisted job", got)
	}
}

func TestEphemeralLaunchpadPendingAuthFlowStoreDoesNotPersistAcrossApps(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	first := NewAppWithOptions(&config.Config{}, discardLogger(), Options{RuntimeMode: RuntimeModeEphemeral})
	flow := &lp.PendingAuthFlow{
		ID:                 "flow-1",
		RequestToken:       "token",
		RequestTokenSecret: "secret",
		CreatedAt:          time.Now().UTC(),
		ExpiresAt:          time.Now().UTC().Add(time.Minute),
	}
	if err := first.LaunchpadPendingAuthFlowStore().Put(context.Background(), flow); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second := NewAppWithOptions(&config.Config{}, discardLogger(), Options{RuntimeMode: RuntimeModeEphemeral})
	defer second.Close()

	if _, err := second.LaunchpadPendingAuthFlowStore().Get(context.Background(), flow.ID); err == nil {
		t.Fatal("expected no persisted flow in ephemeral mode")
	}
}

func TestEphemeralOperationStoreDoesNotPersistAcrossApps(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	first := NewAppWithOptions(&config.Config{}, discardLogger(), Options{RuntimeMode: RuntimeModeEphemeral})
	job := dto.OperationJob{
		ID:        "job-1",
		Kind:      dto.OperationKindBuildTrigger,
		State:     dto.OperationStateSucceeded,
		CreatedAt: time.Now().UTC(),
		Summary:   "done",
	}
	if err := first.OperationStore().Create(context.Background(), job); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second := NewAppWithOptions(&config.Config{}, discardLogger(), Options{RuntimeMode: RuntimeModeEphemeral})
	defer second.Close()

	got, err := second.OperationStore().Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Fatalf("Get() = %+v, want no persisted job in ephemeral mode", got)
	}
}
