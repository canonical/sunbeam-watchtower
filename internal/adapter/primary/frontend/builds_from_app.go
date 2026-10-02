// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"fmt"

	"github.com/canonical/sunbeam-watchtower/internal/app"
)

// NewLocalBuildPreparerFromApp wires a local build preparer from application services.
func NewLocalBuildPreparerFromApp(application *app.App) (*LocalBuildPreparer, error) {
	repoMgr, err := application.BuildRepoManager()
	if err != nil {
		return nil, fmt.Errorf("init repo manager: %w", err)
	}
	if repoMgr == nil {
		return nil, app.ErrLaunchpadAuthRequired
	}

	builders, err := application.BuildRecipeBuilders()
	if err != nil {
		return nil, fmt.Errorf("init recipe builders: %w", err)
	}

	return NewLocalBuildPreparer(application.GitClient(), repoMgr, builders, &ShellCommandRunner{}), nil
}
