// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/artifactdiscovery"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/teamsync"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// TeamServerWorkflow exposes reusable server-side team workflows for the HTTP API.
type TeamServerWorkflow struct {
	application *app.App
	async       *Facade

	// Injected for tests. When nil, each is resolved from application.
	cfg        *config.Config
	cache      port.RepoEnsurer
	discoverer artifactdiscovery.ArtifactDiscoverer
	syncer     teamsync.TeamCollaboratorSyncer
}

// NewTeamServerWorkflow creates a server-side team workflow.
func NewTeamServerWorkflow(application *app.App, async *Facade) *TeamServerWorkflow {
	return &TeamServerWorkflow{
		application: application,
		async:       async,
	}
}

// Sync performs one synchronous team collaborator sync, fanning out one
// SyncTarget per artifact discovered in each project's repository.
func (w *TeamServerWorkflow) Sync(ctx context.Context, req dto.TeamSyncRequest) (*dto.TeamSyncResult, error) {
	cfg := w.getConfig()
	if cfg == nil {
		return nil, errors.New("no configuration loaded")
	}
	if cfg.Collaborators == nil {
		return nil, errors.New("collaborators not configured")
	}

	syncer, err := w.resolveSyncer()
	if err != nil {
		return nil, err
	}
	cache, err := w.resolveCache()
	if err != nil {
		return nil, err
	}
	discoverer, err := w.resolveDiscoverer()
	if err != nil {
		return nil, err
	}

	var warnings []string
	var targets []dto.SyncTarget
	for _, proj := range cfg.Projects {
		if len(req.Projects) > 0 && !slices.Contains(req.Projects, proj.Name) {
			continue
		}
		at, err := dto.ParseArtifactType(proj.ArtifactType)
		if err != nil || (at != dto.ArtifactSnap && at != dto.ArtifactCharm) {
			continue
		}
		cloneURL, err := proj.Code.CloneURL()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: resolving clone URL: %v", proj.Name, err))
			continue
		}
		repoPath, err := cache.EnsureRepo(ctx, cloneURL, nil)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: caching repo: %v", proj.Name, err))
			continue
		}
		artifacts, err := discoverer.Discover(ctx, repoPath, at)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: discovering artifacts: %v", proj.Name, err))
			continue
		}
		if len(artifacts) == 0 {
			warnings = append(warnings, fmt.Sprintf("%s: no %s artifacts discovered", proj.Name, at))
			continue
		}
		for _, a := range artifacts {
			if shouldSkipTeamArtifact(proj, a.Name) {
				warnings = append(warnings, fmt.Sprintf("%s: skipped artifact %s (team.skip_artifacts)", proj.Name, a.Name))
				continue
			}
			targets = append(targets, dto.SyncTarget{
				Project:      proj.Name,
				ArtifactType: at,
				StoreName:    a.Name,
			})
		}
	}

	result, err := syncer.Sync(ctx, cfg.Collaborators.LaunchpadTeam, targets, req.DryRun)
	if err != nil {
		return nil, err
	}
	// Surface discovery warnings before any warnings the syncer emitted.
	result.Warnings = append(append([]string(nil), warnings...), result.Warnings...)
	return result, nil
}

// StartSync queues one asynchronous team collaborator sync. The async runner
// ultimately invokes Sync above, so fan-out happens exactly once.
func (w *TeamServerWorkflow) StartSync(ctx context.Context, req dto.TeamSyncRequest) (*dto.OperationJob, error) {
	return w.async.StartTeamSync(ctx, TeamSyncOptions{
		Projects: req.Projects,
		DryRun:   req.DryRun,
	})
}

func (w *TeamServerWorkflow) getConfig() *config.Config {
	if w.cfg != nil {
		return w.cfg
	}
	if w.application == nil {
		return nil
	}
	return w.application.GetConfig()
}

func (w *TeamServerWorkflow) resolveSyncer() (teamsync.TeamCollaboratorSyncer, error) {
	if w.syncer != nil {
		return w.syncer, nil
	}
	return w.application.TeamSyncService()
}

func (w *TeamServerWorkflow) resolveCache() (port.RepoEnsurer, error) {
	if w.cache != nil {
		return w.cache, nil
	}
	return w.application.GitCache()
}

func (w *TeamServerWorkflow) resolveDiscoverer() (artifactdiscovery.ArtifactDiscoverer, error) {
	if w.discoverer != nil {
		return w.discoverer, nil
	}
	return w.application.ArtifactDiscoveryService()
}

// shouldSkipTeamArtifact honours the per-project team.skip_artifacts filter.
// The filter is a no-op when Team is not configured, keeping behaviour
// backward-compatible until a project opts in.
func shouldSkipTeamArtifact(proj config.ProjectConfig, name string) bool {
	if proj.Team == nil || len(proj.Team.SkipArtifacts) == 0 {
		return false
	}
	for _, s := range proj.Team.SkipArtifacts {
		if s == name {
			return true
		}
	}
	return false
}
