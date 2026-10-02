// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ExcusesCache manages downloading, indexing, and querying migration excuses data.
type ExcusesCache interface {
	Update(ctx context.Context, source dto.ExcusesSource) error
	List(ctx context.Context, opts dto.ExcuseQueryOpts) ([]dto.PackageExcuseSummary, error)
	Get(ctx context.Context, tracker, name, version string) (*dto.PackageExcuse, error)
	Status() ([]dto.ExcusesCacheStatus, error)
	CacheDir() string
	Close() error
}
