// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package bugcache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
	"go.etcd.io/bbolt"
)

const (
	bugsBucketPrefix     = "bugs:"
	tasksBucketPrefix    = "tasks:"
	metaBucket           = "meta"
	lastSyncPrefix       = "last_sync:"
	schemaPrefix         = "schema_version:"
	CurrentSchemaVersion = dto.BugCacheSchemaVersion
)

// Cache implements port.BugCache using bbolt for local storage.
type Cache struct {
	baseDir string
	db      *bbolt.DB
	logger  *slog.Logger
}

// NewCache creates a new bug cache backed by bbolt.
func NewCache(baseDir string, logger *slog.Logger) (*Cache, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating bug cache dir: %w", err)
	}
	dbPath := filepath.Join(baseDir, "bugs.db")
	db, err := bbolt.Open(dbPath, 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bug cache db: %w", err)
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &Cache{baseDir: baseDir, db: db, logger: logger}, nil
}

func bugsBucketName(forgeType forge.ForgeType) []byte {
	return []byte(bugsBucketPrefix + forgeType.String())
}

func tasksBucketName(forgeType forge.ForgeType, project string) []byte {
	return []byte(tasksBucketPrefix + forgeType.String() + ":" + project)
}

func lastSyncKey(forgeType forge.ForgeType, project string) []byte {
	return []byte(lastSyncPrefix + forgeType.String() + ":" + project)
}

func schemaKey(forgeType forge.ForgeType, project string) []byte {
	return []byte(schemaPrefix + forgeType.String() + ":" + project)
}

// taskKey produces a unique key for a bug task within a project bucket.
func taskKey(t *forge.BugTask) []byte {
	return []byte(t.BugID + ":" + t.TargetName)
}

// StoreBugs upserts bugs by (forge, ID).
func (c *Cache) StoreBugs(_ context.Context, bugs []*forge.Bug) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		for _, b := range bugs {
			bkt, err := tx.CreateBucketIfNotExists(bugsBucketName(b.Forge))
			if err != nil {
				return fmt.Errorf("creating bugs bucket for %s: %w", b.Forge, err)
			}
			// Store bug metadata without tasks to avoid stale task data.
			stored := *b
			stored.Tasks = nil
			data, err := json.Marshal(&stored)
			if err != nil {
				return fmt.Errorf("marshalling bug %s: %w", b.ID, err)
			}
			if err := bkt.Put([]byte(b.ID), data); err != nil {
				return fmt.Errorf("storing bug %s: %w", b.ID, err)
			}
		}
		return nil
	})
}

// StoreBugTasks replaces all cached tasks for a (forge, project) pair.
func (c *Cache) StoreBugTasks(_ context.Context, forgeType forge.ForgeType, project string, tasks []forge.BugTask) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		name := tasksBucketName(forgeType, project)
		// Drop and recreate to replace all tasks atomically.
		_ = tx.DeleteBucket(name)
		bkt, err := tx.CreateBucket(name)
		if err != nil {
			return fmt.Errorf("creating tasks bucket for %s:%s: %w", forgeType, project, err)
		}
		for i := range tasks {
			data, err := json.Marshal(&tasks[i])
			if err != nil {
				return fmt.Errorf("marshalling task: %w", err)
			}
			if err := bkt.Put(taskKey(&tasks[i]), data); err != nil {
				return fmt.Errorf("storing task: %w", err)
			}
		}
		return nil
	})
}

