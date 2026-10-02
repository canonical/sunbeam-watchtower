// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/url"

	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// PackagesDiffOptions holds query parameters for the packages diff endpoint.
type PackagesDiffOptions struct {
	Set             string
	Distros         []string
	Releases        []string
	Suites          []string
	Backports       []string
	Merge           bool
	UpstreamRelease string
	BehindUpstream  bool
	OnlyIn          string
	Constraints     string
}

// PackagesDiff compares package versions across distros for the given package set.
func (c *Client) PackagesDiff(ctx context.Context, opts PackagesDiffOptions) ([]dto.PackageDiffResult, error) {
	q := url.Values{}
	for _, v := range opts.Distros {
		q.Add("distro", v)
	}
	for _, v := range opts.Releases {
		q.Add("release", v)
	}
	for _, v := range opts.Suites {
		q.Add("suite", v)
	}
	for _, v := range opts.Backports {
		q.Add("backport", v)
	}
	if opts.Merge {
		q.Set("merge", "true")
	}
	if opts.UpstreamRelease != "" {
		q.Set("upstream_release", opts.UpstreamRelease)
	}
	if opts.BehindUpstream {
		q.Set("behind_upstream", "true")
	}
	if opts.OnlyIn != "" {
		q.Set("only_in", opts.OnlyIn)
	}
	if opts.Constraints != "" {
		q.Set("constraints", opts.Constraints)
	}

	var result []dto.PackageDiffResult
	err := c.get(ctx, "/api/v1/packages/diff/"+url.PathEscape(opts.Set), q, &result)
	return result, err
}

// PackagesShowOptions holds query parameters for the packages show endpoint.
type PackagesShowOptions struct {
	Distros         []string
	Releases        []string
	Suites          []string
	Backports       []string
	Merge           bool
	UpstreamRelease string
}

// PackagesShow returns version information for a single package across distros.
func (c *Client) PackagesShow(ctx context.Context, name string, opts PackagesShowOptions) (*dto.PackageDiffResult, error) {
	q := url.Values{}
	for _, v := range opts.Distros {
		q.Add("distro", v)
	}
	for _, v := range opts.Releases {
		q.Add("release", v)
	}
	for _, v := range opts.Suites {
		q.Add("suite", v)
	}
	for _, v := range opts.Backports {
		q.Add("backport", v)
	}
	if opts.Merge {
		q.Set("merge", "true")
	}
	if opts.UpstreamRelease != "" {
		q.Set("upstream_release", opts.UpstreamRelease)
	}

	var result dto.PackageDiffResult
	err := c.get(ctx, "/api/v1/packages/show/"+url.PathEscape(name), q, &result)
	return &result, err
}

// PackagesDetailOptions holds query parameters for the packages detail endpoint.
type PackagesDetailOptions struct {
	Version   string
	Distros   []string
	Releases  []string
	Suites    []string
	Backports []string
}

// PackagesDetail returns full APT metadata for a specific package version.
func (c *Client) PackagesDetail(ctx context.Context, name string, opts PackagesDetailOptions) (*distro.SourcePackageInfo, error) {
	q := url.Values{}
	if opts.Version != "" {
		q.Set("version", opts.Version)
	}
	for _, v := range opts.Distros {
		q.Add("distro", v)
	}
	for _, v := range opts.Releases {
		q.Add("release", v)
	}
	for _, v := range opts.Suites {
		q.Add("suite", v)
	}
	for _, v := range opts.Backports {
		q.Add("backport", v)
	}

	var result distro.SourcePackageInfo
	err := c.get(ctx, "/api/v1/packages/detail/"+url.PathEscape(name), q, &result)
	return &result, err
}

// PackagesListOptions holds query parameters for the packages list endpoint.
type PackagesListOptions struct {
	Distros    []string
	Releases   []string
	Suites     []string
	Components []string
	Backports  []string
}

// PackagesList lists packages in the given distro(s).
func (c *Client) PackagesList(ctx context.Context, opts PackagesListOptions) ([]distro.SourcePackage, error) {
	q := url.Values{}
	for _, v := range opts.Distros {
		q.Add("distro", v)
	}
	for _, v := range opts.Releases {
		q.Add("release", v)
	}
	for _, v := range opts.Suites {
		q.Add("suite", v)
	}
	for _, v := range opts.Components {
		q.Add("component", v)
	}
	for _, v := range opts.Backports {
		q.Add("backport", v)
	}

	var result []distro.SourcePackage
	err := c.get(ctx, "/api/v1/packages/list", q, &result)
	return result, err
}

// PackagesRdependsOptions holds query parameters for the packages rdepends endpoint.
type PackagesRdependsOptions struct {
	Distros   []string
	Releases  []string
	Suites    []string
	Backports []string
}

// PackagesRdepends finds source packages that build-depend on the given package.
func (c *Client) PackagesRdepends(ctx context.Context, name string, opts PackagesRdependsOptions) ([]distro.SourcePackageDetail, error) {
	q := url.Values{}
	for _, v := range opts.Distros {
		q.Add("distro", v)
	}
	for _, v := range opts.Releases {
		q.Add("release", v)
	}
	for _, v := range opts.Suites {
		q.Add("suite", v)
	}
	for _, v := range opts.Backports {
		q.Add("backport", v)
	}

	var result []distro.SourcePackageDetail
	err := c.get(ctx, "/api/v1/packages/rdepends/"+url.PathEscape(name), q, &result)
	return result, err
}

// PackagesDscOptions holds query parameters for the packages dsc endpoint.
type PackagesDscOptions struct {
	Packages  []string
	Distros   []string
	Releases  []string
	Backports []string
}

// PackagesDsc looks up .dsc file URLs for the given source package/version pairs.
// Each entry in Packages should be in "name=version" format.
func (c *Client) PackagesDsc(ctx context.Context, opts PackagesDscOptions) ([]dto.PackageDscResult, error) {
	q := url.Values{}
	for _, v := range opts.Packages {
		q.Add("packages", v)
	}
	for _, v := range opts.Distros {
		q.Add("distro", v)
	}
	for _, v := range opts.Releases {
		q.Add("release", v)
	}
	for _, v := range opts.Backports {
		q.Add("backport", v)
	}

	var result []dto.PackageDscResult
	err := c.get(ctx, "/api/v1/packages/dsc", q, &result)
	return result, err
}

// PackagesCacheStatus returns the cache status for each indexed source group.
func (c *Client) PackagesCacheStatus(ctx context.Context) ([]dto.CacheStatus, error) {
	var result []dto.CacheStatus
	err := c.get(ctx, "/api/v1/packages/cache/status", nil, &result)
	return result, err
}

// PackagesCacheSyncOptions holds the request body for the cache sync endpoint.
type PackagesCacheSyncOptions struct {
	Distros   []string `json:"distros,omitempty"`
	Releases  []string `json:"releases,omitempty"`
	Backports []string `json:"backports,omitempty"`
}

// PackagesCacheSync triggers a package cache sync.
func (c *Client) PackagesCacheSync(ctx context.Context, opts PackagesCacheSyncOptions) error {
	return c.post(ctx, "/api/v1/packages/cache/sync", opts, nil)
}

// PackageSetsCacheSync refreshes configured Launchpad packageset snapshots.
func (c *Client) PackageSetsCacheSync(ctx context.Context, sets []string) ([]dto.PackageSetCacheStatus, error) {
	var result []dto.PackageSetCacheStatus
	err := c.post(ctx, "/api/v1/packages/sets/cache/sync", struct {
		Sets []string `json:"sets,omitempty"`
	}{Sets: sets}, &result)
	return result, err
}
