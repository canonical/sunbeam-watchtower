// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/canonical/sunbeam-watchtower/internal/app"
	authsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/auth"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// AuthStatusOutput is the response for the auth status endpoint.
type AuthStatusOutput struct {
	Body dto.AuthStatus
}

// AuthLaunchpadBeginOutput is the response for beginning a Launchpad auth flow.
type AuthLaunchpadBeginOutput struct {
	Body dto.LaunchpadAuthBeginResult
}

// AuthLaunchpadFinalizeInput is the request body for finalizing a Launchpad auth flow.
type AuthLaunchpadFinalizeInput struct {
	Body struct {
		FlowID string `json:"flow_id" doc:"Opaque flow ID returned by /api/v1/auth/launchpad/begin"`
	}
}

// AuthLaunchpadFinalizeOutput is the response for completing a Launchpad auth flow.
type AuthLaunchpadFinalizeOutput struct {
	Body dto.LaunchpadAuthFinalizeResult
}

// AuthLaunchpadLogoutOutput is the response for logging out from Launchpad.
type AuthLaunchpadLogoutOutput struct {
	Body dto.LaunchpadAuthLogoutResult
}

// AuthGitHubBeginOutput is the response for beginning a GitHub auth flow.
type AuthGitHubBeginOutput struct {
	Body dto.GitHubAuthBeginResult
}

// AuthGitHubFinalizeInput is the request body for finalizing a GitHub auth flow.
type AuthGitHubFinalizeInput struct {
	Body struct {
		FlowID string `json:"flow_id" doc:"Opaque flow ID returned by /api/v1/auth/github/begin"`
	}
}

// AuthGitHubFinalizeOutput is the response for completing a GitHub auth flow.
type AuthGitHubFinalizeOutput struct {
	Body dto.GitHubAuthFinalizeResult
}

// AuthGitHubLogoutOutput is the response for logging out from GitHub.
type AuthGitHubLogoutOutput struct {
	Body dto.GitHubAuthLogoutResult
}

// AuthSnapStoreBeginOutput is the response for beginning a Snap Store auth flow.
type AuthSnapStoreBeginOutput struct {
	Body dto.SnapStoreAuthBeginResult
}

// AuthSnapStoreSaveInput is the request body for saving a Snap Store credential.
type AuthSnapStoreSaveInput struct {
	Body struct {
		Macaroon string `json:"macaroon" doc:"Discharged macaroon credential"`
	}
}

// AuthSnapStoreSaveOutput is the response for saving a Snap Store credential.
type AuthSnapStoreSaveOutput struct {
	Body dto.SnapStoreAuthSaveResult
}

// AuthSnapStoreLogoutOutput is the response for logging out from Snap Store.
type AuthSnapStoreLogoutOutput struct {
	Body dto.SnapStoreAuthLogoutResult
}

// AuthCharmhubBeginOutput is the response for beginning a Charmhub auth flow.
type AuthCharmhubBeginOutput struct {
	Body dto.CharmhubAuthBeginResult
}

// AuthCharmhubSaveInput is the request body for saving a Charmhub credential.
type AuthCharmhubSaveInput struct {
	Body struct {
		Macaroon string `json:"macaroon" doc:"Discharged macaroon credential"`
	}
}

// AuthCharmhubSaveOutput is the response for saving a Charmhub credential.
type AuthCharmhubSaveOutput struct {
	Body dto.CharmhubAuthSaveResult
}

// AuthCharmhubLogoutOutput is the response for logging out from Charmhub.
type AuthCharmhubLogoutOutput struct {
	Body dto.CharmhubAuthLogoutResult
}

