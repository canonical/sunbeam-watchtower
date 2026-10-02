// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package reviewcache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
	"go.etcd.io/bbolt"
)

const (
	reviewSummariesPrefix = "summaries:"
	reviewDetailsPrefix   = "details:"
	reviewMetaBucket      = "meta"
	reviewLastSyncPrefix  = "last_sync:"
)

var _ port.ReviewCache = (*Cache)(nil)

// Cache stores review summaries and details in bbolt.
type Cache struct {
	baseDir string
	db      *bbolt.DB
	closed  bool
}

// NewCache creates a new review cache rooted at baseDir.
func NewCache(baseDir string) (*Cache, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating review cache dir: %w", err)
	}
	db, err := bbolt.Open(filepath.Join(baseDir, "reviews.db"), 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening review cache db: %w", err)
	}
	return &Cache{baseDir: baseDir, db: db}, nil
}

func (c *Cache) StoreSummaries(_ context.Context, forgeType forge.ForgeType, project string, mrs []forge.MergeRequest) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		name := summaryBucketName(forgeType, project)
		_ = tx.DeleteBucket(name)
		bkt, err := tx.CreateBucket(name)
		if err != nil {
			return fmt.Errorf("creating review summary bucket: %w", err)
		}
		for i := range mrs {
			summary := reviewSummary(mrs[i])
			data, err := json.Marshal(summary)
			if err != nil {
				return fmt.Errorf("marshalling review summary %s: %w", summary.ID, err)
			}
			if err := bkt.Put([]byte(summary.ID), data); err != nil {
				return fmt.Errorf("storing review summary %s: %w", summary.ID, err)
			}
		}
		return nil
	})
}

func (c *Cache) StoreDetail(_ context.Context, forgeType forge.ForgeType, project string, mr forge.MergeRequest) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists(detailBucketName(forgeType, project))
		if err != nil {
			return fmt.Errorf("creating review detail bucket: %w", err)
		}
		data, err := json.Marshal(mr)
		if err != nil {
			return fmt.Errorf("marshalling review detail %s: %w", mr.ID, err)
		}
		return bkt.Put([]byte(mr.ID), data)
	})
}

func (c *Cache) GetDetail(_ context.Context, forgeType forge.ForgeType, project string, id string) (*forge.MergeRequest, error) {
	var mr forge.MergeRequest
	err := c.db.View(func(tx *bbolt.Tx) error {
		bkt := tx.Bucket(detailBucketName(forgeType, project))
		if bkt == nil {
			return fmt.Errorf("review detail %s not found in cache", id)
		}
		data := bkt.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("review detail %s not found in cache", id)
		}
		return json.Unmarshal(data, &mr)
	})
	if err != nil {
		return nil, err
	}
	return &mr, nil
}

func (c *Cache) List(_ context.Context, forgeType forge.ForgeType, project string) ([]forge.MergeRequest, error) {
	var out []forge.MergeRequest
	err := c.db.View(func(tx *bbolt.Tx) error {
		bkt := tx.Bucket(summaryBucketName(forgeType, project))
		if bkt == nil {
			return nil
		}
		return bkt.ForEach(func(_, v []byte) error {
			var mr forge.MergeRequest
			if err := json.Unmarshal(v, &mr); err != nil {
				return fmt.Errorf("unmarshalling review summary: %w", err)
			}
			out = append(out, mr)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b forge.MergeRequest) int {
		switch {
		case a.UpdatedAt.After(b.UpdatedAt):
			return -1
		case a.UpdatedAt.Before(b.UpdatedAt):
			return 1
		default:
			return strings.Compare(a.ID, b.ID)
		}
	})
	return out, nil
}

func (c *Cache) PruneDetailsBefore(_ context.Context, cutoff time.Time) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, bkt *bbolt.Bucket) error {
			if !strings.HasPrefix(string(name), reviewDetailsPrefix) {
				return nil
			}
			var keysToDelete [][]byte
			if err := bkt.ForEach(func(k, v []byte) error {
				var mr forge.MergeRequest
				if err := json.Unmarshal(v, &mr); err != nil {
					return err
				}
				if (mr.State == forge.MergeStateOpen || mr.State == forge.MergeStateWIP) || mr.UpdatedAt.IsZero() || !mr.UpdatedAt.Before(cutoff) {
					return nil
				}
				keysToDelete = append(keysToDelete, append([]byte(nil), k...))
				return nil
			}); err != nil {
				return err
			}
			for _, key := range keysToDelete {
				if err := bkt.Delete(key); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func (c *Cache) SetLastSync(_ context.Context, forgeType forge.ForgeType, project string, t time.Time) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists([]byte(reviewMetaBucket))
		if err != nil {
			return fmt.Errorf("creating review meta bucket: %w", err)
		}
		return bkt.Put(lastSyncKey(forgeType, project), []byte(t.UTC().Format(time.RFC3339)))
	})
}

