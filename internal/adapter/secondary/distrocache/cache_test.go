// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package distrocache

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"

	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	"go.etcd.io/bbolt"
)

// setupTestCache creates a Cache with a pre-populated bbolt database for testing.
func setupTestCache(t *testing.T) *Cache {
	t.Helper()
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cache, err := NewCache(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cache.Close() })

	// Populate with test data directly via bbolt.
	pkgs := []distro.SourcePackage{
		{Package: "nova", Version: "1:29.0.0-0ubuntu1", Suite: "noble", Component: "main"},
		{Package: "nova", Version: "1:29.1.0-0ubuntu1", Suite: "noble-updates", Component: "main"},
		{Package: "keystone", Version: "2:27.0.0-0ubuntu1", Suite: "noble", Component: "main"},
		{Package: "glance", Version: "1:28.0.0-0ubuntu1", Suite: "noble", Component: "main"},
	}

	err = cache.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucket([]byte("ubuntu"))
		if err != nil {
			return err
		}
		for _, pkg := range pkgs {
			key := pkg.Package + "/" + pkg.Suite + "/" + pkg.Component
			val, _ := json.Marshal(pkg)
			if err := b.Put([]byte(key), val); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return cache
}

func TestQueryAll(t *testing.T) {
	cache := setupTestCache(t)
	ctx := context.Background()

	pkgs, err := cache.Query(ctx, "ubuntu", dto.QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 4 {
		t.Fatalf("expected 4 packages, got %d", len(pkgs))
	}
}

func TestQueryByPackageName(t *testing.T) {
	cache := setupTestCache(t)
	ctx := context.Background()
	err := cache.db.Update(func(tx *bbolt.Tx) error {
		pkg := distro.SourcePackage{Package: "nova-extra", Version: "1.0", Suite: "noble", Component: "main"}
		value, err := json.Marshal(pkg)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("ubuntu")).Put([]byte("nova-extra/noble/main"), value)
	})
	if err != nil {
		t.Fatal(err)
	}

	pkgs, err := cache.Query(ctx, "ubuntu", dto.QueryOpts{
		Packages: []string{"nova", "nova"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 nova packages, got %d", len(pkgs))
	}
	for _, p := range pkgs {
		if p.Package != "nova" {
			t.Errorf("expected nova, got %q", p.Package)
		}
	}
}

func TestQueryDetailedByPackageName(t *testing.T) {
	cache := setupTestCache(t)
	pkgs, err := cache.QueryDetailed(context.Background(), "ubuntu", dto.QueryOpts{
		Packages: []string{"nova"},
		Suites:   []string{"noble-updates"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0].Package != "nova" || pkgs[0].Suite != "noble-updates" {
		t.Fatalf("detailed packages = %+v, want nova from noble-updates", pkgs)
	}
}

func TestQueryBySuite(t *testing.T) {
	cache := setupTestCache(t)
	ctx := context.Background()

	pkgs, err := cache.Query(ctx, "ubuntu", dto.QueryOpts{
		Suites: []string{"noble-updates"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].Package != "nova" || pkgs[0].Suite != "noble-updates" {
		t.Errorf("unexpected package: %+v", pkgs[0])
	}
}

func TestQueryNonExistentBucket(t *testing.T) {
	cache := setupTestCache(t)
	ctx := context.Background()

	pkgs, err := cache.Query(ctx, "nonexistent", dto.QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(pkgs))
	}
}

func TestStatus(t *testing.T) {
	cache := setupTestCache(t)

	// Write meta for "ubuntu".
	if err := cache.updateMeta("ubuntu"); err != nil {
		t.Fatal(err)
	}

	statuses, err := cache.Status()
	if err != nil {
		t.Fatal(err)
	}

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Name != "ubuntu" {
		t.Errorf("expected ubuntu, got %q", statuses[0].Name)
	}
	if statuses[0].EntryCount != 4 {
		t.Errorf("expected 4 entries, got %d", statuses[0].EntryCount)
	}
	if statuses[0].LastUpdated.IsZero() {
		t.Error("expected non-zero LastUpdated")
	}
}

func TestCacheDir(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cache, err := NewCache(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	if cache.CacheDir() != dir {
		t.Errorf("expected %q, got %q", dir, cache.CacheDir())
	}
}

func TestSourcesFileName(t *testing.T) {
	tests := []struct {
		suite, component, format, want string
	}{
		{"noble", "main", "xz", "noble_main_Sources.xz"},
		{"noble-updates/gazpacho", "main", "gz", "noble-updates_gazpacho_main_Sources.gz"},
	}
	for _, tt := range tests {
		got := SourcesFileName(tt.suite, tt.component, tt.format)
		if got != tt.want {
			t.Errorf("SourcesFileName(%q, %q, %q) = %q, want %q", tt.suite, tt.component, tt.format, got, tt.want)
		}
	}
}

func TestMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cache, err := NewCache(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	if err := cache.updateMeta("test-source"); err != nil {
		t.Fatal(err)
	}

	meta, err := cache.loadMeta()
	if err != nil {
		t.Fatal(err)
	}

	entry, ok := meta["test-source"]
	if !ok {
		t.Fatal("expected test-source in meta")
	}
	if entry.LastUpdated.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	// Verify it's persisted.
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("meta.json is empty")
	}
}
