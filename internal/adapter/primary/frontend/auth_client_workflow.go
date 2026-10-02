// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"
	"net/url"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	sa "github.com/canonical/sunbeam-watchtower/pkg/storeauth/v1"
)

// LaunchpadAuthorizationHandler handles the user-facing authorization step between begin and finalize.
type LaunchpadAuthorizationHandler func(context.Context, *dto.LaunchpadAuthBeginResult) error

// GitHubAuthorizationHandler handles the user-facing device-code authorization step.
type GitHubAuthorizationHandler func(context.Context, *dto.GitHubAuthBeginResult) error

// SnapStoreAuthorizationHandler handles the user-facing SSO browser authorization step.
// It receives the URL the user must visit to authenticate.
type SnapStoreAuthorizationHandler func(ctx context.Context, visitURL string) error

// CharmhubAuthorizationHandler handles the user-facing SSO browser authorization step.
// It receives the URL the user must visit to authenticate.
type CharmhubAuthorizationHandler func(ctx context.Context, visitURL string) error

// AuthLoginResult contains both halves of a completed login workflow.
type AuthLoginResult struct {
	Begin     *dto.LaunchpadAuthBeginResult
	Finalized *dto.LaunchpadAuthFinalizeResult
}

// GitHubAuthLoginResult contains both halves of a completed GitHub login workflow.
type GitHubAuthLoginResult struct {
	Begin     *dto.GitHubAuthBeginResult
	Finalized *dto.GitHubAuthFinalizeResult
}

// SnapStoreAuthLoginResult contains the result of a completed Snap Store login workflow.
type SnapStoreAuthLoginResult struct {
	Saved *dto.SnapStoreAuthSaveResult
}

// CharmhubAuthLoginResult contains the result of a completed Charmhub login workflow.
type CharmhubAuthLoginResult struct {
	Saved *dto.CharmhubAuthSaveResult
}

// AuthClientWorkflow exposes reusable client-side auth workflows for CLI/TUI/MCP frontends.
type AuthClientWorkflow struct {
	client *ClientTransport
}

// NewAuthClientWorkflow creates a client-side auth workflow.
func NewAuthClientWorkflow(apiClient *ClientTransport) *AuthClientWorkflow {
	return &AuthClientWorkflow{client: apiClient}
}

// Status returns the current remote authentication state.
func (w *AuthClientWorkflow) Status(ctx context.Context) (*dto.AuthStatus, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthStatus(ctx)
}

// BeginLaunchpad starts a remote Launchpad auth flow.
func (w *AuthClientWorkflow) BeginLaunchpad(ctx context.Context) (*dto.LaunchpadAuthBeginResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthLaunchpadBegin(ctx)
}

// FinalizeLaunchpad completes one remote Launchpad auth flow.
func (w *AuthClientWorkflow) FinalizeLaunchpad(ctx context.Context, flowID string) (*dto.LaunchpadAuthFinalizeResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthLaunchpadFinalize(ctx, flowID)
}

// LoginLaunchpad runs the full begin -> authorize -> finalize client workflow.
func (w *AuthClientWorkflow) LoginLaunchpad(
	ctx context.Context,
	handler LaunchpadAuthorizationHandler,
) (*AuthLoginResult, error) {
	if handler == nil {
		return nil, errors.New("launchpad authorization handler is required")
	}

	begin, err := w.BeginLaunchpad(ctx)
	if err != nil {
		return nil, err
	}
	if err := handler(ctx, begin); err != nil {
		return nil, err
	}

	finalized, err := w.FinalizeLaunchpad(ctx, begin.FlowID)
	if err != nil {
		return nil, err
	}
	return &AuthLoginResult{
		Begin:     begin,
		Finalized: finalized,
	}, nil
}

// LogoutLaunchpad clears persisted Launchpad credentials through the API.
func (w *AuthClientWorkflow) LogoutLaunchpad(ctx context.Context) (*dto.LaunchpadAuthLogoutResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthLaunchpadLogout(ctx)
}

// BeginGitHub starts a remote GitHub auth flow.
func (w *AuthClientWorkflow) BeginGitHub(ctx context.Context) (*dto.GitHubAuthBeginResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthGitHubBegin(ctx)
}

// FinalizeGitHub completes one remote GitHub auth flow.
func (w *AuthClientWorkflow) FinalizeGitHub(ctx context.Context, flowID string) (*dto.GitHubAuthFinalizeResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthGitHubFinalize(ctx, flowID)
}

