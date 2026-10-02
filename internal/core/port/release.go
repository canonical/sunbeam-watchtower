// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ReleaseSource fetches the current published state for one tracked artifact.
type ReleaseSource interface {
	ArtifactType() dto.ArtifactType
	Fetch(ctx context.Context, publication dto.TrackedPublication) (*dto.PublishedArtifactSnapshot, error)
}

// ReleaseCache stores normalized publication snapshots for tracked artifacts.
type ReleaseCache interface {
	Store(ctx context.Context, snapshot dto.PublishedArtifactSnapshot) error
	List(ctx context.Context) ([]dto.PublishedArtifactSnapshot, error)
	Status(ctx context.Context) ([]dto.ReleaseCacheStatus, error)
	Remove(project string, name string, artifactType dto.ArtifactType) error
	RemoveAll() error
	CacheDir() string
	Close() error
}
