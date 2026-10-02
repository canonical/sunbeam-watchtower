// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package authflowstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	gh "github.com/canonical/sunbeam-watchtower/pkg/github/v1"
	"go.etcd.io/bbolt"
)

const githubAuthFlowsBucket = "github_auth_flows"

// BoltGitHubFlowStore stores pending GitHub auth flows in bbolt.
type BoltGitHubFlowStore struct {
	db *bbolt.DB
}

// NewBoltGitHubFlowStore creates a bbolt-backed pending auth-flow store.
func NewBoltGitHubFlowStore(baseDir string) (*BoltGitHubFlowStore, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating auth flow store dir: %w", err)
	}

	db, err := bbolt.Open(filepath.Join(baseDir, "github-auth-flows.db"), 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening auth flow store db: %w", err)
	}

	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(githubAuthFlowsBucket))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initializing auth flow store db: %w", err)
	}

	return &BoltGitHubFlowStore{db: db}, nil
}

// Put stores or replaces a pending auth flow.
func (s *BoltGitHubFlowStore) Put(_ context.Context, flow *gh.PendingAuthFlow) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(githubAuthFlowsBucket))
		if err := pruneExpiredGitHubFlows(bucket, time.Now(), ""); err != nil {
			return err
		}

		data, err := json.Marshal(flow)
		if err != nil {
			return fmt.Errorf("marshal auth flow %q: %w", flow.ID, err)
		}
		return bucket.Put([]byte(flow.ID), data)
	})
}

// Get returns a pending auth flow if it exists and has not expired.
func (s *BoltGitHubFlowStore) Get(_ context.Context, id string) (*gh.PendingAuthFlow, error) {
	var (
		result  *gh.PendingAuthFlow
		expired bool
	)

	err := s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(githubAuthFlowsBucket))
		now := time.Now()
		if err := pruneExpiredGitHubFlows(bucket, now, id); err != nil {
			return err
		}

		data := bucket.Get([]byte(id))
		if data == nil {
			return gh.ErrPendingAuthFlowNotFound
		}

		var flow gh.PendingAuthFlow
		if err := json.Unmarshal(data, &flow); err != nil {
			return fmt.Errorf("unmarshal auth flow %q: %w", id, err)
		}
		if !flow.ExpiresAt.IsZero() && now.After(flow.ExpiresAt) {
			if err := bucket.Delete([]byte(id)); err != nil {
				return fmt.Errorf("delete expired auth flow %q: %w", id, err)
			}
			expired = true
			return nil
		}

		result = &flow
		return nil
	})
	if err != nil {
		return nil, err
	}
	if expired {
		return nil, gh.ErrPendingAuthFlowExpired
	}
	return result, nil
}

// Delete removes a pending auth flow.
func (s *BoltGitHubFlowStore) Delete(_ context.Context, id string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte(githubAuthFlowsBucket)).Delete([]byte(id))
	})
}

// Close releases bbolt resources.
func (s *BoltGitHubFlowStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func pruneExpiredGitHubFlows(bucket *bbolt.Bucket, now time.Time, keepID string) error {
	var expired [][]byte

	if err := bucket.ForEach(func(k, v []byte) error {
		if string(k) == keepID {
			return nil
		}

		var flow gh.PendingAuthFlow
		if err := json.Unmarshal(v, &flow); err != nil {
			return fmt.Errorf("unmarshal auth flow %q: %w", string(k), err)
		}
		if !flow.ExpiresAt.IsZero() && now.After(flow.ExpiresAt) {
			expired = append(expired, append([]byte(nil), k...))
		}
		return nil
	}); err != nil {
		return err
	}

	for _, key := range expired {
		if err := bucket.Delete(key); err != nil {
			return fmt.Errorf("delete expired auth flow %q: %w", string(key), err)
		}
	}
	return nil
}
