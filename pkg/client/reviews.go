// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/url"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// ReviewsListOptions holds query parameters for listing merge requests.
type ReviewsListOptions struct {
	Projects []string
	Forges   []string
	State    string
	Author   string
	Since    string
}

// ReviewsListResult is the response returned by ReviewsList.
type ReviewsListResult struct {
	MergeRequests []forge.MergeRequest `json:"merge_requests"`
	Warnings      []string             `json:"warnings,omitempty"`
}

// ReviewsList lists merge requests across all configured forges.
func (c *Client) ReviewsList(ctx context.Context, opts ReviewsListOptions) (*ReviewsListResult, error) {
	q := url.Values{}
	for _, v := range opts.Projects {
		q.Add("project", v)
	}
	for _, v := range opts.Forges {
		q.Add("forge", v)
	}
	if opts.State != "" {
		q.Set("state", opts.State)
	}
	if opts.Author != "" {
		q.Set("author", opts.Author)
	}
	if opts.Since != "" {
		q.Set("since", opts.Since)
	}

	var result ReviewsListResult
	err := c.get(ctx, "/api/v1/reviews", q, &result)
	return &result, err
}

// ReviewsGet retrieves a single merge request by project and ID.
func (c *Client) ReviewsGet(ctx context.Context, project, id string) (*forge.MergeRequest, error) {
	var result forge.MergeRequest
	err := c.get(ctx, "/api/v1/reviews/"+url.PathEscape(project)+"/"+url.PathEscape(id), nil, &result)
	return &result, err
}
