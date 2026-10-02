// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"log/slog"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/authflowstore"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/operationstore"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
)

func newLaunchpadPendingAuthFlowStore(
	logger *slog.Logger,
	mode RuntimeMode,
	stateDir func() (string, error),
) port.LaunchpadPendingAuthFlowStore {
	logger = appLogger(logger)
	if mode == RuntimeModeEphemeral {
		return authflowstore.NewMemoryLaunchpadFlowStore()
	}

	dir, err := stateDir()
	if err != nil {
		logger.Warn("failed to resolve cache dir for auth flow store, falling back to memory", "error", err)
		return authflowstore.NewMemoryLaunchpadFlowStore()
	}

	store, err := authflowstore.NewBoltLaunchpadFlowStore(dir)
	if err != nil {
		logger.Warn("failed to initialize auth flow store, falling back to memory", "error", err)
		return authflowstore.NewMemoryLaunchpadFlowStore()
	}
	return store
}

func newGitHubPendingAuthFlowStore(
	logger *slog.Logger,
	mode RuntimeMode,
	stateDir func() (string, error),
) port.GitHubPendingAuthFlowStore {
	logger = appLogger(logger)
	if mode == RuntimeModeEphemeral {
		return authflowstore.NewMemoryGitHubFlowStore()
	}

	dir, err := stateDir()
	if err != nil {
		logger.Warn("failed to resolve cache dir for auth flow store, falling back to memory", "error", err)
		return authflowstore.NewMemoryGitHubFlowStore()
	}

	store, err := authflowstore.NewBoltGitHubFlowStore(dir)
	if err != nil {
		logger.Warn("failed to initialize auth flow store, falling back to memory", "error", err)
		return authflowstore.NewMemoryGitHubFlowStore()
	}
	return store
}

func newOperationStore(
	logger *slog.Logger,
	mode RuntimeMode,
	stateDir func() (string, error),
) port.OperationStore {
	logger = appLogger(logger)
	if mode == RuntimeModeEphemeral {
		return operationstore.NewMemoryStore()
	}

	dir, err := stateDir()
	if err != nil {
		logger.Warn("failed to resolve cache dir for operation store, falling back to memory", "error", err)
		return operationstore.NewMemoryStore()
	}

	store, err := operationstore.NewBoltStore(dir)
	if err != nil {
		logger.Warn("failed to initialize operation store, falling back to memory", "error", err)
		return operationstore.NewMemoryStore()
	}
	return store
}
