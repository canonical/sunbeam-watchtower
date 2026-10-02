// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestReleaseClientWorkflowListAndShow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/releases":
			if got := r.URL.Query().Get("track"); got != "2024.1" {
				t.Fatalf("track query = %q, want 2024.1", got)
			}
			if got := r.URL.Query().Get("branch"); got != "risc-v" {
				t.Fatalf("branch query = %q, want risc-v", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"releases": []dto.ReleaseListEntry{{Name: "snap-openstack", Track: "2024.1", Risk: dto.ReleaseRiskStable, Branch: "risc-v"}}})
		case "/api/v1/releases/snap-openstack":
			if got := r.URL.Query().Get("branch"); got != "risc-v" {
				t.Fatalf("show branch query = %q, want risc-v", got)
			}
			_ = json.NewEncoder(w).Encode(dto.ReleaseShowResult{Name: "snap-openstack", ArtifactType: dto.ArtifactSnap})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	workflow := NewReleaseClientWorkflow(NewClientTransport(client.NewClient(server.URL)), nil)
	list, err := workflow.List(context.Background(), ReleasesListRequest{Tracks: []string{"2024.1"}, Branches: []string{"risc-v"}})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].Name != "snap-openstack" {
		t.Fatalf("List() = %+v, want one row", list)
	}
	show, err := workflow.Show(context.Background(), ReleasesShowRequest{Name: "snap-openstack", Branch: "risc-v"})
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if show.Name != "snap-openstack" || show.ArtifactType != dto.ArtifactSnap {
		t.Fatalf("Show() = %+v, want snap-openstack", show)
	}
}

func TestReleaseClientWorkflowAppliesTargetProfileFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/releases":
			_ = json.NewEncoder(w).Encode(map[string]any{"releases": []dto.ReleaseListEntry{{
				Project: "openstack",
				Name:    "snap-openstack",
				Targets: []dto.ReleaseTargetSnapshot{
					{Architecture: "amd64", Base: dto.ReleaseBase{Name: "ubuntu", Channel: "22.04"}, Revision: 40, Version: "1.2.2"},
					{Architecture: "amd64", Base: dto.ReleaseBase{Name: "ubuntu", Channel: "24.04"}, Revision: 41, Version: "1.2.3"},
				},
			}}})
		case "/api/v1/releases/snap-openstack":
			_ = json.NewEncoder(w).Encode(dto.ReleaseShowResult{
				Project: "openstack",
				Name:    "snap-openstack",
				Channels: []dto.ReleaseChannelSnapshot{
					{
						Channel: "2024.1/stable",
						Targets: []dto.ReleaseTargetSnapshot{
							{Architecture: "amd64", Base: dto.ReleaseBase{Name: "ubuntu", Channel: "22.04"}, Revision: 40},
						},
					},
					{
						Channel: "2025.1/stable",
						Targets: []dto.ReleaseTargetSnapshot{
							{Architecture: "amd64", Base: dto.ReleaseBase{Name: "ubuntu", Channel: "24.04"}, Revision: 41},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := &config.Config{
		Releases: config.ReleasesConfig{
			DefaultTargetProfile: "noble-and-newer",
			TargetProfiles: map[string]config.ReleaseTargetProfileConfig{
				"noble-and-newer": {
					Include: []config.ReleaseTargetMatcherConfig{{BaseNames: []string{"ubuntu"}, MinBaseChannel: "24.04"}},
				},
			},
		},
	}
	workflow := NewReleaseClientWorkflow(
		NewClientTransport(client.NewClient(server.URL)),
		app.NewAppWithOptions(cfg, nil, app.Options{}),
	)

	list, err := workflow.List(context.Background(), ReleasesListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got := len(list); got != 1 {
		t.Fatalf("len(list) = %d, want 1", got)
	}
	if got := len(list[0].Targets); got != 1 {
		t.Fatalf("len(list[0].Targets) = %d, want 1", got)
	}
	if got := list[0].Targets[0].Base.Channel; got != "24.04" {
		t.Fatalf("filtered list target base = %q, want 24.04", got)
	}

	show, err := workflow.Show(context.Background(), ReleasesShowRequest{Name: "snap-openstack"})
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if got := len(show.Channels); got != 1 {
		t.Fatalf("len(show.Channels) = %d, want 1", got)
	}
	if got := show.Channels[0].Channel; got != "2025.1/stable" {
		t.Fatalf("show.Channels[0].Channel = %q, want 2025.1/stable", got)
	}

	allTargets, err := workflow.List(context.Background(), ReleasesListRequest{AllTargets: true})
	if err != nil {
		t.Fatalf("List(AllTargets) error = %v", err)
	}
	if got := len(allTargets[0].Targets); got != 2 {
		t.Fatalf("len(allTargets[0].Targets) = %d, want 2", got)
	}
}
