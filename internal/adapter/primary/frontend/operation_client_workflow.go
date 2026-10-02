// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

const defaultOperationPollInterval = 250 * time.Millisecond

// OperationWaitOptions configures polling for a remote operation.
type OperationWaitOptions struct {
	PollInterval time.Duration
	States       []dto.OperationState
}

// OperationClientWorkflow exposes reusable client-side operation workflows for CLI/TUI/MCP frontends.
type OperationClientWorkflow struct {
	client *ClientTransport
}

// NewOperationClientWorkflow creates a client-side operation workflow.
func NewOperationClientWorkflow(apiClient *ClientTransport) *OperationClientWorkflow {
	return &OperationClientWorkflow{client: apiClient}
}

// List returns all known remote operations.
func (w *OperationClientWorkflow) List(ctx context.Context) ([]dto.OperationJob, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.OperationsList(ctx)
}

// Get returns one remote operation snapshot.
func (w *OperationClientWorkflow) Get(ctx context.Context, id string) (*dto.OperationJob, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.OperationGet(ctx, id)
}

// Events returns the remote event history for one operation.
func (w *OperationClientWorkflow) Events(ctx context.Context, id string) ([]dto.OperationEvent, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.OperationEvents(ctx, id)
}

// Cancel requests cancellation for one remote operation.
func (w *OperationClientWorkflow) Cancel(ctx context.Context, id string) (*dto.OperationJob, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.OperationCancel(ctx, id)
}

// Wait polls one remote operation until it reaches one of the requested states.
func (w *OperationClientWorkflow) Wait(
	ctx context.Context,
	id string,
	opts OperationWaitOptions,
) (*dto.OperationJob, error) {
	if len(opts.States) == 0 {
		return nil, errors.New("operation wait requires at least one target state")
	}

	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultOperationPollInterval
	}

	for {
		job, err := w.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if job != nil && matchesOperationState(job.State, opts.States) {
			return job, nil
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// WaitForTerminalState polls until the remote operation reaches any terminal lifecycle state.
func (w *OperationClientWorkflow) WaitForTerminalState(ctx context.Context, id string, pollInterval time.Duration) (*dto.OperationJob, error) {
	return w.Wait(ctx, id, OperationWaitOptions{
		PollInterval: pollInterval,
		States: []dto.OperationState{
			dto.OperationStateSucceeded,
			dto.OperationStateFailed,
			dto.OperationStateCancelled,
			dto.OperationStateInterrupted,
		},
	})
}

func (w *OperationClientWorkflow) resolveClient() (*ClientTransport, error) {
	if w.client == nil {
		return nil, errors.New("operation client workflow requires an API client")
	}
	return w.client, nil
}

func matchesOperationState(state dto.OperationState, states []dto.OperationState) bool {
	for _, candidate := range states {
		if state == candidate {
			return true
		}
	}
	return false
}
