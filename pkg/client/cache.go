// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/url"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// CacheSyncGitOptions holds the request body for syncing git caches.
type CacheSyncGitOptions struct {
	Projects []string `json:"projects,omitempty"`
}

// CacheSyncGitResult is the response returned by CacheSyncGit.
type CacheSyncGitResult struct {
	Synced   int      `json:"synced"`
	Warnings []string `json:"warnings"`
}

// CacheSyncGit syncs git caches for configured projects.
func (c *Client) CacheSyncGit(ctx context.Context, opts CacheSyncGitOptions) (*CacheSyncGitResult, error) {
	var result CacheSyncGitResult
	err := c.post(ctx, "/api/v1/cache/sync/git", opts, &result)
	return &result, err
}

// CacheSyncUpstreamResult is the response returned by CacheSyncUpstream.
type CacheSyncUpstreamResult struct {
	Status string `json:"status"`
}

// CacheSyncUpstream syncs upstream repos (releases, requirements).
func (c *Client) CacheSyncUpstream(ctx context.Context) (*CacheSyncUpstreamResult, error) {
	var result CacheSyncUpstreamResult
	err := c.post(ctx, "/api/v1/cache/sync/upstream", nil, &result)
	return &result, err
}

// CacheSyncBugsOptions holds the request body for syncing bug caches.
type CacheSyncBugsOptions struct {
	Projects []string `json:"projects,omitempty"`
}

// CacheSyncBugsResult is the response returned by CacheSyncBugs.
type CacheSyncBugsResult struct {
	Synced          int      `json:"synced"`
	RebuiltProjects []string `json:"rebuilt_projects"`
}

// CacheSyncBugs syncs bug caches for configured projects.
func (c *Client) CacheSyncBugs(ctx context.Context, opts CacheSyncBugsOptions) (*CacheSyncBugsResult, error) {
	var result CacheSyncBugsResult
	err := c.post(ctx, "/api/v1/cache/sync/bugs", opts, &result)
	return &result, err
}

// CacheSyncExcusesOptions holds the request body for syncing excuses caches.
type CacheSyncExcusesOptions struct {
	Trackers []string `json:"trackers,omitempty"`
}

// CacheSyncExcusesResult is the response returned by CacheSyncExcuses.
type CacheSyncExcusesResult struct {
	Status string `json:"status"`
}

// CacheSyncExcuses syncs excuses caches for the requested trackers.
func (c *Client) CacheSyncExcuses(ctx context.Context, opts CacheSyncExcusesOptions) (*CacheSyncExcusesResult, error) {
	var result CacheSyncExcusesResult
	err := c.post(ctx, "/api/v1/cache/sync/excuses", opts, &result)
	return &result, err
}

// CacheSyncReleasesResult is the response returned by CacheSyncReleases.
type CacheSyncReleasesResult = dto.ReleaseSyncResult

// CacheSyncReviewsOptions holds the request body for syncing review caches.
type CacheSyncReviewsOptions struct {
	Projects []string `json:"projects,omitempty"`
	Since    string   `json:"since,omitempty"`
}

// CacheSyncReviewsResult is the response returned by CacheSyncReviews.
type CacheSyncReviewsResult struct {
	ProjectsSynced  int      `json:"projects_synced"`
	SummariesSynced int      `json:"summaries_synced"`
	DetailsSynced   int      `json:"details_synced"`
	Warnings        []string `json:"warnings,omitempty"`
}

// CacheSyncReleases syncs cached published snap/charm release state.
func (c *Client) CacheSyncReleases(ctx context.Context) (*CacheSyncReleasesResult, error) {
	var result CacheSyncReleasesResult
	err := c.post(ctx, "/api/v1/cache/sync/releases", nil, &result)
	return &result, err
}

// CacheSyncReviews syncs cached review state.
func (c *Client) CacheSyncReviews(ctx context.Context, opts CacheSyncReviewsOptions) (*CacheSyncReviewsResult, error) {
	var result CacheSyncReviewsResult
	err := c.post(ctx, "/api/v1/cache/sync/reviews", opts, &result)
	return &result, err
}

// CacheDelete clears a specific cache type.
func (c *Client) CacheDelete(ctx context.Context, cacheType string, projects []string) error {
	return c.CacheDeleteWithTrackers(ctx, cacheType, projects, nil)
}

// CacheDeleteWithTrackers clears a specific cache type with optional excuses tracker filters.
func (c *Client) CacheDeleteWithTrackers(ctx context.Context, cacheType string, projects []string, trackers []string) error {
	q := url.Values{}
	for _, project := range projects {
		q.Add("project", project)
	}
	for _, tracker := range trackers {
		q.Add("tracker", tracker)
	}
	return c.delete(ctx, "/api/v1/cache/"+url.PathEscape(cacheType), q, nil)
}

// CacheEntry describes a single cached directory entry.
type CacheEntry struct {
	Name string `json:"name"`
	Size string `json:"size"`
}

// CacheStatusResult is the response returned by CacheStatus.
type CacheStatusResult struct {
	Git struct {
		Directory string       `json:"directory"`
		Repos     []CacheEntry `json:"repos"`
	} `json:"git"`
	Packages struct {
		Directory string            `json:"directory"`
		Sources   []dto.CacheStatus `json:"sources"`
		Error     string            `json:"error,omitempty"`
	} `json:"packages"`
	PackageSets struct {
		Entries []dto.PackageSetCacheStatus `json:"entries"`
		Error   string                      `json:"error,omitempty"`
	} `json:"packagesets"`
	Upstream struct {
		Directory string       `json:"directory"`
		Repos     []CacheEntry `json:"repos"`
	} `json:"upstream"`
	Bugs struct {
		Directory string               `json:"directory"`
		Entries   []dto.BugCacheStatus `json:"entries"`
		Error     string               `json:"error,omitempty"`
	} `json:"bugs"`
	Excuses struct {
		Directory string                   `json:"directory"`
		Entries   []dto.ExcusesCacheStatus `json:"entries"`
		Error     string                   `json:"error,omitempty"`
	} `json:"excuses"`
	Releases struct {
		Directory string                   `json:"directory"`
		Entries   []dto.ReleaseCacheStatus `json:"entries"`
		Error     string                   `json:"error,omitempty"`
	} `json:"releases"`
	Reviews struct {
		Directory string                  `json:"directory"`
		Entries   []dto.ReviewCacheStatus `json:"entries"`
		Error     string                  `json:"error,omitempty"`
	} `json:"reviews"`
	SRU struct {
		Status dto.SRUCacheStatus `json:"status"`
		Error  string             `json:"error,omitempty"`
	} `json:"sru"`
}

// CacheStatus returns the full cache status (git + packages + upstream).
func (c *Client) CacheStatus(ctx context.Context) (*CacheStatusResult, error) {
	var result CacheStatusResult
	err := c.get(ctx, "/api/v1/cache/status", nil, &result)
	return &result, err
}
