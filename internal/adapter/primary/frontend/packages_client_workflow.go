// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/pkg/client"
	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// PackagesDiffRequest describes one package diff workflow.
type PackagesDiffRequest struct {
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

// PackagesDiffResponse contains diff results plus resolved source metadata for rendering.
type PackagesDiffResponse struct {
	Results     []dto.PackageDiffResult
	Sources     []dto.PackageSource
	Merge       bool
	HasUpstream bool
}

// PackagesShowVersionRequest describes one package show-version workflow.
type PackagesShowVersionRequest struct {
	Package         string
	Distros         []string
	Releases        []string
	Backports       []string
	Merge           bool
	UpstreamRelease string
}

// PackagesShowVersionResponse contains one package version result plus resolved source metadata.
type PackagesShowVersionResponse struct {
	Result      dto.PackageDiffResult
	Sources     []dto.PackageSource
	Merge       bool
	HasUpstream bool
}

// PackagesDetailRequest describes one package detail workflow.
type PackagesDetailRequest struct {
	Package   string
	Version   string
	Distros   []string
	Releases  []string
	Suites    []string
	Backports []string
}

// PackagesListRequest describes one package list workflow.
type PackagesListRequest struct {
	Distros    []string
	Releases   []string
	Suites     []string
	Components []string
	Backports  []string
}

// PackagesDscRequest describes one package dsc workflow.
type PackagesDscRequest struct {
	Packages  []string
	Distros   []string
	Releases  []string
	Backports []string
}

// PackagesRdependsRequest describes one reverse-dependency workflow.
type PackagesRdependsRequest struct {
	Package   string
	Distros   []string
	Releases  []string
	Suites    []string
	Backports []string
}

// PackagesExcusesListRequest describes one excuses-list workflow.
type PackagesExcusesListRequest struct {
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

// PackagesExcusesShowRequest describes one excuses-show workflow.
type PackagesExcusesShowRequest struct {
	Package string
	Tracker string
	Version string
}

// PackagesClientWorkflow exposes reusable client-side package workflows for CLI/TUI/MCP frontends.
type PackagesClientWorkflow struct {
	client      *ClientTransport
	application *app.App
}

// NewPackagesClientWorkflow creates a client-side packages workflow.
func NewPackagesClientWorkflow(apiClient *ClientTransport, application *app.App) *PackagesClientWorkflow {
	return &PackagesClientWorkflow{
		client:      apiClient,
		application: application,
	}
}

// Diff compares package versions and resolves source metadata for rendering.
func (w *PackagesClientWorkflow) Diff(ctx context.Context, req PackagesDiffRequest) (*PackagesDiffResponse, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}

	effectiveUpstreamRelease := w.effectiveUpstreamRelease(ctx, req.UpstreamRelease, req.Constraints, req.Backports)
	results, err := apiClient.PackagesDiff(ctx, client.PackagesDiffOptions{
		Set:             req.Set,
		Distros:         req.Distros,
		Releases:        req.Releases,
		Suites:          req.Suites,
		Backports:       req.Backports,
		Merge:           req.Merge,
		UpstreamRelease: effectiveUpstreamRelease,
		BehindUpstream:  req.BehindUpstream,
		OnlyIn:          req.OnlyIn,
		Constraints:     req.Constraints,
	})
	if err != nil {
		return nil, err
	}

	return &PackagesDiffResponse{
		Results:     results,
		Sources:     w.buildSources(req.Distros, req.Releases, req.Suites, req.Backports),
		Merge:       req.Merge,
		HasUpstream: effectiveUpstreamRelease != "" || hasUpstreamResults(results),
	}, nil
}

// ShowVersion returns version information for one package plus resolved source metadata.
func (w *PackagesClientWorkflow) ShowVersion(ctx context.Context, req PackagesShowVersionRequest) (*PackagesShowVersionResponse, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}

	effectiveUpstreamRelease := w.effectiveUpstreamRelease(ctx, req.UpstreamRelease, "", req.Backports)
	result, err := apiClient.PackagesShow(ctx, req.Package, client.PackagesShowOptions{
		Distros:         req.Distros,
		Releases:        req.Releases,
		Backports:       req.Backports,
		Merge:           req.Merge,
		UpstreamRelease: effectiveUpstreamRelease,
	})
	if err != nil {
		return nil, err
	}

	return &PackagesShowVersionResponse{
		Result:      *result,
		Sources:     w.buildSources(req.Distros, req.Releases, nil, req.Backports),
		Merge:       req.Merge,
		HasUpstream: effectiveUpstreamRelease != "" || result.Upstream != "",
	}, nil
}

