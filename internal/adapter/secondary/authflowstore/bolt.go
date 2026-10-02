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

	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
	"go.etcd.io/bbolt"
)

const authFlowsBucket = "auth_flows"

// BoltLaunchpadFlowStore stores pending Launchpad auth flows in bbolt.
type BoltLaunchpadFlowStore struct {
	db *bbolt.DB
}

// NewBoltLaunchpadFlowStore creates a bbolt-backed pending auth-flow store.
func NewBoltLaunchpadFlowStore(baseDir string) (*BoltLaunchpadFlowStore, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating auth flow store dir: %w", err)
	}

	db, err := bbolt.Open(filepath.Join(baseDir, "launchpad-auth-flows.db"), 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening auth flow store db: %w", err)
	}

	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(authFlowsBucket))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initializing auth flow store db: %w", err)
	}

	return &BoltLaunchpadFlowStore{db: db}, nil
}

// Put stores or replaces a pending auth flow.
func (s *BoltLaunchpadFlowStore) Put(_ context.Context, flow *lp.PendingAuthFlow) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(authFlowsBucket))
		if err := pruneExpiredFlows(bucket, time.Now(), ""); err != nil {
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
func (s *BoltLaunchpadFlowStore) Get(_ context.Context, id string) (*lp.PendingAuthFlow, error) {
	var (
		result  *lp.PendingAuthFlow
		expired bool
	)

	err := s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(authFlowsBucket))
		now := time.Now()
		if err := pruneExpiredFlows(bucket, now, id); err != nil {
			return err
		}

		data := bucket.Get([]byte(id))
		if data == nil {
			return lp.ErrPendingAuthFlowNotFound
		}

		var flow lp.PendingAuthFlow
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
		return nil, lp.ErrPendingAuthFlowExpired
	}
	return result, nil
}

// Delete removes a pending auth flow.
func (s *BoltLaunchpadFlowStore) Delete(_ context.Context, id string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte(authFlowsBucket)).Delete([]byte(id))
	})
}

// Close releases bbolt resources.
func (s *BoltLaunchpadFlowStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func pruneExpiredFlows(bucket *bbolt.Bucket, now time.Time, keepID string) error {
	var expired [][]byte

	if err := bucket.ForEach(func(k, v []byte) error {
		if string(k) == keepID {
			return nil
		}

		var flow lp.PendingAuthFlow
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
