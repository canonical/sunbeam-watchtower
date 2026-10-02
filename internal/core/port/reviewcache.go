// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"
	"errors"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

var (
	ErrReviewCacheNotSynced  = errors.New("review cache not synced")
	ErrReviewDetailNotCached = errors.New("review detail not cached")
)

// ReviewCache stores cached review summaries and details per project.
type ReviewCache interface {
	StoreSummaries(ctx context.Context, forgeType forge.ForgeType, project string, mrs []forge.MergeRequest) error
	StoreDetail(ctx context.Context, forgeType forge.ForgeType, project string, mr forge.MergeRequest) error
	GetDetail(ctx context.Context, forgeType forge.ForgeType, project string, id string) (*forge.MergeRequest, error)
	List(ctx context.Context, forgeType forge.ForgeType, project string) ([]forge.MergeRequest, error)
	PruneDetailsBefore(ctx context.Context, cutoff time.Time) error
	SetLastSync(ctx context.Context, forgeType forge.ForgeType, project string, t time.Time) error
	LastSync(ctx context.Context, forgeType forge.ForgeType, project string) (time.Time, error)
	Remove(ctx context.Context, forgeType forge.ForgeType, project string) error
	RemoveAll(ctx context.Context) error
	Close() error
	CacheDir() string
	Status(ctx context.Context) ([]dto.ReviewCacheStatus, error)
}
