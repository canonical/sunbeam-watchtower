// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/gboutry/sunbeam-watchtower/internal/app"
	pkg "github.com/gboutry/sunbeam-watchtower/internal/core/service/package"
	distro "github.com/gboutry/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
)

// --- Request / Response types ------------------------------------------------

// PackagesDiffInput holds parameters for the diff endpoint.
type PackagesDiffInput struct {
	Set             string   `path:"set" doc:"Package set name" example:"openstack"`
	Distros         []string `query:"distro" required:"false" doc:"Filter by distro name"`
	Releases        []string `query:"release" required:"false" doc:"Filter by distro release"`
	Suites          []string `query:"suite" required:"false" doc:"Filter by suite type (release, updates, proposed)"`
	Backports       []string `query:"backport" required:"false" doc:"Backports to include (default: none)"`
	Merge           bool     `query:"merge" required:"false" doc:"Merge suites per source into highest version"`
	UpstreamRelease string   `query:"upstream_release" doc:"Upstream release to compare against" example:"2025.1"`
	BehindUpstream  bool     `query:"behind_upstream" required:"false" doc:"Show only packages behind upstream"`
	OnlyIn          string   `query:"only_in" doc:"Show only packages present in the named source"`
	Constraints     string   `query:"constraints" doc:"Include upper-constraints packages for the given release"`
}

// PackagesDiffOutput is the response for the diff endpoint.
type PackagesDiffOutput struct {
	Body []dto.PackageDiffResult `doc:"List of package diff results"`
}

// PackagesShowInput holds parameters for the show endpoint.
type PackagesShowInput struct {
	Name            string   `path:"name" doc:"Source package name" example:"nova"`
	Distros         []string `query:"distro" required:"false" doc:"Filter by distro name"`
	Releases        []string `query:"release" required:"false" doc:"Filter by distro release"`
	Suites          []string `query:"suite" required:"false" doc:"Filter by suite type"`
	Backports       []string `query:"backport" required:"false" doc:"Backports to include (default: none)"`
	Merge           bool     `query:"merge" required:"false" doc:"Merge suites per source into highest version"`
	UpstreamRelease string   `query:"upstream_release" doc:"Upstream release to compare against" example:"2025.1"`
}

// PackagesShowOutput is the response for the show endpoint.
type PackagesShowOutput struct {
	Body *dto.PackageDiffResult `doc:"Package version information across sources"`
}

// PackagesDetailInput holds parameters for the detail endpoint.
type PackagesDetailInput struct {
	Name      string   `path:"name" doc:"Source package name" example:"nova"`
	Version   string   `query:"version" required:"false" doc:"Exact Debian version string. If omitted, returns the highest version found."`
	Distros   []string `query:"distro" required:"false" doc:"Filter by distro name"`
	Releases  []string `query:"release" required:"false" doc:"Filter by distro release"`
	Suites    []string `query:"suite" required:"false" doc:"Filter by suite type"`
	Backports []string `query:"backport" required:"false" doc:"Backports to include (default: none)"`
}

// PackagesDetailOutput is the response for the detail endpoint.
type PackagesDetailOutput struct {
	Body *distro.SourcePackageInfo `doc:"Full APT source package metadata"`
}

// PackagesListInput holds parameters for the list endpoint.
type PackagesListInput struct {
	Distros    []string `query:"distro" required:"true" doc:"Distro(s) to list packages from"`
	Releases   []string `query:"release" required:"false" doc:"Filter by distro release"`
	Suites     []string `query:"suite" required:"false" doc:"Filter by suite type"`
	Components []string `query:"component" required:"false" doc:"Filter by component"`
	Backports  []string `query:"backport" required:"false" doc:"Backports to include (default: none)"`
}

// PackagesListOutput is the response for the list endpoint.
type PackagesListOutput struct {
	Body []distro.SourcePackage `doc:"List of source packages"`
}

