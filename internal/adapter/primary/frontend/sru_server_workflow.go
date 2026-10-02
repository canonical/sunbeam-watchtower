// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// SRUServerWorkflow exposes read-only monitoring and explicit local refresh.
type SRUServerWorkflow struct{ application *app.App }

func NewSRUServerWorkflow(application *app.App) *SRUServerWorkflow {
	return &SRUServerWorkflow{application: application}
}

func (w *SRUServerWorkflow) List(_ context.Context, filter dto.SRUFilter) (*dto.SRUSnapshot, error) {
	return w.application.SRUSnapshot(filter)
}

func (w *SRUServerWorkflow) Refresh(ctx context.Context) (*dto.SRUSnapshot, error) {
	return w.application.RefreshSRU(ctx)
}
