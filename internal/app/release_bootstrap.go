// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/charmhub"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/releasecache"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/snapstore"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ReleaseCache returns a lazy-initialized release publication cache singleton.
func (a *App) ReleaseCache() (*releasecache.Cache, error) {
	a.releaseCacheOnce.Do(func() {
		path, err := cacheSubdir("releases")
		if err != nil {
			a.releaseCacheErr = err
			return
		}
		a.releaseCache, a.releaseCacheErr = releasecache.NewCache(path)
	})
	return a.releaseCache, a.releaseCacheErr
}

// TrackedReleases returns the discovered published snap/charm artifacts to track.
func (a *App) TrackedReleases(ctx context.Context) ([]dto.TrackedPublication, error) {
	publications, _, err := a.discoverTrackedReleases(ctx)
	return publications, err
}

func (a *App) discoverTrackedReleases(ctx context.Context) ([]dto.TrackedPublication, []string, error) {
	cfg := a.GetConfig()
	if a == nil || cfg == nil {
		return nil, nil, nil
	}
	cache, err := a.GitCache()
	if err != nil {
		return nil, nil, err
	}

	byKey := make(map[string]dto.TrackedPublication)
	var warnings []string
	for _, project := range cfg.Projects {
		artifactType, err := dto.ParseArtifactType(project.ArtifactType)
		if err != nil || (artifactType != dto.ArtifactSnap && artifactType != dto.ArtifactCharm) {
			warnings = append(warnings, fmt.Sprintf("%s: skipped (artifact_type must be snap or charm)", project.Name))
			continue
		}
		cloneURL, err := project.Code.CloneURL()
		if err != nil {
			return nil, nil, fmt.Errorf("project %s: resolving clone URL: %w", project.Name, err)
		}
		repoPath, err := cache.EnsureRepo(ctx, cloneURL, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("project %s: caching repo: %w", project.Name, err)
		}

		baseTracks, branches, err := resolveReleaseTracking(cfg, project)
		if err != nil {
			return nil, nil, fmt.Errorf("project %s: %w", project.Name, err)
		}
		if len(baseTracks) == 0 && len(branches) == 0 {
			warnings = append(warnings, fmt.Sprintf("%s: skipped (no series, release.tracks, or release.branches configured)", project.Name))
			continue
		}

		switch artifactType {
		case dto.ArtifactSnap:
			snapName, err := discoverSnapName(repoPath)
			if err != nil {
				return nil, nil, fmt.Errorf("project %s: %w", project.Name, err)
			}
			if shouldSkipReleaseArtifact(project, snapName) {
				warnings = append(warnings, fmt.Sprintf("%s: skipped artifact %s (release.skip_artifacts)", project.Name, snapName))
				continue
			}
			key := publicationKey(project.Name, artifactType, snapName)
			byKey[key] = dto.TrackedPublication{
				Project:      project.Name,
				Name:         snapName,
				ArtifactType: artifactType,
				Tracks:       append([]string(nil), baseTracks...),
				Branches:     append([]dto.TrackedReleaseBranch(nil), branches...),
			}
		case dto.ArtifactCharm:
			artifacts, err := discoverCharmPublications(repoPath)
			if err != nil {
				return nil, nil, fmt.Errorf("project %s: %w", project.Name, err)
			}
			for _, artifact := range artifacts {
				if shouldSkipReleaseArtifact(project, artifact.Name) {
					warnings = append(warnings, fmt.Sprintf("%s: skipped artifact %s (release.skip_artifacts)", project.Name, artifact.Name))
					continue
				}
				key := publicationKey(project.Name, artifactType, artifact.Name)
				byKey[key] = dto.TrackedPublication{
					Project:      project.Name,
					Name:         artifact.Name,
					ArtifactType: artifactType,
					Tracks:       append([]string(nil), baseTracks...),
					Resources:    append([]string(nil), artifact.Resources...),
					Branches:     append([]dto.TrackedReleaseBranch(nil), branches...),
				}
			}
		}
	}

	publications := make([]dto.TrackedPublication, 0, len(byKey))
	for _, publication := range byKey {
		publications = append(publications, publication)
	}
	sort.Slice(publications, func(i, j int) bool {
		if publications[i].Project == publications[j].Project {
			if publications[i].ArtifactType == publications[j].ArtifactType {
				return publications[i].Name < publications[j].Name
			}
			return publications[i].ArtifactType.String() < publications[j].ArtifactType.String()
		}
		return publications[i].Project < publications[j].Project
	})
	return publications, warnings, nil
}

// BuildReleaseSources creates the release sources supported by the current process.
func (a *App) BuildReleaseSources() map[dto.ArtifactType]port.ReleaseSource {
	return map[dto.ArtifactType]port.ReleaseSource{
		dto.ArtifactSnap:  snapstore.NewSource(a.upstreamHTTPClient("snapstore", 30*time.Second)),
		dto.ArtifactCharm: charmhub.NewSource(a.upstreamHTTPClient("charmhub", 30*time.Second)),
	}
}

// ReleaseDiscoveryResult captures the tracked artifacts selected for release sync.
type ReleaseDiscoveryResult struct {
	Publications []dto.TrackedPublication
	Warnings     []string
}

// DiscoverTrackedReleases resolves the tracked release inventory and skip reasons.
func (a *App) DiscoverTrackedReleases(ctx context.Context) (*ReleaseDiscoveryResult, error) {
	publications, warnings, err := a.discoverTrackedReleases(ctx)
	if err != nil {
		return nil, err
	}
	return &ReleaseDiscoveryResult{Publications: publications, Warnings: warnings}, nil
}
