// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package sru

import (
	"testing"
	"time"

	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
)

func TestVerificationTargetSpecific(t *testing.T) {
	tags := []string{"verification-done-resolute", "verification-gazpacho-done", "verification-caracal-failed"}
	for _, test := range []struct{ archive, series, state, tag string }{
		{"ubuntu", "resolute", "done", "verification-done-resolute"},
		{"ubuntu", "noble", "none", ""},
		{"uca", "gazpacho", "done", "verification-gazpacho-done"},
		{"uca", "caracal", "failed", "verification-caracal-failed"},
	} {
		state, tag := Verification(tags, test.archive, test.series)
		if state != test.state || tag != test.tag {
			t.Errorf("Verification(%s/%s) = %s/%s; want %s/%s", test.archive, test.series, state, tag, test.state, test.tag)
		}
	}
}

func TestApplyEvidenceKeepsTaskReleaseStatusSeparate(t *testing.T) {
	row := dto.SRURow{TaskStatus: "Fix Released"}
	ApplyEvidence(&row, []Evidence{{Stage: "staging", Version: "1", URL: "stage"}, {Stage: "proposed", Version: "1", URL: "proposed"}})
	if row.Stage != "proposed" || row.EvidenceURL != "proposed" || row.Warning == "" {
		t.Fatalf("unexpected classified row: %+v", row)
	}
	row.TaskStatus = "Fix Committed"
	ApplyEvidence(&row, []Evidence{{Stage: "updates", Version: "1", URL: "updates"}})
	if row.Stage != "updates" || row.Warning == "" {
		t.Fatalf("updates/task disagreement not reported: %+v", row)
	}
}

func TestReleasedTaskWithoutPublicationIsNotAttention(t *testing.T) {
	row := dto.SRURow{BugID: "1881771", TaskStatus: "Fix Released", Verification: "none"}
	ApplyEvidence(&row, nil)
	if row.Stage != "not observed" || row.Warning != "" {
		t.Fatalf("absence of archive evidence reported as contradiction: %+v", row)
	}
	// A snapshot written before this rule changed still has the old warning.
	row.Warning = "task is Fix Released; a matching updates publication was not observed"
	filtered := Filter(dto.SRUSnapshot{Rows: []dto.SRURow{row}}, dto.SRUFilter{NeedsAttention: true})
	if len(filtered.Rows) != 0 {
		t.Fatalf("historical released task selected for attention: %+v", filtered.Rows)
	}
	visible := Filter(dto.SRUSnapshot{Rows: []dto.SRURow{row}}, dto.SRUFilter{})
	if visible.Rows[0].Warning != "" {
		t.Fatalf("obsolete snapshot warning remains visible: %q", visible.Rows[0].Warning)
	}
	row.Warning += "; parent Ubuntu SRU is not observed in updates"
	visible = Filter(dto.SRUSnapshot{Rows: []dto.SRURow{row}}, dto.SRUFilter{})
	if visible.Rows[0].Warning != "parent Ubuntu SRU is not observed in updates" {
		t.Fatalf("obsolete part of combined warning remains visible: %q", visible.Rows[0].Warning)
	}
	row.Verification = "failed"
	filtered = Filter(dto.SRUSnapshot{Rows: []dto.SRURow{row}}, dto.SRUFilter{NeedsAttention: true})
	if len(filtered.Rows) != 1 {
		t.Fatal("failed verification should still require attention")
	}
}

func TestSharedCloudArchiveAssociationIsDiagnosticOnly(t *testing.T) {
	row := dto.SRURow{BugID: "2037332", Package: "aodh", Archive: "uca", Series: "yoga",
		TaskStatus: "Fix Released", Stage: "updates", Verification: "done",
		ParentRequired: true, ParentReady: true, Warning: SharedCloudArchiveTaskWarning}
	snapshot := dto.SRUSnapshot{Rows: []dto.SRURow{row}}
	if got := Filter(snapshot, dto.SRUFilter{NeedsAttention: true}); len(got.Rows) != 0 {
		t.Fatalf("completed shared task selected for attention: %+v", got.Rows)
	}
	if got := Filter(snapshot, dto.SRUFilter{}); len(got.Rows) != 1 || got.Rows[0].Warning != SharedCloudArchiveTaskWarning {
		t.Fatalf("diagnostic should remain visible in full view: %+v", got.Rows)
	}
	for _, test := range []struct {
		name string
		edit func(*dto.SRURow)
	}{
		{"failed verification", func(row *dto.SRURow) { row.Verification = "failed" }},
		{"parent blocked", func(row *dto.SRURow) { row.ParentReady = false }},
		{"other warning", func(row *dto.SRURow) {
			row.Warning += "; matching updates publication exists; task is not Fix Released"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := row
			test.edit(&changed)
			if got := Filter(dto.SRUSnapshot{Rows: []dto.SRURow{changed}}, dto.SRUFilter{NeedsAttention: true}); len(got.Rows) != 1 {
				t.Fatalf("actionable shared task omitted: %+v", got.Rows)
			}
		})
	}
}

func TestApplyEvidenceChoosesNewestVersionBeforePocket(t *testing.T) {
	row := dto.SRURow{TaskStatus: "In Progress"}
	ApplyEvidence(&row, []Evidence{
		{Stage: "updates", Version: "2:28.0.1-0ubuntu1", URL: "old"},
		{Stage: "staging", Version: "2:28.0.2-0ubuntu1~cloud0", URL: "new"},
	})
	if row.Stage != "staging" || row.EvidenceURL != "new" {
		t.Fatalf("new SRU hidden by old released version: %+v", row)
	}
}

func TestFilterSelectsAndOrdersRows(t *testing.T) {
	now := time.Now()
	snapshot := dto.SRUSnapshot{Rows: []dto.SRURow{
		{BugID: "1", Package: "neutron", PackageSets: []string{"openstack"}, Archive: "uca", Series: "gazpacho", Verification: "failed", UpdatedAt: now},
		{BugID: "2", Package: "nova", Archive: "ubuntu", Series: "resolute", UpdatedAt: now.Add(time.Hour)},
	}}
	got := Filter(snapshot, dto.SRUFilter{Package: "neutron", Archive: "uca"})
	if len(got.Rows) != 1 || got.Rows[0].BugID != "1" || len(snapshot.Rows) != 2 {
		t.Fatalf("unexpected filtered snapshot: %+v", got)
	}
	got = Filter(snapshot, dto.SRUFilter{Set: "openstack", NeedsAttention: true})
	if len(got.Rows) != 1 || got.Rows[0].BugID != "1" {
		t.Fatalf("attention/package-set filter: %+v", got.Rows)
	}
}
