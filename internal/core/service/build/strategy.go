// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/canonical/sunbeam-watchtower/internal/core/service/artifactdiscovery"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// manifestName reads metadataPath and returns the declared artifact name via
// the shared manifest-name parser. An empty name (no error) signals to the
// caller that the manifest did not declare one, so a fallback (directory
// base name) can be applied.
func manifestName(metadataPath string) (string, error) {
	content, err := os.ReadFile(metadataPath)
	if err != nil {
		return "", err
	}
	return artifactdiscovery.ParseManifestName(content, filepath.Base(metadataPath))
}

// DiscoveredRecipe is a recipe found on disk during local build preparation.
// RelPath is the directory containing the metadata file, relative to the repo
// root, using forward slashes. It becomes the LP build path for this recipe,
// preserving any nested monorepo layout (e.g. "charms/storage/foo").
type DiscoveredRecipe struct {
	Name    string
	RelPath string
}

// ArtifactStrategy encapsulates artifact-type-specific logic.
type ArtifactStrategy interface {
	// ArtifactType returns the type of artifact this strategy handles.
	ArtifactType() dto.ArtifactType

	// MetadataFileName returns the metadata file name (e.g. "rockcraft.yaml").
	MetadataFileName() string

	// BuildPath returns the default LP build path for an artifact name.
	// Used as a fallback when only a name is available (e.g. --artifacts foo
	// passed explicitly without discovery). Assumes a shallow layout.
	// E.g. for rocks: "rocks/keystone", for charms: "charms/mysql-k8s".
	BuildPath(artifactName string) string

	// ParsePlatforms parses the metadata file content and returns expected platforms.
	// Returns a set of architecture strings like {"amd64", "arm64"}.
	ParsePlatforms(metadataContent []byte) ([]string, error)

	// TempRecipeName generates a temporary recipe name.
	// Format: {prefix}-{shortSHA}-{artifactName}
	TempRecipeName(artifactName, commitSHA, prefix string) string

	// DiscoverRecipes finds artifact recipes by scanning a local repo directory
	// for metadata files. Each entry carries the recipe name and its relative
	// directory path from the repo root so that nested monorepo layouts are
	// preserved when the build path is set on Launchpad.
	DiscoverRecipes(repoPath string) ([]DiscoveredRecipe, error)

	// OfficialRecipeName returns the recipe name for official/remote builds.
	// For the development focus series, returns just the artifact name.
	// For other series, returns artifactName-series.
	OfficialRecipeName(artifactName, series, devFocus string) string

	// BranchForSeries returns the git branch for a given series.
	// For the development focus, returns the repo's default branch (e.g. "main").
	// For other series, returns "stable/<series>".
	BranchForSeries(series, devFocus, defaultBranch string) string
}

// walkRecipes walks repoPath looking for metadata files whose directory sits
// under scanSubdir, and returns one DiscoveredRecipe per match. If a metadata
// file also exists at the repo root, it is reported with an empty RelPath
// (single-artifact repo layout).
//
// The walk stops descending once a recipe directory is found, so sources
// inside a recipe are never mistaken for another recipe. scanSubdir may be
// empty to walk from the repo root.
func walkRecipes(repoPath, scanSubdir, metadataFile string) ([]DiscoveredRecipe, error) {
	var out []DiscoveredRecipe

	rootMeta := filepath.Join(repoPath, metadataFile)
	if _, err := os.Stat(rootMeta); err == nil {
		name, parseErr := manifestName(rootMeta)
		if parseErr != nil {
			return nil, parseErr
		}
		if name == "" {
			name = filepath.Base(repoPath)
		}
		out = append(out, DiscoveredRecipe{Name: name, RelPath: ""})
	}

	scanRoot := filepath.Join(repoPath, scanSubdir)
	info, err := os.Stat(scanRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return out, nil
	}

	err = filepath.WalkDir(scanRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		if path == repoPath {
			return nil
		}
		candidate := filepath.Join(path, metadataFile)
		if _, statErr := os.Stat(candidate); statErr == nil {
			rel, relErr := filepath.Rel(repoPath, path)
			if relErr != nil {
				return relErr
			}
			name, parseErr := manifestName(candidate)
			if parseErr != nil {
				return parseErr
			}
			if name == "" {
				name = filepath.Base(path)
			}
			out = append(out, DiscoveredRecipe{
				Name:    name,
				RelPath: filepath.ToSlash(rel),
			})
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
