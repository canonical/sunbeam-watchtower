// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"time"

	adaptergit "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/git"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/build"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

// BuildRecipeBuilders creates per-project RecipeBuilder instances from config.
func (a *App) BuildRecipeBuilders() (map[string]build.ProjectBuilder, error) {
	var lpClient *lp.Client
	if hasConfiguredBuildProjects(a.GetConfig()) {
		lpClient = newLaunchpadClient(a.LaunchpadCredentialStore(), a.Logger, a.upstreamHTTPClient("launchpad", 2*time.Minute))
	}
	return buildRecipeBuildersFromConfig(a.GetConfig(), a.Logger, lpClient)
}

// BuildService creates the build service with all required dependencies wired.
func (a *App) BuildService() (*build.Service, error) {
	builders, err := a.BuildRecipeBuilders()
	if err != nil {
		return nil, err
	}
	repoMgr, err := a.BuildRepoManager()
	if err != nil {
		return nil, err
	}
	return build.NewService(builders, repoMgr, a.Logger), nil
}

// BuildRepoManager creates a RepoManager backed by Launchpad.
func (a *App) BuildRepoManager() (port.RepoManager, error) {
	var lpClient *lp.Client
	if a.GetConfig() != nil {
		lpClient = newLaunchpadClient(a.LaunchpadCredentialStore(), a.Logger, a.upstreamHTTPClient("launchpad", 2*time.Minute))
	}
	return buildRepoManagerFromConfig(
		a.GetConfig(),
		a.Logger,
		lpClient,
	)
}

// GitClient returns a new git client for local repository operations.
func (a *App) GitClient() port.GitClient {
	return adaptergit.NewClient(a.Logger)
}