// PackagesRdependsInput holds parameters for the rdepends endpoint.
type PackagesRdependsInput struct {
	Name      string   `path:"name" doc:"Source package name to find reverse build-dependencies for" example:"oslo.config"`
	Distros   []string `query:"distro" required:"false" doc:"Filter by distro name"`
	Releases  []string `query:"release" required:"false" doc:"Filter by distro release"`
	Suites    []string `query:"suite" required:"false" doc:"Filter by suite type"`
	Backports []string `query:"backport" required:"false" doc:"Backports to include (default: none)"`
}

// PackagesRdependsOutput is the response for the rdepends endpoint.
type PackagesRdependsOutput struct {
	Body []distro.SourcePackageDetail `doc:"Source packages that build-depend on the queried package"`
}

// PackagesDscInput holds parameters for the dsc endpoint.
type PackagesDscInput struct {
	Packages  []string `query:"packages" required:"true" doc:"Package/version pairs in name=version format" example:"nova=2025.1-0ubuntu1"`
	Distros   []string `query:"distro" required:"false" doc:"Filter by distro name"`
	Releases  []string `query:"release" required:"false" doc:"Filter by distro release"`
	Backports []string `query:"backport" required:"false" doc:"Backports to include (default: none)"`
}

// PackagesDscOutput is the response for the dsc endpoint.
type PackagesDscOutput struct {
	Body []dto.PackageDscResult `doc:"DSC URL lookup results"`
}

// PackagesCacheStatusOutput is the response for the cache status endpoint.
type PackagesCacheStatusOutput struct {
	Body []dto.CacheStatus `doc:"Cache status for each indexed source group"`
}

// PackagesCacheSyncInput holds the request body for the cache sync endpoint.
type PackagesCacheSyncInput struct {
	Body struct {
		Distros   []string `json:"distros,omitempty" required:"false" doc:"Distros to sync (default: all configured)"`
		Releases  []string `json:"releases,omitempty" required:"false" doc:"Releases to sync (default: all configured)"`
		Backports []string `json:"backports,omitempty" required:"false" doc:"Backports to sync (default: all configured)"`
	}
}

// PackagesCacheSyncOutput is the response for the cache sync endpoint.
type PackagesCacheSyncOutput struct {
	Body struct {
		Status string `json:"status" example:"ok" doc:"Sync result status"`
	}
}

// PackageSetsCacheSyncInput selects configured Launchpad sets to refresh.
type PackageSetsCacheSyncInput struct {
	Body struct {
		Sets []string `json:"sets,omitempty" required:"false"`
	}
}

type PackageSetsCacheSyncOutput struct {
	Body []dto.PackageSetCacheStatus
}

// --- Route registration ------------------------------------------------------

