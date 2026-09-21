// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package bugcache

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/gboutry/sunbeam-watchtower/internal/core/port"
	forge "github.com/gboutry/sunbeam-watchtower/pkg/forge/v1"
)

// CachedBugTracker decorates a port.BugTracker with local cache support.
// Read operations serve from cache when populated, falling back to the inner
// tracker. Write operations delegate to the inner tracker and update the cache.
type CachedBugTracker struct {
	inner   port.BugTracker
	cache   port.BugCache
	project string
	logger  *slog.Logger
}

const incrementalModifiedOverlap = 24 * time.Hour
const defaultBugFetchConcurrency = 4

// NewCachedBugTracker wraps a BugTracker with caching support.
func NewCachedBugTracker(inner port.BugTracker, cache port.BugCache, project string, logger *slog.Logger) *CachedBugTracker {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &CachedBugTracker{inner: inner, cache: cache, project: project, logger: logger}
}

func (c *CachedBugTracker) Type() forge.ForgeType {
	return c.inner.Type()
}

func (c *CachedBugTracker) GetBug(ctx context.Context, id string) (*forge.Bug, error) {
	if c.isSynced(ctx) {
		b, err := c.cache.GetBug(ctx, c.inner.Type(), id)
		if err == nil {
			lastSync, _ := c.cache.LastSync(ctx, c.inner.Type(), c.project)
			if b.Sensitive() {
				fresh, freshErr := c.inner.GetBug(ctx, id)
				if freshErr != nil {
					return nil, fmt.Errorf("revalidating sensitive bug visibility: %w", freshErr)
				}
				fresh.Provenance = &forge.BugProvenance{
					Source:     "combined",
					SyncedAt:   lastSync,
					VerifiedAt: time.Now().UTC(),
				}
				return fresh, nil
			}
			b.Provenance = &forge.BugProvenance{
				Source:   "cache",
				SyncedAt: lastSync,
			}
			c.logger.Debug("bug served from cache", "id", id)
			return b, nil
		}
		c.logger.Debug("bug not in cache, falling back to live", "id", id, "error", err)
	}
	bug, err := c.inner.GetBug(ctx, id)
	if err == nil {
		bug.Provenance = &forge.BugProvenance{
			Source:     "remote",
			VerifiedAt: time.Now().UTC(),
		}
	}
	return bug, err
}

// GetBugFresh bypasses cached task state for workflows that may mutate the
// upstream tracker. Planning writes from stale statuses can otherwise produce
// redundant or incorrect actions.
func (c *CachedBugTracker) GetBugFresh(ctx context.Context, id string, refreshCache bool) (*forge.Bug, error) {
	bug, err := c.inner.GetBug(ctx, id)
	if err != nil {
		return nil, err
	}
	bug.Provenance = &forge.BugProvenance{
		Source:     "remote",
		VerifiedAt: time.Now().UTC(),
	}
	if refreshCache && c.isSynced(ctx) {
		if cacheErr := c.cache.StoreBugs(ctx, []*forge.Bug{bug}); cacheErr != nil {
			c.logger.Warn("failed to refresh cached bug metadata", "id", id, "error", cacheErr)
		}
		for i := range bug.Tasks {
			if cacheErr := c.cache.UpdateTask(ctx, c.inner.Type(), &bug.Tasks[i]); cacheErr != nil {
				c.logger.Warn("failed to refresh cached bug task", "id", id, "task", bug.Tasks[i].SelfLink, "error", cacheErr)
			}
		}
	}
	return bug, nil
}

func (c *CachedBugTracker) ListBugTasks(ctx context.Context, project string, opts forge.ListBugTasksOpts) ([]forge.BugTask, error) {
	if c.isSynced(ctx) {
		tasks, err := c.cache.ListBugTasks(ctx, c.inner.Type(), project, opts)
		if err == nil {
			tasks = c.filterVisibleSensitiveTasks(ctx, tasks)
			c.logger.Debug("bug tasks served from cache", "project", project, "count", len(tasks))
			return tasks, nil
		}
		c.logger.Debug("cache read failed, falling back to live", "project", project, "error", err)
	}
	return c.inner.ListBugTasks(ctx, project, opts)
}

func (c *CachedBugTracker) filterVisibleSensitiveTasks(ctx context.Context, tasks []forge.BugTask) []forge.BugTask {
	result := make([]forge.BugTask, 0, len(tasks))
	visibility := make(map[string]bool)
	for _, task := range tasks {
		if !task.Sensitive() {
			result = append(result, task)
			continue
		}
		visible, checked := visibility[task.BugID]
		if !checked {
			_, err := c.inner.GetBug(ctx, task.BugID)
			visible = err == nil
			visibility[task.BugID] = visible
		}
		if visible {
			result = append(result, task)
		}
	}
	return result
}

