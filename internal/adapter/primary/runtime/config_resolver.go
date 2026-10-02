// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"fmt"
	"sync"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/pkg/client"
)

// ConfigResolver lazily resolves configuration from a local file, a remote
// server, or both. When a remote client is available, the server's config is
// authoritative. Results are cached for the session lifetime.
type ConfigResolver struct {
	local  *config.Config
	client *client.Client
	cached *config.Config
	mu     sync.Mutex
}

// NewConfigResolver creates a ConfigResolver with the given local config and
// optional remote client. Either may be nil, but at least one must be non-nil
// for Resolve to succeed.
func NewConfigResolver(local *config.Config, client *client.Client) *ConfigResolver {
	return &ConfigResolver{local: local, client: client}
}

// Resolve returns the effective configuration. If a remote client is set, the
// server config is fetched and returned (authoritative). Otherwise the local
// config is returned. Results are cached after the first successful resolution.
func (r *ConfigResolver) Resolve(ctx context.Context) (*config.Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cached != nil {
		return r.cached, nil
	}
	if r.client != nil {
		dtoConfig, err := r.client.ConfigShow(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not fetch configuration from server: %w", err)
		}
		r.cached = frontend.DTOToConfig(dtoConfig)
		return r.cached, nil
	}
	if r.local != nil {
		return r.local, nil
	}
	return nil, fmt.Errorf("no configuration source available: provide a config file or connect to a server (--server)")
}

// LocalConfig returns the local configuration, if any. This may be nil when
// only a remote client was provided.
func (r *ConfigResolver) LocalConfig() *config.Config {
	return r.local
}
