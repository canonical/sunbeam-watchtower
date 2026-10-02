// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ProjectsSyncOptions holds the request body for syncing LP projects.
type ProjectsSyncOptions struct {
	Projects []string `json:"projects,omitempty"`
	DryRun   bool     `json:"dry_run,omitempty"`
}

// ProjectsSyncResult is the response returned by ProjectsSync.
type ProjectsSyncResult struct {
	Actions []dto.ProjectSyncAction `json:"actions"`
	Errors  []string                `json:"errors,omitempty"`
}

// ProjectsSync syncs LP project series and development focus.
func (c *Client) ProjectsSync(ctx context.Context, opts ProjectsSyncOptions) (*ProjectsSyncResult, error) {
	var result ProjectsSyncResult
	err := c.post(ctx, "/api/v1/projects/sync", opts, &result)
	return &result, err
}

// ProjectsSyncAsync starts project sync as a background operation.
func (c *Client) ProjectsSyncAsync(ctx context.Context, opts ProjectsSyncOptions) (*dto.OperationJob, error) {
	var result dto.OperationJob
	err := c.post(ctx, "/api/v1/projects/sync/async", opts, &result)
	return &result, err
}
