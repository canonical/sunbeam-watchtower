// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import "github.com/canonical/sunbeam-watchtower/pkg/client"

// ClientTransport wraps the reusable API client behind one frontend transport value.
type ClientTransport struct {
	*client.Client
}

// NewClientTransport wraps one API client for frontend workflow consumption.
func NewClientTransport(apiClient *client.Client) *ClientTransport {
	if apiClient == nil {
		return nil
	}
	return &ClientTransport{Client: apiClient}
}
