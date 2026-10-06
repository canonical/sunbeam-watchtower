// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package sru

import (
	"fmt"
	"slices"
	"sort"
	"time"

	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// MigrationChain applies the operator's ordering policy to observed publications.
// Targets must be ordered newest first, with Ubuntu before its UCA backport.
// Missing fix evidence establishes neither applicability nor missing work.
func MigrationChain(query dto.SRUMigrationQuery, targets []dto.SRUMigrationTarget, observedAt time.Time) dto.SRUMigrationChain {
	result := dto.SRUMigrationChain{Query: query, ObservedAt: observedAt, Scope: "configured series; upstream order, not UCA support", Inventory: append([]dto.SRUMigrationTarget{}, targets...), Targets: []dto.SRUMigrationTarget{}, Transitions: []dto.SRUMigrationTransition{}, FirstOutstanding: []string{}}
	selected := -1
	for i, target := range targets {
		if target.Archive == "uca" && target.Series == query.Series {
			selected = i
			break
		}
	}
	if selected < 0 {
		result.Warnings = append(result.Warnings, "selected UCA series is not in observations")
		return result
	}
	fix := make([]Evidence, len(targets))
	for i, target := range targets {
		fix[i] = migrationFix(target, query.BugID)
		targets[i].Relation = "newer"
		if i == selected {
			targets[i].Relation = "selected"
		} else if i > selected {
			targets[i].Relation = "lower; fix not observed"
			if fix[i].Version != "" {
				targets[i].Relation = "lower backport"
			}
		}
		if target.Archive == "ubuntu" {
			targets[i].Relation = "Ubuntu parent"
		}
		target = targets[i]
		if i <= selected || target.Archive == "uca" && fix[i].Version != "" {
			result.Targets = append(result.Targets, target)
		}
	}
	result.Inventory = append([]dto.SRUMigrationTarget{}, targets...)
	steps := make(map[string]dto.SRUMigrationTransition)
	var ensure func(int, string) string
	ensure = func(index int, destination string) string {
		target := targets[index]
		current := fix[index]
		if stageRank[current.Stage] >= stageRank[destination] {
			return ""
		}
		id := target.Archive + "/" + target.Series + "/" + destination
		if _, exists := steps[id]; exists {
			return id
		}
		step := dto.SRUMigrationTransition{ID: id, Package: query.Package, Archive: target.Archive, Series: target.Series, Version: current.Version, From: current.Stage, To: destination, Kind: "fix", State: "pending", Reason: "inferred workflow order; migration eligibility is not established", DependsOn: []string{}}
		if current.Version == "" {
			step.From = "not observed"
			step.State = "unknown"
			step.Kind = "evidence"
			step.Reason = "fix not observed; applicability and required work are unknown"
			steps[id] = step
			return id
		}
		// Every required pocket must be observed, including updates when deciding
		// whether proposed is still occupied. Unknown must not become an empty pocket.
		for _, pocket := range target.Pockets {
			if !pocket.Known {
				step.State = "unknown"
				step.Reason = "pocket state is unavailable; migration order cannot be resolved"
			}
			for _, publication := range pocket.Publications {
				if !publication.BugsKnown && stageRank[pocket.Pocket] > stageRank[current.Stage] {
					step.State = "unknown"
					step.Reason = "fix association in a later pocket is unknown"
				}
			}
		}
		steps[id] = step
		if destination == "updates" && current.Stage == "staging" {
			step.From = "proposed"
			step.DependsOn = appendDependency(step.DependsOn, ensure(index, "proposed"))
		}
		// Pending proposed occupies staging-to-proposed independently of bug IDs.
		if destination == "proposed" && current.Stage == "staging" {
			proposed := migrationLatest(target, "proposed")
			updates := migrationLatest(target, "updates")
			if proposed.Version != "" && (updates.Version == "" || distro.CompareVersions(proposed.Version, updates.Version) > 0) {
				occupantID := target.Archive + "/" + target.Series + "/occupant/updates"
				state := "pending"
				if !migrationPocketKnown(target, "proposed") || !migrationPocketKnown(target, "updates") {
					state = "unknown"
				}
				steps[occupantID] = dto.SRUMigrationTransition{ID: occupantID, Package: query.Package, Archive: target.Archive, Series: target.Series, Version: proposed.Version, From: "proposed", To: "updates", Kind: "occupant", State: state, Reason: "inferred serialization of the observed proposed SRU before staging promotion", DependsOn: []string{}}
				step.DependsOn = appendDependency(step.DependsOn, occupantID)
			}
			if proposed.Version != "" && distro.CompareVersions(current.Version, proposed.Version) <= 0 || updates.Version != "" && distro.CompareVersions(current.Version, updates.Version) <= 0 {
				step.State = "unknown"
				step.Reason = "staging version is not newer than a destination publication; fix retention and applicability need review"
			}
		}
		// A newer configured series must reach the same destination first. There
		// is no cross-release source-version comparison and no assumed applicability.
		for higher := 0; higher < index; higher++ {
			if targets[higher].Archive != target.Archive {
				continue
			}
			step.DependsOn = appendDependency(step.DependsOn, ensure(higher, destination))
		}
		if target.Archive == "uca" && target.ParentRequired {
			for parent := range targets {
				if targets[parent].Archive == "ubuntu" && targets[parent].Series == target.ParentSeries {
					step.DependsOn = appendDependency(step.DependsOn, ensure(parent, "updates"))
				}
			}
		}
		steps[id] = step
		return id
	}
	root := ensure(selected, "updates")
	reachable := make(map[string]bool)
	var visit func(string)
	visit = func(id string) {
		if id == "" || reachable[id] {
			return
		}
		reachable[id] = true
		for _, dependency := range steps[id].DependsOn {
			visit(dependency)
		}
		result.Transitions = append(result.Transitions, steps[id])
		if len(steps[id].DependsOn) == 0 {
			result.FirstOutstanding = append(result.FirstOutstanding, id)
		}
	}
	visit(root)
	for i := selected + 1; i < len(targets); i++ {
		if targets[i].Archive == "uca" && fix[i].Version != "" {
			visit(ensure(i, "updates"))
		}
	}
	usedTargets := make(map[string]bool)
	for id := range reachable {
		step := steps[id]
		usedTargets[step.Archive+"/"+step.Series] = true
	}
	for _, target := range targets {
		included := slices.ContainsFunc(result.Targets, func(t dto.SRUMigrationTarget) bool { return t.Archive == target.Archive && t.Series == target.Series })
		if !included && usedTargets[target.Archive+"/"+target.Series] {
			result.Targets = append(result.Targets, target)
		}
	}

	sort.Strings(result.FirstOutstanding)
	if len(result.Transitions) == 0 {
		result.Warnings = append(result.Warnings, "matching updates publication observed; this is not proof of migration eligibility for other uploads")
	}
	for _, target := range result.Inventory {
		for _, pocket := range target.Pockets {
			if !pocket.Known {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s/%s %s: %s", target.Archive, target.Series, pocket.Pocket, pocket.Warning))
			}
			for _, publication := range pocket.Publications {
				if publication.Warning != "" {
					result.Warnings = append(result.Warnings, fmt.Sprintf("%s/%s %s %s: %s", target.Archive, target.Series, pocket.Pocket, publication.Version, publication.Warning))
				}
			}
		}
	}
	return result
}

func appendDependency(dependencies []string, id string) []string {
	if id != "" && !slices.Contains(dependencies, id) {
		return append(dependencies, id)
	}
	return dependencies
}

func migrationFix(target dto.SRUMigrationTarget, bugID string) Evidence {
	best := Evidence{Stage: "not observed"}
	for _, pocket := range target.Pockets {
		if !pocket.Known {
			continue
		}
		for _, publication := range pocket.Publications {
			if !publication.BugsKnown || !slices.Contains(publication.BugIDs, bugID) {
				continue
			}
			if stageRank[pocket.Pocket] > stageRank[best.Stage] || pocket.Pocket == best.Stage && distro.CompareVersions(publication.Version, best.Version) > 0 {
				best = Evidence{Stage: pocket.Pocket, Version: publication.Version, URL: publication.URL}
			}
		}
	}
	return best
}

func migrationLatest(target dto.SRUMigrationTarget, pocketName string) Evidence {
	best := Evidence{Stage: pocketName}
	for _, pocket := range target.Pockets {
		if pocket.Pocket != pocketName || !pocket.Known {
			continue
		}
		for _, publication := range pocket.Publications {
			if best.Version == "" || distro.CompareVersions(publication.Version, best.Version) > 0 {
				best.Version = publication.Version
				best.URL = publication.URL
			}
		}
	}
	return best
}

func migrationPocketKnown(target dto.SRUMigrationTarget, name string) bool {
	for _, pocket := range target.Pockets {
		if pocket.Pocket == name {
			return pocket.Known
		}
	}
	return false
}
