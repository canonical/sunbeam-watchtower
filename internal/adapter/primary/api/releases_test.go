// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestReleasesListAndShowEndpoints(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	application := newEphemeralTestApp(t, &config.Config{})
	cache, err := application.ReleaseCache()
	if err != nil {
		t.Fatalf("ReleaseCache() error = %v", err)
	}
	defer application.Close()
	if err := cache.Store(context.Background(), releaseFixture("sunbeam", "snap-openstack", "2024.1")); err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())
	RegisterReleasesAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/releases?track=2024.1&branch=risc-v")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var list struct {
		Releases []map[string]any `json:"releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Releases) != 1 {
		t.Fatalf("releases = %+v, want one row", list.Releases)
	}
	if _, ok := list.Releases[0]["released_at"]; !ok {
		t.Fatalf("releases = %+v, want released_at field", list.Releases)
	}

	resp2, err := http.Get(base + "/api/v1/releases/snap-openstack")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
}

func TestReleasesShowEndpointRequiresTypeWhenNameMatchesMultipleArtifactTypes(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	application := newEphemeralTestApp(t, &config.Config{})
	cache, err := application.ReleaseCache()
	if err != nil {
		t.Fatalf("ReleaseCache() error = %v", err)
	}
	defer application.Close()

	snap := releaseFixture("openstack", "keystone", "latest")
	charm := releaseFixture("openstack", "keystone", "2024.1")
	charm.ArtifactType = dto.ArtifactCharm
	if err := cache.Store(context.Background(), snap); err != nil {
		t.Fatalf("Store(snap) error = %v", err)
	}
	if err := cache.Store(context.Background(), charm); err != nil {
		t.Fatalf("Store(charm) error = %v", err)
	}

	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())
	RegisterReleasesAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/releases?name=keystone")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected list 200, got %d", resp.StatusCode)
	}
	var list struct {
		Releases []dto.ReleaseListEntry `json:"releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Releases) != 2 {
		t.Fatalf("list releases = %+v, want both snap and charm rows", list.Releases)
	}

	resp2, err := http.Get(base + "/api/v1/releases/keystone")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("expected show 409, got %d", resp2.StatusCode)
	}
	var apiErr struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&apiErr); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"multiple artifact types", "charm, snap", "use the type filter"} {
		if !strings.Contains(apiErr.Detail, want) {
			t.Fatalf("error detail %q missing %q", apiErr.Detail, want)
		}
	}

	resp3, err := http.Get(base + "/api/v1/releases/keystone?type=charm")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected typed show 200, got %d", resp3.StatusCode)
	}
}

func releaseFixture(project, name, track string) dto.PublishedArtifactSnapshot {
	return dto.PublishedArtifactSnapshot{
		Project:      project,
		Name:         name,
		ArtifactType: dto.ArtifactSnap,
		Tracks:       []string{track},
		Channels: []dto.ReleaseChannelSnapshot{{
			Track:     track,
			Risk:      dto.ReleaseRiskStable,
			Branch:    "risc-v",
			Channel:   track + "/stable/risc-v",
			UpdatedAt: time.Now().UTC(),
			Targets:   []dto.ReleaseTargetSnapshot{{Architecture: "amd64", Revision: 12, Version: "1.2.3"}},
		}},
		UpdatedAt: time.Now().UTC(),
	}
}
