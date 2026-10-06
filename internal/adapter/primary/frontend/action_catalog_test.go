// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import "testing"

func TestAllActionsAreUniqueAndDescribed(t *testing.T) {
	actions := AllActions()
	if len(actions) == 0 {
		t.Fatal("AllActions() returned no actions")
	}

	seen := make(map[ActionID]struct{}, len(actions))
	for _, action := range actions {
		if action.ID == "" {
			t.Fatal("action ID should not be empty")
		}
		if _, ok := seen[action.ID]; ok {
			t.Fatalf("duplicate action ID %q", action.ID)
		}
		seen[action.ID] = struct{}{}

		got := DescribeAction(action.ID)
		if got.ID != action.ID {
			t.Fatalf("DescribeAction(%q).ID = %q", action.ID, got.ID)
		}
	}
}

func TestReadOnlyExportedActionsExcludeWritesAndHidden(t *testing.T) {
	actions := ReadOnlyExportedActions()
	if len(actions) == 0 {
		t.Fatal("ReadOnlyExportedActions() returned no actions")
	}

	for _, action := range actions {
		if action.Mutability != MutabilityRead {
			t.Fatalf("action %q mutability = %q, want read", action.ID, action.Mutability)
		}
		if action.ExportPolicy != ExportPolicyAllowed {
			t.Fatalf("action %q export policy = %q, want allowed", action.ID, action.ExportPolicy)
		}
	}
}

func TestIsAllowedInReadOnlyMode(t *testing.T) {
	tests := []struct {
		name     string
		actionID ActionID
		override bool
		want     bool
	}{
		{name: "read action without override", actionID: ActionBuildDownload, want: true},
		{name: "write action without override", actionID: ActionBuildTrigger, want: false},
		{name: "write action with override", actionID: ActionBuildTrigger, override: true, want: true},
		{name: "hidden write action remains blocked without override", actionID: ActionServerStart, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAllowedInReadOnlyMode(tt.actionID, tt.override); got != tt.want {
				t.Fatalf("IsAllowedInReadOnlyMode(%q, %t) = %t, want %t", tt.actionID, tt.override, got, tt.want)
			}
		})
	}
}

func TestDryRunAndApplyVariantsDiffer(t *testing.T) {
	tests := []struct {
		readAction  ActionID
		writeAction ActionID
	}{
		{readAction: ActionBuildCleanupDryRun, writeAction: ActionBuildCleanupApply},
		{readAction: ActionProjectSyncDryRun, writeAction: ActionProjectSyncApply},
		{readAction: ActionBugSyncDryRun, writeAction: ActionBugSyncApply},
		{readAction: ActionTeamSyncDryRun, writeAction: ActionTeamSyncApply},
	}

	for _, tt := range tests {
		readDesc := DescribeAction(tt.readAction)
		writeDesc := DescribeAction(tt.writeAction)
		if readDesc.Mutability != MutabilityRead {
			t.Fatalf("%q mutability = %q, want read", tt.readAction, readDesc.Mutability)
		}
		if writeDesc.Mutability != MutabilityWrite {
			t.Fatalf("%q mutability = %q, want write", tt.writeAction, writeDesc.Mutability)
		}
	}
}

func TestLogsRefreshIsHiddenReadOnlyLocal(t *testing.T) {
	desc := DescribeAction(ActionLogsRefresh)
	if desc.Mutability != MutabilityRead {
		t.Fatalf("Mutability = %q, want read", desc.Mutability)
	}
	if desc.LocalEffect != LocalEffectRead {
		t.Fatalf("LocalEffect = %q, want local_read", desc.LocalEffect)
	}
	if desc.ExportPolicy != ExportPolicyHidden {
		t.Fatalf("ExportPolicy = %q, want hidden", desc.ExportPolicy)
	}
}

func TestSRUMigrationAccessClassification(t *testing.T) {
	action := DescribeAction(ActionSRUMigration)
	if action.Mutability != MutabilityRead || action.LocalEffect != LocalEffectRead || action.RuntimeRequirement != RuntimeEmbeddedOK || action.ExportPolicy != ExportPolicyAllowed {
		t.Fatalf("migration classification=%+v", action)
	}
}
