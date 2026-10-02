// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

// Package artifactdiscovery finds charm, snap, and rock artifacts in a cached
// git repository by reading their craft-yaml manifests out of the HEAD commit
// tree. It is the canonical replacement for the ad-hoc discovery logic in
// internal/app/release_helpers.go, internal/core/service/team_discovery.go, and
// the worktree-based walker in internal/core/service/build.
package artifactdiscovery

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"sort"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"gopkg.in/yaml.v3"
)

// HeadFile is one file located in the HEAD commit tree of a cached repository.
type HeadFile struct {
	Path    string
	Content []byte
}

// TreeReader reads files from the HEAD commit tree of a cached git repository.
// The canonical production implementation lives in the gitcache adapter; it is
// kept abstract here so the service package stays free of adapter imports.
type TreeReader interface {
	ReadHEADFile(repoPath, filePath string) ([]byte, error)
	FindHEADFilesByBaseName(repoPath, baseName string) ([]HeadFile, error)
}

// DiscoveredArtifact describes one artifact manifest found in a repository.
// RelPath is the directory containing the manifest, relative to the repo root
// with forward slashes. It is empty for root-layout single-artifact repos.
// Resources is populated only for charms.
type DiscoveredArtifact struct {
	Name         string
	RelPath      string
	ArtifactType dto.ArtifactType
	Resources    []string
}

// ArtifactDiscoverer enumerates artifacts of a given type in a cached
// repository. It is the consumption contract primary adapters depend on;
// *Service is the canonical implementation.
type ArtifactDiscoverer interface {
	Discover(ctx context.Context, repoPath string, artifactType dto.ArtifactType) ([]DiscoveredArtifact, error)
}

// Service enumerates artifacts in a cached git repository.
type Service struct {
	reader TreeReader
	logger *slog.Logger
}

// NewService returns a new artifact discovery service. reader supplies access
// to the HEAD tree of a cached repository; logger is optional and defaults to
// slog.Default when nil.
func NewService(reader TreeReader, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{reader: reader, logger: logger}
}

// Discover enumerates artifacts of the given type in the cached repo at
// repoPath.
//
// For a root-layout repository (manifest at the repo root) the result contains
// one DiscoveredArtifact with RelPath == "". For a mono-repo layout the result
// contains one entry per manifest, with RelPath set to the directory of the
// manifest. Results are sorted by Name for determinism.
//
// Behaviour on missing/malformed manifests (consistent with the legacy helpers
// this service replaces):
//   - no manifest found anywhere: returns a typed error
//   - malformed YAML: returns an error mentioning the source path
//   - manifest without a declared name: returns an error
//   - unsupported ArtifactType: returns an error
func (s *Service) Discover(ctx context.Context, repoPath string, artifactType dto.ArtifactType) ([]DiscoveredArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch artifactType {
	case dto.ArtifactCharm:
		return s.discoverCharms(repoPath)
	case dto.ArtifactSnap:
		return s.discoverSnaps(repoPath)
	case dto.ArtifactRock:
		return s.discoverRocks(repoPath)
	default:
		return nil, fmt.Errorf("unsupported artifact type %q", artifactType)
	}
}

type charmcraftMetadata struct {
	Name      string              `yaml:"name"`
	Resources map[string]struct{} `yaml:"resources"`
}

type snapcraftMetadata struct {
	Name string `yaml:"name"`
}

type rockcraftMetadata struct {
	Name string `yaml:"name"`
}

