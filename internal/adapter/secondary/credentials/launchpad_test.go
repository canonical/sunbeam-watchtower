// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package credentials

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/testsupport"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

func TestNewLaunchpadStore_DefaultPath(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	store := NewLaunchpadStore("")

	want := filepath.Join(home, ".config", "sunbeam-watchtower", "credentials.json")
	if store.path != want {
		t.Fatalf("path = %q, want %q", store.path, want)
	}
}

func TestLaunchpadStoreLoad_PrefersEnvironment(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := lp.SaveCredentialsFile(path, &lp.Credentials{
		ConsumerKey:       "ignored",
		AccessToken:       "file-token",
		AccessTokenSecret: "file-secret",
	}); err != nil {
		t.Fatalf("SaveCredentialsFile() error = %v", err)
	}

	t.Setenv(envAccessToken, "env-token")
	t.Setenv(envAccessTokenSecret, "env-secret")

	record, err := NewLaunchpadStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record == nil {
		t.Fatal("Load() returned nil record")
	}
	if record.Source != lp.CredentialSourceEnvironment {
		t.Fatalf("Source = %q, want %q", record.Source, lp.CredentialSourceEnvironment)
	}
	if record.Credentials.AccessToken != "env-token" || record.Credentials.AccessTokenSecret != "env-secret" {
		t.Fatalf("unexpected env credentials: %+v", record.Credentials)
	}
	if record.Path != "" {
		t.Fatalf("Path = %q, want empty for env credentials", record.Path)
	}
}

func TestLaunchpadStoreLoad_FromFile(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := lp.SaveCredentialsFile(path, &lp.Credentials{
		ConsumerKey:       "ignored",
		AccessToken:       "file-token",
		AccessTokenSecret: "file-secret",
	}); err != nil {
		t.Fatalf("SaveCredentialsFile() error = %v", err)
	}

	record, err := NewLaunchpadStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record == nil {
		t.Fatal("Load() returned nil record")
	}
	if record.Source != lp.CredentialSourceFile {
		t.Fatalf("Source = %q, want %q", record.Source, lp.CredentialSourceFile)
	}
	if record.Path != path {
		t.Fatalf("Path = %q, want %q", record.Path, path)
	}
	if record.Credentials.AccessToken != "file-token" || record.Credentials.AccessTokenSecret != "file-secret" {
		t.Fatalf("unexpected file credentials: %+v", record.Credentials)
	}
	if record.Credentials.ConsumerKey != lp.ConsumerKey() {
		t.Fatalf("ConsumerKey = %q, want %q", record.Credentials.ConsumerKey, lp.ConsumerKey())
	}
}

func TestLaunchpadStoreLoad_InvalidFile(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	record, err := NewLaunchpadStore(path).Load(context.Background())
	if err == nil {
		t.Fatal("Load() error = nil, want parse error")
	}
	if record != nil {
		t.Fatalf("Load() record = %+v, want nil on parse error", record)
	}
}

func TestLaunchpadStoreSaveAndClear(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "nested", "credentials.json")
	store := NewLaunchpadStore(path)

	record, err := store.Save(context.Background(), &lp.Credentials{
		ConsumerKey:       lp.ConsumerKey(),
		AccessToken:       "saved-token",
		AccessTokenSecret: "saved-secret",
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if record.Path != path {
		t.Fatalf("Path = %q, want %q", record.Path, path)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 600", info.Mode().Perm())
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat(dir) error = %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o, want 700", dirInfo.Mode().Perm())
	}

	loaded, err := lp.LoadCredentialsFile(path)
	if err != nil {
		t.Fatalf("LoadCredentialsFile() error = %v", err)
	}
	if loaded.AccessToken != "saved-token" || loaded.AccessTokenSecret != "saved-secret" {
		t.Fatalf("unexpected saved credentials: %+v", loaded)
	}

	if err := store.Clear(context.Background()); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected credentials file to be removed, got err = %v", err)
	}
	if err := store.Clear(context.Background()); err != nil {
		t.Fatalf("Clear() second call error = %v", err)
	}
}

func TestLaunchpadStoreSave_WithEmptyPath(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	store := &LaunchpadStore{}

	if _, err := store.Save(context.Background(), &lp.Credentials{}); err == nil {
		t.Fatal("Save() error = nil, want path error")
	}
	if err := store.Clear(context.Background()); err == nil {
		t.Fatal("Clear() error = nil, want path error")
	}
}