// ReplaceProject atomically publishes one complete project snapshot.
func (c *Cache) ReplaceProject(
	_ context.Context,
	forgeType forge.ForgeType,
	project string,
	bugs []*forge.Bug,
	tasks []forge.BugTask,
	syncedAt time.Time,
	schemaVersion int,
) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		bugsBucket, err := tx.CreateBucketIfNotExists(bugsBucketName(forgeType))
		if err != nil {
			return fmt.Errorf("creating bugs bucket for %s: %w", forgeType, err)
		}
		for _, bug := range bugs {
			if bug == nil {
				continue
			}
			stored := *bug
			stored.Tasks = nil
			data, err := json.Marshal(&stored)
			if err != nil {
				return fmt.Errorf("marshalling bug %s: %w", bug.ID, err)
			}
			if err := bugsBucket.Put([]byte(bug.ID), data); err != nil {
				return fmt.Errorf("storing bug %s: %w", bug.ID, err)
			}
		}

		taskBucketName := tasksBucketName(forgeType, project)
		_ = tx.DeleteBucket(taskBucketName)
		taskBucket, err := tx.CreateBucket(taskBucketName)
		if err != nil {
			return fmt.Errorf("creating tasks bucket for %s:%s: %w", forgeType, project, err)
		}
		for i := range tasks {
			data, err := json.Marshal(&tasks[i])
			if err != nil {
				return fmt.Errorf("marshalling task: %w", err)
			}
			if err := taskBucket.Put(taskKey(&tasks[i]), data); err != nil {
				return fmt.Errorf("storing task: %w", err)
			}
		}

		meta, err := tx.CreateBucketIfNotExists([]byte(metaBucket))
		if err != nil {
			return fmt.Errorf("creating meta bucket: %w", err)
		}
		if err := meta.Put(lastSyncKey(forgeType, project), []byte(syncedAt.UTC().Format(time.RFC3339))); err != nil {
			return fmt.Errorf("storing last sync: %w", err)
		}
		return meta.Put(schemaKey(forgeType, project), []byte(strconv.Itoa(schemaVersion)))
	})
}

// ProjectCompatible reports whether one project is safe for offline search.
func (c *Cache) ProjectCompatible(
	_ context.Context,
	forgeType forge.ForgeType,
	project string,
	schemaVersion int,
) (bool, error) {
	var compatible bool
	err := c.db.View(func(tx *bbolt.Tx) error {
		var err error
		compatible, _, err = projectCompatibility(tx, forgeType, project, schemaVersion)
		return err
	})
	return compatible, err
}

func projectCompatibility(
	tx *bbolt.Tx,
	forgeType forge.ForgeType,
	project string,
	currentVersion int,
) (bool, int, error) {
	storedVersion := 0
	if meta := tx.Bucket([]byte(metaBucket)); meta != nil {
		if raw := meta.Get(schemaKey(forgeType, project)); raw != nil {
			version, err := strconv.Atoi(string(raw))
			if err != nil {
				return false, 0, fmt.Errorf("parsing bug cache schema version for %s:%s: %w", forgeType, project, err)
			}
			storedVersion = version
		}
	}
	if storedVersion == currentVersion {
		return true, storedVersion, nil
	}

	taskBucket := tx.Bucket(tasksBucketName(forgeType, project))
	if taskBucket == nil {
		return true, currentVersion, nil
	}
	// A populated project without the explicit schema marker predates the
	// offline-search visibility contract. Field inspection cannot prove that
	// every upstream object was hydrated under that contract.
	return false, storedVersion, nil
}

