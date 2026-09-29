// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/url"

	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
)

func (c *Client) SRUList(ctx context.Context, filter dto.SRUFilter) (*dto.SRUSnapshot, error) {
	query := url.Values{}
	for name, value := range map[string]string{
		"package": filter.Package, "set": filter.Set, "archive": filter.Archive, "series": filter.Series,
		"stage": filter.Stage, "task_status": filter.TaskStatus,
		"verification": filter.Verification, "bug_id": filter.BugID,
	} {
		if value != "" {
			query.Set(name, value)
		}
	}
	if filter.NeedsAttention {
		query.Set("needs_attention", "true")
	}
	var result dto.SRUSnapshot
	err := c.get(ctx, "/api/v1/sru", query, &result)
	return &result, err
}

func (c *Client) SRURefresh(ctx context.Context) (*dto.SRUSnapshot, error) {
	var result dto.SRUSnapshot
	err := c.post(ctx, "/api/v1/cache/sync/sru", struct{}{}, &result)
	return &result, err
}

func (c *Client) SRUShow(ctx context.Context, id string) (*dto.SRUSnapshot, error) {
	var result dto.SRUSnapshot
	err := c.get(ctx, "/api/v1/sru/"+url.PathEscape(id), nil, &result)
	return &result, err
}
