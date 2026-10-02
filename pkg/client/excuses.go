// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/url"
	"strconv"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// PackagesExcusesListOptions holds query parameters for listing excuses.
type PackagesExcusesListOptions struct {
	Trackers     []string
	Name         string
	Component    string
	Team         string
	FTBFS        bool
	Autopkgtest  bool
	BlockedBy    string
	Bugged       bool
	MinAge       int
	MaxAge       int
	Limit        int
	Reverse      bool
	Set          string
	BlockedBySet string
}

// PackagesExcusesList returns excuses matching the given filters.
func (c *Client) PackagesExcusesList(ctx context.Context, opts PackagesExcusesListOptions) ([]dto.PackageExcuseSummary, error) {
	q := url.Values{}
	for _, tracker := range opts.Trackers {
		q.Add("tracker", tracker)
	}
	if opts.Name != "" {
		q.Set("name", opts.Name)
	}
	if opts.Component != "" {
		q.Set("component", opts.Component)
	}
	if opts.Team != "" {
		q.Set("team", opts.Team)
	}
	if opts.FTBFS {
		q.Set("ftbfs", "true")
	}
	if opts.Autopkgtest {
		q.Set("autopkgtest", "true")
	}
	if opts.BlockedBy != "" {
		q.Set("blocked_by", opts.BlockedBy)
	}
	if opts.Bugged {
		q.Set("bugged", "true")
	}
	if opts.MinAge > 0 {
		q.Set("min_age", strconv.Itoa(opts.MinAge))
	}
	if opts.MaxAge > 0 {
		q.Set("max_age", strconv.Itoa(opts.MaxAge))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Reverse {
		q.Set("reverse", "true")
	}
	if opts.Set != "" {
		q.Set("set", opts.Set)
	}
	if opts.BlockedBySet != "" {
		q.Set("blocked_by_set", opts.BlockedBySet)
	}

	var result []dto.PackageExcuseSummary
	err := c.get(ctx, "/api/v1/packages/excuses", q, &result)
	return result, err
}

// PackagesExcusesShowOptions holds query parameters for showing a single excuse.
type PackagesExcusesShowOptions struct {
	Tracker string
	Version string
}

// PackagesExcusesShow returns one package excuse.
func (c *Client) PackagesExcusesShow(ctx context.Context, name string, opts PackagesExcusesShowOptions) (*dto.PackageExcuse, error) {
	q := url.Values{}
	if opts.Tracker != "" {
		q.Set("tracker", opts.Tracker)
	}
	if opts.Version != "" {
		q.Set("version", opts.Version)
	}

	var result dto.PackageExcuse
	err := c.get(ctx, "/api/v1/packages/excuses/"+url.PathEscape(name), q, &result)
	return &result, err
}
