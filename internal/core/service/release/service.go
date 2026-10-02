// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

var (
	ErrNotFound  = errors.New("release not found")
	ErrAmbiguous = errors.New("release lookup is ambiguous")
)

// AmbiguousReleaseError reports that one release name matched multiple artifact types.
type AmbiguousReleaseError struct {
	Name          string
	ArtifactTypes []dto.ArtifactType
}

func (e *AmbiguousReleaseError) Error() string {
	return ErrAmbiguous.Error()
}

func (e *AmbiguousReleaseError) Unwrap() error {
	return ErrAmbiguous
}

// Service manages cached published snap/charm release state.
type Service struct {
	cache   port.ReleaseCache
	sources map[dto.ArtifactType]port.ReleaseSource
	logger  *slog.Logger
}

// NewService creates a release service.
func NewService(cache port.ReleaseCache, sources map[dto.ArtifactType]port.ReleaseSource, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	copiedSources := make(map[dto.ArtifactType]port.ReleaseSource, len(sources))
	for typ, source := range sources {
		copiedSources[typ] = source
	}
	return &Service{cache: cache, sources: copiedSources, logger: logger}
}

// SyncCache refreshes tracked publication snapshots from their backing stores.
func (s *Service) SyncCache(ctx context.Context, publications []dto.TrackedPublication) (int, error) {
	synced := 0
	for _, publication := range publications {
		source, ok := s.sources[publication.ArtifactType]
		if !ok {
			return synced, fmt.Errorf("no release source configured for %s", publication.ArtifactType)
		}
		s.logger.Info("syncing published artifact", "project", publication.Project, "name", publication.Name, "type", publication.ArtifactType.String())
		result, err := source.Fetch(ctx, publication)
		if err != nil {
			return synced, fmt.Errorf("fetching %s (%s): %w", publication.Name, publication.ArtifactType.String(), err)
		}
		if err := s.cache.Store(ctx, dto.NormalizePublicationSnapshot(*result)); err != nil {
			return synced, fmt.Errorf("storing %s (%s): %w", publication.Name, publication.ArtifactType.String(), err)
		}
		synced++
	}
	return synced, nil
}

// List returns a flat row-per-channel view of cached publication state.
func (s *Service) List(ctx context.Context, query dto.ReleaseListQuery) ([]dto.ReleaseListEntry, error) {
	snapshots, err := s.cache.List(ctx)
	if err != nil {
		return nil, err
	}
	projectSet := toStringSet(query.Projects)
	nameSet := toStringSet(query.Names)
	trackSet := toStringSet(query.Tracks)
	branchSet := toStringSet(query.Branches)
	riskSet := make(map[dto.ReleaseRisk]bool, len(query.Risks))
	for _, risk := range query.Risks {
		riskSet[risk] = true
	}

	var results []dto.ReleaseListEntry
	for _, snapshot := range snapshots {
		if len(projectSet) > 0 && !projectSet[snapshot.Project] {
			continue
		}
		if len(nameSet) > 0 && !nameSet[snapshot.Name] {
			continue
		}
		if query.ArtifactType != nil && snapshot.ArtifactType != *query.ArtifactType {
			continue
		}
		for _, channel := range snapshot.Channels {
			if len(trackSet) > 0 && !trackSet[channel.Track] {
				continue
			}
			if len(branchSet) > 0 && !branchSet[channel.Branch] {
				continue
			}
			if len(riskSet) > 0 && !riskSet[channel.Risk] {
				continue
			}
			results = append(results, dto.ReleaseListEntry{
				Project:      snapshot.Project,
				Name:         snapshot.Name,
				ArtifactType: snapshot.ArtifactType,
				Track:        channel.Track,
				Risk:         channel.Risk,
				Branch:       channel.Branch,
				Channel:      channel.Channel,
				Targets:      append([]dto.ReleaseTargetSnapshot(nil), channel.Targets...),
				Resources:    append([]dto.ReleaseResourceSnapshot(nil), channel.Resources...),
				ReleasedAt:   channel.UpdatedAt,
			})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Name == results[j].Name {
			if results[i].Track == results[j].Track {
				if results[i].Branch == results[j].Branch {
					return riskRank(results[i].Risk) < riskRank(results[j].Risk)
				}
				return results[i].Branch < results[j].Branch
			}
			return results[i].Track < results[j].Track
		}
		if results[i].Project == results[j].Project {
			if results[i].ArtifactType == results[j].ArtifactType {
				return results[i].Name < results[j].Name
			}
			return results[i].ArtifactType.String() < results[j].ArtifactType.String()
		}
		return results[i].Project < results[j].Project
	})
	return results, nil
}

// Show returns the full cached matrix for one artifact.
func (s *Service) Show(ctx context.Context, name string, artifactType *dto.ArtifactType, track string, branch string) (*dto.ReleaseShowResult, error) {
	snapshots, err := s.cache.List(ctx)
	if err != nil {
		return nil, err
	}
	var matches []dto.PublishedArtifactSnapshot
	for _, snapshot := range snapshots {
		if snapshot.Name != name {
			continue
		}
		if artifactType != nil && snapshot.ArtifactType != *artifactType {
			continue
		}
		matches = append(matches, snapshot)
	}
	if len(matches) == 0 {
		return nil, ErrNotFound
	}
	if len(matches) > 1 {
		typeSet := make(map[dto.ArtifactType]bool, len(matches))
		types := make([]dto.ArtifactType, 0, len(matches))
		for _, match := range matches {
			if typeSet[match.ArtifactType] {
				continue
			}
			typeSet[match.ArtifactType] = true
			types = append(types, match.ArtifactType)
		}
		sort.Slice(types, func(i, j int) bool {
			return types[i].String() < types[j].String()
		})
		return nil, &AmbiguousReleaseError{Name: name, ArtifactTypes: types}
	}
	result := matches[0]
	if track != "" || branch != "" {
		filtered := result.Channels[:0]
		tracks := make([]string, 0, len(result.Tracks))
		for _, channel := range result.Channels {
			if track != "" && channel.Track != track {
				continue
			}
			if branch != "" && channel.Branch != branch {
				continue
			}
			filtered = append(filtered, channel)
			tracks = append(tracks, channel.Track)
		}
		result.Channels = filtered
		result.Tracks = uniqueTracks(tracks)
	}
	show := &dto.ReleaseShowResult{
		Project:      result.Project,
		Name:         result.Name,
		ArtifactType: result.ArtifactType,
		Tracks:       append([]string(nil), result.Tracks...),
		Channels:     append([]dto.ReleaseChannelSnapshot(nil), result.Channels...),
		UpdatedAt:    result.UpdatedAt,
	}
	return show, nil
}

// CacheStatus reports metadata about cached tracked artifacts.
func (s *Service) CacheStatus(ctx context.Context) ([]dto.ReleaseCacheStatus, error) {
	return s.cache.Status(ctx)
}

func toStringSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func riskRank(risk dto.ReleaseRisk) int {
	for idx, known := range dto.KnownReleaseRisks() {
		if risk == known {
			return idx
		}
	}
	return len(dto.KnownReleaseRisks())
}

func uniqueTracks(values []string) []string {
	set := toStringSet(values)
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
