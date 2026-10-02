// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package reviewcache

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

const (
	defaultReviewFetchConcurrency = 4
	defaultReviewDetailWindow     = 30 * 24 * time.Hour
)

var (
	ErrNotSynced       = port.ErrReviewCacheNotSynced
	ErrDetailNotCached = port.ErrReviewDetailNotCached
)

// CachedForge wraps a live forge with cache-backed review reads.
type CachedForge struct {
	inner   port.Forge
	cache   port.ReviewCache
	project string
	logger  *slog.Logger
}

// SyncResult reports what one review cache sync stored.
type SyncResult struct {
	Summaries int
	Details   int
	Warnings  []string
}

// NewCachedForge wraps a forge with review-cache support.
func NewCachedForge(inner port.Forge, cache port.ReviewCache, project string, logger *slog.Logger) *CachedForge {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &CachedForge{inner: inner, cache: cache, project: project, logger: logger}
}

func (c *CachedForge) Type() forge.ForgeType {
	return c.inner.Type()
}

func (c *CachedForge) ListMergeRequests(ctx context.Context, _ string, opts forge.ListMergeRequestsOpts) ([]forge.MergeRequest, error) {
	if !c.isSynced(ctx) {
		return nil, ErrNotSynced
	}
	mrs, err := c.cache.List(ctx, c.inner.Type(), c.project)
	if err != nil {
		return nil, err
	}
	filtered := mrs[:0]
	for _, mr := range mrs {
		if opts.Author != "" && mr.Author != opts.Author {
			continue
		}
		if opts.State != 0 && mr.State != opts.State {
			continue
		}
		filtered = append(filtered, mr)
	}
	return filtered, nil
}

func (c *CachedForge) GetMergeRequest(ctx context.Context, _ string, id string) (*forge.MergeRequest, error) {
	if !c.isSynced(ctx) {
		return nil, ErrNotSynced
	}
	mr, err := c.cache.GetDetail(ctx, c.inner.Type(), c.project, id)
	if err == nil {
		return mr, nil
	}
	summaries, listErr := c.cache.List(ctx, c.inner.Type(), c.project)
	if listErr == nil {
		for _, summary := range summaries {
			if summary.ID == id {
				return nil, ErrDetailNotCached
			}
		}
	}
	return nil, err
}

func (c *CachedForge) ListCommits(ctx context.Context, repo string, opts forge.ListCommitsOpts) ([]forge.Commit, error) {
	return c.inner.ListCommits(ctx, repo, opts)
}

// Sync refreshes cached review summaries and details for one project.
func (c *CachedForge) Sync(ctx context.Context, repo string, since *time.Time) (*SyncResult, error) {
	mrs, err := c.inner.ListMergeRequests(ctx, repo, forge.ListMergeRequestsOpts{})
	if err != nil {
		return nil, err
	}
	if err := c.cache.StoreSummaries(ctx, c.inner.Type(), c.project, mrs); err != nil {
		return nil, err
	}
	cutoff := time.Now().Add(-defaultReviewDetailWindow)
	if since != nil {
		cutoff = *since
	}

	detailCandidates := make([]forge.MergeRequest, 0, len(mrs))
	for _, mr := range mrs {
		if mr.State == forge.MergeStateOpen || mr.State == forge.MergeStateWIP || !mr.UpdatedAt.Before(cutoff) {
			detailCandidates = append(detailCandidates, mr)
		}
	}

	warnings := c.syncDetails(ctx, repo, detailCandidates)
	if err := c.cache.PruneDetailsBefore(ctx, cutoff); err != nil {
		return nil, err
	}
	if err := c.cache.SetLastSync(ctx, c.inner.Type(), c.project, time.Now()); err != nil {
		return nil, err
	}
	detailCount := 0
	if statuses, err := c.cache.Status(ctx); err == nil {
		for _, status := range statuses {
			if status.Project == c.project && status.ForgeType == c.inner.Type().String() {
				detailCount = status.DetailCount
				break
			}
		}
	}

	return &SyncResult{
		Summaries: len(mrs),
		Details:   detailCount,
		Warnings:  warnings,
	}, nil
}

func (c *CachedForge) isSynced(ctx context.Context) bool {
	lastSync, err := c.cache.LastSync(ctx, c.inner.Type(), c.project)
	return err == nil && !lastSync.IsZero()
}

func (c *CachedForge) syncDetails(ctx context.Context, repo string, candidates []forge.MergeRequest) []string {
	if len(candidates) == 0 {
		return nil
	}
	workerCount := smallerInt(defaultReviewFetchConcurrency, len(candidates))
	type result struct {
		mr      *forge.MergeRequest
		warning string
	}
	jobs := make(chan forge.MergeRequest)
	results := make(chan result, len(candidates))

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for candidate := range jobs {
				detail, err := c.inner.GetMergeRequest(ctx, repo, candidate.ID)
				if err != nil {
					results <- result{warning: fmt.Sprintf("%s: %v", candidate.ID, err)}
					continue
				}
				if detail != nil {
					detail.Repo = c.project
					results <- result{mr: detail}
				}
			}
		}()
	}

	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		jobs <- candidate
	}
	close(jobs)
	wg.Wait()
	close(results)

	var warnings []string
	for result := range results {
		if result.warning != "" {
			warnings = append(warnings, result.warning)
			continue
		}
		if result.mr == nil {
			continue
		}
		if err := c.cache.StoreDetail(ctx, c.inner.Type(), c.project, *result.mr); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", result.mr.ID, err))
		}
	}
	slices.Sort(warnings)
	return warnings
}

func smallerInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
