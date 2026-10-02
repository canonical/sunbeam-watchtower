// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package distrocache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	"go.etcd.io/bbolt"
)

var _ port.DistroCache = (*Cache)(nil)

// Cache implements port.DistroCache using bbolt for indexing and raw
// Sources files on disk.
type Cache struct {
	baseDir string
	db      *bbolt.DB
	client  *http.Client
	logger  *slog.Logger
	closed  bool
}

// NewCache creates a new distro cache rooted at baseDir.
func NewCache(baseDir string, logger *slog.Logger, clients ...*http.Client) (*Cache, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating cache dir: %w", err)
	}

	dbPath := filepath.Join(baseDir, "distro.db")
	db, err := bbolt.Open(dbPath, 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bbolt db: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	if len(clients) > 0 && clients[0] != nil {
		client = clients[0]
	}
	return &Cache{
		baseDir: baseDir,
		db:      db,
		client:  client,
		logger:  logger,
	}, nil
}

// Close releases resources held by the cache.
func (c *Cache) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.db.Close()
}

// CacheDir returns the base directory for the cache.
func (c *Cache) CacheDir() string {
	return c.baseDir
}

// RemoveAll closes the database and deletes the entire cache directory.
func (c *Cache) RemoveAll() error {
	if err := c.Close(); err != nil {
		return fmt.Errorf("closing db before removal: %w", err)
	}
	return os.RemoveAll(c.baseDir)
}

// sourcesDir returns the directory for raw Sources files for a given source name.
func (c *Cache) sourcesDir(name string) string {
	return filepath.Join(c.baseDir, "sources", name)
}

// SourcesFileName returns the filename for a Sources file.
func SourcesFileName(suite, component, format string) string {
	return distro.SourcesFileName(suite, component, format)
}

// Update downloads Sources indexes and rebuilds the bbolt index for the named source.
func (c *Cache) Update(ctx context.Context, name string, entries []dto.SourceEntry) error {
	dir := c.sourcesDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating sources dir: %w", err)
	}

	var allPkgs []distro.SourcePackageDetail

	for _, entry := range entries {
		c.logger.Info("downloading sources index",
			"name", name,
			"mirror", entry.Mirror,
			"suite", entry.Suite,
			"component", entry.Component)

		// Download to a temp name, then rename once we know the format.
		tmpPath := filepath.Join(dir, "tmp_download")
		format, err := downloadSourcesFile(ctx, c.client, entry.Mirror, entry.Suite, entry.Component, tmpPath)
		if err != nil {
			return fmt.Errorf("downloading %s/%s/%s: %w", entry.Mirror, entry.Suite, entry.Component, err)
		}

		destName := SourcesFileName(entry.Suite, entry.Component, format)
		destPath := filepath.Join(dir, destName)
		if err := os.Rename(tmpPath, destPath); err != nil {
			return fmt.Errorf("renaming sources file: %w", err)
		}

		pkgs, err := distro.ParseSourcesFileDetailed(destPath, format, entry.Suite, entry.Component)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", destPath, err)
		}

		c.logger.Debug("parsed sources file",
			"name", name,
			"suite", entry.Suite,
			"component", entry.Component,
			"packages", len(pkgs))

		allPkgs = append(allPkgs, pkgs...)
	}

	// Rebuild bbolt bucket for this source.
	if err := c.db.Update(func(tx *bbolt.Tx) error {
		// Drop existing bucket to rebuild.
		_ = tx.DeleteBucket([]byte(name))

		b, err := tx.CreateBucket([]byte(name))
		if err != nil {
			return fmt.Errorf("creating bucket %q: %w", name, err)
		}

		for _, pkg := range allPkgs {
			key := fmt.Sprintf("%s/%s/%s", pkg.Package, pkg.Suite, pkg.Component)
			val, err := json.Marshal(pkg)
			if err != nil {
				return fmt.Errorf("marshalling package: %w", err)
			}
			if err := b.Put([]byte(key), val); err != nil {
				return fmt.Errorf("writing key %q: %w", key, err)
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("updating bbolt: %w", err)
	}

	// Update meta.json timestamp.
	return c.updateMeta(name)
}

// Query returns source packages matching the given criteria from the bbolt index.
func (c *Cache) Query(_ context.Context, name string, opts dto.QueryOpts) ([]distro.SourcePackage, error) {
	pkgFilter := make(map[string]bool, len(opts.Packages))
	for _, p := range opts.Packages {
		pkgFilter[p] = true
	}
	suiteFilter := make(map[string]bool, len(opts.Suites))
	for _, s := range opts.Suites {
		suiteFilter[s] = true
	}
	compFilter := make(map[string]bool, len(opts.Components))
	for _, c := range opts.Components {
		compFilter[c] = true
	}

	var results []distro.SourcePackage

	err := c.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(name))
		if b == nil {
			return nil // no data for this source
		}

		return forEachPackage(b, opts.Packages, func(k, v []byte) error {
			var pkg distro.SourcePackage
			if err := json.Unmarshal(v, &pkg); err != nil {
				return fmt.Errorf("unmarshalling value for key %q: %w", string(k), err)
			}

			if len(pkgFilter) > 0 && !pkgFilter[pkg.Package] {
				return nil
			}
			if len(suiteFilter) > 0 && !suiteFilter[pkg.Suite] {
				return nil
			}
			if len(compFilter) > 0 && !compFilter[pkg.Component] {
				return nil
			}

			results = append(results, pkg)
			return nil
		})
	})

	return results, err
}

