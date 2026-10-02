// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package charmhub

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
)

// CredentialProvider serves the current Charmhub publisher token to
// publisher-API callers and silently re-exchanges the stored discharged
// bundle when that token has expired. Instances are safe for concurrent use
// — Refresh serialises concurrent callers so one refresh covers them all.
type CredentialProvider struct {
	store port.CharmhubCredentialStore
	auth  port.CharmhubAuthenticator

	mu sync.Mutex
}

// Compile-time interface compliance check.
var _ port.CharmhubCredentialProvider = (*CredentialProvider)(nil)

// NewCredentialProvider wires a CredentialProvider around a store and an
// authenticator capable of re-running the `/v1/tokens/exchange` step.
func NewCredentialProvider(store port.CharmhubCredentialStore, auth port.CharmhubAuthenticator) *CredentialProvider {
	return &CredentialProvider{store: store, auth: auth}
}

// Token returns the currently stored short-lived publisher token. An empty
// string is returned when no credential has been saved — callers treat that
// as "not logged in".
func (p *CredentialProvider) Token(ctx context.Context) (string, error) {
	if p == nil || p.store == nil {
		return "", nil
	}
	record, err := p.store.Load(ctx)
	if err != nil {
		return "", fmt.Errorf("loading charmhub credentials: %w", err)
	}
	if record == nil {
		return "", nil
	}
	return record.Macaroon, nil
}

// Refresh re-exchanges the stored discharged bundle for a fresh publisher
// token and persists it. It returns ErrCharmhubReloginRequired when the
// bundle itself is exhausted or missing, so the caller can surface an
// actionable re-login message instead of looping.
func (p *CredentialProvider) Refresh(ctx context.Context) error {
	if p == nil {
		return port.ErrCharmhubReloginRequired
	}
	if p.store == nil || p.auth == nil {
		return fmt.Errorf("charmhub credential refresh not configured")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	record, err := p.store.Load(ctx)
	if err != nil {
		return fmt.Errorf("loading charmhub credentials: %w", err)
	}
	if record == nil || record.DischargedBundle == "" {
		return port.ErrCharmhubReloginRequired
	}

	exchanged, err := p.auth.ExchangeToken(ctx, record.DischargedBundle)
	if err != nil {
		// The exchange endpoint rejects an exhausted discharge as an
		// auth-class error; fold that into the actionable re-login hint
		// so the CLI does not loop on it.
		if errors.Is(err, port.ErrStoreAuthExpired) {
			return port.ErrCharmhubReloginRequired
		}
		return fmt.Errorf("exchanging charmhub macaroon: %w", err)
	}

	if _, err := p.store.Save(ctx, record.DischargedBundle, exchanged); err != nil {
		return fmt.Errorf("saving refreshed charmhub credentials: %w", err)
	}
	return nil
}