// RegisterPackagesAPI registers all package-related endpoints on the given huma API.
func RegisterPackagesAPI(api huma.API, application *app.App) {
	// GET /api/v1/packages/diff/{set}
	huma.Register(api, huma.Operation{
		OperationID: "packages-diff",
		Method:      http.MethodGet,
		Path:        "/api/v1/packages/diff/{set}",
		Summary:     "Compare package versions across distros",
		Tags:        []string{"packages"},
	}, func(ctx context.Context, input *PackagesDiffInput) (*PackagesDiffOutput, error) {
		setName := input.Set
		packages, err := application.PackageSet(setName)
		if err != nil {
			return nil, packageSetHTTPError(err)
		}

		// When --only-in names a backport source, auto-scope distro and backport filters.
		distros := input.Distros
		backports := input.Backports
		if input.OnlyIn != "" {
			if parts := strings.SplitN(input.OnlyIn, "/", 2); len(parts) == 2 {
				if len(distros) == 0 {
					distros = []string{parts[0]}
				}
				bpName := parts[1]
				found := false
				for _, bp := range backports {
					if bp == bpName {
						found = true
						break
					}
				}
				if !found {
					if len(backports) == 1 && backports[0] == "none" {
						backports = []string{bpName}
					} else {
						backports = append(backports, bpName)
					}
				}
			}
		}

		if len(backports) == 0 {
			backports = []string{"none"}
		}

		cache, err := application.DistroCache()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to open distro cache: %v", err))
		}

		sources := application.BuildPackageSources(distros, input.Releases, input.Suites, backports)
		if len(sources) == 0 {
			return nil, huma.Error400BadRequest("no distros configured or matched filters")
		}

		svc := pkg.NewService(cache, application.Logger)
		results, err := svc.Diff(ctx, pkg.DiffOpts{
			Packages: packages,
			Sources:  sources,
		})
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("diff failed: %v", err))
		}

		// --constraints: merge upper-constraints packages into the diff.
		if input.Constraints != "" {
			provider, pErr := application.BuildUpstreamProvider()
			if pErr != nil {
				return nil, huma.Error500InternalServerError(fmt.Sprintf("upstream provider error: %v", pErr))
			}
			if provider == nil {
				return nil, huma.Error400BadRequest("constraints requires upstream provider configuration")
			}
			constraintMap, cErr := provider.GetConstraints(ctx, input.Constraints)
			if cErr != nil {
				return nil, huma.Error500InternalServerError(
					fmt.Sprintf("fetching constraints for %s", input.Constraints), cErr)
			}

			existing := make(map[string]bool, len(results))
			for _, r := range results {
				existing[r.Package] = true
			}
			for pypiName, ver := range constraintMap {
				pkgName := provider.MapPackageName(pypiName, dto.DeliverableOther)
				if existing[pkgName] {
					continue
				}
				extra, qErr := svc.Show(ctx, pkgName, sources)
				if qErr != nil {
					continue
				}
				extra.Upstream = ver
				results = append(results, *extra)
				existing[pkgName] = true
			}
			sort.Slice(results, func(i, j int) bool {
				return results[i].Package < results[j].Package
			})
		}

		// Annotate upstream versions when requested.
		effectiveRelease := effectivePackagesUpstreamRelease(ctx, application, input.UpstreamRelease, input.Constraints, backports)
		if effectiveRelease != "" || hasPackagesUpstreamProvider(application) {
			_ = annotateUpstreamResults(ctx, application, results, effectiveRelease)
		}

		// --behind-upstream: keep only packages where distro < upstream.
		if input.BehindUpstream {
			results = filterBehindUpstreamResults(results, sources, input.Merge)
		}

		// --only-in: keep only packages present in the named source.
		if input.OnlyIn != "" {
			var filtered []dto.PackageDiffResult
			for _, r := range results {
				if len(r.Versions[input.OnlyIn]) > 0 {
					filtered = append(filtered, r)
				}
			}
			results = filtered
		}

		return &PackagesDiffOutput{Body: results}, nil
	})

	// GET /api/v1/packages/show/{name}
	huma.Register(api, huma.Operation{
		OperationID: "packages-show",
		Method:      http.MethodGet,
		Path:        "/api/v1/packages/show/{name}",
		Summary:     "Show all versions of a package across distros",
		Tags:        []string{"packages"},
	}, func(ctx context.Context, input *PackagesShowInput) (*PackagesShowOutput, error) {
		backports := input.Backports
		if len(backports) == 0 {
			backports = []string{"none"}
		}

		cache, err := application.DistroCache()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to open distro cache: %v", err))
		}

		sources := application.BuildPackageSources(input.Distros, input.Releases, input.Suites, backports)
		svc := pkg.NewService(cache, application.Logger)

		result, err := svc.Show(ctx, input.Name, sources)
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("show failed: %v", err))
		}

		effectiveRelease := effectivePackagesUpstreamRelease(ctx, application, input.UpstreamRelease, "", backports)
		if effectiveRelease != "" || hasPackagesUpstreamProvider(application) {
			results := []dto.PackageDiffResult{*result}
			_ = annotateUpstreamResults(ctx, application, results, effectiveRelease)
			result = &results[0]
		}

		return &PackagesShowOutput{Body: result}, nil
	})

	// GET /api/v1/packages/detail/{name}
	huma.Register(api, huma.Operation{
		OperationID: "packages-detail",
		Method:      http.MethodGet,
		Path:        "/api/v1/packages/detail/{name}",
		Summary:     "Show full APT metadata for a package",
		Description: "Returns all fields from the APT Sources paragraph for a specific package version. If version is omitted, returns the highest version found.",
		Tags:        []string{"packages"},
	}, func(ctx context.Context, input *PackagesDetailInput) (*PackagesDetailOutput, error) {
		backports := input.Backports
		if len(backports) == 0 {
			if input.Version != "" {
				// Exact version: search all sources including backports.
				backports = nil
			} else {
				backports = []string{"none"}
			}
		}

		cache, err := application.DistroCache()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to open distro cache: %v", err))
		}

		sources := application.BuildPackageSources(input.Distros, input.Releases, input.Suites, backports)
		svc := pkg.NewService(cache, application.Logger)

		result, err := svc.ShowDetail(ctx, input.Name, input.Version, sources)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}

		return &PackagesDetailOutput{Body: result}, nil
	})

	// GET /api/v1/packages/list
	huma.Register(api, huma.Operation{
		OperationID: "packages-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/packages/list",
		Summary:     "List packages in a distro",
		Tags:        []string{"packages"},
	}, func(ctx context.Context, input *PackagesListInput) (*PackagesListOutput, error) {
		backports := input.Backports
		if len(backports) == 0 {
			backports = []string{"none"}
		}

		cache, err := application.DistroCache()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to open distro cache: %v", err))
		}

		sources := application.BuildPackageSources(input.Distros, input.Releases, input.Suites, backports)
		if len(sources) == 0 {
			return nil, huma.Error400BadRequest("no distros configured or matched filters")
		}

		svc := pkg.NewService(cache, application.Logger)
		var allPkgs []distro.SourcePackage
		for _, src := range sources {
			srcSuites := make([]string, 0, len(src.Entries))
			for _, e := range src.Entries {
				srcSuites = append(srcSuites, e.Suite)
			}
			pkgs, qErr := svc.List(ctx, src.Name, dto.QueryOpts{
				Suites:     srcSuites,
				Components: input.Components,
			})
			if qErr != nil {
				return nil, huma.Error500InternalServerError(fmt.Sprintf("list failed: %v", qErr))
			}
			allPkgs = append(allPkgs, pkgs...)
		}

		return &PackagesListOutput{Body: allPkgs}, nil
	})

	// GET /api/v1/packages/rdepends/{name}
	huma.Register(api, huma.Operation{
		OperationID: "packages-rdepends",
		Method:      http.MethodGet,
		Path:        "/api/v1/packages/rdepends/{name}",
		Summary:     "Find source packages that build-depend on a given package",
		Tags:        []string{"packages"},
	}, func(ctx context.Context, input *PackagesRdependsInput) (*PackagesRdependsOutput, error) {
		backports := input.Backports
		if len(backports) == 0 {
			backports = []string{"none"}
		}

		cache, err := application.DistroCache()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to open distro cache: %v", err))
		}

		sources := application.BuildPackageSources(input.Distros, input.Releases, input.Suites, backports)
		if len(sources) == 0 {
			return nil, huma.Error400BadRequest("no distros configured or matched filters")
		}

		svc := pkg.NewService(cache, application.Logger)

		var results []distro.SourcePackageDetail
		for _, src := range sources {
			suiteSet := make(map[string]bool, len(src.Entries))
			for _, e := range src.Entries {
				suiteSet[e.Suite] = true
			}
			var srcSuites []string
			for s := range suiteSet {
				srcSuites = append(srcSuites, s)
			}
			queryOpts := dto.QueryOpts{Suites: srcSuites}

			srcResults, qErr := svc.ReverseDepends(ctx, input.Name, []dto.PackageSource{src}, queryOpts)
			if qErr != nil {
				return nil, huma.Error500InternalServerError(fmt.Sprintf("rdepends failed: %v", qErr))
			}
			results = append(results, srcResults...)
		}

		return &PackagesRdependsOutput{Body: results}, nil
	})

	// GET /api/v1/packages/dsc
	huma.Register(api, huma.Operation{
		OperationID: "packages-dsc",
		Method:      http.MethodGet,
		Path:        "/api/v1/packages/dsc",
		Summary:     "Look up .dsc file URLs for source package/version pairs",
		Tags:        []string{"packages"},
	}, func(ctx context.Context, input *PackagesDscInput) (*PackagesDscOutput, error) {
		var pairs []pkg.PackageVersionPair
		for _, p := range input.Packages {
			parts := strings.SplitN(p, "=", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return nil, huma.Error400BadRequest(
					fmt.Sprintf("invalid package format %q; expected name=version", p))
			}
			pairs = append(pairs, pkg.PackageVersionPair{
				Package: parts[0],
				Version: parts[1],
			})
		}

		backports := input.Backports
		if len(backports) == 0 {
			backports = []string{"none"}
		}

		cache, err := application.DistroCache()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to open distro cache: %v", err))
		}

		sources := application.BuildPackageSources(input.Distros, input.Releases, nil, backports)
		if len(sources) == 0 {
			return nil, huma.Error400BadRequest("no distros configured or matched filters")
		}

		svc := pkg.NewService(cache, application.Logger)
		results, err := svc.FindDsc(ctx, pairs, sources)
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("dsc lookup failed: %v", err))
		}

		return &PackagesDscOutput{Body: results}, nil
	})

	// GET /api/v1/packages/cache/status
	huma.Register(api, huma.Operation{
		OperationID: "packages-cache-status",
		Method:      http.MethodGet,
		Path:        "/api/v1/packages/cache/status",
		Summary:     "Get package cache status",
		Tags:        []string{"packages"},
	}, func(ctx context.Context, _ *struct{}) (*PackagesCacheStatusOutput, error) {
		cache, err := application.DistroCache()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to open distro cache: %v", err))
		}

		svc := pkg.NewService(cache, application.Logger)
		statuses, err := svc.CacheStatus()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("cache status failed: %v", err))
		}

		return &PackagesCacheStatusOutput{Body: statuses}, nil
	})

	// POST /api/v1/packages/cache/sync
	huma.Register(api, huma.Operation{
		OperationID: "packages-cache-sync",
		Method:      http.MethodPost,
		Path:        "/api/v1/packages/cache/sync",
		Summary:     "Sync the package cache",
		Tags:        []string{"packages"},
	}, func(ctx context.Context, input *PackagesCacheSyncInput) (*PackagesCacheSyncOutput, error) {
		sources, err := application.BuildPackageCacheSyncSources(
			input.Body.Distros,
			input.Body.Releases,
			nil,
			input.Body.Backports,
		)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		cache, err := application.DistroCache()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to open distro cache: %v", err))
		}
		svc := pkg.NewService(cache, application.Logger)
		if err := svc.UpdateCache(ctx, sources); err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("cache sync failed: %v", err))
		}

		out := &PackagesCacheSyncOutput{}
		out.Body.Status = "ok"
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "package-sets-cache-sync",
		Method:      http.MethodPost,
		Path:        "/api/v1/packages/sets/cache/sync",
		Summary:     "Sync configured Launchpad packagesets",
		Tags:        []string{"packages"},
	}, func(ctx context.Context, input *PackageSetsCacheSyncInput) (*PackageSetsCacheSyncOutput, error) {
		snapshots, err := application.SyncPackageSets(ctx, input.Body.Sets)
		if err != nil {
			if errors.Is(err, app.ErrUnknownPackageSet) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError(err.Error())
		}
		out := &PackageSetsCacheSyncOutput{Body: make([]dto.PackageSetCacheStatus, 0, len(snapshots))}
		for _, snapshot := range snapshots {
			out.Body = append(out.Body, dto.PackageSetCacheStatus{
				Name: snapshot.Name, ConfiguredSeries: snapshot.ConfiguredSeries,
				Series: snapshot.Series, PackageCount: len(snapshot.Packages), SyncedAt: snapshot.SyncedAt,
			})
		}
		return out, nil
	})

	registerPackagesExcusesAPI(api, application)
}

