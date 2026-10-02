// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package teamsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// TeamCollaboratorSyncer reconciles the members of a Launchpad team against
// the collaborator lists of a set of store artifacts. It is the consumption
// contract primary adapters depend on; *Service is the canonical
// implementation.
type TeamCollaboratorSyncer interface {
	Sync(ctx context.Context, teamName string, targets []dto.SyncTarget, dryRun bool) (*dto.TeamSyncResult, error)
}

// Service coordinates team collaborator synchronization.
type Service struct {
	teamProvider port.LaunchpadTeamProvider
	stores       map[dto.ArtifactType]port.StoreCollaboratorManager
	logger       *slog.Logger
}

// NewService creates a team sync service.
func NewService(
	teamProvider port.LaunchpadTeamProvider,
	stores map[dto.ArtifactType]port.StoreCollaboratorManager,
	logger *slog.Logger,
) *Service {
	return &Service{teamProvider: teamProvider, stores: stores, logger: logger}
}

// Sync compares LP team members against store collaborators for each target.
func (s *Service) Sync(ctx context.Context, teamName string, targets []dto.SyncTarget, dryRun bool) (*dto.TeamSyncResult, error) {
	members, err := s.teamProvider.GetTeamMembers(ctx, teamName)
	if err != nil {
		return nil, fmt.Errorf("fetching team %s members: %w", teamName, err)
	}

	result := &dto.TeamSyncResult{}

	// Separate members with and without emails.
	var memberEmails []string
	for _, m := range members {
		if m.Email == "" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("member %q has hidden email — cannot match to store collaborators", m.Username))
			continue
		}
		memberEmails = append(memberEmails, m.Email)
	}
	teamSet := toSet(memberEmails)

	for _, target := range targets {
		art := s.syncTarget(ctx, target, teamSet, dryRun)
		result.Artifacts = append(result.Artifacts, art)
	}
	return result, nil
}

func (s *Service) syncTarget(ctx context.Context, target dto.SyncTarget, teamEmails map[string]bool, dryRun bool) dto.ArtifactSyncResult {
	art := dto.ArtifactSyncResult{
		Project:      target.Project,
		ArtifactType: target.ArtifactType,
		StoreName:    target.StoreName,
	}

	store, ok := s.stores[target.ArtifactType]
	if !ok {
		art.Error = fmt.Sprintf("no store manager for artifact type %s", target.ArtifactType)
		return art
	}

	collabs, err := store.ListCollaborators(ctx, target.StoreName)
	if err != nil {
		if errors.Is(err, port.ErrCollaboratorsUnsupported) {
			art.Unsupported = true
			art.UnsupportedURL = unsupportedDashboardURL(target)
			return art
		}
		if errors.Is(err, port.ErrStoreAuthExpired) {
			art.AuthExpired = true
			art.AuthHint = reauthHint(target)
			return art
		}
		art.Error = fmt.Sprintf("listing collaborators for %s: %v", target.StoreName, err)
		return art
	}

	collabPending := make(map[string]bool)
	allCollabEmails := make(map[string]bool)
	for _, c := range collabs {
		email := strings.ToLower(c.Email)
		if email == "" {
			continue
		}
		allCollabEmails[email] = true
		if c.Status == "pending" {
			collabPending[email] = true
		}
	}

	// Missing: in team, not in store.
	for email := range teamEmails {
		lower := strings.ToLower(email)
		if allCollabEmails[lower] {
			if collabPending[lower] {
				art.Pending = append(art.Pending, email)
			}
			continue
		}
		art.Invited = append(art.Invited, email)
		if !dryRun {
			if err := store.InviteCollaborator(ctx, target.StoreName, email); err != nil {
				s.logger.Warn("failed to invite collaborator", "store", target.StoreName, "email", email, "error", err)
			}
		}
	}

	// Extra: in store, not in team.
	for _, c := range collabs {
		lower := strings.ToLower(c.Email)
		if !teamEmails[lower] {
			art.Extra = append(art.Extra, collaboratorIdentity(c))
		}
	}

	if len(art.Invited) == 0 && len(art.Extra) == 0 && len(art.Pending) == 0 {
		art.AlreadySync = true
	}

	return art
}

func collaboratorIdentity(c dto.StoreCollaborator) string {
	switch {
	case c.Email != "":
		return c.Email
	case c.Username != "":
		return c.Username
	case c.DisplayName != "":
		return c.DisplayName
	default:
		return "(unknown collaborator)"
	}
}

// reauthHint returns the operator-facing command to re-authenticate with
// the artifact's backing store. Returns an empty string when the artifact
// type has no known login command.
func reauthHint(target dto.SyncTarget) string {
	switch target.ArtifactType {
	case dto.ArtifactCharm:
		return `run "watchtower auth charmhub login"`
	case dto.ArtifactSnap:
		return `run "watchtower auth snapstore login"`
	default:
		return ""
	}
}

// unsupportedDashboardURL returns the store-side web UI URL where operators
// can manage collaborators by hand for artifacts whose stores have no public
// collaborator API. Only populated for snaps today; other types fall back to
// an empty string and rely on ArtifactSyncResult.Unsupported alone.
func unsupportedDashboardURL(target dto.SyncTarget) string {
	if target.ArtifactType == dto.ArtifactSnap {
		return fmt.Sprintf("https://dashboard.snapcraft.io/snaps/%s/collaboration/", target.StoreName)
	}
	return ""
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[strings.ToLower(s)] = true
	}
	return m
}
