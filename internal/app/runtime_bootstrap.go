// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"
	"time"

	chadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/charmhub"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/credentials"
	ghadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/githubauth"
	lpadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/launchpad"
	ssadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/snapstore"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	authsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/auth"
	opsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/operation"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

func (a *App) stateDir() (string, error) {
	return cacheSubdir("state")
}

// LaunchpadCredentialStore returns the shared Launchpad credential store.
func (a *App) LaunchpadCredentialStore() port.LaunchpadCredentialStore {
	a.lpCredsOnce.Do(func() {
		a.lpCredsStore = credentials.NewLaunchpadStore("")
	})
	return a.lpCredsStore
}

// GitHubCredentialStore returns the shared GitHub credential store.
func (a *App) GitHubCredentialStore() port.GitHubCredentialStore {
	a.ghCredsOnce.Do(func() {
		a.ghCredsStore = credentials.NewGitHubStore("")
	})
	return a.ghCredsStore
}

// LaunchpadPendingAuthFlowStore returns the shared pending Launchpad auth flow store.
func (a *App) LaunchpadPendingAuthFlowStore() port.LaunchpadPendingAuthFlowStore {
	a.lpFlowOnce.Do(func() {
		a.lpFlowStore = newLaunchpadPendingAuthFlowStore(a.Logger, a.runtimeMode, a.stateDir)
	})
	return a.lpFlowStore
}

// GitHubPendingAuthFlowStore returns the shared pending GitHub auth flow store.
func (a *App) GitHubPendingAuthFlowStore() port.GitHubPendingAuthFlowStore {
	a.ghFlowOnce.Do(func() {
		a.ghFlowStore = newGitHubPendingAuthFlowStore(a.Logger, a.runtimeMode, a.stateDir)
	})
	return a.ghFlowStore
}

// OperationStore returns the shared long-running operation store.
func (a *App) OperationStore() port.OperationStore {
	a.operationStoreOnce.Do(func() {
		a.operationStore = newOperationStore(a.Logger, a.runtimeMode, a.stateDir)
	})
	return a.operationStore
}

// OperationService returns the shared async operation service.
func (a *App) OperationService() (*opsvc.Service, error) {
	a.operationServiceOnce.Do(func() {
		a.operationService = opsvc.NewService(a.OperationStore(), a.Logger)
	})

	return a.operationService, nil
}

// SnapStoreCredentialStore returns the shared Snap Store credential store.
func (a *App) SnapStoreCredentialStore() port.SnapStoreCredentialStore {
	a.snapStoreCredsOnce.Do(func() {
		a.snapStoreCredsStore = credentials.NewSnapStoreStore("")
	})
	return a.snapStoreCredsStore
}

// CharmhubCredentialStore returns the shared Charmhub credential store.
func (a *App) CharmhubCredentialStore() port.CharmhubCredentialStore {
	a.charmhubCredsOnce.Do(func() {
		a.charmhubCredsStore = credentials.NewCharmhubStore("")
	})
	return a.charmhubCredsStore
}

// AuthService creates the shared auth service.
func (a *App) AuthService() (*authsvc.Service, error) {
	var githubMutableErr error
	if a.GetConfig() != nil && a.GetConfig().GitHub.UseKeyring {
		githubMutableErr = authsvc.ErrGitHubKeyringNotImplemented
	}
	return authsvc.NewServiceWithStores(
		a.LaunchpadCredentialStore(),
		a.LaunchpadPendingAuthFlowStore(),
		lpadapter.NewAuthenticator(lp.ConsumerKey(), a.Logger),
		a.GitHubCredentialStore(),
		a.GitHubPendingAuthFlowStore(),
		ghadapter.NewAuthenticator(a.GitHubClientID(), a.Logger, a.upstreamHTTPClient("github", 30*time.Second)),
		githubMutableErr,
		a.SnapStoreCredentialStore(),
		ssadapter.NewAuthenticator(a.Logger, a.upstreamHTTPClient("snapstore", 30*time.Second)),
		a.CharmhubCredentialStore(),
		chadapter.NewAuthenticator(a.Logger, a.upstreamHTTPClient("charmhub", 30*time.Second)),
		a.Logger,
	), nil
}

// GitHubClientID resolves the configured GitHub OAuth app client ID.
func (a *App) GitHubClientID() string {
	if v := os.Getenv("WATCHTOWER_GITHUB_CLIENT_ID"); v != "" {
		return v
	}
	cfg := a.GetConfig()
	if cfg == nil {
		return ""
	}
	return cfg.GitHub.ClientID
}
