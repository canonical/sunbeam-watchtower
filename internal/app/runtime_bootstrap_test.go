// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
)

func TestCacheSubdir(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	got, err := cacheSubdir("state")
	if err != nil {
		t.Fatalf("cacheSubdir() error = %v", err)
	}

	want := filepath.Join(cacheHome, "sunbeam-watchtower", "state")
	if got != want {
		t.Fatalf("cacheSubdir() = %q, want %q", got, want)
	}
}

func TestStateDirUsesWatchtowerCacheRoot(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	app := NewApp(&config.Config{}, slog.Default())
	got, err := app.stateDir()
	if err != nil {
		t.Fatalf("stateDir() error = %v", err)
	}

	want := filepath.Join(cacheHome, "sunbeam-watchtower", "state")
	if got != want {
		t.Fatalf("stateDir() = %q, want %q", got, want)
	}
}

func TestLaunchpadCredentialStoreReturnsSharedInstance(t *testing.T) {
	app := NewApp(&config.Config{}, slog.Default())

	first := app.LaunchpadCredentialStore()
	second := app.LaunchpadCredentialStore()

	if first == nil {
		t.Fatal("LaunchpadCredentialStore() returned nil")
	}
	if first != second {
		t.Fatal("LaunchpadCredentialStore() did not return shared instance")
	}
}
