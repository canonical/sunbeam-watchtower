// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package credentials

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/testsupport"
	gh "github.com/canonical/sunbeam-watchtower/pkg/github/v1"
)

func TestNewGitHubStore_DefaultPath(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	store := NewGitHubStore("")

	want := filepath.Join(home, ".config", "sunbeam-watchtower", "github-credentials.json")
	if store.path != want {
		t.Fatalf("path = %q, want %q", store.path, want)
	}
}

func TestGitHubStoreLoad_PrefersEnvironment(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := gh.SaveCredentialsFile(path, &gh.Credentials{AccessToken: "file-token"}); err != nil {
		t.Fatalf("SaveCredentialsFile() error = %v", err)
	}

	t.Setenv(envGitHubToken, "env-token")

	record, err := NewGitHubStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record == nil {
		t.Fatal("Load() returned nil record")
	}
	if record.Source != gh.CredentialSourceEnvironment {
		t.Fatalf("Source = %q, want %q", record.Source, gh.CredentialSourceEnvironment)
	}
	if record.Credentials.AccessToken != "env-token" {
		t.Fatalf("unexpected env credentials: %+v", record.Credentials)
	}
}

func TestGitHubStoreLoad_FromFile(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := gh.SaveCredentialsFile(path, &gh.Credentials{AccessToken: "file-token"}); err != nil {
		t.Fatalf("SaveCredentialsFile() error = %v", err)
	}

	record, err := NewGitHubStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record == nil {
		t.Fatal("Load() returned nil record")
	}
	if record.Source != gh.CredentialSourceFile {
		t.Fatalf("Source = %q, want %q", record.Source, gh.CredentialSourceFile)
	}
	if record.Path != path {
		t.Fatalf("Path = %q, want %q", record.Path, path)
	}
	if record.Credentials.AccessToken != "file-token" {
		t.Fatalf("unexpected file credentials: %+v", record.Credentials)
	}
}

func TestGitHubStoreSaveAndClear(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "nested", "credentials.json")
	store := NewGitHubStore(path)

	record, err := store.Save(context.Background(), &gh.Credentials{AccessToken: "saved-token"})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if record.Path != path {
		t.Fatalf("Path = %q, want %q", record.Path, path)
	}

	if err := store.Clear(context.Background()); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected credentials file to be removed, got err = %v", err)
	}
}