// GetBug retrieves a bug by ID, collecting tasks from all project buckets.
func (c *Cache) GetBug(_ context.Context, forgeType forge.ForgeType, id string) (*forge.Bug, error) {
	var b forge.Bug
	err := c.db.View(func(tx *bbolt.Tx) error {
		bkt := tx.Bucket(bugsBucketName(forgeType))
		if bkt == nil {
			return fmt.Errorf("bug %s not found in cache", id)
		}
		data := bkt.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("bug %s not found in cache", id)
		}
		if err := json.Unmarshal(data, &b); err != nil {
			return fmt.Errorf("unmarshalling bug %s: %w", id, err)
		}

		// Collect tasks from all project buckets for this forge.
		prefix := tasksBucketPrefix + forgeType.String() + ":"
		return tx.ForEach(func(name []byte, bkt *bbolt.Bucket) error {
			if !strings.HasPrefix(string(name), prefix) {
				return nil
			}
			return bkt.ForEach(func(k, v []byte) error {
				key := string(k)
				if !strings.HasPrefix(key, id+":") {
					return nil
				}
				var t forge.BugTask
				if err := json.Unmarshal(v, &t); err != nil {
					return fmt.Errorf("unmarshalling task %s: %w", key, err)
				}
				b.Tasks = append(b.Tasks, t)
				return nil
			})
		})
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBugTasks returns cached tasks for a (forge, project) pair with in-memory filtering.
func (c *Cache) ListBugTasks(_ context.Context, forgeType forge.ForgeType, project string, opts forge.ListBugTasksOpts) ([]forge.BugTask, error) {
	var result []forge.BugTask
	err := c.db.View(func(tx *bbolt.Tx) error {
		bkt := tx.Bucket(tasksBucketName(forgeType, project))
		if bkt == nil {
			return nil
		}
		return bkt.ForEach(func(_, v []byte) error {
			var t forge.BugTask
			if err := json.Unmarshal(v, &t); err != nil {
				return fmt.Errorf("unmarshalling task: %w", err)
			}
			if len(opts.Tags) > 0 && len(t.Tags) == 0 {
				t.Tags = cachedBugTags(tx, forgeType, t.BugID)
			}
			if matchesOpts(&t, &opts) {
				result = append(result, t)
			}
			return nil
		})
	})
	return result, err
}

// Snapshot returns complete cached bugs, task bucket provenance, and cache
// status from one read transaction.
func (c *Cache) Snapshot(_ context.Context) (*dto.BugCacheSnapshot, error) {
	snapshot := &dto.BugCacheSnapshot{}
	err := c.db.View(func(tx *bbolt.Tx) error {
		documents := make(map[string]*dto.BugCacheDocument)

		for _, forgeType := range []forge.ForgeType{
			forge.ForgeGitHub,
			forge.ForgeLaunchpad,
			forge.ForgeGerrit,
		} {
			bkt := tx.Bucket(bugsBucketName(forgeType))
			if bkt == nil {
				continue
			}
			if err := bkt.ForEach(func(_, value []byte) error {
				var bug forge.Bug
				if err := json.Unmarshal(value, &bug); err != nil {
					return fmt.Errorf("unmarshalling cached bug: %w", err)
				}
				key := forgeType.String() + ":" + bug.ID
				documents[key] = &dto.BugCacheDocument{Bug: &bug}
				return nil
			}); err != nil {
				return err
			}
		}

		if err := tx.ForEach(func(name []byte, bkt *bbolt.Bucket) error {
			nameStr := string(name)
			if !strings.HasPrefix(nameStr, tasksBucketPrefix) {
				return nil
			}
			remainder := strings.TrimPrefix(nameStr, tasksBucketPrefix)
			parts := strings.SplitN(remainder, ":", 2)
			if len(parts) != 2 {
				return nil
			}
			forgeType, ok := forgeTypeFromString(parts[0])
			if !ok {
				return nil
			}
			trackerProject := parts[1]
			taskCount := 0
			bugIDs := make(map[string]struct{})
			if err := bkt.ForEach(func(_, value []byte) error {
				var task forge.BugTask
				if err := json.Unmarshal(value, &task); err != nil {
					return fmt.Errorf("unmarshalling cached task: %w", err)
				}
				taskCount++
				bugIDs[task.BugID] = struct{}{}
				key := forgeType.String() + ":" + task.BugID
				document := documents[key]
				if document == nil {
					// A failed detail hydration must not make a task title
					// searchable as if complete bug data were available.
					return nil
				}
				document.Tasks = append(document.Tasks, dto.BugCacheTask{
					TrackerProject: trackerProject,
					Task:           task,
				})
				return nil
			}); err != nil {
				return err
			}

			var lastSync time.Time
			if meta := tx.Bucket([]byte(metaBucket)); meta != nil {
				if data := meta.Get(lastSyncKey(forgeType, trackerProject)); data != nil {
					lastSync, _ = time.Parse(time.RFC3339, string(data))
				}
			}
			compatible, schemaVersion, err := projectCompatibility(
				tx,
				forgeType,
				trackerProject,
				CurrentSchemaVersion,
			)
			if err != nil {
				return err
			}
			snapshot.Status = append(snapshot.Status, dto.BugCacheStatus{
				ForgeType:     forgeType.String(),
				Project:       trackerProject,
				BugCount:      len(bugIDs),
				TaskCount:     taskCount,
				LastSync:      lastSync,
				SchemaVersion: schemaVersion,
				NeedsRefresh:  !compatible,
			})
			return nil
		}); err != nil {
			return err
		}

		for _, document := range documents {
			if len(document.Tasks) > 0 {
				snapshot.Documents = append(snapshot.Documents, *document)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(snapshot.Documents, func(i, j int) bool {
		left := snapshot.Documents[i].Bug
		right := snapshot.Documents[j].Bug
		if left.Forge != right.Forge {
			return left.Forge.String() < right.Forge.String()
		}
		return left.ID < right.ID
	})
	sort.Slice(snapshot.Status, func(i, j int) bool {
		if snapshot.Status[i].ForgeType != snapshot.Status[j].ForgeType {
			return snapshot.Status[i].ForgeType < snapshot.Status[j].ForgeType
		}
		return snapshot.Status[i].Project < snapshot.Status[j].Project
	})
	return snapshot, nil
}

func forgeTypeFromString(value string) (forge.ForgeType, bool) {
	for _, forgeType := range []forge.ForgeType{
		forge.ForgeGitHub,
		forge.ForgeLaunchpad,
		forge.ForgeGerrit,
	} {
		if forgeType.String() == value {
			return forgeType, true
		}
	}
	return 0, false
}

func cachedBugTags(tx *bbolt.Tx, forgeType forge.ForgeType, bugID string) []string {
	bkt := tx.Bucket(bugsBucketName(forgeType))
	if bkt == nil {
		return nil
	}
	data := bkt.Get([]byte(bugID))
	if data == nil {
		return nil
	}
	var bug forge.Bug
	if err := json.Unmarshal(data, &bug); err != nil {
		return nil
	}
	return bug.Tags
}

// SetLastSync records the last sync time for a (forge, project) pair.
func (c *Cache) SetLastSync(_ context.Context, forgeType forge.ForgeType, project string, t time.Time) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists([]byte(metaBucket))
		if err != nil {
			return fmt.Errorf("creating meta bucket: %w", err)
		}
		return bkt.Put(lastSyncKey(forgeType, project), []byte(t.UTC().Format(time.RFC3339)))
	})
}

// LastSync returns the last sync time for a (forge, project) pair.
func (c *Cache) LastSync(_ context.Context, forgeType forge.ForgeType, project string) (time.Time, error) {
	var t time.Time
	err := c.db.View(func(tx *bbolt.Tx) error {
		bkt := tx.Bucket([]byte(metaBucket))
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

// Remove clears cached data for a specific (forge, project) pair.
func (c *Cache) Remove(_ context.Context, forgeType forge.ForgeType, project string) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		_ = tx.DeleteBucket(tasksBucketName(forgeType, project))
		bkt := tx.Bucket([]byte(metaBucket))
		if bkt != nil {
			_ = bkt.Delete(lastSyncKey(forgeType, project))
		}
		return nil
	})
}

// RemoveAll clears all cached bug data.
func (c *Cache) RemoveAll(_ context.Context) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		var toDelete [][]byte
		err := tx.ForEach(func(name []byte, _ *bbolt.Bucket) error {
			toDelete = append(toDelete, append([]byte(nil), name...))
			return nil
		})
		if err != nil {
			return err
		}
		for _, name := range toDelete {
			if err := tx.DeleteBucket(name); err != nil {
				return fmt.Errorf("deleting bucket %s: %w", name, err)
			}
		}
		return nil
	})
}