func (c *Cache) LastSync(_ context.Context, forgeType forge.ForgeType, project string) (time.Time, error) {
	var t time.Time
	err := c.db.View(func(tx *bbolt.Tx) error {
		bkt := tx.Bucket([]byte(reviewMetaBucket))
		if bkt == nil {
			return nil
		}
		data := bkt.Get(lastSyncKey(forgeType, project))
		if data == nil {
			return nil
		}
		var err error
		t, err = time.Parse(time.RFC3339, string(data))
		return err
	})
	return t, err
}

func (c *Cache) Remove(_ context.Context, forgeType forge.ForgeType, project string) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		_ = tx.DeleteBucket(summaryBucketName(forgeType, project))
		_ = tx.DeleteBucket(detailBucketName(forgeType, project))
		if bkt := tx.Bucket([]byte(reviewMetaBucket)); bkt != nil {
			_ = bkt.Delete(lastSyncKey(forgeType, project))
		}
		return nil
	})
}

func (c *Cache) RemoveAll(_ context.Context) error {
	if err := c.Close(); err != nil {
		return fmt.Errorf("closing review cache db: %w", err)
	}
	return os.RemoveAll(c.baseDir)
}

func (c *Cache) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.db.Close()
}

func (c *Cache) CacheDir() string { return c.baseDir }

func (c *Cache) Status(_ context.Context) ([]dto.ReviewCacheStatus, error) {
	var statuses []dto.ReviewCacheStatus
	err := c.db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, _ *bbolt.Bucket) error {
			raw := string(name)
			if !strings.HasPrefix(raw, reviewSummariesPrefix) {
				return nil
			}
			forgeType, project, err := parseBucketKey(raw, reviewSummariesPrefix)
			if err != nil {
				return err
			}
			summaryCount := 0
			if bkt := tx.Bucket(summaryBucketName(forgeType, project)); bkt != nil {
				_ = bkt.ForEach(func(_, _ []byte) error {
					summaryCount++
					return nil
				})
			}
			detailCount := 0
			if bkt := tx.Bucket(detailBucketName(forgeType, project)); bkt != nil {
				_ = bkt.ForEach(func(_, _ []byte) error {
					detailCount++
					return nil
				})
			}
			var lastSync time.Time
			if meta := tx.Bucket([]byte(reviewMetaBucket)); meta != nil {
				if data := meta.Get(lastSyncKey(forgeType, project)); data != nil {
					lastSync, err = time.Parse(time.RFC3339, string(data))
					if err != nil {
						return err
					}
				}
			}
			statuses = append(statuses, dto.ReviewCacheStatus{
				ForgeType:    forgeType.String(),
				Project:      project,
				SummaryCount: summaryCount,
				DetailCount:  detailCount,
				LastSync:     lastSync,
			})
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("listing review cache status: %w", err)
	}
	slices.SortFunc(statuses, func(a, b dto.ReviewCacheStatus) int {
		if cmp := strings.Compare(a.Project, b.Project); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ForgeType, b.ForgeType)
	})
	return statuses, nil
}

func reviewSummary(mr forge.MergeRequest) forge.MergeRequest {
	mr.Comments = nil
	mr.Files = nil
	mr.DiffText = ""
	return mr
}

func summaryBucketName(forgeType forge.ForgeType, project string) []byte {
	return []byte(reviewSummariesPrefix + forgeType.String() + ":" + project)
}

func detailBucketName(forgeType forge.ForgeType, project string) []byte {
	return []byte(reviewDetailsPrefix + forgeType.String() + ":" + project)
}

func lastSyncKey(forgeType forge.ForgeType, project string) []byte {
	return []byte(reviewLastSyncPrefix + forgeType.String() + ":" + project)
}

func parseBucketKey(name string, prefix string) (forge.ForgeType, string, error) {
	raw := strings.TrimPrefix(name, prefix)
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return forge.ForgeType(0), "", fmt.Errorf("invalid review cache bucket %q", name)
	}
	forgeType, err := parseForgeType(parts[0])
	if err != nil {
		return forge.ForgeType(0), "", err
	}
	return forgeType, parts[1], nil
}

func parseForgeType(value string) (forge.ForgeType, error) {
	switch value {
	case forge.ForgeGitHub.String():
		return forge.ForgeGitHub, nil
	case forge.ForgeLaunchpad.String():
		return forge.ForgeLaunchpad, nil
	case forge.ForgeGerrit.String():
		return forge.ForgeGerrit, nil
	default:
		return forge.ForgeType(0), fmt.Errorf("unknown forge type %q", value)
	}
}
