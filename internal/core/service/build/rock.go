// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package build

import (
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"

	"gopkg.in/yaml.v3"
)

// RockStrategy implements ArtifactStrategy for rock artifacts.
type RockStrategy struct{}

func (s *RockStrategy) ArtifactType() dto.ArtifactType { return dto.ArtifactRock }
func (s *RockStrategy) MetadataFileName() string       { return "rockcraft.yaml" }
func (s *RockStrategy) BuildPath(name string) string   { return "rocks/" + name }

// DiscoverRecipes walks repoPath/rocks/** for rockcraft.yaml files and
// reports each match as a DiscoveredRecipe. Nested monorepo layouts are
// preserved so the LP build path points at the real recipe directory.
func (s *RockStrategy) DiscoverRecipes(repoPath string) ([]DiscoveredRecipe, error) {
	return walkRecipes(repoPath, "rocks", s.MetadataFileName())
}

func (s *RockStrategy) TempRecipeName(name, sha, prefix string) string {
	short := sha
	if len(sha) > 8 {
		short = sha[:8]
	}
	return prefix + "-" + short + "-" + name
}

func (s *RockStrategy) OfficialRecipeName(artifactName, series, devFocus string) string {
	if series == devFocus {
		return artifactName
	}
	return artifactName + "-" + series
}

func (s *RockStrategy) BranchForSeries(series, devFocus, defaultBranch string) string {
	if series == devFocus {
		return defaultBranch
	}
	return "stable/" + series
}

type rockcraftPlatformEntry struct {
	BuildOn  yaml.Node `yaml:"build-on"`
	BuildFor yaml.Node `yaml:"build-for"`
}

type rockcraftYAML struct {
	Platforms map[string]*rockcraftPlatformEntry `yaml:"platforms"`
}

func (s *RockStrategy) ParsePlatforms(content []byte) ([]string, error) {
	var rc rockcraftYAML
	if err := yaml.Unmarshal(content, &rc); err != nil {
		return nil, err
	}

	if len(rc.Platforms) == 0 {
		return []string{"amd64"}, nil
	}

	seen := make(map[string]struct{})
	for key, val := range rc.Platforms {
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

// decodeStringOrSlice decodes a yaml.Node that is either a scalar string or a sequence of strings.
func decodeStringOrSlice(node *yaml.Node) ([]string, error) {
	if node.Kind == yaml.ScalarNode {
		var s string
		if err := node.Decode(&s); err != nil {
			return nil, err
		}
		return []string{s}, nil
	}
	var ss []string
	if err := node.Decode(&ss); err != nil {
		return nil, err
	}
	return ss, nil
}
