// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package sru

import (
	"regexp"
	"strings"

	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"pault.ag/go/debian/version"
)

var cloudRevision = regexp.MustCompile(`~cloud[0-9]+$`)

// VersionCells expects an Ubuntu parent first, followed by UCA pockets.
// UCA comparisons preserve full Debian versions, including cloud rebuilds.
// Only parent/UCA comparisons remove the trailing UCA backport suffix.
func VersionCells(cells []dto.SRUVersionCell) []dto.SRUVersionCell {
	result := append([]dto.SRUVersionCell(nil), cells...)
	bestParent, bestUCA := "", ""
	complete := true
	for i := range result {
		cell := &result[i]
		if cell.State == "unknown" || cell.State == "unconfigured" {
			complete = false
			continue
		}
		if cell.Version == "" {
			cell.State = "empty"
			continue
		}
		if _, err := version.Parse(cell.Version); err != nil || strings.ContainsAny(cell.Version, "\r\n") {
			cell.State = "unknown"
			cell.Warning = "invalid published Debian version"
			complete = false
			continue
		}
		if i == 0 {
			bestParent = cell.Version
		} else if bestUCA == "" || distro.CompareVersions(cell.Version, bestUCA) > 0 {
			bestUCA = cell.Version
		}
	}
	for i := range result {
		cell := &result[i]
		if cell.State == "unknown" || cell.State == "unconfigured" || cell.State == "empty" {
			continue
		}
		var behind bool
		if i == 0 {
			behind = bestUCA != "" && distro.CompareVersions(cell.Version, cloudRevision.ReplaceAllString(bestUCA, "")) < 0
		} else {
			behind = distro.CompareVersions(cell.Version, bestUCA) < 0 ||
				(bestParent != "" && distro.CompareVersions(cloudRevision.ReplaceAllString(cell.Version, ""), bestParent) < 0)
		}
		switch {
		case behind:
			cell.State = "behind"
		case !complete:
			cell.State = "unknown"
			cell.Warning = "cannot establish latest version while another cell is unavailable"
		default:
			cell.State = "current"
		}
	}
	return result
}
