// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// AuthStatus returns the current authentication state.
func (c *Client) AuthStatus(ctx context.Context) (*dto.AuthStatus, error) {
	var result dto.AuthStatus
	err := c.get(ctx, "/api/v1/auth/status", nil, &result)
	return &result, err
}

// AuthLaunchpadBegin starts a Launchpad auth flow.
func (c *Client) AuthLaunchpadBegin(ctx context.Context) (*dto.LaunchpadAuthBeginResult, error) {
	var result dto.LaunchpadAuthBeginResult
	err := c.post(ctx, "/api/v1/auth/launchpad/begin", nil, &result)
	return &result, err
}

// AuthLaunchpadFinalize completes a pending Launchpad auth flow.
func (c *Client) AuthLaunchpadFinalize(
	ctx context.Context,
	flowID string,
) (*dto.LaunchpadAuthFinalizeResult, error) {
	var result dto.LaunchpadAuthFinalizeResult
	err := c.post(ctx, "/api/v1/auth/launchpad/finalize", map[string]string{"flow_id": flowID}, &result)
	return &result, err
}

// AuthLaunchpadLogout clears persisted Launchpad credentials.
func (c *Client) AuthLaunchpadLogout(ctx context.Context) (*dto.LaunchpadAuthLogoutResult, error) {
	var result dto.LaunchpadAuthLogoutResult
	err := c.post(ctx, "/api/v1/auth/launchpad/logout", nil, &result)
	return &result, err
}

// AuthGitHubBegin starts a GitHub auth flow.
func (c *Client) AuthGitHubBegin(ctx context.Context) (*dto.GitHubAuthBeginResult, error) {
	var result dto.GitHubAuthBeginResult
	err := c.post(ctx, "/api/v1/auth/github/begin", nil, &result)
	return &result, err
}

// AuthGitHubFinalize completes a pending GitHub auth flow.
func (c *Client) AuthGitHubFinalize(
	ctx context.Context,
	flowID string,
) (*dto.GitHubAuthFinalizeResult, error) {
	var result dto.GitHubAuthFinalizeResult
	err := c.post(ctx, "/api/v1/auth/github/finalize", map[string]string{"flow_id": flowID}, &result)
	return &result, err
}

// AuthGitHubLogout clears persisted GitHub credentials.
func (c *Client) AuthGitHubLogout(ctx context.Context) (*dto.GitHubAuthLogoutResult, error) {
	var result dto.GitHubAuthLogoutResult
	err := c.post(ctx, "/api/v1/auth/github/logout", nil, &result)
	return &result, err
}

// AuthSnapStoreBegin starts a Snap Store auth flow and returns the root macaroon.
func (c *Client) AuthSnapStoreBegin(ctx context.Context) (*dto.SnapStoreAuthBeginResult, error) {
	var result dto.SnapStoreAuthBeginResult
	err := c.post(ctx, "/api/v1/auth/snapstore/begin", nil, &result)
	return &result, err
}

// AuthSnapStoreSave persists a discharged Snap Store credential.
func (c *Client) AuthSnapStoreSave(ctx context.Context, macaroon string) (*dto.SnapStoreAuthSaveResult, error) {
	var result dto.SnapStoreAuthSaveResult
	err := c.post(ctx, "/api/v1/auth/snapstore/save", map[string]string{"macaroon": macaroon}, &result)
	return &result, err
}

// AuthSnapStoreLogout clears persisted Snap Store credentials.
func (c *Client) AuthSnapStoreLogout(ctx context.Context) (*dto.SnapStoreAuthLogoutResult, error) {
	var result dto.SnapStoreAuthLogoutResult
	err := c.post(ctx, "/api/v1/auth/snapstore/logout", nil, &result)
	return &result, err
}

// AuthCharmhubBegin starts a Charmhub auth flow and returns the root macaroon.
func (c *Client) AuthCharmhubBegin(ctx context.Context) (*dto.CharmhubAuthBeginResult, error) {
	var result dto.CharmhubAuthBeginResult
	err := c.post(ctx, "/api/v1/auth/charmhub/begin", nil, &result)
	return &result, err
}

// AuthCharmhubSave persists a discharged Charmhub credential.
func (c *Client) AuthCharmhubSave(ctx context.Context, macaroon string) (*dto.CharmhubAuthSaveResult, error) {
	var result dto.CharmhubAuthSaveResult
	err := c.post(ctx, "/api/v1/auth/charmhub/save", map[string]string{"macaroon": macaroon}, &result)
	return &result, err
}

// AuthCharmhubLogout clears persisted Charmhub credentials.
func (c *Client) AuthCharmhubLogout(ctx context.Context) (*dto.CharmhubAuthLogoutResult, error) {
	var result dto.CharmhubAuthLogoutResult
	err := c.post(ctx, "/api/v1/auth/charmhub/logout", nil, &result)
	return &result, err
}
