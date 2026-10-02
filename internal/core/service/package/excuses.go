// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package pkg

import (
	"context"
	"io"
	"log/slog"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ExcusesService exposes package excuses operations backed by an ExcusesCache.
type ExcusesService struct {
	cache  port.ExcusesCache
	logger *slog.Logger
}

// NewExcusesService creates a new excuses service.
func NewExcusesService(cache port.ExcusesCache, logger *slog.Logger) *ExcusesService {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &ExcusesService{cache: cache, logger: logger}
}

// UpdateCache downloads and indexes excuses for the given trackers.
func (s *ExcusesService) UpdateCache(ctx context.Context, sources []dto.ExcusesSource) error {
	for _, source := range sources {
		s.logger.Info("updating excuses cache", "tracker", source.Tracker)
		if err := s.cache.Update(ctx, source); err != nil {
			return err
		}
	}
	return nil
}

// List returns excuses matching the given query options.
func (s *ExcusesService) List(ctx context.Context, opts dto.ExcuseQueryOpts) ([]dto.PackageExcuseSummary, error) {
	return s.cache.List(ctx, opts)
}

// Show returns one excuse by tracker, package, and optional version.
func (s *ExcusesService) Show(ctx context.Context, tracker, name, version string) (*dto.PackageExcuse, error) {
	return s.cache.Get(ctx, tracker, name, version)
}

// CacheStatus returns metadata about the cached excuses trackers.
func (s *ExcusesService) CacheStatus() ([]dto.ExcusesCacheStatus, error) {
	return s.cache.Status()
}
