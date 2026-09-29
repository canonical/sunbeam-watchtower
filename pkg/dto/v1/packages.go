// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package dto

import (
	"time"

	distro "github.com/gboutry/sunbeam-watchtower/pkg/distro/v1"
)

// PackageSetCacheStatus describes one cached Launchpad packageset snapshot.
type PackageSetCacheStatus struct {
	Name             string    `json:"name" yaml:"name"`
	ConfiguredSeries string    `json:"configured_series" yaml:"configured_series"`
	Series           string    `json:"series" yaml:"series"`
	PackageCount     int       `json:"package_count" yaml:"package_count"`
	SyncedAt         time.Time `json:"synced_at" yaml:"synced_at"`
}

// PackageSource associates a name (e.g. "ubuntu", "uca") with its source entries.
type PackageSource struct {
	Name    string        `json:"name" yaml:"name"`
	Entries []SourceEntry `json:"entries" yaml:"entries"`
}

// PackageDiffResult holds one package's versions across all queried sources.
type PackageDiffResult struct {
	Package  string                            `json:"package" yaml:"package"`
	Versions map[string][]distro.SourcePackage `json:"versions" yaml:"versions"`
	Upstream string                            `json:"upstream,omitempty" yaml:"upstream,omitempty"`
}

// PackageDscResult holds .dsc URL lookup results for a package/version pair.
type PackageDscResult struct {
	Package string   `json:"package" yaml:"package"`
	Version string   `json:"version" yaml:"version"`
	URLs    []string `json:"urls" yaml:"urls"`
}