// QueryDetailed returns source packages with build dependency information from the bbolt index.
func (c *Cache) QueryDetailed(_ context.Context, name string, opts dto.QueryOpts) ([]distro.SourcePackageDetail, error) {
	pkgFilter := make(map[string]bool, len(opts.Packages))
	for _, p := range opts.Packages {
		pkgFilter[p] = true
	}
	suiteFilter := make(map[string]bool, len(opts.Suites))
	for _, s := range opts.Suites {
		suiteFilter[s] = true
	}
	compFilter := make(map[string]bool, len(opts.Components))
	for _, comp := range opts.Components {
		compFilter[comp] = true
	}

	var results []distro.SourcePackageDetail

	err := c.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(name))
		if b == nil {
			return nil
		}

		return forEachPackage(b, opts.Packages, func(k, v []byte) error {
			var pkg distro.SourcePackageDetail
			if err := json.Unmarshal(v, &pkg); err != nil {
				return fmt.Errorf("unmarshalling value for key %q: %w", string(k), err)
			}

			if len(pkgFilter) > 0 && !pkgFilter[pkg.Package] {
				return nil
			}
			if len(suiteFilter) > 0 && !suiteFilter[pkg.Suite] {
				return nil
			}
			if len(compFilter) > 0 && !compFilter[pkg.Component] {
				return nil
			}

			results = append(results, pkg)
			return nil
		})
	})

	return results, err
}

// forEachPackage uses the package prefix in the bbolt key for named queries.
// Unfiltered queries still walk the entire bucket for list and rdepends paths.
func forEachPackage(b *bbolt.Bucket, packages []string, fn func(k, v []byte) error) error {
	if len(packages) == 0 {
		return b.ForEach(fn)
	}
	names := make([]string, 0, len(packages))
	seen := make(map[string]bool, len(packages))
	for _, name := range packages {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	cursor := b.Cursor()
	for _, name := range names {
		prefix := []byte(name + "/")
		for k, v := cursor.Seek(prefix); bytes.HasPrefix(k, prefix); k, v = cursor.Next() {
			if err := fn(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// Status returns cache metadata for all indexed source groups.
func (c *Cache) Status() ([]dto.CacheStatus, error) {
	meta, err := c.loadMeta()
	if err != nil {
		return nil, err
	}

	var statuses []dto.CacheStatus

	err = c.db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bbolt.Bucket) error {
			count := 0
			_ = b.ForEach(func(_, _ []byte) error {
				count++
				return nil
			})

			s := dto.CacheStatus{
				Name:       string(name),
				EntryCount: count,
			}

			if m, ok := meta[string(name)]; ok {
				s.LastUpdated = m.LastUpdated
			}

			// Calculate disk size for raw Sources files.
			dir := c.sourcesDir(string(name))
			s.DiskSize = dirSize(dir)

			statuses = append(statuses, s)
			return nil
		})
	})

	return statuses, err
}

// metaEntry stores per-source metadata on disk.
type metaEntry struct {
	LastUpdated time.Time `json:"last_updated"`
}

func (c *Cache) metaPath() string {
	return filepath.Join(c.baseDir, "meta.json")
}

func (c *Cache) loadMeta() (map[string]metaEntry, error) {
	data, err := os.ReadFile(c.metaPath())
	if os.IsNotExist(err) {
		return make(map[string]metaEntry), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading meta: %w", err)
	}

	var meta map[string]metaEntry
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshalling meta: %w", err)
	}
	return meta, nil
}

func (c *Cache) updateMeta(name string) error {
	meta, err := c.loadMeta()
	if err != nil {
		return err
	}

	meta[name] = metaEntry{LastUpdated: time.Now()}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling meta: %w", err)
	}
	return os.WriteFile(c.metaPath(), data, 0o644)
}

// dirSize computes the total size of files in a directory.
func dirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}
