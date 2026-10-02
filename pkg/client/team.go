// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// TeamSyncOptions holds the request body for syncing team collaborators.
type TeamSyncOptions struct {
	Projects []string `json:"projects,omitempty"`
	DryRun   bool     `json:"dry_run,omitempty"`
}

// TeamSyncResult is the response returned by TeamSync.
type TeamSyncResult = dto.TeamSyncResult

// TeamSync syncs LP team members as store collaborators.
func (c *Client) TeamSync(ctx context.Context, opts TeamSyncOptions) (*TeamSyncResult, error) {
	var result TeamSyncResult
	err := c.post(ctx, "/api/v1/team/sync", opts, &result)
	return &result, err
}

// TeamSyncAsync starts team sync as a background operation.
func (c *Client) TeamSyncAsync(ctx context.Context, opts TeamSyncOptions) (*dto.OperationJob, error) {
	var result dto.OperationJob
	err := c.post(ctx, "/api/v1/team/sync/async", opts, &result)
	return &result, err
}
