// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/url"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// CommitsListOptions holds query parameters for listing commits.
type CommitsListOptions struct {
	Projects   []string
	Forges     []string
	Branch     string
	Author     string
	IncludeMRs bool
}

// CommitsListResult is the response returned by CommitsList.
type CommitsListResult struct {
	Commits  []forge.Commit `json:"commits"`
	Warnings []string       `json:"warnings,omitempty"`
}

// CommitsList lists commits across all configured forges.
func (c *Client) CommitsList(ctx context.Context, opts CommitsListOptions) (*CommitsListResult, error) {
	q := commitQuery(opts)

	var result CommitsListResult
	err := c.get(ctx, "/api/v1/commits", q, &result)
	return &result, err
}

// CommitsTrackOptions holds query parameters for tracking commits by bug ID.
type CommitsTrackOptions struct {
	BugID      string
	Projects   []string
	Forges     []string
	Branch     string
	IncludeMRs bool
}

// CommitsTrackResult is the response returned by CommitsTrack.
type CommitsTrackResult struct {
	Commits  []forge.Commit `json:"commits"`
	Warnings []string       `json:"warnings,omitempty"`
}

// CommitsTrack finds commits referencing a specific bug ID.
func (c *Client) CommitsTrack(ctx context.Context, opts CommitsTrackOptions) (*CommitsTrackResult, error) {
	q := commitQuery(CommitsListOptions{
		Projects:   opts.Projects,
		Forges:     opts.Forges,
		Branch:     opts.Branch,
		IncludeMRs: opts.IncludeMRs,
	})
	q.Set("bug_id", opts.BugID)

	var result CommitsTrackResult
	err := c.get(ctx, "/api/v1/commits/track", q, &result)
	return &result, err
}

func commitQuery(opts CommitsListOptions) url.Values {
	q := url.Values{}
	for _, v := range opts.Projects {
		q.Add("project", v)
	}
	for _, v := range opts.Forges {
		q.Add("forge", v)
	}
	if opts.Branch != "" {
		q.Set("branch", opts.Branch)
	}
	if opts.Author != "" {
		q.Set("author", opts.Author)
	}
	if opts.IncludeMRs {
		q.Set("include_mrs", "true")
	}
	return q
}