func (s *Service) discoverCharms(repoPath string) ([]DiscoveredArtifact, error) {
	if content, err := s.reader.ReadHEADFile(repoPath, "charmcraft.yaml"); err == nil {
		art, perr := parseCharm(content, "charmcraft.yaml", "")
		if perr != nil {
			return nil, perr
		}
		return []DiscoveredArtifact{art}, nil
	}
	files, err := s.reader.FindHEADFilesByBaseName(repoPath, "charmcraft.yaml")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no charmcraft.yaml found in repository")
	}
	result := make([]DiscoveredArtifact, 0, len(files))
	for _, f := range files {
		art, err := parseCharm(f.Content, f.Path, path.Dir(f.Path))
		if err != nil {
			return nil, err
		}
		result = append(result, art)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func parseCharm(content []byte, source, relPath string) (DiscoveredArtifact, error) {
	var md charmcraftMetadata
	if err := yaml.Unmarshal(content, &md); err != nil {
		return DiscoveredArtifact{}, fmt.Errorf("parsing %s: %w", source, err)
	}
	if md.Name == "" {
		return DiscoveredArtifact{}, fmt.Errorf("%s does not declare a charm name", source)
	}
	resources := make([]string, 0, len(md.Resources))
	for name := range md.Resources {
		resources = append(resources, name)
	}
	sort.Strings(resources)
	return DiscoveredArtifact{
		Name:         md.Name,
		RelPath:      relPath,
		ArtifactType: dto.ArtifactCharm,
		Resources:    resources,
	}, nil
}

func (s *Service) discoverSnaps(repoPath string) ([]DiscoveredArtifact, error) {
	for _, candidate := range []string{"snap/snapcraft.yaml", "snapcraft.yaml"} {
		content, err := s.reader.ReadHEADFile(repoPath, candidate)
		if err != nil {
			continue
		}
		art, perr := parseSnap(content, candidate, "")
		if perr != nil {
			return nil, perr
		}
		return []DiscoveredArtifact{art}, nil
	}
	files, err := s.reader.FindHEADFilesByBaseName(repoPath, "snapcraft.yaml")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no snapcraft.yaml found at repo root or snap/")
	}
	result := make([]DiscoveredArtifact, 0, len(files))
	for _, f := range files {
		art, err := parseSnap(f.Content, f.Path, path.Dir(f.Path))
		if err != nil {
			return nil, err
		}
		result = append(result, art)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func parseSnap(content []byte, source, relPath string) (DiscoveredArtifact, error) {
	var md snapcraftMetadata
	if err := yaml.Unmarshal(content, &md); err != nil {
		return DiscoveredArtifact{}, fmt.Errorf("parsing %s: %w", source, err)
	}
	if md.Name == "" {
		return DiscoveredArtifact{}, fmt.Errorf("%s does not declare a snap name", source)
	}
	return DiscoveredArtifact{
		Name:         md.Name,
		RelPath:      relPath,
		ArtifactType: dto.ArtifactSnap,
	}, nil
}

func (s *Service) discoverRocks(repoPath string) ([]DiscoveredArtifact, error) {
	if content, err := s.reader.ReadHEADFile(repoPath, "rockcraft.yaml"); err == nil {
		art, perr := parseRock(content, "rockcraft.yaml", "")
		if perr != nil {
			return nil, perr
		}
		return []DiscoveredArtifact{art}, nil
	}
	files, err := s.reader.FindHEADFilesByBaseName(repoPath, "rockcraft.yaml")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no rockcraft.yaml found in repository")
	}
	result := make([]DiscoveredArtifact, 0, len(files))
	for _, f := range files {
		art, err := parseRock(f.Content, f.Path, path.Dir(f.Path))
		if err != nil {
			return nil, err
		}
		result = append(result, art)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func parseRock(content []byte, source, relPath string) (DiscoveredArtifact, error) {
	var md rockcraftMetadata
	if err := yaml.Unmarshal(content, &md); err != nil {
		return DiscoveredArtifact{}, fmt.Errorf("parsing %s: %w", source, err)
	}
	if md.Name == "" {
		return DiscoveredArtifact{}, fmt.Errorf("%s does not declare a rock name", source)
	}
	return DiscoveredArtifact{
		Name:         md.Name,
		RelPath:      relPath,
		ArtifactType: dto.ArtifactRock,
	}, nil
}
