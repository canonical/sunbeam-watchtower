// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	projectsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/project"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ProjectServerWorkflow exposes reusable server-side project workflows for the HTTP API.
type ProjectServerWorkflow struct {
	application *app.App
	async       *Facade
}

// NewProjectServerWorkflow creates a server-side project workflow.
func NewProjectServerWorkflow(application *app.App, async *Facade) *ProjectServerWorkflow {
	return &ProjectServerWorkflow{
		application: application,
		async:       async,
	}
}

// Sync performs one synchronous project sync.
func (w *ProjectServerWorkflow) Sync(ctx context.Context, req ProjectSyncRequest) (*ProjectSyncResponse, error) {
	service, err := w.application.ProjectService()
	if err != nil {
		return nil, err
	}

	result, err := service.Sync(ctx, projectsvc.SyncOptions{
		Projects: req.Projects,
		DryRun:   req.DryRun,
	})
	if err != nil {
		return nil, err
	}

	response := &ProjectSyncResponse{
		Actions: result.Actions,
	}
	for _, syncErr := range result.Errors {
		response.Errors = append(response.Errors, syncErr.Error())
	}
	return response, nil
}

// StartSync queues one asynchronous project sync.
func (w *ProjectServerWorkflow) StartSync(ctx context.Context, req ProjectSyncRequest) (*dto.OperationJob, error) {
	return w.async.StartProjectSync(ctx, ProjectSyncOptions(req))
}