func packageSetHTTPError(err error) error {
	switch {
	case errors.Is(err, app.ErrUnknownPackageSet):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, app.ErrPackageSetNotSynced):
		return huma.Error409Conflict(err.Error())
	default:
		return huma.Error500InternalServerError(err.Error())
	}
}

// --- Helpers -----------------------------------------------------------------

// annotateUpstreamResults populates the Upstream field on each DiffResult.
func annotateUpstreamResults(ctx context.Context, application *app.App, results []dto.PackageDiffResult, release string) error {
	provider, err := application.BuildUpstreamProvider()
	if err != nil {
		return err
	}
	if provider == nil {
		return fmt.Errorf("no upstream provider configured")
	}

	deliverables, err := provider.ListDeliverables(ctx, release)
	if err != nil {
		return fmt.Errorf("listing deliverables: %w", err)
	}

	upstreamVersions := make(map[string]string, len(deliverables))
	for _, d := range deliverables {
		pkgName := provider.MapPackageName(d.Name, d.Type)
		if d.Version != "" {
			upstreamVersions[pkgName] = d.Version
		}
	}

	constraints, cErr := provider.GetConstraints(ctx, release)
	if cErr != nil {
		application.Logger.Debug("upstream constraints unavailable", "error", cErr)
	}

	for i := range results {
		if v, ok := upstreamVersions[results[i].Package]; ok {
			results[i].Upstream = v
		} else if constraints != nil {
			name := results[i].Package
			if v, ok := constraints[name]; ok {
				results[i].Upstream = v
			} else {
				for cName, cVer := range constraints {
					mapped := provider.MapPackageName(cName, dto.DeliverableOther)
					if mapped == name {
						results[i].Upstream = cVer
						break
					}
				}
			}
		}
	}
	return nil
}

