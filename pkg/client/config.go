// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ConfigShow returns the loaded configuration.
func (c *Client) ConfigShow(ctx context.Context) (*dto.Config, error) {
	var result dto.Config
	err := c.get(ctx, "/api/v1/config", nil, &result)
	return &result, err
}

// ConfigReloadResult is the response from the config reload endpoint.
type ConfigReloadResult struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ConfigReload requests the server to reload its configuration from file.
func (c *Client) ConfigReload(ctx context.Context) (*ConfigReloadResult, error) {
	var result ConfigReloadResult
	err := c.post(ctx, "/api/v1/config/reload", nil, &result)
	return &result, err
}
