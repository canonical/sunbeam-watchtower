// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// BugCache manages local storage of bug and bug task data.
// Operations are keyed by (forgeType, project) to support multiple trackers.
type BugCache interface {
	// StoreBugs upserts bugs by (forge, ID). Only metadata is stored;
	// tasks are managed separately via StoreBugTasks.
	StoreBugs(ctx context.Context, bugs []*forge.Bug) error

	// StoreBugTasks replaces all cached tasks for a (forge, project) pair.
	StoreBugTasks(ctx context.Context, forgeType forge.ForgeType, project string, tasks []forge.BugTask) error

	// UpdateTask refreshes an existing cached task identified by self link.
	UpdateTask(ctx context.Context, forgeType forge.ForgeType, task *forge.BugTask) error

	// ReplaceProject atomically stores one complete tracker-project snapshot
	// and its cache metadata.
	ReplaceProject(ctx context.Context, forgeType forge.ForgeType, project string, bugs []*forge.Bug, tasks []forge.BugTask, syncedAt time.Time, schemaVersion int) error

	// ProjectCompatible reports whether all cached tasks for a tracker project
	// have visibility-qualified bug details in the current schema.
	ProjectCompatible(ctx context.Context, forgeType forge.ForgeType, project string, schemaVersion int) (bool, error)

	// GetBug retrieves a bug by ID, collecting tasks from all project buckets.
	GetBug(ctx context.Context, forgeType forge.ForgeType, id string) (*forge.Bug, error)

	// ListBugTasks returns cached tasks for a (forge, project) pair.
	// Filtering by status, importance, assignee, and tags is applied in-memory.
	ListBugTasks(ctx context.Context, forgeType forge.ForgeType, project string, opts forge.ListBugTasksOpts) ([]forge.BugTask, error)

	// Snapshot returns complete cached bugs with their tracker-project-scoped
	// tasks and per-project cache provenance.
	Snapshot(ctx context.Context) (*dto.BugCacheSnapshot, error)

	// SetLastSync records the last sync time for a (forge, project) pair.
	SetLastSync(ctx context.Context, forgeType forge.ForgeType, project string, t time.Time) error

	// LastSync returns the last sync time for a (forge, project) pair.
	LastSync(ctx context.Context, forgeType forge.ForgeType, project string) (time.Time, error)

	// Remove clears cached data for a specific (forge, project) pair.
	Remove(ctx context.Context, forgeType forge.ForgeType, project string) error

	// RemoveAll clears all cached bug data.
	RemoveAll(ctx context.Context) error

	// Close releases resources held by the cache.
	Close() error

	// CacheDir returns the cache directory path.
	CacheDir() string

	// Status returns per-project cache statistics.
	Status(ctx context.Context) ([]dto.BugCacheStatus, error)
}
