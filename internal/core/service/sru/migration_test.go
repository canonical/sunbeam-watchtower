// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package sru

import (
	"slices"
	"strings"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func migrationTestTarget(series, release, staging, proposed, updates string, fixPockets ...string) dto.SRUMigrationTarget {
	target := dto.SRUMigrationTarget{Archive: "uca", Series: series, UbuntuBase: "test", ReleaseID: release}
	for i, pocket := range []string{"staging", "proposed", "updates"} {
		observation := dto.SRUPocketObservation{Pocket: pocket, Known: true, Publications: []dto.SRUPublication{}}
		version := []string{staging, proposed, updates}[i]
		if version != "" {
			bugs := []string{"other"}
			if slices.Contains(fixPockets, pocket) {
				bugs = []string{"42"}
			}
			observation.Publications = append(observation.Publications, dto.SRUPublication{Version: version, BugsKnown: true, BugIDs: bugs})
		}
		target.Pockets = append(target.Pockets, observation)
	}
	return target
}

func migrationTestQuery() dto.SRUMigrationQuery {
	return dto.SRUMigrationQuery{Package: "nova", Series: "caracal", BugID: "42"}
}
func migrationTestStep(t *testing.T, result dto.SRUMigrationChain, id string) dto.SRUMigrationTransition {
	t.Helper()
	for _, step := range result.Transitions {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("missing %s: %+v", id, result.Transitions)
	return dto.SRUMigrationTransition{}
}

func TestMigrationChainIncludesUnrelatedProposedOccupant(t *testing.T) {
	targets := []dto.SRUMigrationTarget{
		migrationTestTarget("epoxy", "2025.1", "3", "2", "1", "staging"),
		migrationTestTarget("caracal", "2024.1", "8", "7", "7", "staging"),
	}
	result := MigrationChain(migrationTestQuery(), targets, time.Now())
	if !slices.Equal(result.FirstOutstanding, []string{"uca/epoxy/occupant/updates"}) {
		t.Fatalf("first=%v", result.FirstOutstanding)
	}
	positions := make(map[string]int)
	for i, step := range result.Transitions {
		positions[step.ID] = i
	}
	if positions["uca/epoxy/occupant/updates"] >= positions["uca/epoxy/proposed"] || positions["uca/epoxy/proposed"] >= positions["uca/caracal/proposed"] {
		t.Fatalf("prerequisites are not displayed first: %+v", result.Transitions)
	}
	occupant := migrationTestStep(t, result, "uca/epoxy/occupant/updates")
	if occupant.Version != "2" || occupant.Kind != "occupant" || occupant.State != "pending" || occupant.Package != "nova" {
		t.Fatalf("occupant=%+v", occupant)
	}
	epoxy := migrationTestStep(t, result, "uca/epoxy/proposed")
	caracal := migrationTestStep(t, result, "uca/caracal/proposed")
	if !slices.Contains(epoxy.DependsOn, occupant.ID) || !slices.Contains(caracal.DependsOn, epoxy.ID) {
		t.Fatalf("chain=%+v", result.Transitions)
	}
}

func TestMigrationChainReleasedFixIgnoresUnrelatedNewerSRU(t *testing.T) {
	result := MigrationChain(migrationTestQuery(), []dto.SRUMigrationTarget{
		migrationTestTarget("epoxy", "2025.1", "5", "4", "3", "updates"),
		migrationTestTarget("caracal", "2024.1", "8", "7", "7", "staging"),
	}, time.Now())
	if !slices.Equal(result.FirstOutstanding, []string{"uca/caracal/proposed"}) {
		t.Fatalf("first=%v", result.FirstOutstanding)
	}
	for _, step := range result.Transitions {
		if step.Series == "epoxy" {
			t.Fatalf("released higher fix has dependency: %+v", step)
		}
	}
}

func TestMigrationChainUnknownStateAndApplicabilityAreNotClearance(t *testing.T) {
	for _, mode := range []string{"fetch failed", "fix absent", "changes unavailable", "stale staging"} {
		t.Run(mode, func(t *testing.T) {
			higher := migrationTestTarget("epoxy", "2025.1", "3", "2", "1", "staging")
			switch mode {
			case "fetch failed":
				higher.Pockets[1].Known = false
				higher.Pockets[1].Warning = "fetch failed"
			case "fix absent":
				higher.Pockets[0].Publications[0].BugIDs = []string{"other"}
			case "changes unavailable":
				higher.Pockets[2].Publications[0].BugsKnown = false
			case "stale staging":
				higher.Pockets[1].Publications[0].Version = "4"
			}
			result := MigrationChain(migrationTestQuery(), []dto.SRUMigrationTarget{higher, migrationTestTarget("caracal", "2024.1", "8", "7", "7", "staging")}, time.Now())
			step := migrationTestStep(t, result, "uca/epoxy/proposed")
			if step.State != "unknown" {
				t.Fatalf("step=%+v", step)
			}
			if mode == "fix absent" && (!strings.Contains(step.Reason, "applicability") || step.Kind != "evidence") {
				t.Fatalf("absence inferred as work: %+v", step)
			}
			selected := migrationTestStep(t, result, "uca/caracal/proposed")
			if !slices.Contains(selected.DependsOn, step.ID) {
				t.Fatalf("uncertainty was ignored: %+v", selected)
			}
		})
	}
}

func TestMigrationChainLowerBackportsAreVisibleOnlyWithEvidence(t *testing.T) {
	for _, backported := range []bool{false, true} {
		pockets := []string{}
		if backported {
			pockets = append(pockets, "staging")
		}
		targets := []dto.SRUMigrationTarget{
			migrationTestTarget("epoxy", "2025.1", "", "", "3", "updates"),
			migrationTestTarget("caracal", "2024.1", "8", "7", "7", "staging"),
			migrationTestTarget("yoga", "2022.1", "3", "2", "2", pockets...),
		}
		result := MigrationChain(migrationTestQuery(), targets, time.Now())
		found := slices.ContainsFunc(result.Targets, func(target dto.SRUMigrationTarget) bool { return target.Series == "yoga" })
		if found != backported {
			t.Fatalf("lower targets=%+v, backported=%v", result.Targets, backported)
		}
		for _, step := range result.Transitions {
			if step.Series == "caracal" && slices.Contains(step.DependsOn, "uca/yoga/proposed") {
				t.Fatalf("lower backport gates selected target: %+v", step)
			}
			if !backported && step.Series == "yoga" {
				t.Fatalf("absent backport inferred as work: %+v", step)
			}
		}
		if backported {
			migrationTestStep(t, result, "uca/yoga/proposed")
		}
	}
}

func TestMigrationChainMultipleNewerWaitsAndParentRequirement(t *testing.T) {
	targets := []dto.SRUMigrationTarget{
		migrationTestTarget("gazpacho", "2026.1", "3", "2", "1", "staging"),
		migrationTestTarget("epoxy", "2025.1", "3", "2", "1", "staging"),
		migrationTestTarget("noble", "2024.1", "", "2", "1", "proposed"),
		migrationTestTarget("caracal", "2024.1", "8", "7", "7", "staging"),
	}
	targets[2].Archive = "ubuntu"
	targets[3].ParentSeries = "noble"
	targets[3].ParentRequired = true
	result := MigrationChain(migrationTestQuery(), targets, time.Now())
	step := migrationTestStep(t, result, "uca/caracal/proposed")
	for _, dependency := range []string{"uca/gazpacho/proposed", "uca/epoxy/proposed", "ubuntu/noble/updates"} {
		if !slices.Contains(step.DependsOn, dependency) {
			t.Fatalf("missing %s: %+v", dependency, step)
		}
	}
	if !slices.Contains(result.FirstOutstanding, "uca/gazpacho/occupant/updates") || !slices.Contains(result.FirstOutstanding, "uca/epoxy/occupant/updates") || !slices.Contains(result.FirstOutstanding, "ubuntu/noble/updates") {
		t.Fatalf("first=%v", result.FirstOutstanding)
	}
}

func TestMigrationChainAlreadyReleasedAndMissingSelection(t *testing.T) {
	target := migrationTestTarget("caracal", "2024.1", "", "", "3", "updates")
	result := MigrationChain(migrationTestQuery(), []dto.SRUMigrationTarget{target}, time.Now())
	if len(result.Transitions) != 0 || len(result.Warnings) == 0 {
		t.Fatalf("released=%+v", result)
	}
	result = MigrationChain(migrationTestQuery(), nil, time.Now())
	if len(result.Transitions) != 0 || len(result.Warnings) == 0 {
		t.Fatalf("missing=%+v", result)
	}
}

func TestMigrationInventoryPreservesUnassociatedLowerPublications(t *testing.T) {
	lower := migrationTestTarget("yoga", "", "3", "2", "1")
	for i := range lower.Pockets {
		lower.Pockets[i].Publications[0].BugsKnown = false
		lower.Pockets[i].Publications[0].BugIDs = nil
		lower.Pockets[i].Publications[0].Warning = "no bug references"
	}
	result := MigrationChain(migrationTestQuery(), []dto.SRUMigrationTarget{
		migrationTestTarget("caracal", "2024.1", "8", "7", "7", "staging"), lower,
	}, time.Now())
	if len(result.Inventory) != 2 || result.Inventory[1].Series != "yoga" || len(result.Inventory[1].Pockets[1].Publications) != 1 || result.Inventory[1].Pockets[1].Publications[0].Version != "2" {
		t.Fatalf("unassociated inventory lost: %+v", result.Inventory)
	}
	if result.Inventory[1].Relation != "lower; fix not observed" || len(result.Warnings) != 3 {
		t.Fatalf("unassociated inventory mislabeled: %+v", result)
	}
	if slices.ContainsFunc(result.Transitions, func(step dto.SRUMigrationTransition) bool { return step.Series == "yoga" }) {
		t.Fatalf("unassociated lower upload inferred as required work: %+v", result.Transitions)
	}
}
