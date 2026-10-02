// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestServiceSyncListAndShow(t *testing.T) {
	cache := &fakeReleaseCache{}
	source := &fakeReleaseSource{snapshot: &dto.PublishedArtifactSnapshot{
		Project:      "sunbeam",
		Name:         "snap-openstack",
		ArtifactType: dto.ArtifactSnap,
		Tracks:       []string{"2024.1", "2025.1"},
		Channels: []dto.ReleaseChannelSnapshot{{
			Track:     "2024.1",
			Risk:      dto.ReleaseRiskStable,
			Channel:   "2024.1/stable",
			UpdatedAt: time.Now().UTC(),
			Targets:   []dto.ReleaseTargetSnapshot{{Architecture: "amd64", Revision: 12, Version: "1.2.3"}},
		}, {
			Track:     "2025.1",
			Risk:      dto.ReleaseRiskEdge,
			Branch:    "risc-v",
			Channel:   "2025.1/edge/risc-v",
			UpdatedAt: time.Now().UTC(),
			Targets:   []dto.ReleaseTargetSnapshot{{Architecture: "amd64", Revision: 18, Version: "1.2.4"}},
		}},
		UpdatedAt: time.Now().UTC(),
	}}
	service := NewService(cache, map[dto.ArtifactType]port.ReleaseSource{dto.ArtifactSnap: source}, slog.Default())

	synced, err := service.SyncCache(context.Background(), []dto.TrackedPublication{{
		Project:      "sunbeam",
		Name:         "snap-openstack",
		ArtifactType: dto.ArtifactSnap,
		Tracks:       []string{"2024.1", "2025.1"},
	}})
	if err != nil {
		t.Fatalf("SyncCache() error = %v", err)
	}
	if synced != 1 {
		t.Fatalf("SyncCache() synced = %d, want 1", synced)
	}
	if len(cache.snapshots) != 1 {
		t.Fatalf("snapshots stored = %d, want 1", len(cache.snapshots))
	}

	list, err := service.List(context.Background(), dto.ReleaseListQuery{Tracks: []string{"2024.1"}})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].Track != "2024.1" || list[0].Risk != dto.ReleaseRiskStable {
		t.Fatalf("List() = %+v, want 2024.1/stable row", list)
	}
	if list[0].ReleasedAt.IsZero() {
		t.Fatalf("List() released_at = %v, want per-channel release timestamp", list[0].ReleasedAt)
	}

	branchList, err := service.List(context.Background(), dto.ReleaseListQuery{Branches: []string{"risc-v"}})
	if err != nil {
		t.Fatalf("List(branch) error = %v", err)
	}
	if len(branchList) != 1 || branchList[0].Branch != "risc-v" {
		t.Fatalf("List(branch) = %+v, want one branch row", branchList)
	}

	show, err := service.Show(context.Background(), "snap-openstack", &[]dto.ArtifactType{dto.ArtifactSnap}[0], "", "")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if show.Name != "snap-openstack" || len(show.Channels) != 2 {
		t.Fatalf("Show() = %+v, want full matrix", show)
	}
}

func TestServiceShowAmbiguous(t *testing.T) {
	cache := &fakeReleaseCache{snapshots: []dto.PublishedArtifactSnapshot{{
		Project:      "snaps",
		Name:         "keystone",
		ArtifactType: dto.ArtifactSnap,
	}, {
		Project:      "charms",
		Name:         "keystone",
		ArtifactType: dto.ArtifactCharm,
	}}}
	service := NewService(cache, nil, slog.Default())

	_, err := service.Show(context.Background(), "keystone", nil, "", "")
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("Show() error = %v, want ErrAmbiguous", err)
	}
	var ambiguousErr *AmbiguousReleaseError
	if !errors.As(err, &ambiguousErr) {
		t.Fatalf("Show() error = %T, want *AmbiguousReleaseError", err)
	}
	if !slices.Equal(ambiguousErr.ArtifactTypes, []dto.ArtifactType{dto.ArtifactCharm, dto.ArtifactSnap}) {
		t.Fatalf("Show() ambiguous types = %+v, want charm+snap", ambiguousErr.ArtifactTypes)
	}
}

type fakeReleaseCache struct {
	snapshots []dto.PublishedArtifactSnapshot
}

func (f *fakeReleaseCache) Store(_ context.Context, snapshot dto.PublishedArtifactSnapshot) error {
	for i := range f.snapshots {
		if f.snapshots[i].Project == snapshot.Project && f.snapshots[i].Name == snapshot.Name && f.snapshots[i].ArtifactType == snapshot.ArtifactType {
			f.snapshots[i] = snapshot
			return nil
		}
	}
	f.snapshots = append(f.snapshots, snapshot)
	return nil
}
func (f *fakeReleaseCache) List(_ context.Context) ([]dto.PublishedArtifactSnapshot, error) {
	return append([]dto.PublishedArtifactSnapshot(nil), f.snapshots...), nil
}
func (f *fakeReleaseCache) Status(_ context.Context) ([]dto.ReleaseCacheStatus, error) {
	return nil, nil
}
func (f *fakeReleaseCache) Remove(string, string, dto.ArtifactType) error { return nil }
func (f *fakeReleaseCache) RemoveAll() error                              { return nil }
func (f *fakeReleaseCache) CacheDir() string                              { return "" }
func (f *fakeReleaseCache) Close() error                                  { return nil }

type fakeReleaseSource struct {
	snapshot *dto.PublishedArtifactSnapshot
}

func (f *fakeReleaseSource) ArtifactType() dto.ArtifactType { return dto.ArtifactSnap }
func (f *fakeReleaseSource) Fetch(_ context.Context, publication dto.TrackedPublication) (*dto.PublishedArtifactSnapshot, error) {
	result := *f.snapshot
	result.Project = publication.Project
	result.Name = publication.Name
	result.ArtifactType = publication.ArtifactType
	result.Tracks = append([]string(nil), publication.Tracks...)
	return &result, nil
}
