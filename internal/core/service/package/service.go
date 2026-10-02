// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package pkg

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ProjectSource associates a name (e.g. "ubuntu", "uca") with its source entries.
type ProjectSource = dto.PackageSource

// Service orchestrates package-related operations using a DistroCache.
type Service struct {
	cache  port.DistroCache
	logger *slog.Logger
}

// NewService creates a new package service.
func NewService(cache port.DistroCache, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{cache: cache, logger: logger}
}

// Show returns version information for a single package across all given sources.
func (s *Service) Show(ctx context.Context, pkgName string, sources []ProjectSource) (*DiffResult, error) {
	results, err := s.Diff(ctx, DiffOpts{
		Packages: []string{pkgName},
		Sources:  sources,
	})
	if err != nil {
		return nil, err
	}
	for i := range results {
		if results[i].Package == pkgName {
			return &results[i], nil
		}
	}
	return &DiffResult{Package: pkgName, Versions: make(map[string][]distro.SourcePackage)}, nil
}

// List returns all source packages from a single source group matching the given opts.
func (s *Service) List(ctx context.Context, source string, opts dto.QueryOpts) ([]distro.SourcePackage, error) {
	return s.cache.Query(ctx, source, opts)
}

// UpdateCache downloads and indexes Sources files for the given sources.
func (s *Service) UpdateCache(ctx context.Context, sources []ProjectSource) error {
	for _, src := range sources {
		s.logger.Info("updating cache", "source", src.Name, "entries", len(src.Entries))
		if err := s.cache.Update(ctx, src.Name, src.Entries); err != nil {
			return err
		}
	}
	return nil
}

// PackageVersionPair identifies a source package by name and version.
type PackageVersionPair struct {
	Package string
	Version string
}

// DscResult holds .dsc URL lookup results for a package/version pair.
type DscResult = dto.PackageDscResult

// FindDsc searches raw Sources files for .dsc URLs matching the given package/version pairs.
// It first uses the bbolt cache to locate which source files contain each pair,
// then parses only the necessary files.
func (s *Service) FindDsc(ctx context.Context, pairs []PackageVersionPair, sources []ProjectSource) ([]DscResult, error) {
	type pairKey struct{ pkg, ver string }

	// Use cache to find which source/suite/component holds each pair.
	type location struct {
		srcName   string
		suite     string
		component string
		mirror    string
	}
	pairLocations := make(map[pairKey][]location, len(pairs))

	// Build a source→entries lookup with mirrors.
	type entryWithMirror struct {
		suite     string
		component string
		mirror    string
	}
	srcEntries := make(map[string][]entryWithMirror, len(sources))
	for _, src := range sources {
		for _, entry := range src.Entries {
			srcEntries[src.Name] = append(srcEntries[src.Name], entryWithMirror{
				suite:     entry.Suite,
				component: entry.Component,
				mirror:    strings.TrimRight(entry.Mirror, "/"),
			})
		}
	}

	for _, p := range pairs {
		result, err := s.Show(ctx, p.Package, sources)
		if err != nil {
			continue
		}
		for srcName, versions := range result.Versions {
			for _, v := range versions {
				if v.Version != p.Version {
					continue
				}
				// Find the mirror for this suite/component from source entries.
				mirror := ""
				for _, e := range srcEntries[srcName] {
					if e.suite == v.Suite && e.component == v.Component {
						mirror = e.mirror
						break
					}
				}
				pairLocations[pairKey{p.Package, p.Version}] = append(
					pairLocations[pairKey{p.Package, p.Version}],
					location{srcName: srcName, suite: v.Suite, component: v.Component, mirror: mirror},
				)
			}
		}
	}

	cacheDir := s.cache.CacheDir()

	// Parse only the specific Sources files we need.
	type pairResult struct{ urls []string }
	found := make(map[pairKey]*pairResult, len(pairs))
	for _, p := range pairs {
		found[pairKey{p.Package, p.Version}] = &pairResult{}
	}

	// Track already-parsed files to avoid re-parsing.
	type fileKey struct{ srcName, suite, component string }
	parsed := make(map[fileKey][]distro.SourcePackageFiles)

	for pk, locs := range pairLocations {
		for _, loc := range locs {
			fk := fileKey{loc.srcName, loc.suite, loc.component}
			pkgs, ok := parsed[fk]
			if !ok {
				for _, format := range []string{"xz", "gz"} {
					fname := distro.SourcesFileName(loc.suite, loc.component, format)
					path := filepath.Join(cacheDir, "sources", loc.srcName, fname)
					p, err := distro.ParseSourcesFileWithFiles(path, format, loc.suite, loc.component)
					if err != nil {
						continue
					}
					pkgs = p
					break
				}
				parsed[fk] = pkgs
			}

			for _, p := range pkgs {
				if p.Package != pk.pkg || p.Version != pk.ver {
					continue
				}
				for _, f := range p.Files {
					if strings.HasSuffix(f, ".dsc") {
						url := loc.mirror + "/" + p.Directory + "/" + f
						found[pk].urls = append(found[pk].urls, url)
					}
				}
			}
		}
	}

	results := make([]DscResult, 0, len(pairs))
	for _, p := range pairs {
		k := pairKey{p.Package, p.Version}
		results = append(results, DscResult{
			Package: p.Package,
			Version: p.Version,
			URLs:    found[k].urls,
		})
	}

	return results, nil
}

