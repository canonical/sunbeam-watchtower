// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"os"
	"path/filepath"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"

	"gopkg.in/yaml.v3"
)

// SnapStrategy implements ArtifactStrategy for snap artifacts.
type SnapStrategy struct{}

func (s *SnapStrategy) ArtifactType() dto.ArtifactType { return dto.ArtifactSnap }
func (s *SnapStrategy) MetadataFileName() string       { return "snapcraft.yaml" }
func (s *SnapStrategy) BuildPath(name string) string   { return "" }

// DiscoverRecipes looks for snapcraft.yaml at the repo root or snap/
// subdirectory. Snaps are single-artifact repos, so RelPath is always empty
// (Launchpad picks up the metadata from the repo root). The declared name
// from the manifest wins; we fall back to the repo directory name when the
// manifest omits one.
func (s *SnapStrategy) DiscoverRecipes(repoPath string) ([]DiscoveredRecipe, error) {
	candidates := []string{
		filepath.Join(repoPath, "snap", s.MetadataFileName()),
		filepath.Join(repoPath, s.MetadataFileName()),
	}
	for _, metaPath := range candidates {
		if _, err := os.Stat(metaPath); err != nil {
			continue
		}
		name, err := manifestName(metaPath)
		if err != nil {
			return nil, err
		}
		if name == "" {
			name = filepath.Base(repoPath)
		}
		return []DiscoveredRecipe{{Name: name}}, nil
	}
	return nil, nil
}

func (s *SnapStrategy) TempRecipeName(name, sha, prefix string) string {
	short := sha
	if len(sha) > 8 {
		short = sha[:8]
	}
	return prefix + "-" + short + "-" + name
}

func (s *SnapStrategy) OfficialRecipeName(artifactName, series, devFocus string) string {
	if series == devFocus {
		return artifactName
	}
	return artifactName + "-" + series
}

func (s *SnapStrategy) BranchForSeries(series, devFocus, defaultBranch string) string {
	if series == devFocus {
		return defaultBranch
	}
	return "stable/" + series
}

type snapcraftYAML struct {
	Architectures []snapArchitecture            `yaml:"architectures"`
	Platforms     map[string]*snapPlatformEntry `yaml:"platforms"`
}

type snapArchitecture struct {
	BuildOn  yaml.Node `yaml:"build-on"`
	BuildFor yaml.Node `yaml:"build-for"`
}

type snapPlatformEntry struct {
	BuildOn  yaml.Node `yaml:"build-on"`
	BuildFor yaml.Node `yaml:"build-for"`
}

func (s *SnapStrategy) ParsePlatforms(content []byte) ([]string, error) {
	var sc snapcraftYAML
	if err := yaml.Unmarshal(content, &sc); err != nil {
		return nil, err
	}

	// New syntax: platforms
	if len(sc.Platforms) > 0 {
		seen := make(map[string]struct{})
		for key, val := range sc.Platforms {
			if val == nil || (val.BuildFor.Kind == 0 && val.BuildOn.Kind == 0) {
				seen[key] = struct{}{}
				continue
			}
			if val.BuildFor.Kind != 0 {
				archs, err := decodeStringOrSlice(&val.BuildFor)
				if err != nil {
					return nil, err
				}
				for _, a := range archs {
					seen[a] = struct{}{}
				}
			}
		}
		result := make([]string, 0, len(seen))
		for k := range seen {
			result = append(result, k)
		}
		return result, nil
	}

	// Old syntax: architectures
	if len(sc.Architectures) > 0 {
		seen := make(map[string]struct{})
		for _, arch := range sc.Architectures {
			if arch.BuildFor.Kind != 0 {
				archs, err := decodeStringOrSlice(&arch.BuildFor)
				if err != nil {
					return nil, err
				}
				for _, a := range archs {
					seen[a] = struct{}{}
				}
			} else if arch.BuildOn.Kind != 0 {
				archs, err := decodeStringOrSlice(&arch.BuildOn)
				if err != nil {
					return nil, err
				}
				for _, a := range archs {
					seen[a] = struct{}{}
				}
			}
		}
		if len(seen) > 0 {
			result := make([]string, 0, len(seen))
			for k := range seen {
				result = append(result, k)
			}
			return result, nil
		}
	}

	return []string{"amd64"}, nil
}
