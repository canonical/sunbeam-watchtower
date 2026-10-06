// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/canonical/sunbeam-watchtower/internal/app"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"github.com/danielgtaylor/huma/v2"
)

type sruListInput struct {
	Package        string `query:"package"`
	Set            string `query:"set"`
	Archive        string `query:"archive"`
	Series         string `query:"series"`
	Stage          string `query:"stage"`
	TaskStatus     string `query:"task_status"`
	Verification   string `query:"verification"`
	BugID          string `query:"bug_id"`
	NeedsAttention bool   `query:"needs_attention" required:"false"`
}

type sruOutput struct{ Body dto.SRUSnapshot }

type sruMigrationInput struct {
	Package string `path:"package"`
	Series  string `path:"series"`
	ID      string `path:"id"`
}
type sruMigrationOutput struct{ Body dto.SRUMigrationChain }

type sruShowInput struct {
	ID string `path:"id"`
}

func RegisterSRUAPI(api huma.API, application *app.App) {
	workflow := frontend.NewServerFacade(application).SRU()
	huma.Register(api, huma.Operation{
		OperationID: "sru-migration", Method: http.MethodGet, Path: "/api/v1/sru/migration/{package}/{series}/{id}",
		Summary: "Inspect inferred migration chains across configured series", Tags: []string{"sru"},
	}, func(ctx context.Context, input *sruMigrationInput) (*sruMigrationOutput, error) {
		result, err := workflow.Migration(ctx, dto.SRUMigrationQuery{Package: input.Package, Series: input.Series, BugID: input.ID})
		if err != nil {
			if errors.Is(err, app.ErrSRUMigrationTarget) {
				return nil, huma.Error404NotFound(err.Error())
			}
			if errors.Is(err, app.ErrSRUMigrationUnavailable) {
				return nil, huma.NewError(http.StatusConflict, err.Error())
			}
			if errors.Is(err, app.ErrSRUMigrationPrivate) {
				return nil, huma.Error403Forbidden(err.Error())
			}
			if errors.Is(err, app.ErrSRUMigrationQuery) {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("inspecting SRU migration: %v", err))
		}
		return &sruMigrationOutput{Body: *result}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-sru", Method: http.MethodGet, Path: "/api/v1/sru",
		Summary: "List monitored OpenStack SRU targets", Tags: []string{"sru"},
	}, func(ctx context.Context, input *sruListInput) (*sruOutput, error) {
		result, err := workflow.List(ctx, dto.SRUFilter{
			Package: input.Package, Set: input.Set, Archive: input.Archive, Series: input.Series,
			Stage: input.Stage, TaskStatus: input.TaskStatus, Verification: input.Verification,
			BugID: input.BugID, NeedsAttention: input.NeedsAttention,
		})
		if err != nil {
			if errors.Is(err, app.ErrSRUNotSynced) {
				return nil, huma.NewError(http.StatusConflict, err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("listing SRUs: %v", err))
		}
		return &sruOutput{Body: *result}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "show-sru", Method: http.MethodGet, Path: "/api/v1/sru/{id}",
		Summary: "Show monitored targets for one Launchpad bug", Tags: []string{"sru"},
	}, func(ctx context.Context, input *sruShowInput) (*sruOutput, error) {
		result, err := workflow.List(ctx, dto.SRUFilter{BugID: input.ID})
		if err != nil {
			if errors.Is(err, app.ErrSRUNotSynced) {
				return nil, huma.NewError(http.StatusConflict, err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("showing SRU: %v", err))
		}
		if len(result.Rows) == 0 {
			return nil, huma.Error404NotFound("no monitored SRU targets for bug " + input.ID)
		}
		return &sruOutput{Body: *result}, nil
	})
}
