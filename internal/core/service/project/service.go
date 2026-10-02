// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// ActionType describes the kind of sync action.
type ActionType = dto.ProjectSyncActionType

const (
	ActionCreateSeries      ActionType = dto.ProjectSyncActionCreateSeries
	ActionSetDevFocus       ActionType = dto.ProjectSyncActionSetDevFocus
	ActionDevFocusUnchanged ActionType = dto.ProjectSyncActionDevFocusUnchanged
)

// SyncAction represents a single action taken (or planned) during sync.
type SyncAction = dto.ProjectSyncAction

// SyncResult holds the outcome of a sync operation.
type SyncResult = dto.ProjectSyncResult

// SyncOptions controls the sync behavior.
type SyncOptions struct {
	Projects []string // filter to these LP project names (empty = all)
	DryRun   bool
}

// ProjectSyncConfig holds the series and development focus for a single LP project.
type ProjectSyncConfig struct {
	Series           []string
	DevelopmentFocus string
}

// Service performs project metadata synchronization on LP.
type Service struct {
	manager        port.ProjectManager
	projectConfigs map[string]ProjectSyncConfig
	logger         *slog.Logger
}

// NewService creates a project sync service.
// projectConfigs maps LP project names to their series/dev-focus configuration.
func NewService(
	manager port.ProjectManager,
	projectConfigs map[string]ProjectSyncConfig,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		manager:        manager,
		projectConfigs: projectConfigs,
		logger:         logger,
	}
}

// Sync ensures every LP project has the declared series and development focus.
func (s *Service) Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	projFilter := make(map[string]bool, len(opts.Projects))
	for _, p := range opts.Projects {
		projFilter[p] = true
	}

	result := &SyncResult{}

	for lpProject, cfg := range s.projectConfigs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if len(projFilter) > 0 && !projFilter[lpProject] {
			continue
		}

		s.logger.Debug("syncing project", "project", lpProject)

		if err := s.syncProject(ctx, lpProject, cfg, opts.DryRun, result); err != nil {
			s.logger.Warn("failed to sync project", "project", lpProject, "error", err)
			result.Errors = append(result.Errors, fmt.Errorf("project %s: %w", lpProject, err))
		}
	}

	return result, nil
}

func (s *Service) syncProject(ctx context.Context, lpProject string, cfg ProjectSyncConfig, dryRun bool, result *SyncResult) error {
	// Fetch existing series.
	existing, err := s.manager.GetProjectSeries(ctx, lpProject)
	if err != nil {
		return fmt.Errorf("fetching series: %w", err)
	}

	existingSet := make(map[string]forge.ProjectSeries, len(existing))
	for _, ps := range existing {
		existingSet[ps.Name] = ps
	}

	// Ensure each declared series exists.
	for _, seriesName := range cfg.Series {
		if _, ok := existingSet[seriesName]; ok {
			s.logger.Debug("series already exists", "project", lpProject, "series", seriesName)
			continue
		}

		action := SyncAction{
			Project:    lpProject,
			Series:     seriesName,
			ActionType: ActionCreateSeries,
		}

		if !dryRun {
			created, err := s.manager.CreateSeries(ctx, lpProject, seriesName, seriesName+" series")
			if err != nil {
				s.logger.Warn("failed to create series", "project", lpProject, "series", seriesName, "error", err)
				result.Errors = append(result.Errors, fmt.Errorf("project %s series %s: %w", lpProject, seriesName, err))
				continue
			}
			// Store in map so dev focus resolution can find it.
			existingSet[seriesName] = forge.ProjectSeries{
				Name:     created.Name,
				SelfLink: created.SelfLink,
				Active:   created.Active,
			}
		}

		result.Actions = append(result.Actions, action)
	}

	// Set development focus if configured.
	if cfg.DevelopmentFocus == "" {
		return nil
	}

	targetSeries, ok := existingSet[cfg.DevelopmentFocus]
	if !ok {
		// In dry-run, series may not have been created yet — still plan the action.
		if dryRun {
			result.Actions = append(result.Actions, SyncAction{
				Project:    lpProject,
				Series:     cfg.DevelopmentFocus,
				ActionType: ActionSetDevFocus,
				NewValue:   cfg.DevelopmentFocus,
			})
			return nil
		}
		return fmt.Errorf("development focus series %q not found on project %s", cfg.DevelopmentFocus, lpProject)
	}

	// Check current development focus.
	proj, err := s.manager.GetProject(ctx, lpProject)
	if err != nil {
		return fmt.Errorf("fetching project: %w", err)
	}

	if proj.DevelopmentFocusLink == targetSeries.SelfLink {
		s.logger.Debug("development focus already set", "project", lpProject, "series", cfg.DevelopmentFocus)
		result.Actions = append(result.Actions, SyncAction{
			Project:    lpProject,
			Series:     cfg.DevelopmentFocus,
			ActionType: ActionDevFocusUnchanged,
		})
		return nil
	}

	action := SyncAction{
		Project:    lpProject,
		Series:     cfg.DevelopmentFocus,
		ActionType: ActionSetDevFocus,
		OldValue:   proj.DevelopmentFocusLink,
		NewValue:   targetSeries.SelfLink,
	}

	if !dryRun {
		if err := s.manager.SetDevelopmentFocus(ctx, lpProject, targetSeries.SelfLink); err != nil {
			return fmt.Errorf("setting development focus: %w", err)
		}
	}

	result.Actions = append(result.Actions, action)
	return nil
}
