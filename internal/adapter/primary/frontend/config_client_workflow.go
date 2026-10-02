// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"
	"fmt"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ConfigReloadResult is the frontend-facing result of a config reload request.
type ConfigReloadResult struct {
	Status  string `json:"status"  yaml:"status"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// ConfigClientWorkflow exposes reusable client-side config workflows for CLI/TUI/MCP frontends.
type ConfigClientWorkflow struct {
	client *ClientTransport
}

// NewConfigClientWorkflow creates a client-side config workflow.
func NewConfigClientWorkflow(apiClient *ClientTransport) *ConfigClientWorkflow {
	return &ConfigClientWorkflow{client: apiClient}
}

// Show returns the loaded remote configuration.
func (w *ConfigClientWorkflow) Show(ctx context.Context) (*dto.Config, error) {
	if w.client == nil {
		return nil, errors.New("config client workflow requires an API client")
	}
	return w.client.ConfigShow(ctx)
}

// Reload requests the server to reload its configuration from file.
func (w *ConfigClientWorkflow) Reload(ctx context.Context) (*ConfigReloadResult, error) {
	if w.client == nil {
		return nil, errors.New("config client workflow requires an API client")
	}
	result, err := w.client.ConfigReload(ctx)
	if err != nil {
		return nil, err
	}
	out := &ConfigReloadResult{
		Status:  result.Status,
		Message: result.Message,
	}
	if result.Status != "ok" {
		return out, fmt.Errorf("config reload failed: %s", result.Message)
	}
	return out, nil
}
