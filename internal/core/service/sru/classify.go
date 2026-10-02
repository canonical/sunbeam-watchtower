// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

// Package sru classifies Launchpad bug tasks and archive evidence without
// changing either source of truth.
package sru

import (
	"sort"
	"strings"

	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// Evidence is a publication or queue item whose .changes file names the bug.
type Evidence struct {
	Stage   string
	Version string
	URL     string
}

var stageRank = map[string]int{
	"not observed": 0,
	"unapproved":   1,
	"staging":      1,
	"proposed":     2,
	"updates":      3,
}

// SharedCloudArchiveTaskWarning records an inferred package association on a
// shared Cloud Archive task. It is diagnostic context, not work to perform.
const SharedCloudArchiveTaskWarning = "Cloud Archive task is shared by multiple source packages; package association is inferred"

// Verification reads the tags for exactly one target series. Ubuntu and UCA
// use different tag orderings; the tag belongs to the bug, not the task.
func Verification(tags []string, archive, series string) (state, tag string) {
	prefix := "verification-" + series + "-"
	if archive == "ubuntu" {
		prefix = "verification-"
	}
	for _, candidate := range tags {
		var suffix string
		if archive == "ubuntu" {
			for _, value := range []string{"failed", "needed", "done"} {
				if candidate == "verification-"+value+"-"+series {
					suffix = value
				}
			}
		} else if strings.HasPrefix(candidate, prefix) {
			suffix = strings.TrimPrefix(candidate, prefix)
		}
		if suffix != "done" && suffix != "needed" && suffix != "failed" {
			continue
		}
		if state == "" || verificationRank(suffix) > verificationRank(state) {
			state, tag = suffix, candidate
		}
	}
	if state == "" {
		return "none", ""
	}
	return state, tag
}

func verificationRank(value string) int {
	switch value {
	case "failed":
		return 3
	case "needed":
		return 2
	default:
		return 1
	}
}

// ApplyEvidence selects the newest observed version, then its furthest pocket.
// Fix Released remains the displayed release state, as configured by the user.
func ApplyEvidence(row *dto.SRURow, evidence []Evidence) {
	best := Evidence{Stage: "not observed"}
	row.Evidence = row.Evidence[:0]
	for _, item := range evidence {
		row.Evidence = append(row.Evidence, dto.SRUArchiveEvidence{Stage: item.Stage, Version: item.Version, URL: item.URL})
		if best.Version == "" || distro.CompareVersions(item.Version, best.Version) > 0 ||
			(item.Version == best.Version && stageRank[item.Stage] > stageRank[best.Stage]) {
			best = item
		}
	}
	sort.Slice(row.Evidence, func(i, j int) bool {
		cmp := distro.CompareVersions(row.Evidence[i].Version, row.Evidence[j].Version)
		if cmp != 0 {
			return cmp > 0
		}
		return stageRank[row.Evidence[i].Stage] > stageRank[row.Evidence[j].Stage]
	})
	row.Stage, row.Version, row.EvidenceURL = best.Stage, best.Version, best.URL
	var evidenceWarning string
	if row.TaskStatus == "Fix Released" && row.Stage != "updates" && row.Stage != "not observed" {
		evidenceWarning = "task is Fix Released; a matching updates publication was not observed"
	} else if row.TaskStatus != "Fix Released" && row.Stage == "updates" {
		evidenceWarning = "matching updates publication exists; task is not Fix Released"
	}
	if evidenceWarning != "" {
		if row.Warning != "" {
			row.Warning += "; "
		}
		row.Warning += evidenceWarning
	}
}

// Filter returns a stable subset of one snapshot.
func Filter(snapshot dto.SRUSnapshot, filter dto.SRUFilter) dto.SRUSnapshot {
	rows := make([]dto.SRURow, 0, len(snapshot.Rows))
	for _, row := range snapshot.Rows {
		row.Warning = normalizedWarning(row)
		if filter.BugID != "" && row.BugID != filter.BugID ||
			filter.Package != "" && row.Package != filter.Package ||
			filter.Set != "" && !contains(row.PackageSets, filter.Set) ||
			filter.Archive != "" && row.Archive != filter.Archive ||
			filter.Series != "" && row.Series != filter.Series ||
			filter.Stage != "" && row.Stage != filter.Stage ||
			filter.TaskStatus != "" && row.TaskStatus != filter.TaskStatus ||
			filter.Verification != "" && row.Verification != filter.Verification ||
			filter.NeedsAttention && !needsAttention(row) {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
			if rows[i].BugID == rows[j].BugID {
				return rows[i].Series < rows[j].Series
			}
			return rows[i].BugID < rows[j].BugID
		}
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})
	snapshot.Rows = rows
	return snapshot
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func needsAttention(row dto.SRURow) bool {
	return (row.Warning != "" && row.Warning != SharedCloudArchiveTaskWarning) ||
		row.Verification == "failed" ||
		(row.Stage == "proposed" && row.Verification != "done") ||
		(row.ParentRequired && !row.ParentReady)
}

func normalizedWarning(row dto.SRURow) string {
	if row.TaskStatus != "Fix Released" || row.Stage != "not observed" {
		return row.Warning
	}
	const obsolete = "task is Fix Released; a matching updates publication was not observed"
	if row.Warning == obsolete {
		return ""
	}
	warning := strings.ReplaceAll(row.Warning, obsolete+"; ", "")
	return strings.ReplaceAll(warning, "; "+obsolete, "")
}
