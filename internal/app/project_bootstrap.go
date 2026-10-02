// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"time"

	lpadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/launchpad"
	projectsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/project"
)

// BuildProjectSyncConfigs resolves project sync configuration from the loaded config.
func (a *App) BuildProjectSyncConfigs() (map[string]projectsvc.ProjectSyncConfig, error) {
	cfg := a.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("no configuration loaded")
	}

	projectConfigs := make(map[string]projectsvc.ProjectSyncConfig)
	for _, proj := range cfg.Projects {
		for _, b := range proj.Bugs {
			if b.Forge != "launchpad" {
				continue
			}
			if _, ok := projectConfigs[b.Project]; ok {
				continue
			}
			psc := projectsvc.ProjectSyncConfig{
				Series:           cfg.Launchpad.Series,
				DevelopmentFocus: cfg.Launchpad.DevelopmentFocus,
			}
			if len(proj.Series) > 0 {
				psc.Series = proj.Series
			}
			if proj.DevelopmentFocus != "" {
				psc.DevelopmentFocus = proj.DevelopmentFocus
			}
			projectConfigs[b.Project] = psc
		}
	}

	return projectConfigs, nil
}

// ProjectService creates the project sync service with config-derived project settings.
func (a *App) ProjectService() (*projectsvc.Service, error) {
	projectConfigs, err := a.BuildProjectSyncConfigs()
	if err != nil {
		return nil, err
	}
	if len(projectConfigs) == 0 {
		return projectsvc.NewService(nil, projectConfigs, a.Logger), nil
	}

	lpClient := newLaunchpadClient(a.LaunchpadCredentialStore(), a.Logger, a.upstreamHTTPClient("launchpad", 2*time.Minute))
	if lpClient == nil {
		return nil, ErrLaunchpadAuthRequired
	}

	manager := lpadapter.NewProjectManager(lpClient)
	return projectsvc.NewService(manager, projectConfigs, a.Logger), nil
}
