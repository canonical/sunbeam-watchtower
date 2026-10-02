// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	opsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/operation"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// OperationWorkflow exposes frontend-facing long-running operation workflows.
type OperationWorkflow struct {
	application      *app.App
	operationService *opsvc.Service
}

// NewOperationWorkflow creates a frontend operation workflow.
func NewOperationWorkflow(application *app.App) *OperationWorkflow {
	return &OperationWorkflow{application: application}
}

// NewOperationWorkflowFromService creates a frontend operation workflow from a concrete service.
func NewOperationWorkflowFromService(service *opsvc.Service) *OperationWorkflow {
	return &OperationWorkflow{operationService: service}
}

func (w *OperationWorkflow) resolveService() (*opsvc.Service, error) {
	if w.operationService != nil {
		return w.operationService, nil
	}
	return w.application.OperationService()
}

// Start starts one long-running operation.
func (w *OperationWorkflow) Start(
	ctx context.Context,
	kind dto.OperationKind,
	attributes map[string]string,
	run opsvc.Func,
) (*dto.OperationJob, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.Start(ctx, kind, attributes, run)
}

// List returns all known operations.
func (w *OperationWorkflow) List(ctx context.Context) ([]dto.OperationJob, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.List(ctx)
}

// Get returns one operation snapshot.
func (w *OperationWorkflow) Get(ctx context.Context, id string) (*dto.OperationJob, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.Get(ctx, id)
}

// Events returns the event history for one operation.
func (w *OperationWorkflow) Events(ctx context.Context, id string) ([]dto.OperationEvent, error) {
	service, err := w.resolveService()
	if err != nil {
		return nil, err
	}
	return service.Events(ctx, id)
}

// Cancel requests cancellation for a running operation.
func (w *OperationWorkflow) Cancel(ctx context.Context, id string) error {
	service, err := w.resolveService()
	if err != nil {
		return err
	}
	return service.Cancel(ctx, id)
}
