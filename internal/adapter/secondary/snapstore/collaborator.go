// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package snapstore

import (
	"context"
	"fmt"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// Compile-time interface compliance check.
var _ port.StoreCollaboratorManager = (*CollaboratorManager)(nil)

// CollaboratorManager implements port.StoreCollaboratorManager for the Snap
// Store. Per-snap collaborator management is intentionally unsupported: the
// real endpoint lives inside the closed-source dashboard.snapcraft.io Django
// app and is not publicly documented, and none of the open-source Canonical
// clients expose it. See docs/agents/specs/snapstore-collaborator-api.md for
// the research behind this decision. Both methods return
// port.ErrCollaboratorsUnsupported wrapped with a message pointing operators
// at the dashboard UI.
type CollaboratorManager struct{}

// NewCollaboratorManager creates a CollaboratorManager for the Snap Store.
func NewCollaboratorManager() *CollaboratorManager {
	return &CollaboratorManager{}
}

// DashboardCollaborationURL returns the dashboard URL where operators can
// manage collaborators for the given snap by hand.
func DashboardCollaborationURL(snapName string) string {
	return fmt.Sprintf("https://dashboard.snapcraft.io/snaps/%s/collaboration/", snapName)
}

// ListCollaborators always returns port.ErrCollaboratorsUnsupported.
func (m *CollaboratorManager) ListCollaborators(_ context.Context, storeName string) ([]dto.StoreCollaborator, error) {
	return nil, unsupportedError(storeName)
}

// InviteCollaborator always returns port.ErrCollaboratorsUnsupported.
func (m *CollaboratorManager) InviteCollaborator(_ context.Context, storeName, _ string) error {
	return unsupportedError(storeName)
}

func unsupportedError(storeName string) error {
	return fmt.Errorf("snap store per-snap collaborator management is unsupported; manage collaborators at %s: %w",
		DashboardCollaborationURL(storeName), port.ErrCollaboratorsUnsupported)
}
