// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

const (
	envSnapcraftStoreCredentials = "SNAPCRAFT_STORE_CREDENTIALS"
	snapStoreCredentialFile      = "snapstore-credentials.json"
)

// snapStoreFileCredentials is the JSON-serialized form of Snap Store credentials.
type snapStoreFileCredentials struct {
	Macaroon string `json:"macaroon"`
}

// SnapStoreStore loads credentials from environment variables or a file cache.
type SnapStoreStore struct {
	path string
}

// Compile-time interface compliance check.
var _ port.SnapStoreCredentialStore = (*SnapStoreStore)(nil)

// NewSnapStoreStore creates a Snap Store credential store.
// If dir is empty, the default config directory is used.
func NewSnapStoreStore(dir string) *SnapStoreStore {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, ".config", "sunbeam-watchtower")
		}
	}
	path := ""
	if dir != "" {
		path = filepath.Join(dir, snapStoreCredentialFile)
	}
	return &SnapStoreStore{path: path}
}

// Load returns Snap Store credentials from the environment first, then the file cache.
func (s *SnapStoreStore) Load(_ context.Context) (*dto.StoreCredentialRecord, error) {
	if macaroon := os.Getenv(envSnapcraftStoreCredentials); macaroon != "" {
		return &dto.StoreCredentialRecord{
			Macaroon: macaroon,
			Source:   storeCredentialSourceEnvironment,
		}, nil
	}

	if s.path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading snap store credentials: %w", err)
	}

	var creds snapStoreFileCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing snap store credentials: %w", err)
	}
	if creds.Macaroon == "" {
		return nil, nil
	}
	return &dto.StoreCredentialRecord{
		Macaroon: creds.Macaroon,
		Source:   storeCredentialSourceFile,
		Path:     s.path,
	}, nil
}

// Save persists Snap Store credentials to the configured file path.
func (s *SnapStoreStore) Save(_ context.Context, macaroon string) (*dto.StoreCredentialRecord, error) {
	if s.path == "" {
		return nil, fmt.Errorf("cannot determine credentials path")
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}

	creds := snapStoreFileCredentials{Macaroon: macaroon}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling snap store credentials: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return nil, fmt.Errorf("writing snap store credentials: %w", err)
	}
	return &dto.StoreCredentialRecord{
		Macaroon: creds.Macaroon,
		Source:   storeCredentialSourceFile,
		Path:     s.path,
	}, nil
}

// Clear removes the persisted credentials file. Environment-provided credentials
// are not affected.
func (s *SnapStoreStore) Clear(_ context.Context) error {
	if s.path == "" {
		return fmt.Errorf("cannot determine credentials path")
	}
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing snap store credentials: %w", err)
	}
	return nil
}
