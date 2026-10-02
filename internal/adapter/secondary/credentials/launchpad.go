// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package credentials

import (
	"context"
	"fmt"
	"os"

	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

const (
	envAccessToken       = "LP_ACCESS_TOKEN"
	envAccessTokenSecret = "LP_ACCESS_TOKEN_SECRET"
)

// LaunchpadStore loads credentials from environment variables or the default file cache.
type LaunchpadStore struct {
	path string
}

// NewLaunchpadStore creates a Launchpad credential store. If path is empty, the
// default Launchpad credentials path is used.
func NewLaunchpadStore(path string) *LaunchpadStore {
	if path == "" {
		path = lp.CredentialsPath()
	}
	return &LaunchpadStore{path: path}
}

// Load returns Launchpad credentials from the environment first, then the file cache.
func (s *LaunchpadStore) Load(_ context.Context) (*lp.CredentialRecord, error) {
	token := os.Getenv(envAccessToken)
	secret := os.Getenv(envAccessTokenSecret)
	if token != "" && secret != "" {
		return &lp.CredentialRecord{
			Credentials: &lp.Credentials{
				ConsumerKey:       lp.ConsumerKey(),
				AccessToken:       token,
				AccessTokenSecret: secret,
			},
			Source: lp.CredentialSourceEnvironment,
		}, nil
	}

	if s.path == "" {
		return nil, nil
	}

	creds, err := lp.LoadCredentialsFile(s.path)
	if err != nil {
		return nil, err
	}
	if creds == nil {
		return nil, nil
	}

	return &lp.CredentialRecord{
		Credentials: creds,
		Source:      lp.CredentialSourceFile,
		Path:        s.path,
	}, nil
}

// Save persists Launchpad credentials to the configured file cache path.
func (s *LaunchpadStore) Save(_ context.Context, creds *lp.Credentials) (*lp.CredentialRecord, error) {
	if s.path == "" {
		return nil, fmt.Errorf("cannot determine credentials path")
	}
	if err := lp.SaveCredentialsFile(s.path, creds); err != nil {
		return nil, err
	}
	return &lp.CredentialRecord{
		Credentials: creds,
		Source:      lp.CredentialSourceFile,
		Path:        s.path,
	}, nil
}

// Clear removes the persisted credentials file. Environment-provided credentials
// are not affected.
func (s *LaunchpadStore) Clear(_ context.Context) error {
	if s.path == "" {
		return fmt.Errorf("cannot determine credentials path")
	}
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing credentials: %w", err)
	}
	return nil
}
