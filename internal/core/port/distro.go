// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"

	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// DistroCache manages downloading, indexing, and querying APT Sources data.
type DistroCache interface {
	Update(ctx context.Context, name string, entries []dto.SourceEntry) error
	Query(ctx context.Context, name string, opts dto.QueryOpts) ([]distro.SourcePackage, error)
	QueryDetailed(ctx context.Context, name string, opts dto.QueryOpts) ([]distro.SourcePackageDetail, error)
	Status() ([]dto.CacheStatus, error)
	CacheDir() string
	Close() error
}
