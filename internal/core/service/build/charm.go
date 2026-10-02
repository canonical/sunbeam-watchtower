// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package build

import (
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"

	"gopkg.in/yaml.v3"
)

// CharmStrategy implements ArtifactStrategy for charm artifacts.
type CharmStrategy struct{}

func (s *CharmStrategy) ArtifactType() dto.ArtifactType { return dto.ArtifactCharm }
func (s *CharmStrategy) MetadataFileName() string       { return "charmcraft.yaml" }
func (s *CharmStrategy) BuildPath(name string) string   { return "charms/" + name }

// DiscoverRecipes walks the repo for charmcraft.yaml files and returns one
// DiscoveredRecipe per match. A single-charm repo (charmcraft.yaml at root)
// is reported with an empty RelPath. Monorepo layouts of any depth (for
// example charms/storage/foo/charmcraft.yaml) are preserved verbatim so
// Launchpad's build path points at the actual recipe directory.
func (s *CharmStrategy) DiscoverRecipes(repoPath string) ([]DiscoveredRecipe, error) {
	return walkRecipes(repoPath, "charms", s.MetadataFileName())
}

func (s *CharmStrategy) TempRecipeName(name, sha, prefix string) string {
	short := sha
	if len(sha) > 8 {
		short = sha[:8]
	}
	return prefix + "-" + short + "-" + name
}

func (s *CharmStrategy) OfficialRecipeName(artifactName, series, devFocus string) string {
	if series == devFocus {
		return artifactName
	}
	return artifactName + "-" + series
}

func (s *CharmStrategy) BranchForSeries(series, devFocus, defaultBranch string) string {
	if series == devFocus {
		return defaultBranch
	}
	return "stable/" + series
}

type charmcraftBase struct {
	Name          string   `yaml:"name"`
	Channel       string   `yaml:"channel"`
	Architectures []string `yaml:"architectures"`
}

type charmcraftBaseEntry struct {
	BuildOn []charmcraftBase `yaml:"build-on"`
	RunOn   []charmcraftBase `yaml:"run-on"`
}

type charmcraftYAML struct {
	Platforms map[string]*rockcraftPlatformEntry `yaml:"platforms"`
	Bases     []charmcraftBaseEntry              `yaml:"bases"`
}

func (s *CharmStrategy) ParsePlatforms(content []byte) ([]string, error) {
	var cc charmcraftYAML
	if err := yaml.Unmarshal(content, &cc); err != nil {
		return nil, err
	}

	// New syntax: platforms (same as rocks)
	if len(cc.Platforms) > 0 {
		return parsePlatformsMap(cc.Platforms)
	}

	// Old syntax: bases
	if len(cc.Bases) > 0 {
		return parseCharmBases(cc.Bases)
	}

	return []string{"amd64"}, nil
}

func parsePlatformsMap(platforms map[string]*rockcraftPlatformEntry) ([]string, error) {
	seen := make(map[string]struct{})
	for key, val := range platforms {
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

func parseCharmBases(bases []charmcraftBaseEntry) ([]string, error) {
	seen := make(map[string]struct{})
	for _, entry := range bases {
		sources := entry.RunOn
		if len(sources) == 0 {
			sources = entry.BuildOn
		}
		for _, base := range sources {
			for _, arch := range base.Architectures {
				seen[arch] = struct{}{}
			}
		}
	}
	if len(seen) == 0 {
		return []string{"amd64"}, nil
	}
	result := make([]string, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	return result, nil
}
