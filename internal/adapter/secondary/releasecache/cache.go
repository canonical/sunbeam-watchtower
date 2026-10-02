// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package releasecache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"go.etcd.io/bbolt"
)

var _ port.ReleaseCache = (*Cache)(nil)

const snapshotsBucket = "snapshots"

// Cache stores published artifact snapshots in bbolt.
type Cache struct {
	baseDir string
	db      *bbolt.DB
	closed  bool
}

// NewCache creates a release cache rooted at baseDir.
func NewCache(baseDir string) (*Cache, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating release cache dir: %w", err)
	}
	db, err := bbolt.Open(filepath.Join(baseDir, "releases.db"), 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening release cache db: %w", err)
	}
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(snapshotsBucket))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initializing release cache db: %w", err)
	}
	return &Cache{baseDir: baseDir, db: db}, nil
}

// Store writes one normalized publication snapshot.
func (c *Cache) Store(_ context.Context, snapshot dto.PublishedArtifactSnapshot) error {
	key := snapshotKey(snapshot.Project, snapshot.Name, snapshot.ArtifactType)
	data, err := json.Marshal(dto.NormalizePublicationSnapshot(snapshot))
	if err != nil {
		return fmt.Errorf("marshalling release snapshot: %w", err)
	}
	return c.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte(snapshotsBucket)).Put([]byte(key), data)
	})
}

// List returns all cached publication snapshots.
func (c *Cache) List(_ context.Context) ([]dto.PublishedArtifactSnapshot, error) {
	var results []dto.PublishedArtifactSnapshot
	err := c.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte(snapshotsBucket)).ForEach(func(_, value []byte) error {
			var snapshot dto.PublishedArtifactSnapshot
			if err := json.Unmarshal(value, &snapshot); err != nil {
				return err
			}
			results = append(results, snapshot)
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("listing release cache: %w", err)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Project == results[j].Project {
			if results[i].ArtifactType == results[j].ArtifactType {
				return results[i].Name < results[j].Name
			}
			return results[i].ArtifactType.String() < results[j].ArtifactType.String()
		}
		return results[i].Project < results[j].Project
	})
	return results, nil
}

// Status reports metadata per cached tracked artifact.
func (c *Cache) Status(ctx context.Context) ([]dto.ReleaseCacheStatus, error) {
	snapshots, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	statuses := make([]dto.ReleaseCacheStatus, 0, len(snapshots))
	for _, snapshot := range snapshots {
		trackSet := make(map[string]bool, len(snapshot.Channels))
		for _, channel := range snapshot.Channels {
			trackSet[channel.Track] = true
		}
		statuses = append(statuses, dto.ReleaseCacheStatus{
			Project:      snapshot.Project,
			Name:         snapshot.Name,
			ArtifactType: snapshot.ArtifactType,
			TrackCount:   len(trackSet),
			ChannelCount: len(snapshot.Channels),
			LastUpdated:  snapshot.UpdatedAt,
		})
	}
	return statuses, nil
}

// Remove deletes one cached artifact snapshot.
func (c *Cache) Remove(project string, name string, artifactType dto.ArtifactType) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte(snapshotsBucket)).Delete([]byte(snapshotKey(project, name, artifactType)))
	})
}

// RemoveAll clears all release cache data.
func (c *Cache) RemoveAll() error {
	if err := c.Close(); err != nil {
		return fmt.Errorf("closing release cache db: %w", err)
	}
	return os.RemoveAll(c.baseDir)
}

// CacheDir returns the root cache directory.
func (c *Cache) CacheDir() string { return c.baseDir }

// Close releases the underlying DB.
func (c *Cache) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.db.Close()
}

func snapshotKey(project string, name string, artifactType dto.ArtifactType) string {
	return project + ":" + artifactType.String() + ":" + name
}
