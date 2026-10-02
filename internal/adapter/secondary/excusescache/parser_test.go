// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package excusescache

import (
	"strings"
	"testing"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestParseUbuntuExcusesYAML(t *testing.T) {
	data := `
sources:
  - item-name: nova
    source: nova
    component: main
    old-version: 1:29.0.0-0ubuntu1
    new-version: 1:29.1.0-0ubuntu1
    reason:
      - autopkgtest
    excuses:
      - "autopkgtest for nova/1:29.1.0-0ubuntu1 failed on amd64"
    missing-builds:
      on-architectures:
        - arm64
    dependencies:
      blocked-by:
        - keystone
      migrate-after:
        - placement
    policy_info:
      age:
        current-age: 4
      update-excuse:
        "1234567": true
`
	results, err := parseExcusesYAML([]byte(data), dto.ExcusesSource{Tracker: dto.ExcusesTrackerUbuntu})
	if err != nil {
		t.Fatalf("parseExcusesYAML() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got := results[0]
	if got.Package != "nova" {
		t.Fatalf("expected package nova, got %q", got.Package)
	}
	if got.PrimaryReason != "ftbfs" {
		t.Fatalf("expected primary reason ftbfs, got %q", got.PrimaryReason)
	}
	if !got.FTBFS {
		t.Fatal("expected FTBFS to be true")
	}
	if got.AgeDays != 4 {
		t.Fatalf("expected age 4, got %d", got.AgeDays)
	}
	if got.Bug != "LP: #1234567" {
		t.Fatalf("expected LP bug, got %q", got.Bug)
	}
	if len(got.BlockedBy) != 1 || got.BlockedBy[0] != "keystone" {
		t.Fatalf("expected blocked-by keystone, got %#v", got.BlockedBy)
	}
}

func TestParseDebianExcusesYAML(t *testing.T) {
	data := `
generated-date: 2026-03-06 17:13:07.261525+00:00
sources:
  - excuses:
      - "Migration status for biomaj3-cli (3.1.11-5 to -): Will attempt migration"
      - "Additional info (not blocking):"
      - "∙ ∙ Removal request by auto-removals"
    hints:
      - hint-from: auto-removals
        hint-type: remove
    is-candidate: true
    item-name: -biomaj3-cli
    migration-policy-verdict: PASS
    new-version: "-"
    old-version: 3.1.11-5
    reason: []
    source: biomaj3-cli
    maintainer: Debian Python Team
`
	results, err := parseExcusesYAML([]byte(data), dto.ExcusesSource{Tracker: dto.ExcusesTrackerDebian})
	if err != nil {
		t.Fatalf("parseExcusesYAML() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got := results[0]
	if got.Package != "biomaj3-cli" {
		t.Fatalf("expected package biomaj3-cli, got %q", got.Package)
	}
	if got.Maintainer != "Debian Python Team" {
		t.Fatalf("expected maintainer, got %q", got.Maintainer)
	}
	if !got.Candidate {
		t.Fatal("expected candidate to be true")
	}
	if got.PrimaryReason != "removal" {
		t.Fatalf("expected primary reason removal, got %q", got.PrimaryReason)
	}
	if len(got.Messages) == 0 || !strings.Contains(got.Messages[0], "Migration status") {
		t.Fatalf("expected parsed messages, got %#v", got.Messages)
	}
}

func TestParseUbuntuExcusesByTeamYAML(t *testing.T) {
	data := `
debcrafters-packages:
- kind: package-in-proposed
  package_in_proposed: libimagequant
  regressions: []
  waiting: []
  data: !!python/object/apply:collections.defaultdict
    args:
    - &id001 !!python/name:builtins.dict ''
    dictitems:
      item-name: libimagequant
      new-version: 4.4.1-1
      old-version: 2.18.0-1build1
      source: libimagequant
ubuntu-desktop:
- kind: package-in-proposed
  package_in_proposed: gnome-shell
  regressions: []
  waiting: []
  data: !!python/object/apply:collections.defaultdict
    args:
    - *id001
    dictitems:
      item-name: gnome-shell
      new-version: 49.0-1ubuntu1
      source: gnome-shell
`
	exactTeams, packageTeams, err := parseUbuntuExcusesByTeamYAML([]byte(data))
	if err != nil {
		t.Fatalf("parseUbuntuExcusesByTeamYAML() error = %v", err)
	}
	if got := exactTeams[recordKey("libimagequant", "4.4.1-1")]; got != "debcrafters-packages" {
		t.Fatalf("expected exact libimagequant team, got %q", got)
	}
	if got := packageTeams["gnome-shell"]; got != "ubuntu-desktop" {
		t.Fatalf("expected gnome-shell team, got %q", got)
	}
}

func TestParseUbuntuExcuse_HTMLAutopkgtests(t *testing.T) {
	data := `
sources:
  - item-name: alembic
    source: alembic
    new-version: 1.18.4-1
    old-version: 1.16.4-4
    excuses:
      - 'autopkgtest for alembic/1.18.4-1: <a href="https://autopkgtest.ubuntu.com/packages/a/alembic/resolute/amd64">amd64</a>: <a href="https://example.com/log1"><span style="background:#87d96c">Pass</span></a>, <a href="https://autopkgtest.ubuntu.com/packages/a/alembic/resolute/arm64">arm64</a>: <a href="https://example.com/log2"><span style="background:#99ddff">Test in progress</span></a>'
      - "Additional info:"
    reason:
      - autopkgtest
    policy_info:
      age:
        current-age: 3
`
	results, err := parseExcusesYAML([]byte(data), dto.ExcusesSource{Tracker: dto.ExcusesTrackerUbuntu})
	if err != nil {
		t.Fatalf("parseExcusesYAML() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got := results[0]
	if len(got.Autopkgtests) != 2 {
		t.Fatalf("expected 2 autopkgtest entries, got %d", len(got.Autopkgtests))
	}
	if got.Autopkgtests[0].Package != "alembic/1.18.4-1" {
		t.Errorf("Autopkgtests[0].Package = %q", got.Autopkgtests[0].Package)
	}
	if got.Autopkgtests[0].Architecture != "amd64" {
		t.Errorf("Autopkgtests[0].Architecture = %q", got.Autopkgtests[0].Architecture)
	}
	if got.Autopkgtests[0].Status != "pass" {
		t.Errorf("Autopkgtests[0].Status = %q", got.Autopkgtests[0].Status)
	}
	if got.Autopkgtests[0].URL != "https://example.com/log1" {
		t.Errorf("Autopkgtests[0].URL = %q", got.Autopkgtests[0].URL)
	}
	if got.Autopkgtests[1].Architecture != "arm64" {
		t.Errorf("Autopkgtests[1].Architecture = %q", got.Autopkgtests[1].Architecture)
	}
	if got.Autopkgtests[1].Status != "in-progress" {
		t.Errorf("Autopkgtests[1].Status = %q", got.Autopkgtests[1].Status)
	}
	// Messages should be clean (no autopkgtest lines, no HTML) and contain
	// only the residual human-readable lines.
	if len(got.Messages) != 1 || !strings.Contains(got.Messages[0], "Additional info") {
		t.Errorf("expected Messages to contain only the non-autopkgtest line, got %#v", got.Messages)
	}
	for _, msg := range got.Messages {
		if strings.Contains(msg, "<a") || strings.Contains(msg, "<span") {
			t.Errorf("Messages should not contain HTML: %q", msg)
		}
		if strings.Contains(strings.ToLower(msg), "autopkgtest") {
			t.Errorf("autopkgtest lines should be separated from Messages: %q", msg)
		}
	}
}

func TestApplyExcuseTeams(t *testing.T) {
	excuses := []dto.PackageExcuse{
		{PackageExcuseSummary: dto.PackageExcuseSummary{Package: "libimagequant", Version: "4.4.1-1"}},
		{PackageExcuseSummary: dto.PackageExcuseSummary{Package: "gnome-shell", Version: "49.0-1ubuntu1"}},
	}

	applyExcuseTeams(excuses,
		map[string]string{recordKey("libimagequant", "4.4.1-1"): "debcrafters-packages"},
		map[string]string{"gnome-shell": "ubuntu-desktop"},
	)

	if excuses[0].Team != "debcrafters-packages" {
		t.Fatalf("expected exact team on libimagequant, got %q", excuses[0].Team)
	}
	if excuses[1].Team != "ubuntu-desktop" {
		t.Fatalf("expected package fallback team on gnome-shell, got %q", excuses[1].Team)
	}
}
