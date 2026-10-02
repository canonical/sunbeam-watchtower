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
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// TeamSyncInput holds the request body for syncing team collaborators.
type TeamSyncInput struct {
	Body struct {
		Projects []string `json:"projects,omitempty" required:"false" doc:"Filter by project name"`
		DryRun   bool     `json:"dry_run,omitempty" required:"false" doc:"Preview changes without applying"`
	}
}

// TeamSyncOutput is the response for a team collaborator sync operation.
type TeamSyncOutput struct {
	Body dto.TeamSyncResult
}

// RegisterTeamAPI registers the /api/v1/team endpoints on the given huma API.
func RegisterTeamAPI(api huma.API, application *app.App) {
	facade := frontend.NewServerFacade(application)

	huma.Register(api, huma.Operation{
		OperationID: "sync-team",
		Method:      http.MethodPost,
		Path:        "/api/v1/team/sync",
		Summary:     "Sync team collaborators",
		Description: "Compare LP team members against store collaborators and invite missing members.",
		Tags:        []string{"team"},
	}, func(ctx context.Context, input *TeamSyncInput) (*TeamSyncOutput, error) {
		result, err := facade.Teams().Sync(ctx, dto.TeamSyncRequest{
			Projects: input.Body.Projects,
			DryRun:   input.Body.DryRun,
		})
		if err != nil {
			if errors.Is(err, app.ErrLaunchpadAuthRequired) {
				return nil, huma.NewError(http.StatusUnauthorized, err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("sync failed: %v", err))
		}
		out := &TeamSyncOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sync-team-async",
		Method:      http.MethodPost,
		Path:        "/api/v1/team/sync/async",
		Summary:     "Sync team collaborators asynchronously",
		Description: "Queue team collaborator sync as a long-running operation job.",
		Tags:        []string{"team", "operations"},
	}, func(ctx context.Context, input *TeamSyncInput) (*OperationOutput, error) {
		job, err := facade.Teams().StartSync(ctx, dto.TeamSyncRequest{
			Projects: input.Body.Projects,
			DryRun:   input.Body.DryRun,
		})
		if err != nil {
			if errors.Is(err, app.ErrLaunchpadAuthRequired) {
				return nil, huma.NewError(http.StatusUnauthorized, err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("async sync failed: %v", err))
		}
		out := &OperationOutput{}
		out.Body = *job
		return out, nil
	})
}