func (c *CachedBugTracker) UpdateBugTaskStatus(ctx context.Context, taskSelfLink, status string) error {
	if err := c.inner.UpdateBugTaskStatus(ctx, taskSelfLink, status); err != nil {
		return err
	}
	// Write-through: update cached task if the cache is populated.
	if c.isSynced(ctx) {
		c.updateCachedTaskStatus(ctx, taskSelfLink, status)
	}
	return nil
}

func (c *CachedBugTracker) AddBugTask(ctx context.Context, bugID int, seriesSelfLink string) error {
	return c.inner.AddBugTask(ctx, bugID, seriesSelfLink)
}

func (c *CachedBugTracker) GetProjectSeries(ctx context.Context, projectName string) ([]forge.ProjectSeries, error) {
	return c.inner.GetProjectSeries(ctx, projectName)
}

func (c *CachedBugTracker) GetProject(ctx context.Context, projectName string) (*forge.Project, error) {
	return c.inner.GetProject(ctx, projectName)
}

// Sync fetches bugs from the inner tracker and stores them in the cache.
// If the cache was previously synced, only tasks modified since the last sync
// are fetched (incremental sync).
func (c *CachedBugTracker) Sync(ctx context.Context) (synced int, err error) {
	forgeType := c.inner.Type()

	opts := forge.ListBugTasksOpts{}
	lastSync, lsErr := c.cache.LastSync(ctx, forgeType, c.project)
	compatible, compatibilityErr := c.cache.ProjectCompatible(
		ctx,
		forgeType,
		c.project,
		CurrentSchemaVersion,
	)
	if compatibilityErr != nil {
		return 0, fmt.Errorf("checking bug cache compatibility for %s: %w", c.project, compatibilityErr)
	}
	rebuild := !lastSync.IsZero() && !compatible
	if lsErr == nil && !lastSync.IsZero() && !rebuild {
		opts.CreatedSince = lastSync.UTC().Format(time.RFC3339)
		modifiedSince := lastSync.Add(-incrementalModifiedOverlap)
		opts.ModifiedSince = modifiedSince.UTC().Format(time.RFC3339)
		c.logger.Debug(
			"incremental bug cache sync",
			"project", c.project,
			"created_since", opts.CreatedSince,
			"modified_since", opts.ModifiedSince,
			"modified_overlap", incrementalModifiedOverlap.String(),
		)
	} else if rebuild {
		c.logger.Info("rebuilding incompatible bug cache", "project", c.project)
	} else {
		c.logger.Debug("full bug cache sync", "project", c.project)
	}

	incoming, err := c.inner.ListBugTasks(ctx, c.project, opts)
	if err != nil {
		return 0, fmt.Errorf("fetching bug tasks for %s: %w", c.project, err)
	}

	// Only fetch bug details for newly returned tasks, not the entire cache.
	bugIDs := uniqueBugIDs(incoming)
	bugs, err := c.fetchBugs(ctx, bugIDs)
	if err != nil {
		return 0, err
	}
	incoming = enrichTasksWithBugMetadata(incoming, bugs)
	if err := validateVisibilityMetadata(incoming, bugs); err != nil {
		return 0, err
	}

	// For incremental sync, merge new tasks with existing cached tasks.
	tasks := incoming
	if opts.ModifiedSince != "" || opts.CreatedSince != "" {
		existing, _ := c.cache.ListBugTasks(ctx, forgeType, c.project, forge.ListBugTasksOpts{})
		tasks = mergeTasks(existing, incoming)
	}

	if err := c.cache.ReplaceProject(
		ctx,
		forgeType,
		c.project,
		bugs,
		tasks,
		time.Now(),
		CurrentSchemaVersion,
	); err != nil {
		return 0, fmt.Errorf("publishing bug cache snapshot for %s: %w", c.project, err)
	}

	c.logger.Debug("bug cache sync complete", "project", c.project, "tasks", len(tasks), "bugs", len(bugs))
	return len(tasks), nil
}

// NeedsRebuild reports whether the configured project requires a full cache
// refresh before it is safe for offline search.
func (c *CachedBugTracker) NeedsRebuild(ctx context.Context) (bool, error) {
	compatible, err := c.cache.ProjectCompatible(
		ctx,
		c.inner.Type(),
		c.project,
		CurrentSchemaVersion,
	)
	return !compatible, err
}

func validateVisibilityMetadata(tasks []forge.BugTask, bugs []*forge.Bug) error {
	bugsByID := make(map[string]*forge.Bug, len(bugs))
	for _, bug := range bugs {
		if bug != nil {
			bugsByID[bug.ID] = bug
		}
	}
	for _, task := range tasks {
		bug := bugsByID[task.BugID]
		if bug == nil {
			return fmt.Errorf("bug %s has no hydrated details", task.BugID)
		}
		if !bug.VisibilityKnown || !task.VisibilityKnown {
			return fmt.Errorf("bug %s has unknown visibility", task.BugID)
		}
	}
	return nil
}

