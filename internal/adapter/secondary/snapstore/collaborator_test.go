// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package snapstore

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
)

func TestCollaboratorManager_ListCollaborators_Unsupported(t *testing.T) {
	mgr := NewCollaboratorManager()

	got, err := mgr.ListCollaborators(context.Background(), "my-snap")
	if err == nil {
		t.Fatal("ListCollaborators() expected error, got nil")
	}
	if got != nil {
		t.Fatalf("ListCollaborators() = %v, want nil slice", got)
	}
	if !errors.Is(err, port.ErrCollaboratorsUnsupported) {
		t.Fatalf("errors.Is(err, ErrCollaboratorsUnsupported) = false; err = %v", err)
	}
	if want := "https://dashboard.snapcraft.io/snaps/my-snap/collaboration/"; !strings.Contains(err.Error(), want) {
		t.Fatalf("ListCollaborators() error = %q, want containing %q", err.Error(), want)
	}
}

func TestCollaboratorManager_InviteCollaborator_Unsupported(t *testing.T) {
	mgr := NewCollaboratorManager()

	err := mgr.InviteCollaborator(context.Background(), "another-snap", "carol@example.com")
	if err == nil {
		t.Fatal("InviteCollaborator() expected error, got nil")
	}
	if !errors.Is(err, port.ErrCollaboratorsUnsupported) {
		t.Fatalf("errors.Is(err, ErrCollaboratorsUnsupported) = false; err = %v", err)
	}
	if want := "https://dashboard.snapcraft.io/snaps/another-snap/collaboration/"; !strings.Contains(err.Error(), want) {
		t.Fatalf("InviteCollaborator() error = %q, want containing %q", err.Error(), want)
	}
}

func TestDashboardCollaborationURL(t *testing.T) {
	got := DashboardCollaborationURL("my-snap")
	want := "https://dashboard.snapcraft.io/snaps/my-snap/collaboration/"
	if got != want {
		t.Fatalf("DashboardCollaborationURL() = %q, want %q", got, want)
	}
}
