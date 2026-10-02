// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package credentials

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/testsupport"
)

func TestNewCharmhubStore_DefaultPath(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	store := NewCharmhubStore("")

	want := filepath.Join(home, ".config", "sunbeam-watchtower", "charmhub-credentials.json")
	if store.path != want {
		t.Fatalf("path = %q, want %q", store.path, want)
	}
}

func TestCharmhubStoreLoad_PrefersEnvironment(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	store := NewCharmhubStore(dir)
	if _, err := store.Save(context.Background(), "bundle", "file-macaroon"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	t.Setenv(envCharmcraftAuth, "env-macaroon")

	record, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record == nil {
		t.Fatal("Load() returned nil record")
	}
	if record.Source != storeCredentialSourceEnvironment {
		t.Fatalf("Source = %q, want %q", record.Source, storeCredentialSourceEnvironment)
	}
	if record.Macaroon != "env-macaroon" {
		t.Fatalf("Macaroon = %q, want %q", record.Macaroon, "env-macaroon")
	}
}

func TestCharmhubStoreLoad_FromFile(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	store := NewCharmhubStore(dir)
	if _, err := store.Save(context.Background(), "saved-bundle", "file-macaroon"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	record, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record == nil {
		t.Fatal("Load() returned nil record")
	}
	if record.Source != storeCredentialSourceFile {
		t.Fatalf("Source = %q, want %q", record.Source, storeCredentialSourceFile)
	}
	if record.Macaroon != "file-macaroon" {
		t.Fatalf("Macaroon = %q, want %q", record.Macaroon, "file-macaroon")
	}
	if record.DischargedBundle != "saved-bundle" {
		t.Fatalf("DischargedBundle = %q, want %q", record.DischargedBundle, "saved-bundle")
	}
}

// TestCharmhubStoreLoad_LegacyFileWithoutBundle: records written by the
// b2793 era only carried `macaroon`. The new loader must still accept
// them; the missing bundle surfaces later as a re-login-required error.
func TestCharmhubStoreLoad_LegacyFileWithoutBundle(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	store := NewCharmhubStore(dir)
	legacyPath := filepath.Join(dir, charmhubCredentialFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"macaroon":"legacy-token"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	record, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record == nil || record.Macaroon != "legacy-token" {
		t.Fatalf("Load() = %+v, want legacy-token", record)
	}
	if record.DischargedBundle != "" {
		t.Fatalf("DischargedBundle = %q, want empty for legacy record", record.DischargedBundle)
	}
}

func TestCharmhubStoreLoad_Missing(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	store := NewCharmhubStore(dir)

	record, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record != nil {
		t.Fatalf("Load() = %+v, want nil", record)
	}
}

func TestCharmhubStoreSaveAndClear(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	store := NewCharmhubStore(dir)
	path := filepath.Join(dir, charmhubCredentialFile)

	record, err := store.Save(context.Background(), "saved-bundle", "saved-macaroon")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if record.Path != path {
		t.Fatalf("Path = %q, want %q", record.Path, path)
	}
	if record.DischargedBundle != "saved-bundle" {
		t.Fatalf("DischargedBundle = %q, want saved-bundle", record.DischargedBundle)
	}

	if err := store.Clear(context.Background()); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected credentials file to be removed, got err = %v", err)
	}
}