func (w *PackagesClientWorkflow) effectiveUpstreamRelease(ctx context.Context, requested, constraints string, backports []string) string {
	if w == nil || w.application == nil || w.application.GetConfig() == nil {
		return ""
	}
	cfg := w.application.GetConfig()
	if cfg.Packages.Upstream == nil {
		return ""
	}
	if requested == "" && constraints == "" && len(backports) == 1 && backports[0] != "none" {
		requested = backports[0]
	}

	provider, err := w.application.BuildUpstreamProvider()
	if err == nil && provider != nil {
		switch {
		case requested != "":
			if release, rErr := provider.ResolveRelease(ctx, requested); rErr == nil {
				return release
			}
		case constraints != "":
			if release, rErr := provider.ResolveRelease(ctx, constraints); rErr == nil {
				return release
			}
		}
	}

	if requested != "" {
		return requested
	}
	if constraints != "" {
		return constraints
	}
	return ""
}

func hasUpstreamResults(results []dto.PackageDiffResult) bool {
	for _, result := range results {
		if result.Upstream != "" {
			return true
		}
	}
	return false
}

// Detail returns full package metadata.
func (w *PackagesClientWorkflow) Detail(ctx context.Context, req PackagesDetailRequest) (*distro.SourcePackageInfo, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.PackagesDetail(ctx, req.Package, client.PackagesDetailOptions{
		Version:   req.Version,
		Distros:   req.Distros,
		Releases:  req.Releases,
		Suites:    req.Suites,
		Backports: req.Backports,
	})
}

// List lists source packages for the requested distro filters.
func (w *PackagesClientWorkflow) List(ctx context.Context, req PackagesListRequest) ([]distro.SourcePackage, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.PackagesList(ctx, client.PackagesListOptions{
		Distros:    req.Distros,
		Releases:   req.Releases,
		Suites:     req.Suites,
		Components: req.Components,
		Backports:  req.Backports,
	})
}

// Dsc resolves .dsc URLs for source package/version pairs.
func (w *PackagesClientWorkflow) Dsc(ctx context.Context, req PackagesDscRequest) ([]dto.PackageDscResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.PackagesDsc(ctx, client.PackagesDscOptions{
		Packages:  req.Packages,
		Distros:   req.Distros,
		Releases:  req.Releases,
		Backports: req.Backports,
	})
}

// Rdepends finds source packages that build-depend on the given package.
func (w *PackagesClientWorkflow) Rdepends(ctx context.Context, req PackagesRdependsRequest) ([]distro.SourcePackageDetail, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.PackagesRdepends(ctx, req.Package, client.PackagesRdependsOptions{
		Distros:   req.Distros,
		Releases:  req.Releases,
		Suites:    req.Suites,
		Backports: req.Backports,
	})
}

// ExcusesList lists package migration excuses.
func (w *PackagesClientWorkflow) ExcusesList(ctx context.Context, req PackagesExcusesListRequest) ([]dto.PackageExcuseSummary, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.PackagesExcusesList(ctx, client.PackagesExcusesListOptions{
		Trackers:     req.Trackers,
		Name:         req.Name,
		Component:    req.Component,
		Team:         req.Team,
		FTBFS:        req.FTBFS,
		Autopkgtest:  req.Autopkgtest,
		BlockedBy:    req.BlockedBy,
		Bugged:       req.Bugged,
		MinAge:       req.MinAge,
		MaxAge:       req.MaxAge,
		Limit:        req.Limit,
		Reverse:      req.Reverse,
		Set:          req.Set,
		BlockedBySet: req.BlockedBySet,
	})
}

// ExcusesShow returns one package migration excuse.
func (w *PackagesClientWorkflow) ExcusesShow(ctx context.Context, req PackagesExcusesShowRequest) (*dto.PackageExcuse, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.PackagesExcusesShow(ctx, req.Package, client.PackagesExcusesShowOptions{
		Tracker: req.Tracker,
		Version: req.Version,
	})
}

func (w *PackagesClientWorkflow) resolveClient() (*ClientTransport, error) {
	if w.client == nil {
		return nil, errors.New("packages client workflow requires an API client")
	}
	return w.client, nil
}

func (w *PackagesClientWorkflow) buildSources(distros, releases, suites, backports []string) []dto.PackageSource {
	if w.application == nil || w.application.GetConfig() == nil {
		return nil
	}
	return w.application.BuildPackageSources(distros, releases, suites, backports)
}
