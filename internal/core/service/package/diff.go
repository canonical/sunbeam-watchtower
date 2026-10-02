// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package pkg

import (
	"context"
	"fmt"
	"sort"

	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// DiffOpts controls the diff operation.
type DiffOpts struct {
	Packages   []string        // package names to compare
	Sources    []ProjectSource // which distros/backports to query
	Suites     []string        // optional suite filter
	Components []string        // optional component filter
}

// DiffResult holds one package's versions across all queried sources.
type DiffResult = dto.PackageDiffResult

// Diff compares package versions across multiple sources.
func (s *Service) Diff(ctx context.Context, opts DiffOpts) ([]DiffResult, error) {
	// Map: package name → source name → []SourcePackage
	grouped := make(map[string]map[string][]distro.SourcePackage)

	for _, src := range opts.Sources {
		// Derive per-source suite filter from entries when available,
		// so that suites from one source don't leak into another's query.
		suites := opts.Suites
		if len(src.Entries) > 0 {
			set := make(map[string]bool, len(src.Entries))
			for _, e := range src.Entries {
				set[e.Suite] = true
			}
			suites = make([]string, 0, len(set))
			for s := range set {
				suites = append(suites, s)
			}
		}

		pkgs, err := s.cache.Query(ctx, src.Name, dto.QueryOpts{
			Packages:   opts.Packages,
			Suites:     suites,
			Components: opts.Components,
		})
		if err != nil {
			return nil, fmt.Errorf("querying %s: %w", src.Name, err)
		}

		for _, pkg := range pkgs {
			if grouped[pkg.Package] == nil {
				grouped[pkg.Package] = make(map[string][]distro.SourcePackage)
			}
			grouped[pkg.Package][src.Name] = append(grouped[pkg.Package][src.Name], pkg)
		}
	}

	results := make([]DiffResult, 0, len(grouped))
	for name, versions := range grouped {
		results = append(results, DiffResult{
			Package:  name,
			Versions: versions,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Package < results[j].Package
	})

	return results, nil
}