func enrichTasksWithBugMetadata(tasks []forge.BugTask, bugs []*forge.Bug) []forge.BugTask {
	if len(tasks) == 0 || len(bugs) == 0 {
		return tasks
	}
	bugsByID := make(map[string]*forge.Bug, len(bugs))
	for _, bug := range bugs {
		if bug == nil {
			continue
		}
		bugsByID[bug.ID] = bug
	}
	if len(bugsByID) == 0 {
		return tasks
	}
	enriched := make([]forge.BugTask, len(tasks))
	copy(enriched, tasks)
	for i := range enriched {
		bug := bugsByID[enriched[i].BugID]
		if bug == nil {
			continue
		}
		if len(enriched[i].Tags) == 0 {
			enriched[i].Tags = bug.Tags
		}
		enriched[i].Private = bug.Private
		enriched[i].SecurityRelated = bug.SecurityRelated
		enriched[i].InformationType = bug.InformationType
		enriched[i].VisibilityKnown = bug.VisibilityKnown
	}
	return enriched
}

// Project returns the project ID this cached tracker operates on.
func (c *CachedBugTracker) Project() string {
	return c.project
}

func (c *CachedBugTracker) isSynced(ctx context.Context) bool {
	t, err := c.cache.LastSync(ctx, c.inner.Type(), c.project)
	return err == nil && !t.IsZero()
}

// updateCachedTaskStatus finds a task by self link in the cache and updates its status.
func (c *CachedBugTracker) updateCachedTaskStatus(ctx context.Context, selfLink, newStatus string) {
	forgeType := c.inner.Type()
	tasks, err := c.cache.ListBugTasks(ctx, forgeType, c.project, forge.ListBugTasksOpts{})
	if err != nil {
		return
	}
	for i := range tasks {
		if tasks[i].SelfLink == selfLink {
			tasks[i].Status = newStatus
			if uErr := c.cache.UpdateTask(ctx, forgeType, &tasks[i]); uErr != nil {
				c.logger.Warn("failed to update cached task status", "selfLink", selfLink, "error", uErr)
			}
			return
		}
	}
}

// uniqueBugIDs returns deduplicated bug IDs from a set of tasks.
func uniqueBugIDs(tasks []forge.BugTask) []string {
	seen := make(map[string]struct{}, len(tasks))
	var ids []string
	for _, t := range tasks {
		if _, ok := seen[t.BugID]; !ok {
			seen[t.BugID] = struct{}{}
			ids = append(ids, t.BugID)
		}
	}
	return ids
}

// mergeTasks merges newly fetched tasks into the existing cached task list.
// New tasks replace existing tasks with the same key (BugID:TargetName).
func mergeTasks(existing, incoming []forge.BugTask) []forge.BugTask {
	merged := make(map[string]forge.BugTask, len(existing)+len(incoming))
	for _, t := range existing {
		merged[t.BugID+":"+t.TargetName] = t
	}
	for _, t := range incoming {
		merged[t.BugID+":"+t.TargetName] = t
	}
	result := make([]forge.BugTask, 0, len(merged))
	for _, t := range merged {
		result = append(result, t)
	}
	return result
}

func (c *CachedBugTracker) fetchBugs(ctx context.Context, bugIDs []string) ([]*forge.Bug, error) {
	if len(bugIDs) == 0 {
		return nil, nil
	}

	type fetchResult struct {
		index int
		bug   *forge.Bug
		err   error
	}

	workerCount := min(defaultBugFetchConcurrency, len(bugIDs))
	jobs := make(chan int)
	results := make(chan fetchResult, len(bugIDs))

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if ctx.Err() != nil {
					return
				}
				id := bugIDs[idx]
				b, err := c.inner.GetBug(ctx, id)
				if err != nil {
					results <- fetchResult{index: idx, err: fmt.Errorf("fetching bug %s: %w", id, err)}
					continue
				}
				results <- fetchResult{index: idx, bug: b}
			}
		}()
	}

	for idx := range bugIDs {
		if ctx.Err() != nil {
			break
		}
		jobs <- idx
	}
	close(jobs)
	wg.Wait()
	close(results)

	ordered := make([]*forge.Bug, len(bugIDs))
	for result := range results {
		if result.err != nil {
			return nil, result.err
		}
		ordered[result.index] = result.bug
	}

	bugs := make([]*forge.Bug, 0, len(bugIDs))
	for _, bug := range ordered {
		if bug != nil {
			bugs = append(bugs, bug)
		}
	}
	return bugs, nil
}
