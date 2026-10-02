// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	authsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/auth"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// AuthWorkflow exposes frontend-facing authentication workflows.
type AuthWorkflow struct {
	application *app.App
	authService *authsvc.Service
}

// NewAuthWorkflow creates a frontend auth workflow.
func NewAuthWorkflow(application *app.App) *AuthWorkflow {
	return &AuthWorkflow{application: application}
}

// NewAuthWorkflowFromService creates a frontend auth workflow from a concrete service.
func NewAuthWorkflowFromService(service *authsvc.Service) *AuthWorkflow {
	return &AuthWorkflow{authService: service}
}

func (w *AuthWorkflow) resolveService() (*authsvc.Service, error) {
	if w.authService != nil {
		return w.authService, nil
	}
	return w.application.AuthService()
}

// Status returns the current authentication state.
func (w *AuthWorkflow) Status(ctx context.Context) (*dto.AuthStatus, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.Status(ctx)
}

// BeginLaunchpad starts a Launchpad authentication flow.
func (w *AuthWorkflow) BeginLaunchpad(ctx context.Context) (*dto.LaunchpadAuthBeginResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.BeginLaunchpad(ctx)
}

// FinalizeLaunchpad completes a pending Launchpad authentication flow.
func (w *AuthWorkflow) FinalizeLaunchpad(ctx context.Context, flowID string) (*dto.LaunchpadAuthFinalizeResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.FinalizeLaunchpad(ctx, flowID)
}

// LogoutLaunchpad clears persisted Launchpad credentials.
func (w *AuthWorkflow) LogoutLaunchpad(ctx context.Context) (*dto.LaunchpadAuthLogoutResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.LogoutLaunchpad(ctx)
}

// BeginGitHub starts a GitHub authentication flow.
func (w *AuthWorkflow) BeginGitHub(ctx context.Context) (*dto.GitHubAuthBeginResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.BeginGitHub(ctx)
}

// FinalizeGitHub completes a pending GitHub authentication flow.
func (w *AuthWorkflow) FinalizeGitHub(ctx context.Context, flowID string) (*dto.GitHubAuthFinalizeResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.FinalizeGitHub(ctx, flowID)
}

// LogoutGitHub clears persisted GitHub credentials.
func (w *AuthWorkflow) LogoutGitHub(ctx context.Context) (*dto.GitHubAuthLogoutResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.LogoutGitHub(ctx)
}

// BeginSnapStore starts a Snap Store authentication flow.
func (w *AuthWorkflow) BeginSnapStore(ctx context.Context) (*dto.SnapStoreAuthBeginResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.BeginSnapStore(ctx)
}

// SaveSnapStoreCredential persists a discharged Snap Store credential.
func (w *AuthWorkflow) SaveSnapStoreCredential(ctx context.Context, macaroon string) (*dto.SnapStoreAuthSaveResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.SaveSnapStoreCredential(ctx, macaroon)
}

// LogoutSnapStore clears persisted Snap Store credentials.
func (w *AuthWorkflow) LogoutSnapStore(ctx context.Context) (*dto.SnapStoreAuthLogoutResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.LogoutSnapStore(ctx)
}

// BeginCharmhub starts a Charmhub authentication flow.
func (w *AuthWorkflow) BeginCharmhub(ctx context.Context) (*dto.CharmhubAuthBeginResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.BeginCharmhub(ctx)
}

// SaveCharmhubCredential persists a discharged Charmhub credential.
func (w *AuthWorkflow) SaveCharmhubCredential(ctx context.Context, macaroon string) (*dto.CharmhubAuthSaveResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.SaveCharmhubCredential(ctx, macaroon)
}

// LogoutCharmhub clears persisted Charmhub credentials.
func (w *AuthWorkflow) LogoutCharmhub(ctx context.Context) (*dto.CharmhubAuthLogoutResult, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.LogoutCharmhub(ctx)
}
