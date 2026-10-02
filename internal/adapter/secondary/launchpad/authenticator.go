// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package launchpad

import (
	"context"
	"io"
	"log/slog"

	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

// Authenticator bridges the core auth service to the Launchpad OAuth client.
type Authenticator struct {
	consumerKey string
	logger      *slog.Logger
}

// NewAuthenticator creates a Launchpad OAuth authenticator adapter.
func NewAuthenticator(consumerKey string, logger *slog.Logger) *Authenticator {
	if consumerKey == "" {
		consumerKey = lp.ConsumerKey()
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Authenticator{consumerKey: consumerKey, logger: logger}
}

// ConsumerKey returns the configured Launchpad OAuth consumer key.
func (a *Authenticator) ConsumerKey() string {
	return a.consumerKey
}

// ObtainRequestToken starts the Launchpad OAuth flow.
func (a *Authenticator) ObtainRequestToken(_ context.Context) (*lp.RequestToken, error) {
	token, err := lp.ObtainRequestToken(a.consumerKey)
	if err != nil {
		return nil, err
	}
	return token, nil
}

// ExchangeAccessToken completes the Launchpad OAuth flow.
func (a *Authenticator) ExchangeAccessToken(_ context.Context, token *lp.RequestToken) (*lp.Credentials, error) {
	return lp.ExchangeAccessToken(a.consumerKey, token)
}

// CurrentUser returns the authenticated Launchpad identity for the given credentials.
func (a *Authenticator) CurrentUser(ctx context.Context, creds *lp.Credentials) (lp.Person, error) {
	client := lp.NewClient(creds, a.logger)
	me, err := client.Me(ctx)
	if err != nil {
		return lp.Person{}, err
	}
	return me, nil
}
