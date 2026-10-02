// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/bugcache"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/openstack"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/commit"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// UpstreamCacheDir returns the path to the upstream repos cache directory.
func UpstreamCacheDir() (string, error) {
	cacheDir, err := ResolveCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "upstream"), nil
}

// UpstreamRepoPath returns the local path for a given upstream repo URL.
func UpstreamRepoPath(cacheDir, repoURL string) string {
	name := filepath.Base(repoURL)
	name = strings.TrimSuffix(name, ".git")
	return filepath.Join(cacheDir, name)
}

// BuildUpstreamProvider creates an UpstreamProvider from config, or returns nil
// if upstream is not configured.
func (a *App) BuildUpstreamProvider() (port.UpstreamProvider, error) {
	cfg := a.GetConfig()
	if cfg == nil || cfg.Packages.Upstream == nil {
		return nil, nil
	}

	up := cfg.Packages.Upstream
	if up.Provider != "openstack" {
		return nil, fmt.Errorf("unsupported upstream provider %q", up.Provider)
	}

	upDir, err := UpstreamCacheDir()
	if err != nil {
		return nil, err
	}

	releasesDir := UpstreamRepoPath(upDir, up.ReleasesRepo)
	requirementsDir := UpstreamRepoPath(upDir, up.RequirementsRepo)

	return openstack.NewProvider(releasesDir, requirementsDir), nil
}

// BuildCommitSources creates commit sources backed by the local git cache.
func (a *App) BuildCommitSources() (map[string]port.CommitSource, error) {
	cfg := a.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("no configuration loaded")
	}

	a.Logger.Debug("building commit sources", "project_count", len(cfg.Projects))

	cache, err := a.GitCache()
	if err != nil {
		return nil, err
	}

	result := make(map[string]port.CommitSource, len(cfg.Projects))
	for _, proj := range cfg.Projects {
		cloneURL, err := proj.Code.CloneURL()
		if err != nil {
			return nil, fmt.Errorf("project %s: %w", proj.Name, err)
		}

		a.Logger.Debug("configured commit source", "project", proj.Name, "cloneURL", cloneURL)

		forgeType := ForgeTypeFromConfig(proj.Code.Forge)
		result[proj.Name] = &commit.CachedGitSource{
			Cache:     cache,
			CloneURL:  cloneURL,
			Type:      forgeType,
			CommitURL: proj.Code.CommitURL,
		}
	}

	return result, nil
}

// SyncBugCache syncs the bug cache for configured projects.
// If projects is empty, all configured projects are synced.
func (a *App) SyncBugCache(ctx context.Context, projects []string) (int, []string, error) {
	trackers, bindings, err := a.BuildBugTrackers()
	if err != nil {
		return 0, nil, err
	}

	selected := stringSet(projects)
	total := 0
	var rebuiltProjects []string
	for key, pbt := range trackers {
		if len(selected) > 0 {
			matched := selected[pbt.ProjectID]
			for _, binding := range bindings[key] {
				matched = matched || selected[binding.ProjectName]
			}
			if !matched {
				continue
			}
		}
		ct, ok := pbt.Tracker.(*bugcache.CachedBugTracker)
		if !ok {
			continue
		}
		needsRebuild, rebuildErr := ct.NeedsRebuild(ctx)
		if rebuildErr != nil {
			return total, rebuiltProjects, fmt.Errorf("checking %s cache compatibility: %w", pbt.ProjectID, rebuildErr)
		}
		synced, sErr := ct.Sync(ctx)
		if sErr != nil {
			return total, rebuiltProjects, fmt.Errorf("syncing %s: %w", pbt.ProjectID, sErr)
		}
		if needsRebuild {
			rebuiltProjects = append(rebuiltProjects, pbt.ProjectID)
		}
		total += synced
	}

	sort.Strings(rebuiltProjects)
	return total, rebuiltProjects, nil
}

// BuildPackageSources resolves distro, release, suite, and backport filters against config
// to produce source entries for the package cache.
//
// Suite types are expanded relative to each release: "release" -> release name,
// "updates" -> "<release>-updates", etc.
//
// Backport filter semantics:
//   - empty/nil or ["none"]: skip all backports (default)
//   - ["gazpacho", "flamingo"]: include only those backports
func (a *App) BuildPackageSources(distros, releases, suites, backports []string) []dto.PackageSource {
	return buildPackageSources(a.GetConfig().Packages, distros, releases, suites, backports, a.Logger)
}

// BuildPackageCacheSyncSources resolves package cache sync filters against config.
// Unlike query paths, explicit cache sync filters are validated and return
// actionable errors instead of silently selecting no sources.
func (a *App) BuildPackageCacheSyncSources(distros, releases, suites, backports []string) ([]dto.PackageSource, error) {
	return buildPackageCacheSyncSources(a.GetConfig().Packages, distros, releases, suites, backports, a.Logger)
}
