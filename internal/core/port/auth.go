// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	gh "github.com/canonical/sunbeam-watchtower/pkg/github/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
	sa "github.com/canonical/sunbeam-watchtower/pkg/storeauth/v1"
)

// LaunchpadCredentialStore manages persisted Launchpad credentials.
type LaunchpadCredentialStore interface {
	Load(ctx context.Context) (*lp.CredentialRecord, error)
	Save(ctx context.Context, creds *lp.Credentials) (*lp.CredentialRecord, error)
	Clear(ctx context.Context) error
}

// LaunchpadAuthenticator performs Launchpad OAuth and identity lookups.
type LaunchpadAuthenticator interface {
	ConsumerKey() string
	ObtainRequestToken(ctx context.Context) (*lp.RequestToken, error)
	ExchangeAccessToken(ctx context.Context, token *lp.RequestToken) (*lp.Credentials, error)
	CurrentUser(ctx context.Context, creds *lp.Credentials) (lp.Person, error)
}

// LaunchpadPendingAuthFlowStore stores short-lived pending auth flows server-side.
type LaunchpadPendingAuthFlowStore interface {
	Put(ctx context.Context, flow *lp.PendingAuthFlow) error
	Get(ctx context.Context, id string) (*lp.PendingAuthFlow, error)
	Delete(ctx context.Context, id string) error
}

// GitHubCredentialStore manages persisted GitHub credentials.
type GitHubCredentialStore interface {
	Load(ctx context.Context) (*gh.CredentialRecord, error)
	Save(ctx context.Context, creds *gh.Credentials) (*gh.CredentialRecord, error)
	Clear(ctx context.Context) error
}

// GitHubAuthenticator performs GitHub device-flow auth and identity lookups.
type GitHubAuthenticator interface {
	ClientID() string
	BeginDeviceFlow(ctx context.Context) (*gh.PendingAuthFlow, error)
	PollAccessToken(ctx context.Context, flow *gh.PendingAuthFlow) (*gh.Credentials, error)
	CurrentUser(ctx context.Context, creds *gh.Credentials) (gh.User, error)
}

// GitHubPendingAuthFlowStore stores short-lived pending GitHub auth flows server-side.
type GitHubPendingAuthFlowStore interface {
	Put(ctx context.Context, flow *gh.PendingAuthFlow) error
	Get(ctx context.Context, id string) (*gh.PendingAuthFlow, error)
	Delete(ctx context.Context, id string) error
}

// SnapStoreCredentialStore manages persisted Snap Store credentials.
type SnapStoreCredentialStore interface {
	Load(ctx context.Context) (*dto.StoreCredentialRecord, error)
	Save(ctx context.Context, macaroon string) (*dto.StoreCredentialRecord, error)
	Clear(ctx context.Context) error
}

// CharmhubCredentialStore manages persisted Charmhub credentials.
//
// Save takes both the long-lived discharged bundle (kept so the short-lived
// token can be silently re-exchanged on expiry) and the short-lived
// exchanged publisher token that actually authorises `/v1/charm/...`
// requests.
type CharmhubCredentialStore interface {
	Load(ctx context.Context) (*dto.StoreCredentialRecord, error)
	Save(ctx context.Context, dischargedBundle, exchangedMacaroon string) (*dto.StoreCredentialRecord, error)
	Clear(ctx context.Context) error
}

// CharmhubCredentialProvider returns the current Charmhub publisher
// token and can silently re-exchange the stored discharged bundle when
// that token has expired. Callers invoke Refresh reactively after a
// publisher request comes back as ErrStoreAuthExpired and retry the
// request exactly once. Repeated refresh failure surfaces
// ErrCharmhubReloginRequired so the CLI can point the user at
// `watchtower auth charmhub login`.
type CharmhubCredentialProvider interface {
	Token(ctx context.Context) (string, error)
	Refresh(ctx context.Context) error
}

// StoreAuthenticator requests root macaroons from a store API.
// Used for both Snap Store and Charmhub. Discharge happens client-side.
type StoreAuthenticator interface {
	// BeginAuth requests a root macaroon from the store.
	BeginAuth(ctx context.Context) (*sa.PendingAuthFlow, error)
}

// SnapStoreAuthenticator is an alias for StoreAuthenticator used for Snap Store auth.
type SnapStoreAuthenticator = StoreAuthenticator

// CharmhubAuthenticator requests root macaroons and exchanges discharged
// macaroon bundles for short-lived Charmhub publisher tokens.
//
// Charmhub's publisher API has a two-step auth: the root macaroon returned by
// BeginAuth must be discharged client-side via httpbakery, then exchanged via
// ExchangeToken for the macaroon that the /v1/charm/... endpoints accept on
// `Authorization: Macaroon <token>`.
type CharmhubAuthenticator interface {
	StoreAuthenticator
	ExchangeToken(ctx context.Context, dischargedBundle string) (string, error)
}
