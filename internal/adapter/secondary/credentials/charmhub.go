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
	envCharmcraftAuth      = "CHARMCRAFT_AUTH"
	charmhubCredentialFile = "charmhub-credentials.json"
)

// charmhubFileCredentials is the JSON-serialized form of Charmhub credentials.
//
// DischargedBundle is the long-lived client-discharged macaroon bundle kept
// so the short-lived Macaroon (exchanged publisher token) can be silently
// re-exchanged when it expires. Older records without this field still load
// fine — the refresh path surfaces ErrCharmhubReloginRequired in that case.
type charmhubFileCredentials struct {
	Macaroon         string `json:"macaroon"`
	DischargedBundle string `json:"discharged_bundle,omitempty"`
}

// CharmhubStore loads credentials from environment variables or a file cache.
type CharmhubStore struct {
	path string
}

// Compile-time interface compliance check.
var _ port.CharmhubCredentialStore = (*CharmhubStore)(nil)

// NewCharmhubStore creates a Charmhub credential store.
// If dir is empty, the default config directory is used.
func NewCharmhubStore(dir string) *CharmhubStore {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, ".config", "sunbeam-watchtower")
		}
	}
	path := ""
	if dir != "" {
		path = filepath.Join(dir, charmhubCredentialFile)
	}
	return &CharmhubStore{path: path}
}

// Load returns Charmhub credentials from the environment first, then the file cache.
func (s *CharmhubStore) Load(_ context.Context) (*dto.StoreCredentialRecord, error) {
	if macaroon := os.Getenv(envCharmcraftAuth); macaroon != "" {
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
		return nil, fmt.Errorf("reading charmhub credentials: %w", err)
	}

	var creds charmhubFileCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing charmhub credentials: %w", err)
	}
	if creds.Macaroon == "" {
		return nil, nil
	}
	return &dto.StoreCredentialRecord{
		Macaroon:         creds.Macaroon,
		DischargedBundle: creds.DischargedBundle,
		Source:           storeCredentialSourceFile,
		Path:             s.path,
	}, nil
}

// Save persists Charmhub credentials to the configured file path.
func (s *CharmhubStore) Save(_ context.Context, dischargedBundle, exchangedMacaroon string) (*dto.StoreCredentialRecord, error) {
	if s.path == "" {
		return nil, fmt.Errorf("cannot determine credentials path")
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}

	creds := charmhubFileCredentials{
		Macaroon:         exchangedMacaroon,
		DischargedBundle: dischargedBundle,
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling charmhub credentials: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return nil, fmt.Errorf("writing charmhub credentials: %w", err)
	}
	return &dto.StoreCredentialRecord{
		Macaroon:         creds.Macaroon,
		DischargedBundle: creds.DischargedBundle,
		Source:           storeCredentialSourceFile,
		Path:             s.path,
	}, nil
}

// Clear removes the persisted credentials file. Environment-provided credentials
// are not affected.
func (s *CharmhubStore) Clear(_ context.Context) error {
	if s.path == "" {
		return fmt.Errorf("cannot determine credentials path")
	}
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing charmhub credentials: %w", err)
	}
	return nil
}
