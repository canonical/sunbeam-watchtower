// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package releasecache

import (
	"context"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestCacheStoreListStatusAndRemove(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}
	defer cache.Close()

	snapshot := dto.PublishedArtifactSnapshot{
		Project:      "sunbeam",
		Name:         "keystone-k8s",
		ArtifactType: dto.ArtifactCharm,
		Tracks:       []string{"2024.1"},
		Channels: []dto.ReleaseChannelSnapshot{{
			Track:   "2024.1",
			Risk:    dto.ReleaseRiskStable,
			Channel: "2024.1/stable",
		}},
		UpdatedAt: time.Now().UTC(),
	}
	if err := cache.Store(context.Background(), snapshot); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	list, err := cache.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].Name != "keystone-k8s" {
		t.Fatalf("List() = %+v, want stored snapshot", list)
	}

	status, err := cache.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(status) != 1 || status[0].ChannelCount != 1 || status[0].TrackCount != 1 {
		t.Fatalf("Status() = %+v, want one cached channel", status)
	}

	if err := cache.Remove("sunbeam", "keystone-k8s", dto.ArtifactCharm); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	list, err = cache.List(context.Background())
	if err != nil {
		t.Fatalf("List() after remove error = %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List() after remove = %+v, want empty", list)
	}
}

func TestCacheKeepsSameNameAcrossArtifactTypes(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}
	defer cache.Close()

	now := time.Now().UTC()
	for _, snapshot := range []dto.PublishedArtifactSnapshot{
		{
			Project:      "openstack",
			Name:         "keystone",
			ArtifactType: dto.ArtifactSnap,
			Tracks:       []string{"latest"},
			Channels:     []dto.ReleaseChannelSnapshot{{Track: "latest", Risk: dto.ReleaseRiskStable, Channel: "latest/stable"}},
			UpdatedAt:    now,
		},
		{
			Project:      "openstack",
			Name:         "keystone",
			ArtifactType: dto.ArtifactCharm,
			Tracks:       []string{"2024.1"},
			Channels:     []dto.ReleaseChannelSnapshot{{Track: "2024.1", Risk: dto.ReleaseRiskStable, Channel: "2024.1/stable"}},
			UpdatedAt:    now,
		},
	} {
		if err := cache.Store(context.Background(), snapshot); err != nil {
			t.Fatalf("Store(%s) error = %v", snapshot.ArtifactType.String(), err)
		}
	}

	list, err := cache.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List() = %+v, want both snap and charm entries", list)
	}

	status, err := cache.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(status) != 2 {
		t.Fatalf("Status() = %+v, want both snap and charm entries", status)
	}
}

func TestCacheStoresSnapTargetMetadata(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}
	defer cache.Close()

	snapshot := dto.PublishedArtifactSnapshot{
		Project:      "openstack-hypervisor",
		Name:         "openstack-hypervisor",
		ArtifactType: dto.ArtifactSnap,
		Tracks:       []string{"2024.1"},
		Channels: []dto.ReleaseChannelSnapshot{{
			Track:   "2024.1",
			Risk:    dto.ReleaseRiskStable,
			Channel: "2024.1/stable",
			Targets: []dto.ReleaseTargetSnapshot{{
				Architecture: "amd64",
				Base:         dto.ReleaseBase{Name: "core24"},
				Revision:     527,
				Version:      "2024.1",
			}},
		}},
		UpdatedAt: time.Now().UTC(),
	}
	if err := cache.Store(context.Background(), snapshot); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	list, err := cache.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() = %+v, want one stored snapshot", list)
	}
	got := list[0].Channels[0].Targets[0]
	if got.Base.Name != "core24" || got.Revision != 527 || got.Version != "2024.1" {
		t.Fatalf("stored target = %+v, want base/revision/version preserved", got)
	}
}
