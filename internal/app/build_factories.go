// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"io"
	"log/slog"

	lpadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/launchpad"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/build"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

func buildRecipeBuildersFromConfig(cfg *config.Config, logger *slog.Logger, lpClient *lp.Client) (map[string]build.ProjectBuilder, error) {
	if cfg == nil {
		return nil, fmt.Errorf("no configuration loaded")
	}

	logger = appLogger(logger)
	result := make(map[string]build.ProjectBuilder)
	if !hasConfiguredBuildProjects(cfg) {
		return result, nil
	}
	if lpClient == nil {
		logger.Warn("skipping build projects (no LP auth configured)")
		return result, nil
	}

	for _, proj := range cfg.Projects {
		if proj.Build == nil && proj.ArtifactType == "" {
			continue
		}

		builder, strategy, err := newBuildRecipeFactory(proj.ArtifactType, lpClient)
		if err != nil {
			return nil, fmt.Errorf("project %s: %w", proj.Name, err)
		}

		var owner string
		var artifacts []string
		var skipArtifacts []string
		var lpProject string
		var officialCodehosting bool
		var prepareCommand string
		var channels map[string]string
		if proj.Build != nil {
			owner = proj.Build.Owner
			artifacts = proj.Build.Artifacts
			skipArtifacts = proj.Build.SkipArtifacts
			lpProject = proj.Build.LPProject
			officialCodehosting = proj.Build.OfficialCodehosting
			prepareCommand = proj.Build.PrepareCommand
			channels = proj.Build.Channels
		}

		series := proj.Series
		if len(series) == 0 {
			series = cfg.Launchpad.Series
		}
		devFocus := proj.DevelopmentFocus
		if devFocus == "" {
			devFocus = cfg.Launchpad.DevelopmentFocus
		}

		result[proj.Name] = build.ProjectBuilder{
			Builder:             builder,
			Owner:               owner,
			Project:             proj.Code.Project,
			LPProject:           lpProject,
			Artifacts:           artifacts,
			SkipArtifacts:       skipArtifacts,
			Series:              series,
			DevFocus:            devFocus,
			OfficialCodehosting: officialCodehosting,
			Strategy:            strategy,
			PrepareCommand:      prepareCommand,
			Channels:            channels,
		}
	}

	return result, nil
}

func buildRepoManagerFromConfig(cfg *config.Config, logger *slog.Logger, lpClient *lp.Client) (port.RepoManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("no configuration loaded")
	}
	if lpClient == nil {
		return nil, nil
	}

	return lpadapter.NewRepoManager(lpClient, appLogger(logger)), nil
}

func hasConfiguredBuildProjects(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}

	for _, proj := range cfg.Projects {
		if proj.Build != nil || proj.ArtifactType != "" {
			return true
		}
	}
	return false
}

func newBuildRecipeFactory(artifactType string, lpClient *lp.Client) (port.RecipeBuilder, build.ArtifactStrategy, error) {
	switch artifactType {
	case "rock":
		return lpadapter.NewRockBuilder(lpClient), &build.RockStrategy{}, nil
	case "charm":
		return lpadapter.NewCharmBuilder(lpClient), &build.CharmStrategy{}, nil
	case "snap":
		return lpadapter.NewSnapBuilder(lpClient, "", ""), &build.SnapStrategy{}, nil
	default:
		return nil, nil, fmt.Errorf("unsupported artifact type %q", artifactType)
	}
}

func appLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return logger
}
