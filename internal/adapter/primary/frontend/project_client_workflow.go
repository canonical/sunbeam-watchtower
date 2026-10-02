// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"
	"time"

	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ProjectSyncRequest describes one client-side project sync workflow.
type ProjectSyncRequest struct {
	Projects []string
	DryRun   bool
}

// ProjectSyncResponse contains the rendered sync result for frontend consumers.
type ProjectSyncResponse struct {
	Actions []dto.ProjectSyncAction
	Errors  []string
}

// ProjectClientWorkflow exposes reusable client-side project workflows for CLI/TUI/MCP frontends.
type ProjectClientWorkflow struct {
	client     *ClientTransport
	operations *OperationClientWorkflow
}

// NewProjectClientWorkflow creates a client-side project workflow.
func NewProjectClientWorkflow(apiClient *ClientTransport) *ProjectClientWorkflow {
	return &ProjectClientWorkflow{
		client:     apiClient,
		operations: NewOperationClientWorkflow(apiClient),
	}
}

// Sync performs one synchronous project metadata sync through the API.
func (w *ProjectClientWorkflow) Sync(ctx context.Context, req ProjectSyncRequest) (*ProjectSyncResponse, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}

	result, err := apiClient.ProjectsSync(ctx, client.ProjectsSyncOptions{
		Projects: req.Projects,
		DryRun:   req.DryRun,
	})
	if err != nil {
		return nil, err
	}

	return &ProjectSyncResponse{
		Actions: result.Actions,
		Errors:  result.Errors,
	}, nil
}

// StartSync queues one asynchronous project metadata sync through the API.
func (w *ProjectClientWorkflow) StartSync(ctx context.Context, req ProjectSyncRequest) (*dto.OperationJob, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}

	return apiClient.ProjectsSyncAsync(ctx, client.ProjectsSyncOptions{
		Projects: req.Projects,
		DryRun:   req.DryRun,
	})
}

// WaitForSyncCompletion waits for one asynchronous project sync operation to reach a terminal state.
func (w *ProjectClientWorkflow) WaitForSyncCompletion(ctx context.Context, operationID string, pollInterval time.Duration) (*dto.OperationJob, error) {
	if w.operations == nil {
		return nil, errors.New("project client workflow requires an operation workflow")
	}
	return w.operations.WaitForTerminalState(ctx, operationID, pollInterval)
}

func (w *ProjectClientWorkflow) resolveClient() (*ClientTransport, error) {
	if w.client == nil {
		return nil, errors.New("project client workflow requires an API client")
	}
	return w.client, nil
}
