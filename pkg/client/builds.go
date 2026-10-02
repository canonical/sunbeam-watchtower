// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/url"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// BuildsTriggerOptions holds the request body for triggering builds.
type BuildsTriggerOptions struct {
	Project    string                   `json:"project"`
	Artifacts  []string                 `json:"artifacts,omitempty"`
	Wait       bool                     `json:"wait,omitempty"`
	Timeout    string                   `json:"timeout,omitempty"`
	Owner      string                   `json:"owner,omitempty"`
	Prefix     string                   `json:"prefix,omitempty"`
	TargetRef  string                   `json:"target_ref,omitempty"`
	Prepared   *dto.PreparedBuildSource `json:"prepared,omitempty"`
	RetryCount int                      `json:"retry_count,omitempty"`
}

// BuildsTrigger triggers builds for a project.
func (c *Client) BuildsTrigger(ctx context.Context, opts BuildsTriggerOptions) (*dto.BuildTriggerResult, error) {
	var result dto.BuildTriggerResult
	err := c.post(ctx, "/api/v1/builds/trigger", opts, &result)
	return &result, err
}

// BuildsTriggerAsync triggers builds as a background operation.
func (c *Client) BuildsTriggerAsync(ctx context.Context, opts BuildsTriggerOptions) (*dto.OperationJob, error) {
	var result dto.OperationJob
	err := c.post(ctx, "/api/v1/builds/trigger/async", opts, &result)
	return &result, err
}

// BuildsListOptions holds query parameters for listing builds.
type BuildsListOptions struct {
	Projects     []string
	All          bool
	State        string
	Owner        string
	TargetRef    string
	RecipeNames  []string
	RecipePrefix string
}

// BuildsListResult is the response returned by BuildsList.
type BuildsListResult struct {
	Builds []dto.Build `json:"builds"`
}

// BuildsList lists builds across projects.
func (c *Client) BuildsList(ctx context.Context, opts BuildsListOptions) ([]dto.Build, error) {
	q := url.Values{}
	for _, v := range opts.Projects {
		q.Add("project", v)
	}
	if opts.All {
		q.Set("all", "true")
	}
	if opts.State != "" {
		q.Set("state", opts.State)
	}
	if opts.Owner != "" {
		q.Set("owner", opts.Owner)
	}
	if opts.TargetRef != "" {
		q.Set("target_ref", opts.TargetRef)
	}
	for _, v := range opts.RecipeNames {
		q.Add("recipe", v)
	}
	if opts.RecipePrefix != "" {
		q.Set("recipe_prefix", opts.RecipePrefix)
	}

	var result BuildsListResult
	err := c.get(ctx, "/api/v1/builds", q, &result)
	return result.Builds, err
}

// BuildsDownloadOptions holds the request body for downloading build artifacts.
type BuildsDownloadOptions struct {
	Project      string   `json:"project"`
	Artifacts    []string `json:"artifacts,omitempty"`
	RecipePrefix string   `json:"recipe_prefix,omitempty"`
	Owner        string   `json:"owner,omitempty"`
	TargetRef    string   `json:"target_ref,omitempty"`
	ArtifactsDir string   `json:"artifacts_dir,omitempty"`
	RetryCount   int      `json:"retry_count,omitempty"`
}

// BuildsDownload downloads build artifacts.
func (c *Client) BuildsDownload(ctx context.Context, opts BuildsDownloadOptions) error {
	return c.post(ctx, "/api/v1/builds/download", opts, nil)
}

// BuildsCleanupOptions holds the request body for cleaning up temporary recipes.
type BuildsCleanupOptions struct {
	Project string `json:"project,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Prefix  string `json:"prefix,omitempty"`
	DryRun  bool   `json:"dry_run,omitempty"`
}

// BuildsCleanupResult is the response returned by BuildsCleanup.
type BuildsCleanupResult struct {
	DeletedRecipes  []string `json:"deleted_recipes"`
	DeletedBranches []string `json:"deleted_branches"`
}

// BuildsCleanup deletes temporary build recipes and branches.
func (c *Client) BuildsCleanup(ctx context.Context, opts BuildsCleanupOptions) (*BuildsCleanupResult, error) {
	var result BuildsCleanupResult
	err := c.post(ctx, "/api/v1/builds/cleanup", opts, &result)
	return &result, err
}

// BuildsRetryOptions holds the request body for retrying a build.
type BuildsRetryOptions struct {
	BuildSelfLink string `json:"build_self_link"`
	ArtifactType  string `json:"artifact_type"`
}

// BuildsRetry retries a failed build.
func (c *Client) BuildsRetry(ctx context.Context, opts BuildsRetryOptions) error {
	return c.post(ctx, "/api/v1/builds/retry", opts, nil)
}

// BuildsCancelOptions holds the request body for cancelling a build.
type BuildsCancelOptions struct {
	BuildSelfLink string `json:"build_self_link"`
	ArtifactType  string `json:"artifact_type"`
}

// BuildsCancel cancels an active build.
func (c *Client) BuildsCancel(ctx context.Context, opts BuildsCancelOptions) error {
	return c.post(ctx, "/api/v1/builds/cancel", opts, nil)
}
