// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package snapstore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestSourceFetch(t *testing.T) {
	oldEndpoint := infoEndpoint
	defer func() { infoEndpoint = oldEndpoint }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Snap-Device-Series"); got != "16" {
			t.Fatalf("Snap-Device-Series = %q, want 16", got)
		}
		if got := r.URL.Query().Get("fields"); got != "channel-map,base,revision,version" {
			t.Fatalf("fields = %q, want channel-map,base,revision,version", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"channel-map": []map[string]any{
				{"channel": map[string]any{"architecture": "amd64", "name": "2024.1/stable", "released-at": "2026-03-01T10:00:00Z", "risk": "stable", "track": "2024.1"}, "base": "core24", "revision": 41, "version": "1.2.3"},
				{"channel": map[string]any{"architecture": "arm64", "name": "2024.1/stable", "released-at": "2026-03-01T11:00:00Z", "risk": "stable", "track": "2024.1"}, "base": "core24", "revision": 42, "version": "1.2.3"},
				{"channel": map[string]any{"architecture": "amd64", "name": "2024.1/edge/risc-v", "released-at": "2026-03-02T10:00:00Z", "risk": "edge", "track": "2024.1"}, "base": "core26", "revision": 50, "version": "1.3.0"},
			},
		})
	}))
	defer server.Close()
	infoEndpoint = server.URL + "/"

	source := NewSource(server.Client())
	got, err := source.Fetch(context.Background(), dto.TrackedPublication{
		Project:      "sunbeam",
		Name:         "snap-openstack",
		ArtifactType: dto.ArtifactSnap,
		Tracks:       []string{"2024.1"},
		Branches: []dto.TrackedReleaseBranch{{
			Track:  "2024.1",
			Branch: "risc-v",
			Risks:  []dto.ReleaseRisk{dto.ReleaseRiskEdge},
		}},
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got.Name != "snap-openstack" || len(got.Channels) != 2 {
		t.Fatalf("Fetch() = %+v, want base and branch channels", got)
	}
	var foundBranch bool
	for _, channel := range got.Channels {
		if channel.Branch == "risc-v" {
			foundBranch = true
			if len(channel.Targets) != 1 {
				t.Fatalf("branch channel targets = %+v, want 1 target", channel.Targets)
			}
			if channel.Targets[0].Base.Name != "core26" || channel.Targets[0].Revision != 50 || channel.Targets[0].Version != "1.3.0" {
				t.Fatalf("branch target = %+v, want base/revision/version populated", channel.Targets[0])
			}
		}
	}
	if !foundBranch {
		t.Fatalf("Fetch() channels = %+v, want risc-v branch", got.Channels)
	}
	var foundStable bool
	for _, channel := range got.Channels {
		if channel.Channel != "2024.1/stable" {
			continue
		}
		foundStable = true
		if len(channel.Targets) != 2 {
			t.Fatalf("stable channel targets = %+v, want 2 targets", channel.Targets)
		}
		parts := make([]string, 0, len(channel.Targets))
		for _, target := range channel.Targets {
			parts = append(parts, target.Base.Name+":"+target.Version)
		}
		if !strings.Contains(strings.Join(parts, ","), "core24:1.2.3") {
			t.Fatalf("stable targets = %v, want core24 with version", parts)
		}
	}
	if !foundStable {
		t.Fatalf("Fetch() channels = %+v, want stable channel", got.Channels)
	}
}
