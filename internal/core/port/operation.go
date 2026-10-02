// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// OperationStore persists frontend-facing long-running job state.
type OperationStore interface {
	Create(ctx context.Context, job dto.OperationJob) error
	Get(ctx context.Context, id string) (*dto.OperationJob, error)
	List(ctx context.Context) ([]dto.OperationJob, error)
	Update(ctx context.Context, job dto.OperationJob) error
	AppendEvent(ctx context.Context, id string, event dto.OperationEvent) error
	Events(ctx context.Context, id string) ([]dto.OperationEvent, error)
}
