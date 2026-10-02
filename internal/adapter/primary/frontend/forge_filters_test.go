// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"errors"
	"testing"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func TestParseForgeTypeAndMergeState(t *testing.T) {
	forgeType, err := parseForgeType("launchpad")
	if err != nil {
		t.Fatalf("parseForgeType() error = %v", err)
	}
	if forgeType != forge.ForgeLaunchpad {
		t.Fatalf("parseForgeType() = %v, want %v", forgeType, forge.ForgeLaunchpad)
	}

	state, err := parseMergeState("merged")
	if err != nil {
		t.Fatalf("parseMergeState() error = %v", err)
	}
	if state != forge.MergeStateMerged {
		t.Fatalf("parseMergeState() = %v, want %v", state, forge.MergeStateMerged)
	}
}

func TestParseForgeTypeAndMergeStateRejectInvalidValues(t *testing.T) {
	if _, err := parseForgeType("nope"); err == nil {
		t.Fatal("parseForgeType() expected error")
	}
	if _, err := parseMergeState("nope"); err == nil {
		t.Fatal("parseMergeState() expected error")
	}
	if !errors.Is(ErrNoBugTrackerConfigured, ErrNoBugTrackerConfigured) {
		t.Fatal("expected ErrNoBugTrackerConfigured to remain comparable")
	}
}