// Close releases bbolt resources.
func (c *Cache) Close() error {
	return c.db.Close()
}

// CacheDir returns the base directory for the bug cache.
func (c *Cache) CacheDir() string {
	return c.baseDir
}

// Status returns per-project cache statistics.
func (c *Cache) Status(_ context.Context) ([]dto.BugCacheStatus, error) {
	var statuses []dto.BugCacheStatus
	err := c.db.View(func(tx *bbolt.Tx) error {
		// Collect all task buckets to count tasks per (forge, project).
		return tx.ForEach(func(name []byte, bkt *bbolt.Bucket) error {
			nameStr := string(name)
			if !strings.HasPrefix(nameStr, tasksBucketPrefix) {
				return nil
			}
			remainder := strings.TrimPrefix(nameStr, tasksBucketPrefix)
			parts := strings.SplitN(remainder, ":", 2)
			if len(parts) != 2 {
				return nil
			}
			forgeType, project := parts[0], parts[1]

			taskCount := 0
			bugIDs := map[string]struct{}{}
			_ = bkt.ForEach(func(k, _ []byte) error {
				taskCount++
				key := string(k)
				if idx := strings.IndexByte(key, ':'); idx > 0 {
					bugIDs[key[:idx]] = struct{}{}
				}
				return nil
			})

			var lastSync time.Time
			schemaVersion := 0
			needsRefresh := false
			metaBkt := tx.Bucket([]byte(metaBucket))
			if metaBkt != nil {
				data := metaBkt.Get([]byte(lastSyncPrefix + remainder))
				if data != nil {
					lastSync, _ = time.Parse(time.RFC3339, string(data))
				}
			}
			parsedForge, ok := forgeTypeFromString(forgeType)
			if ok {
				compatible, effectiveVersion, err := projectCompatibility(
					tx,
					parsedForge,
					project,
					CurrentSchemaVersion,
				)
				if err != nil {
					return err
				}
				schemaVersion = effectiveVersion
				needsRefresh = !compatible
			}

			statuses = append(statuses, dto.BugCacheStatus{
				ForgeType:     forgeType,
				Project:       project,
				BugCount:      len(bugIDs),
				TaskCount:     taskCount,
				LastSync:      lastSync,
				SchemaVersion: schemaVersion,
				NeedsRefresh:  needsRefresh,
			})
			return nil
		})
	})
	return statuses, err
}