// RegisterAuthAPI registers authentication endpoints on the given huma API.
func RegisterAuthAPI(api huma.API, application *app.App) {
	facade := frontend.NewServerFacade(application)

	huma.Register(api, huma.Operation{
		OperationID: "auth-status",
		Method:      http.MethodGet,
		Path:        "/api/v1/auth/status",
		Summary:     "Show authentication status",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, _ *struct{}) (*AuthStatusOutput, error) {
		status, err := facade.Auth().Status(ctx)
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to read auth status: %v", err))
		}

		out := &AuthStatusOutput{}
		out.Body = *status
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-launchpad-begin",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/launchpad/begin",
		Summary:     "Begin Launchpad authentication",
		Description: "Starts a Launchpad OAuth flow and returns an authorization URL plus an opaque flow ID.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, _ *struct{}) (*AuthLaunchpadBeginOutput, error) {
		result, err := facade.Auth().BeginLaunchpad(ctx)
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to begin Launchpad auth: %v", err))
		}

		out := &AuthLaunchpadBeginOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-launchpad-finalize",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/launchpad/finalize",
		Summary:     "Finalize Launchpad authentication",
		Description: "Completes a Launchpad OAuth flow using the opaque flow ID returned by begin.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, input *AuthLaunchpadFinalizeInput) (*AuthLaunchpadFinalizeOutput, error) {
		result, err := facade.Auth().FinalizeLaunchpad(ctx, input.Body.FlowID)
		if err != nil {
			switch {
			case errors.Is(err, authsvc.ErrLaunchpadAuthFlowNotFound):
				return nil, huma.Error404NotFound(err.Error())
			case errors.Is(err, authsvc.ErrLaunchpadAuthFlowExpired):
				return nil, huma.Error400BadRequest(err.Error())
			default:
				return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to finalize Launchpad auth: %v", err))
			}
		}

		out := &AuthLaunchpadFinalizeOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-launchpad-logout",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/launchpad/logout",
		Summary:     "Logout from Launchpad",
		Description: "Clears persisted Launchpad credentials when they are file-backed.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, _ *struct{}) (*AuthLaunchpadLogoutOutput, error) {
		result, err := facade.Auth().LogoutLaunchpad(ctx)
		if err != nil {
			if errors.Is(err, authsvc.ErrLaunchpadEnvironmentCredentials) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to logout from Launchpad: %v", err))
		}

		out := &AuthLaunchpadLogoutOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-github-begin",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/github/begin",
		Summary:     "Begin GitHub authentication",
		Description: "Starts a GitHub device flow and returns the verification URI, user code, and an opaque flow ID.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, _ *struct{}) (*AuthGitHubBeginOutput, error) {
		result, err := facade.Auth().BeginGitHub(ctx)
		if err != nil {
			switch {
			case errors.Is(err, authsvc.ErrGitHubEnvironmentCredentials):
				return nil, huma.Error400BadRequest(err.Error())
			case errors.Is(err, authsvc.ErrGitHubClientIDRequired), errors.Is(err, authsvc.ErrGitHubKeyringNotImplemented):
				return nil, huma.Error500InternalServerError(err.Error())
			default:
				return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to begin GitHub auth: %v", err))
			}
		}

		out := &AuthGitHubBeginOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-github-finalize",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/github/finalize",
		Summary:     "Finalize GitHub authentication",
		Description: "Polls the GitHub device flow using the opaque flow ID returned by begin.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, input *AuthGitHubFinalizeInput) (*AuthGitHubFinalizeOutput, error) {
		result, err := facade.Auth().FinalizeGitHub(ctx, input.Body.FlowID)
		if err != nil {
			switch {
			case errors.Is(err, authsvc.ErrGitHubAuthFlowNotFound):
				return nil, huma.Error404NotFound(err.Error())
			case errors.Is(err, authsvc.ErrGitHubAuthFlowExpired), errors.Is(err, authsvc.ErrGitHubAccessDenied):
				return nil, huma.Error400BadRequest(err.Error())
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return nil, huma.Error400BadRequest(err.Error())
			case errors.Is(err, authsvc.ErrGitHubKeyringNotImplemented):
				return nil, huma.Error500InternalServerError(err.Error())
			default:
				return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to finalize GitHub auth: %v", err))
			}
		}

		out := &AuthGitHubFinalizeOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-github-logout",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/github/logout",
		Summary:     "Logout from GitHub",
		Description: "Clears persisted GitHub credentials when they are file-backed.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, _ *struct{}) (*AuthGitHubLogoutOutput, error) {
		result, err := facade.Auth().LogoutGitHub(ctx)
		if err != nil {
			switch {
			case errors.Is(err, authsvc.ErrGitHubEnvironmentCredentials):
				return nil, huma.Error400BadRequest(err.Error())
			case errors.Is(err, authsvc.ErrGitHubKeyringNotImplemented):
				return nil, huma.Error500InternalServerError(err.Error())
			default:
				return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to logout from GitHub: %v", err))
			}
		}

		out := &AuthGitHubLogoutOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-snapstore-begin",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/snapstore/begin",
		Summary:     "Begin Snap Store authentication",
		Description: "Requests a root macaroon from the Snap Store. The client must discharge it locally.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, _ *struct{}) (*AuthSnapStoreBeginOutput, error) {
		result, err := facade.Auth().BeginSnapStore(ctx)
		if err != nil {
			if errors.Is(err, authsvc.ErrSnapStoreEnvironmentCredentials) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to begin Snap Store auth: %v", err))
		}

		out := &AuthSnapStoreBeginOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-snapstore-save",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/snapstore/save",
		Summary:     "Save Snap Store credential",
		Description: "Persists a discharged Snap Store macaroon credential.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, input *AuthSnapStoreSaveInput) (*AuthSnapStoreSaveOutput, error) {
		result, err := facade.Auth().SaveSnapStoreCredential(ctx, input.Body.Macaroon)
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to save Snap Store credential: %v", err))
		}

		out := &AuthSnapStoreSaveOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-snapstore-logout",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/snapstore/logout",
		Summary:     "Logout from Snap Store",
		Description: "Clears persisted Snap Store credentials when they are file-backed.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, _ *struct{}) (*AuthSnapStoreLogoutOutput, error) {
		result, err := facade.Auth().LogoutSnapStore(ctx)
		if err != nil {
			if errors.Is(err, authsvc.ErrSnapStoreEnvironmentCredentials) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to logout from Snap Store: %v", err))
		}

		out := &AuthSnapStoreLogoutOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-charmhub-begin",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/charmhub/begin",
		Summary:     "Begin Charmhub authentication",
		Description: "Requests a root macaroon from Charmhub. The client must discharge it locally.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, _ *struct{}) (*AuthCharmhubBeginOutput, error) {
		result, err := facade.Auth().BeginCharmhub(ctx)
		if err != nil {
			if errors.Is(err, authsvc.ErrCharmhubEnvironmentCredentials) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to begin Charmhub auth: %v", err))
		}

		out := &AuthCharmhubBeginOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-charmhub-save",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/charmhub/save",
		Summary:     "Save Charmhub credential",
		Description: "Persists a discharged Charmhub macaroon credential.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, input *AuthCharmhubSaveInput) (*AuthCharmhubSaveOutput, error) {
		result, err := facade.Auth().SaveCharmhubCredential(ctx, input.Body.Macaroon)
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to save Charmhub credential: %v", err))
		}

		out := &AuthCharmhubSaveOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-charmhub-logout",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/charmhub/logout",
		Summary:     "Logout from Charmhub",
		Description: "Clears persisted Charmhub credentials when they are file-backed.",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, _ *struct{}) (*AuthCharmhubLogoutOutput, error) {
		result, err := facade.Auth().LogoutCharmhub(ctx)
		if err != nil {
			if errors.Is(err, authsvc.ErrCharmhubEnvironmentCredentials) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to logout from Charmhub: %v", err))
		}

		out := &AuthCharmhubLogoutOutput{}
		out.Body = *result
		return out, nil
	})
}
