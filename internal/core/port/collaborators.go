// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"
	"errors"
	"fmt"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ErrCollaboratorsUnsupported signals that a store backend does not support
// per-artifact collaborator management through a public API. Adapters return
// this sentinel (wrapped with an operator-facing message) when there is no
// safe programmatic path and operators must use the store's web UI instead.
var ErrCollaboratorsUnsupported = errors.New("store collaborator management is unsupported")

// ErrStoreAuthExpired signals that a store adapter rejected the request
// because its credentials were missing, expired, or otherwise unaccepted.
// Service and CLI layers use this sentinel to surface an actionable
// re-authentication hint instead of the raw HTTP status.
var ErrStoreAuthExpired = errors.New("store authentication expired or invalid")

// ErrCharmhubReloginRequired signals that the stored Charmhub discharged
// bundle can no longer be re-exchanged for a fresh publisher token and a
// full interactive login is required. Wraps ErrStoreAuthExpired so generic
// store-auth handling keeps working.
var ErrCharmhubReloginRequired = fmt.Errorf(
	"charmhub re-login required: run `watchtower auth charmhub login`: %w",
	ErrStoreAuthExpired,
)

// StoreCollaboratorManager manages collaborators on a backing store artifact.
type StoreCollaboratorManager interface {
	ListCollaborators(ctx context.Context, storeName string) ([]dto.StoreCollaborator, error)
	InviteCollaborator(ctx context.Context, storeName string, email string) error
}

// LaunchpadTeamProvider fetches members of a Launchpad team.
type LaunchpadTeamProvider interface {
	GetTeamMembers(ctx context.Context, teamName string) ([]dto.TeamMember, error)
}
