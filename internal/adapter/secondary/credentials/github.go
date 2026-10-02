// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package credentials

import (
	"context"
	"fmt"
	"os"

	gh "github.com/canonical/sunbeam-watchtower/pkg/github/v1"
)

const (
	envGitHubToken   = "GH_TOKEN"
	envGitHubTokenV2 = "GITHUB_TOKEN"
)

// GitHubStore loads credentials from environment variables or the default file cache.
type GitHubStore struct {
	path string
}

// NewGitHubStore creates a GitHub credential store. If path is empty, the
// default GitHub credentials path is used.
func NewGitHubStore(path string) *GitHubStore {
	if path == "" {
		path = gh.CredentialsPath()
	}
	return &GitHubStore{path: path}
}

// Load returns GitHub credentials from the environment first, then the file cache.
func (s *GitHubStore) Load(_ context.Context) (*gh.CredentialRecord, error) {
	if token := os.Getenv(envGitHubToken); token != "" {
		return &gh.CredentialRecord{
			Credentials: &gh.Credentials{AccessToken: token, TokenType: "bearer"},
			Source:      gh.CredentialSourceEnvironment,
		}, nil
	}
	if token := os.Getenv(envGitHubTokenV2); token != "" {
		return &gh.CredentialRecord{
			Credentials: &gh.Credentials{AccessToken: token, TokenType: "bearer"},
			Source:      gh.CredentialSourceEnvironment,
		}, nil
	}

	if s.path == "" {
		return nil, nil
	}

	creds, err := gh.LoadCredentialsFile(s.path)
	if err != nil {
		return nil, err
	}
	if creds == nil {
		return nil, nil
	}
	return &gh.CredentialRecord{
		Credentials: creds,
		Source:      gh.CredentialSourceFile,
		Path:        s.path,
	}, nil
}

// Save persists GitHub credentials to the configured file cache path.
func (s *GitHubStore) Save(_ context.Context, creds *gh.Credentials) (*gh.CredentialRecord, error) {
	if s.path == "" {
		return nil, fmt.Errorf("cannot determine credentials path")
	}
	if err := gh.SaveCredentialsFile(s.path, creds); err != nil {
		return nil, err
	}
	return &gh.CredentialRecord{
		Credentials: creds,
		Source:      gh.CredentialSourceFile,
		Path:        s.path,
	}, nil
}

// Clear removes the persisted credentials file. Environment-provided credentials
// are not affected.
func (s *GitHubStore) Clear(_ context.Context) error {
	if s.path == "" {
		return fmt.Errorf("cannot determine credentials path")
	}
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing credentials: %w", err)
	}
	return nil
}
