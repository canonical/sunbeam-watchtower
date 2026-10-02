// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/url"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// OperationsListResult is the response returned by OperationsList.
type OperationsListResult struct {
	Jobs []dto.OperationJob `json:"jobs"`
}

// OperationEventsResult is the response returned by OperationEvents.
type OperationEventsResult struct {
	Events []dto.OperationEvent `json:"events"`
}

// OperationsList lists known long-running operations.
func (c *Client) OperationsList(ctx context.Context) ([]dto.OperationJob, error) {
	var result OperationsListResult
	err := c.get(ctx, "/api/v1/operations", nil, &result)
	return result.Jobs, err
}

// OperationGet fetches one operation snapshot.
func (c *Client) OperationGet(ctx context.Context, id string) (*dto.OperationJob, error) {
	var result dto.OperationJob
	err := c.get(ctx, "/api/v1/operations/"+url.PathEscape(id), nil, &result)
	return &result, err
}

// OperationEvents fetches the event history for one operation.
func (c *Client) OperationEvents(ctx context.Context, id string) ([]dto.OperationEvent, error) {
	var result OperationEventsResult
	err := c.get(ctx, "/api/v1/operations/"+url.PathEscape(id)+"/events", nil, &result)
	return result.Events, err
}

// OperationCancel requests cancellation for one operation.
func (c *Client) OperationCancel(ctx context.Context, id string) (*dto.OperationJob, error) {
	var result dto.OperationJob
	err := c.post(ctx, "/api/v1/operations/"+url.PathEscape(id)+"/cancel", nil, &result)
	return &result, err
}