func effectivePackagesUpstreamRelease(ctx context.Context, application *app.App, requested, constraints string, backports []string) string {
	if application == nil || application.GetConfig() == nil {
		return ""
	}
	cfg := application.GetConfig()
	if cfg.Packages.Upstream == nil {
		return ""
	}
	if requested == "" && constraints == "" && len(backports) == 1 && backports[0] != "none" {
		requested = backports[0]
	}

	provider, err := application.BuildUpstreamProvider()
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

func hasPackagesUpstreamProvider(application *app.App) bool {
	return application != nil &&
		application.GetConfig() != nil &&
		application.GetConfig().Packages.Upstream != nil
}

// filterBehindUpstreamResults keeps only results where distro version < upstream.
func filterBehindUpstreamResults(results []dto.PackageDiffResult, sources []dto.PackageSource, merge bool) []dto.PackageDiffResult {
	var filtered []dto.PackageDiffResult
	for _, r := range results {
		if r.Upstream == "" {
			continue
		}
		// Check if any source version is behind upstream.
		behind := false
		for _, src := range sources {
			versions := r.Versions[src.Name]
			if merge {
				if highest := distro.PickHighest(versions); highest != nil {
					versions = []distro.SourcePackage{*highest}
				}
			}
			for _, sp := range versions {
				if sp.Version != "" && distro.CompareUpstreamVersions(sp.Version, r.Upstream) < 0 {
					behind = true
					break
				}
			}
			if behind {
				break
			}
		}
		if behind {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
