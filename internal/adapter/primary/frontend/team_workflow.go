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

// TeamSyncResponse contains the rendered sync result for frontend consumers.
type TeamSyncResponse struct {
	Artifacts []dto.ArtifactSyncResult
	Warnings  []string
}

// TeamClientWorkflow exposes reusable client-side team workflows for CLI/TUI/MCP frontends.
type TeamClientWorkflow struct {
	client     *ClientTransport
	operations *OperationClientWorkflow
}

// NewTeamClientWorkflow creates a client-side team workflow.
func NewTeamClientWorkflow(apiClient *ClientTransport) *TeamClientWorkflow {
	return &TeamClientWorkflow{
		client:     apiClient,
		operations: NewOperationClientWorkflow(apiClient),
	}
}

// Sync performs one synchronous team collaborator sync through the API.
func (w *TeamClientWorkflow) Sync(ctx context.Context, req dto.TeamSyncRequest) (*TeamSyncResponse, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}

	result, err := apiClient.TeamSync(ctx, client.TeamSyncOptions{
		Projects: req.Projects,
		DryRun:   req.DryRun,
	})
	if err != nil {
		return nil, err
	}

	return &TeamSyncResponse{
		Artifacts: result.Artifacts,
		Warnings:  result.Warnings,
	}, nil
}

// StartSync queues one asynchronous team collaborator sync through the API.
func (w *TeamClientWorkflow) StartSync(ctx context.Context, req dto.TeamSyncRequest) (*dto.OperationJob, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}

	return apiClient.TeamSyncAsync(ctx, client.TeamSyncOptions{
		Projects: req.Projects,
		DryRun:   req.DryRun,
	})
}

// WaitForSyncCompletion waits for one asynchronous team sync operation to reach a terminal state.
func (w *TeamClientWorkflow) WaitForSyncCompletion(ctx context.Context, operationID string, pollInterval time.Duration) (*dto.OperationJob, error) {
	if w.operations == nil {
		return nil, errors.New("team client workflow requires an operation workflow")
	}
	return w.operations.WaitForTerminalState(ctx, operationID, pollInterval)
}

func (w *TeamClientWorkflow) resolveClient() (*ClientTransport, error) {
	if w.client == nil {
		return nil, errors.New("team client workflow requires an API client")
	}
	return w.client, nil
}