// ShowDetail returns the full APT metadata for a specific package. It uses the
// bbolt cache index to locate the exact source/suite/component, then parses only
// that single Sources file for the full RFC822 fields.
func (s *Service) ShowDetail(ctx context.Context, pkgName, version string, sources []ProjectSource) (*distro.SourcePackageInfo, error) {
	// Use the cache to find which source/suite/component holds this package.
	result, err := s.Show(ctx, pkgName, sources)
	if err != nil {
		return nil, err
	}

	// Find the target entry: either exact version match or highest version.
	var target *distro.SourcePackage
	var targetSource string
	for srcName, versions := range result.Versions {
		for i := range versions {
			v := &versions[i]
			if version != "" {
				if v.Version == version {
					target = v
					targetSource = srcName
					break
				}
			} else {
				if target == nil || distro.CompareVersions(v.Version, target.Version) > 0 {
					target = v
					targetSource = srcName
				}
			}
		}
		if target != nil && version != "" {
			break
		}
	}

	if target == nil {
		if version != "" {
			return nil, fmt.Errorf("package %s=%s not found in cache", pkgName, version)
		}
		return nil, fmt.Errorf("package %q not found in configured sources", pkgName)
	}

	// Parse only the single Sources file that contains our package.
	return s.parseOneSourcesFile(pkgName, target.Version, targetSource, target.Suite, target.Component)
}

// parseOneSourcesFile parses the full metadata for a specific package from
// a single cached Sources file identified by source name, suite, and component.
func (s *Service) parseOneSourcesFile(pkgName, version, srcName, suite, component string) (*distro.SourcePackageInfo, error) {
	cacheDir := s.cache.CacheDir()

	for _, format := range []string{"xz", "gz"} {
		fname := distro.SourcesFileName(suite, component, format)
		path := filepath.Join(cacheDir, "sources", srcName, fname)
		pkgs, err := distro.ParseSourcesFileFull(path, format, suite, component)
		if err != nil {
			continue
		}
		for i := range pkgs {
			if pkgs[i].Package == pkgName && pkgs[i].Version == version {
				return &pkgs[i], nil
			}
		}
		// File parsed successfully but package not found — don't try gz.
		break
	}

	return nil, fmt.Errorf("package %s=%s not found in Sources files", pkgName, version)
}

// CacheStatus returns metadata about the cached source groups.
func (s *Service) CacheStatus() ([]dto.CacheStatus, error) {
	return s.cache.Status()
}
func (s *Service) ReverseDepends(ctx context.Context, pkg string, sources []ProjectSource, opts dto.QueryOpts) ([]distro.SourcePackageDetail, error) {
	// Build the set of names to match (python-foo ↔ python3-foo).
	matchNames := map[string]bool{pkg: true}
	if strings.HasPrefix(pkg, "python3-") {
		matchNames["python-"+strings.TrimPrefix(pkg, "python3-")] = true
	} else if strings.HasPrefix(pkg, "python-") {
		matchNames["python3-"+strings.TrimPrefix(pkg, "python-")] = true
	}

	var results []distro.SourcePackageDetail
	for _, src := range sources {
		pkgs, err := s.cache.QueryDetailed(ctx, src.Name, opts)
		if err != nil {
			return nil, fmt.Errorf("querying detailed %s: %w", src.Name, err)
		}
		for _, p := range pkgs {
			for _, dep := range p.BuildDepends {
				if matchNames[dep] {
					results = append(results, p)
					break
				}
			}
		}
	}
	return results, nil
}
