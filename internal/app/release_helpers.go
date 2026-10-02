// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/gitcache"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"gopkg.in/yaml.v3"
)

type charmcraftMetadata struct {
	Name      string              `yaml:"name"`
	Resources map[string]struct{} `yaml:"resources"`
}

type snapcraftMetadata struct {
	Name string `yaml:"name"`
}

type discoveredCharm struct {
	Name      string
	Resources []string
}

func resolveReleaseTracking(cfg *config.Config, project config.ProjectConfig) ([]string, []dto.TrackedReleaseBranch, error) {
	var tracks []string
	if project.Release != nil && len(project.Release.Tracks) > 0 {
		tracks = append([]string(nil), project.Release.Tracks...)
	} else {
		tracks = effectiveProjectSeries(cfg, project)
		if project.Release != nil && len(project.Release.TrackMap) > 0 {
			for idx, series := range tracks {
				if mapped, ok := project.Release.TrackMap[series]; ok {
					tracks[idx] = mapped
				}
			}
		}
	}
	if err := dto.ValidateReleaseTracks(tracks); err != nil {
		return nil, nil, err
	}
	tracks = dedupeStrings(tracks)

	var branches []dto.TrackedReleaseBranch
	if project.Release != nil {
		for _, branch := range project.Release.Branches {
			track := branch.Track
			if track == "" {
				track = branch.Series
				if mapped, ok := project.Release.TrackMap[track]; ok {
					track = mapped
				}
			}
			risks := dto.KnownReleaseRisks()
			if len(branch.Risks) > 0 {
				risks = make([]dto.ReleaseRisk, 0, len(branch.Risks))
				for _, raw := range branch.Risks {
					risk, err := dto.ParseReleaseRisk(raw)
					if err != nil {
						return nil, nil, err
					}
					risks = append(risks, risk)
				}
			}
			branches = append(branches, dto.TrackedReleaseBranch{
				Track:  track,
				Branch: branch.Branch,
				Risks:  risks,
			})
		}
	}
	return tracks, branches, nil
}

func effectiveProjectSeries(cfg *config.Config, project config.ProjectConfig) []string {
	if len(project.Series) > 0 {
		return append([]string(nil), project.Series...)
	}
	if cfg != nil && len(cfg.Launchpad.Series) > 0 {
		return append([]string(nil), cfg.Launchpad.Series...)
	}
	return nil
}

func discoverSnapName(repoPath string) (string, error) {
	for _, candidate := range []string{"snap/snapcraft.yaml", "snapcraft.yaml"} {
		content, err := gitcache.ReadHEADFile(repoPath, candidate)
		if err != nil {
			continue
		}
		var metadata snapcraftMetadata
		if err := yaml.Unmarshal(content, &metadata); err != nil {
			return "", fmt.Errorf("parsing %s: %w", candidate, err)
		}
		if metadata.Name == "" {
			return "", fmt.Errorf("%s does not declare a snap name", candidate)
		}
		return metadata.Name, nil
	}
	return "", fmt.Errorf("no snapcraft.yaml found at repo root or snap/")
}

func discoverCharmPublications(repoPath string) ([]discoveredCharm, error) {
	rootContent, err := gitcache.ReadHEADFile(repoPath, "charmcraft.yaml")
	if err == nil {
		charm, err := parseCharmcraft(rootContent, "charmcraft.yaml")
		if err != nil {
			return nil, err
		}
		return []discoveredCharm{charm}, nil
	}

	files, err := gitcache.FindHEADFilesByBaseName(repoPath, "charmcraft.yaml")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no charmcraft.yaml found in repository")
	}

	result := make([]discoveredCharm, 0, len(files))
	for _, file := range files {
		charm, err := parseCharmcraft(file.Content, file.Path)
		if err != nil {
			return nil, err
		}
		result = append(result, charm)
	}
	return result, nil
}

func parseCharmcraft(content []byte, source string) (discoveredCharm, error) {
	var metadata charmcraftMetadata
	if err := yaml.Unmarshal(content, &metadata); err != nil {
		return discoveredCharm{}, fmt.Errorf("parsing %s: %w", source, err)
	}
	if metadata.Name == "" {
		return discoveredCharm{}, fmt.Errorf("%s does not declare a charm name", source)
	}
	resources := make([]string, 0, len(metadata.Resources))
	for name := range metadata.Resources {
		resources = append(resources, name)
	}
	sort.Strings(resources)
	return discoveredCharm{Name: metadata.Name, Resources: resources}, nil
}

func publicationKey(project string, artifactType dto.ArtifactType, name string) string {
	return project + "\x00" + artifactType.String() + "\x00" + name
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func shouldSkipReleaseArtifact(project config.ProjectConfig, name string) bool {
	if project.Release == nil || len(project.Release.SkipArtifacts) == 0 {
		return false
	}
	for _, skipped := range project.Release.SkipArtifacts {
		if skipped == name {
			return true
		}
	}
	return false
}

func stringSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result[value] = true
	}
	return result
}
