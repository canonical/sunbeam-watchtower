// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package dto

import "time"

type SRUVersionsQuery struct {
	Package string `json:"package" yaml:"package"`
	Series  string `json:"series" yaml:"series"`
}

// SRUVersionCell retains raw publication versions and reports version currency,
// never support, bug applicability or migration eligibility.
type SRUVersionCell struct {
	Label   string `json:"label" yaml:"label"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	State   string `json:"state" yaml:"state"`
	Warning string `json:"warning,omitempty" yaml:"warning,omitempty"`
}

type SRUVersions struct {
	ObservedAt   time.Time        `json:"observed_at" yaml:"observed_at"`
	Query        SRUVersionsQuery `json:"query" yaml:"query"`
	UbuntuBase   string           `json:"ubuntu_base" yaml:"ubuntu_base"`
	ParentSeries string           `json:"parent_series,omitempty" yaml:"parent_series,omitempty"`
	Cells        []SRUVersionCell `json:"cells" yaml:"cells"`
}

// SRUVersionList contains independent currency rows for configured UCA series.
// Versions are never compared across rows with different Ubuntu parents.
type SRUVersionList struct {
	ObservedAt time.Time     `json:"observed_at" yaml:"observed_at"`
	Package    string        `json:"package" yaml:"package"`
	Rows       []SRUVersions `json:"rows" yaml:"rows"`
}

// SRUPocketView is an offline snapshot scoped by the cached staging inventory.
type SRUPocketView struct {
	ObservedAt   time.Time     `json:"observed_at" yaml:"observed_at"`
	Series       string        `json:"series" yaml:"series"`
	UbuntuBase   string        `json:"ubuntu_base" yaml:"ubuntu_base"`
	ParentSeries string        `json:"parent_series,omitempty" yaml:"parent_series,omitempty"`
	CacheStatus  []CacheStatus `json:"cache_status" yaml:"cache_status"`
	Rows         []SRUVersions `json:"rows" yaml:"rows"`
}
