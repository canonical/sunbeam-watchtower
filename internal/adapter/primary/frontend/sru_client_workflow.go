// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// SRUClientWorkflow is shared by the CLI and TUI.
type SRUClientWorkflow struct{ client *ClientTransport }

func NewSRUClientWorkflow(client *ClientTransport) *SRUClientWorkflow {
	return &SRUClientWorkflow{client: client}
}

func (w *SRUClientWorkflow) List(ctx context.Context, filter dto.SRUFilter) (*dto.SRUSnapshot, error) {
	if w.client == nil || w.client.Client == nil {
		return nil, errors.New("no server client configured")
	}
	return w.client.SRUList(ctx, filter)
}

func (w *SRUClientWorkflow) Show(ctx context.Context, id string) (*dto.SRUSnapshot, error) {
	if w.client == nil || w.client.Client == nil {
		return nil, errors.New("no server client configured")
	}
	return w.client.SRUShow(ctx, id)
}

func (w *SRUClientWorkflow) Migration(ctx context.Context, query dto.SRUMigrationQuery) (*dto.SRUMigrationChain, error) {
	if w.client == nil || w.client.Client == nil {
		return nil, errors.New("no server client configured")
	}
	return w.client.SRUMigration(ctx, query)
}

func (w *SRUClientWorkflow) Versions(ctx context.Context, query dto.SRUVersionsQuery) (*dto.SRUVersions, error) {
	if w.client == nil || w.client.Client == nil {
		return nil, errors.New("no server client configured")
	}
	return w.client.SRUVersions(ctx, query)
}

func (w *SRUClientWorkflow) AllVersions(ctx context.Context, source string) (*dto.SRUVersionList, error) {
	if w.client == nil || w.client.Client == nil {
		return nil, errors.New("no server client configured")
	}
	return w.client.SRUAllVersions(ctx, source)
}

func (w *SRUClientWorkflow) PocketView(ctx context.Context, series string) (*dto.SRUPocketView, error) {
	if w.client == nil || w.client.Client == nil {
		return nil, errors.New("no server client configured")
	}
	return w.client.SRUPocketView(ctx, series)
}
