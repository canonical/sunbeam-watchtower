// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestReleaseServerWorkflowListShowAndStatus(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	application := app.NewApp(&config.Config{}, nil)
	cache, err := application.ReleaseCache()
	if err != nil {
		t.Fatalf("ReleaseCache() error = %v", err)
	}
	defer application.Close()
	if err := cache.Store(context.Background(), dto.PublishedArtifactSnapshot{
		Project:      "sunbeam",
		Name:         "snap-openstack",
		ArtifactType: dto.ArtifactSnap,
		Tracks:       []string{"2024.1"},
		Channels: []dto.ReleaseChannelSnapshot{{
			Track:     "2024.1",
			Risk:      dto.ReleaseRiskStable,
			Channel:   "2024.1/stable",
			UpdatedAt: time.Now().UTC(),
		}},
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	workflow := NewReleaseServerWorkflow(application)
	list, err := workflow.List(context.Background(), dto.ReleaseListQuery{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].Name != "snap-openstack" {
		t.Fatalf("List() = %+v, want one row", list)
	}
	show, err := workflow.Show(context.Background(), "snap-openstack", &[]dto.ArtifactType{dto.ArtifactSnap}[0], "", "")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if show.Name != "snap-openstack" {
		t.Fatalf("Show() = %+v, want snap-openstack", show)
	}
	status, err := workflow.CacheStatus(context.Background())
	if err != nil {
		t.Fatalf("CacheStatus() error = %v", err)
	}
	if len(status) != 1 || status[0].Name != "snap-openstack" {
		t.Fatalf("CacheStatus() = %+v, want cached artifact", status)
	}
}

func TestReleaseServerWorkflowSyncCacheReportsCounts(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	application := app.NewApp(&config.Config{}, nil)
	defer application.Close()

	workflow := NewReleaseServerWorkflow(application)
	result, err := workflow.SyncCache(context.Background())
	if err != nil {
		t.Fatalf("SyncCache() error = %v", err)
	}
	if result.Status != "ok" || result.Discovered != 0 || result.Synced != 0 || result.Skipped != 0 {
		t.Fatalf("SyncCache() = %+v, want zero-count ok result", result)
	}
}