// LoginGitHub runs the full begin -> authorize -> finalize client workflow.
func (w *AuthClientWorkflow) LoginGitHub(
	ctx context.Context,
	handler GitHubAuthorizationHandler,
) (*GitHubAuthLoginResult, error) {
	if handler == nil {
		return nil, errors.New("github authorization handler is required")
	}

	begin, err := w.BeginGitHub(ctx)
	if err != nil {
		return nil, err
	}
	if err := handler(ctx, begin); err != nil {
		return nil, err
	}

	finalized, err := w.FinalizeGitHub(ctx, begin.FlowID)
	if err != nil {
		return nil, err
	}
	return &GitHubAuthLoginResult{
		Begin:     begin,
		Finalized: finalized,
	}, nil
}

// LogoutGitHub clears persisted GitHub credentials through the API.
func (w *AuthClientWorkflow) LogoutGitHub(ctx context.Context) (*dto.GitHubAuthLogoutResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthGitHubLogout(ctx)
}

// BeginSnapStore starts a remote Snap Store auth flow.
func (w *AuthClientWorkflow) BeginSnapStore(ctx context.Context) (*dto.SnapStoreAuthBeginResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthSnapStoreBegin(ctx)
}

// SaveSnapStoreCredential persists a discharged Snap Store credential through the API.
func (w *AuthClientWorkflow) SaveSnapStoreCredential(ctx context.Context, macaroon string) (*dto.SnapStoreAuthSaveResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthSnapStoreSave(ctx, macaroon)
}

// LoginSnapStore runs the full begin -> discharge -> save client workflow.
// The macaroon discharge runs client-side via httpbakery so the browser opens locally.
func (w *AuthClientWorkflow) LoginSnapStore(
	ctx context.Context,
	handler SnapStoreAuthorizationHandler,
) (*SnapStoreAuthLoginResult, error) {
	if handler == nil {
		return nil, errors.New("snap store authorization handler is required")
	}

	begin, err := w.BeginSnapStore(ctx)
	if err != nil {
		return nil, err
	}

	// Discharge the root macaroon client-side.
	credential, err := sa.DischargeAll(ctx, begin.RootMacaroon, func(u *url.URL) error {
		return handler(ctx, u.String())
	})
	if err != nil {
		return nil, err
	}

	saved, err := w.SaveSnapStoreCredential(ctx, credential)
	if err != nil {
		return nil, err
	}
	return &SnapStoreAuthLoginResult{
		Saved: saved,
	}, nil
}

// LogoutSnapStore clears persisted Snap Store credentials through the API.
func (w *AuthClientWorkflow) LogoutSnapStore(ctx context.Context) (*dto.SnapStoreAuthLogoutResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthSnapStoreLogout(ctx)
}

// BeginCharmhub starts a remote Charmhub auth flow.
func (w *AuthClientWorkflow) BeginCharmhub(ctx context.Context) (*dto.CharmhubAuthBeginResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthCharmhubBegin(ctx)
}

// SaveCharmhubCredential persists a discharged Charmhub credential through the API.
func (w *AuthClientWorkflow) SaveCharmhubCredential(ctx context.Context, macaroon string) (*dto.CharmhubAuthSaveResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthCharmhubSave(ctx, macaroon)
}

// LoginCharmhub runs the full begin -> discharge -> save client workflow.
// The macaroon discharge runs client-side via httpbakery so the browser opens locally.
func (w *AuthClientWorkflow) LoginCharmhub(
	ctx context.Context,
	handler CharmhubAuthorizationHandler,
) (*CharmhubAuthLoginResult, error) {
	if handler == nil {
		return nil, errors.New("charmhub authorization handler is required")
	}

	begin, err := w.BeginCharmhub(ctx)
	if err != nil {
		return nil, err
	}

	// Discharge the root macaroon client-side.
	credential, err := sa.DischargeAll(ctx, begin.RootMacaroon, func(u *url.URL) error {
		return handler(ctx, u.String())
	})
	if err != nil {
		return nil, err
	}

	saved, err := w.SaveCharmhubCredential(ctx, credential)
	if err != nil {
		return nil, err
	}
	return &CharmhubAuthLoginResult{
		Saved: saved,
	}, nil
}

// LogoutCharmhub clears persisted Charmhub credentials through the API.
func (w *AuthClientWorkflow) LogoutCharmhub(ctx context.Context) (*dto.CharmhubAuthLogoutResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.AuthCharmhubLogout(ctx)
}

func (w *AuthClientWorkflow) resolveClient() (*ClientTransport, error) {
	if w.client == nil {
		return nil, errors.New("auth client workflow requires an API client")
	}
	return w.client, nil
}
