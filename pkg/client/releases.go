// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/url"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ReleasesListOptions holds query parameters for listing published artifact releases.
type ReleasesListOptions struct {
	Names        []string
	Projects     []string
	ArtifactType string
	Tracks       []string
	Branches     []string
	Risks        []string
}

// ReleasesList lists cached published artifact release rows.
func (c *Client) ReleasesList(ctx context.Context, opts ReleasesListOptions) ([]dto.ReleaseListEntry, error) {
	q := url.Values{}
	for _, value := range opts.Names {
		q.Add("name", value)
	}
	for _, value := range opts.Projects {
		q.Add("project", value)
	}
	if opts.ArtifactType != "" {
		q.Set("type", opts.ArtifactType)
	}
	for _, value := range opts.Tracks {
		q.Add("track", value)
	}
	for _, value := range opts.Branches {
		q.Add("branch", value)
	}
	for _, value := range opts.Risks {
		q.Add("risk", value)
	}
	var result struct {
		Releases []dto.ReleaseListEntry `json:"releases"`
	}
	err := c.get(ctx, "/api/v1/releases", q, &result)
	return result.Releases, err
}

// ReleasesShowOptions holds query parameters for showing one published artifact.
type ReleasesShowOptions struct {
	ArtifactType string
	Track        string
	Branch       string
}

// ReleasesShow fetches the cached full matrix for one published artifact.
func (c *Client) ReleasesShow(ctx context.Context, name string, opts ReleasesShowOptions) (*dto.ReleaseShowResult, error) {
	q := url.Values{}
	if opts.ArtifactType != "" {
		q.Set("type", opts.ArtifactType)
	}
	if opts.Track != "" {
		q.Set("track", opts.Track)
	}
	if opts.Branch != "" {
		q.Set("branch", opts.Branch)
	}
	var result dto.ReleaseShowResult
	err := c.get(ctx, "/api/v1/releases/"+url.PathEscape(name), q, &result)
	return &result, err
}
