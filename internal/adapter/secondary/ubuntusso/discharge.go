// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

// Package ubuntusso re-exports macaroon discharge helpers from the shared
// pkg/storeauth/v1 package for backward compatibility.
package ubuntusso

import (
	"context"
	"net/url"

	sa "github.com/canonical/sunbeam-watchtower/pkg/storeauth/v1"
)

// DischargeAll delegates to sa.DischargeAll. See that function for documentation.
func DischargeAll(ctx context.Context, serializedMacaroon string, openURL func(u *url.URL) error) (string, error) {
	return sa.DischargeAll(ctx, serializedMacaroon, openURL)
}