// UpdateTask updates a single cached task identified by its self link.
// Used for write-through after status updates.
func (c *Cache) UpdateTask(_ context.Context, forgeType forge.ForgeType, task *forge.BugTask) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		type pendingUpdate struct {
			bucket []byte
			key    []byte
			data   []byte
		}
		var updates []pendingUpdate

		prefix := tasksBucketPrefix + forgeType.String() + ":"
		if err := tx.ForEach(func(name []byte, bkt *bbolt.Bucket) error {
			if !strings.HasPrefix(string(name), prefix) {
				return nil
			}
			return bkt.ForEach(func(k, v []byte) error {
				var existing forge.BugTask
				if err := json.Unmarshal(v, &existing); err != nil {
					return nil //nolint:nilerr // skip corrupt entries
				}
				if existing.SelfLink != task.SelfLink {
					return nil
				}
				data, err := json.Marshal(task)
				if err != nil {
					return fmt.Errorf("marshalling updated task: %w", err)
				}
				updates = append(updates, pendingUpdate{
					bucket: append([]byte(nil), name...),
					key:    append([]byte(nil), k...),
					data:   data,
				})
				return nil
			})
		}); err != nil {
			return err
		}

		for _, u := range updates {
			bkt := tx.Bucket(u.bucket)
			if bkt == nil {
				continue
			}
			if err := bkt.Put(u.key, u.data); err != nil {
				return err
			}
		}
		return nil
	})
}

// matchesOpts checks whether a task matches the filter options.
func matchesOpts(t *forge.BugTask, opts *forge.ListBugTasksOpts) bool {
	if len(opts.Status) > 0 && !slices.Contains(opts.Status, t.Status) {
		return false
	}
	if len(opts.Importance) > 0 && !slices.Contains(opts.Importance, t.Importance) {
		return false
	}
	if opts.Assignee != "" && t.Assignee != opts.Assignee {
		return false
	}
	if len(opts.Tags) > 0 {
		for _, tag := range opts.Tags {
			if !slices.Contains(t.Tags, tag) {
				return false
			}
		}
	}
	createdPass := true
	if opts.CreatedSince != "" {
		since, err := time.Parse(time.RFC3339, opts.CreatedSince)
		if err == nil {
			createdPass = !t.CreatedAt.Before(since)
		}
	}
	modifiedPass := true
	if opts.ModifiedSince != "" {
		since, err := time.Parse(time.RFC3339, opts.ModifiedSince)
		if err == nil {
			modifiedPass = !t.UpdatedAt.Before(since) || !t.CreatedAt.Before(since)
		}
	}
	if opts.CreatedSince != "" && opts.ModifiedSince != "" {
		if !createdPass && !modifiedPass {
			return false
		}
		return true
	}
	if !createdPass || !modifiedPass {
		return false
	}
	return true
}
