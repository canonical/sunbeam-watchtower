// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/charmhub"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/snapstore"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/teamsync"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
	"gopkg.in/yaml.v3"
)

// lpTeamProvider adapts the LP client to the port.LaunchpadTeamProvider interface.
type lpTeamProvider struct {
	client         *lp.Client
	emailOverrides map[string]string // username → email
}

// Compile-time interface compliance check.
var _ port.LaunchpadTeamProvider = (*lpTeamProvider)(nil)

func (p *lpTeamProvider) GetTeamMembers(ctx context.Context, teamName string) ([]dto.TeamMember, error) {
	members, err := p.client.GetTeamMembersWithEmails(ctx, teamName)
	if err != nil {
		return nil, err
	}
	result := make([]dto.TeamMember, len(members))
	for i, m := range members {
		email := m.Email
		if email == "" {
			if override, ok := p.emailOverrides[m.Username]; ok {
				email = override
			}
		}
		result[i] = dto.TeamMember{Username: m.Username, Email: email}
	}
	return result, nil
}

// loadEmailOverrides reads a YAML file mapping LP usernames to email addresses.
// Returns nil map if path is empty or file doesn't exist.
func loadEmailOverrides(path string) (map[string]string, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading email overrides: %w", err)
	}
	var overrides map[string]string
	if err := yaml.Unmarshal(data, &overrides); err != nil {
		return nil, fmt.Errorf("parsing email overrides: %w", err)
	}
	return overrides, nil
}

// TeamSyncService returns the lazily initialized team sync service.
func (a *App) TeamSyncService() (*teamsync.Service, error) {
	a.teamSyncServiceOnce.Do(func() {
		cfg := a.GetConfig()
		if cfg == nil {
			a.teamSyncServiceErr = fmt.Errorf("no configuration loaded")
			return
		}
		if cfg.Collaborators == nil {
			a.teamSyncServiceErr = fmt.Errorf("collaborators not configured")
			return
		}

		lpClient := newLaunchpadClient(a.LaunchpadCredentialStore(), a.Logger, a.upstreamHTTPClient("launchpad", 2*time.Minute))
		if lpClient == nil {
			a.teamSyncServiceErr = ErrLaunchpadAuthRequired
			return
		}

		emailOverrides, err := loadEmailOverrides(cfg.Collaborators.EmailOverrides)
		if err != nil {
			a.teamSyncServiceErr = err
			return
		}

		teamProvider := &lpTeamProvider{client: lpClient, emailOverrides: emailOverrides}

		// The snap store adapter ignores credentials because per-snap
		// collaborator management is intentionally unsupported (see
		// docs/agents/specs/snapstore-collaborator-api.md). The Charmhub
		// provider re-reads the store on every request and can re-exchange
		// the stored discharged bundle when the short-lived token expires.
		charmhubProvider := charmhub.NewCredentialProvider(
			a.CharmhubCredentialStore(),
			charmhub.NewAuthenticator(a.Logger, a.upstreamHTTPClient("charmhub", 30*time.Second)),
		)

		stores := map[dto.ArtifactType]port.StoreCollaboratorManager{
			dto.ArtifactSnap:  snapstore.NewCollaboratorManager(),
			dto.ArtifactCharm: charmhub.NewCollaboratorManager(charmhubProvider),
		}

		a.teamSyncService = teamsync.NewService(teamProvider, stores, a.Logger)
	})
	return a.teamSyncService, a.teamSyncServiceErr
}
