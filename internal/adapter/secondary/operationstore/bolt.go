// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package operationstore

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"go.etcd.io/bbolt"
)

const (
	jobsBucket   = "jobs"
	eventsBucket = "events"
)

// BoltStore persists operation snapshots and events in bbolt.
type BoltStore struct {
	db *bbolt.DB
}

// NewBoltStore creates a bbolt-backed operation store.
func NewBoltStore(baseDir string) (*BoltStore, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating operation store dir: %w", err)
	}

	db, err := bbolt.Open(filepath.Join(baseDir, "operations.db"), 0o600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening operation store db: %w", err)
	}

	if err := db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(jobsBucket)); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists([]byte(eventsBucket))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initializing operation store db: %w", err)
	}

	return &BoltStore{db: db}, nil
}

// Create stores a new job snapshot.
func (s *BoltStore) Create(_ context.Context, job dto.OperationJob) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		jobs := tx.Bucket([]byte(jobsBucket))
		if jobs.Get([]byte(job.ID)) != nil {
			return fmt.Errorf("operation job %q already exists", job.ID)
		}

		data, err := json.Marshal(job)
		if err != nil {
			return fmt.Errorf("marshal operation job %q: %w", job.ID, err)
		}
		return jobs.Put([]byte(job.ID), data)
	})
}

// Get returns a job snapshot by ID.
func (s *BoltStore) Get(_ context.Context, id string) (*dto.OperationJob, error) {
	var job *dto.OperationJob

	err := s.db.View(func(tx *bbolt.Tx) error {
		data := tx.Bucket([]byte(jobsBucket)).Get([]byte(id))
		if data == nil {
			return nil
		}

		var stored dto.OperationJob
		if err := json.Unmarshal(data, &stored); err != nil {
			return fmt.Errorf("unmarshal operation job %q: %w", id, err)
		}
		job = &stored
		return nil
	})
	if err != nil {
		return nil, err
	}
	return job, nil
}

// List returns all known jobs ordered by creation time descending.
func (s *BoltStore) List(_ context.Context) ([]dto.OperationJob, error) {
	jobs := make([]dto.OperationJob, 0)

	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte(jobsBucket)).ForEach(func(_, v []byte) error {
			var job dto.OperationJob
			if err := json.Unmarshal(v, &job); err != nil {
				return fmt.Errorf("unmarshal operation job: %w", err)
			}
			jobs = append(jobs, job)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})

	return jobs, nil
}

// Update replaces a previously created job snapshot.
func (s *BoltStore) Update(_ context.Context, job dto.OperationJob) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		jobs := tx.Bucket([]byte(jobsBucket))
		if jobs.Get([]byte(job.ID)) == nil {
			return fmt.Errorf("operation job %q not found", job.ID)
		}

		data, err := json.Marshal(job)
		if err != nil {
			return fmt.Errorf("marshal operation job %q: %w", job.ID, err)
		}
		return jobs.Put([]byte(job.ID), data)
	})
}

// AppendEvent adds an event to a job's event stream.
func (s *BoltStore) AppendEvent(_ context.Context, id string, event dto.OperationEvent) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		jobs := tx.Bucket([]byte(jobsBucket))
		if jobs.Get([]byte(id)) == nil {
			return fmt.Errorf("operation job %q not found", id)
		}

		eventsRoot := tx.Bucket([]byte(eventsBucket))
		jobEvents, err := eventsRoot.CreateBucketIfNotExists([]byte(id))
		if err != nil {
			return fmt.Errorf("creating event bucket for %q: %w", id, err)
		}

		seq, err := jobEvents.NextSequence()
		if err != nil {
			return fmt.Errorf("allocating event sequence for %q: %w", id, err)
		}
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, seq)

		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("marshal operation event for %q: %w", id, err)
		}
		return jobEvents.Put(key, data)
	})
}

// Events returns the full event history for a job.
func (s *BoltStore) Events(_ context.Context, id string) ([]dto.OperationEvent, error) {
	events := make([]dto.OperationEvent, 0)

	err := s.db.View(func(tx *bbolt.Tx) error {
		if tx.Bucket([]byte(jobsBucket)).Get([]byte(id)) == nil {
			return nil
		}

		jobEvents := tx.Bucket([]byte(eventsBucket)).Bucket([]byte(id))
		if jobEvents == nil {
			return nil
		}

		return jobEvents.ForEach(func(_, v []byte) error {
			var event dto.OperationEvent
			if err := json.Unmarshal(v, &event); err != nil {
				return fmt.Errorf("unmarshal operation event for %q: %w", id, err)
			}
			events = append(events, event)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	return events, nil
}

// Close releases bbolt resources.
func (s *BoltStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
