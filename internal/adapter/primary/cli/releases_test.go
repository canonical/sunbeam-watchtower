// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestReleasesCommandsRenderListAndShow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/releases":
			_ = json.NewEncoder(w).Encode(map[string]any{"releases": []dto.ReleaseListEntry{{
				Project:      "sunbeam",
				ArtifactType: dto.ArtifactSnap,
				Name:         "snap-openstack",
				Track:        "2024.1",
				Risk:         dto.ReleaseRiskStable,
				Branch:       "risc-v",
				ReleasedAt:   time.Date(2026, 3, 7, 21, 0, 0, 0, time.UTC),
				Targets: []dto.ReleaseTargetSnapshot{
					{Architecture: "amd64", Base: dto.ReleaseBase{Name: "ubuntu", Channel: "22.04"}, Revision: 40, Version: "1.2.2"},
					{Architecture: "amd64", Base: dto.ReleaseBase{Name: "ubuntu", Channel: "24.04"}, Revision: 41, Version: "1.2.3"},
				},
			}}})
		case "/api/v1/releases/snap-openstack":
			_ = json.NewEncoder(w).Encode(dto.ReleaseShowResult{
				Project:      "sunbeam",
				ArtifactType: dto.ArtifactSnap,
				Name:         "snap-openstack",
				Tracks:       []string{"2024.1"},
				Channels: []dto.ReleaseChannelSnapshot{{
					Track:   "2024.1",
					Risk:    dto.ReleaseRiskStable,
					Branch:  "risc-v",
					Targets: []dto.ReleaseTargetSnapshot{{Architecture: "amd64", Base: dto.ReleaseBase{Name: "ubuntu", Channel: "24.04"}, Revision: 41, Version: "1.2.3"}},
				}},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "table",
		Client: client.NewClient(server.URL),
		App: app.NewAppWithOptions(&config.Config{
			Releases: config.ReleasesConfig{
				TargetProfiles: map[string]config.ReleaseTargetProfileConfig{
					"noble-and-newer": {
						Include: []config.ReleaseTargetMatcherConfig{{BaseNames: []string{"ubuntu"}, MinBaseChannel: "24.04"}},
					},
				},
			},
		}, nil, app.Options{}),
		Logger: discardTestLogger(),
	}
	cmd := newReleasesCmd(opts)
	cmd.SetArgs([]string{"list", "--target-profile", "noble-and-newer"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("list Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "snap-openstack") || !strings.Contains(out.String(), "risc-v") || !strings.Contains(out.String(), "amd64@ubuntu/24.04:r41/1.2.3") {
		t.Fatalf("unexpected list output: %q", out.String())
	}
	if strings.Contains(out.String(), "22.04") {
		t.Fatalf("filtered list output should hide 22.04 targets: %q", out.String())
	}

	out.Reset()
	cmd = newReleasesCmd(opts)
	cmd.SetArgs([]string{"show", "snap-openstack", "--target-profile", "noble-and-newer"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("show Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Project:") || !strings.Contains(out.String(), "amd64@ubuntu/24.04:r41/1.2.3") {
		t.Fatalf("unexpected show output: %q", out.String())
	}

	out.Reset()
	cmd = newReleasesCmd(opts)
	cmd.SetArgs([]string{"list", "--target-profile", "noble-and-newer", "--all-targets"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("list --all-targets Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "amd64@ubuntu/22.04:r40/1.2.2") {
		t.Fatalf("--all-targets should restore hidden targets: %q", out.String())
	}
}

func TestReleasesCommandsRequireTypeForSameNameAcrossArtifactTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/releases":
			_ = json.NewEncoder(w).Encode(map[string]any{"releases": []dto.ReleaseListEntry{{
				Project:      "openstack",
				ArtifactType: dto.ArtifactSnap,
				Name:         "keystone",
				Track:        "latest",
				Risk:         dto.ReleaseRiskStable,
			}, {
				Project:      "openstack",
				ArtifactType: dto.ArtifactCharm,
				Name:         "keystone",
				Track:        "2024.1",
				Risk:         dto.ReleaseRiskStable,
			}}})
		case "/api/v1/releases/keystone":
			if r.URL.Query().Get("type") == "charm" {
				_ = json.NewEncoder(w).Encode(dto.ReleaseShowResult{
					Project:      "openstack",
					ArtifactType: dto.ArtifactCharm,
					Name:         "keystone",
					Tracks:       []string{"2024.1"},
				})
				return
			}
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"title":  "Conflict",
				"status": http.StatusConflict,
				"detail": `release "keystone" matched multiple artifact types (charm, snap); use the type filter`,
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	opts := &Options{
		Out:    &out,
		ErrOut: &bytes.Buffer{},
		Output: "table",
		Client: client.NewClient(server.URL),
		App:    app.NewAppWithOptions(&config.Config{}, nil, app.Options{}),
		Logger: discardTestLogger(),
	}

	cmd := newReleasesCmd(opts)
	cmd.SetArgs([]string{"list", "keystone"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("list Execute() error = %v", err)
	}
	if strings.Count(out.String(), "keystone") < 2 || !strings.Contains(out.String(), "snap") || !strings.Contains(out.String(), "charm") {
		t.Fatalf("list output should show both same-name entries: %q", out.String())
	}

	out.Reset()
	cmd = newReleasesCmd(opts)
	cmd.SetArgs([]string{"show", "keystone"})
	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("show Execute() error = nil, want ambiguity error")
	}
	if !strings.Contains(err.Error(), "multiple artifact types") || !strings.Contains(err.Error(), "use the type filter") {
		t.Fatalf("show error = %v, want explicit ambiguity message", err)
	}

	out.Reset()
	cmd = newReleasesCmd(opts)
	cmd.SetArgs([]string{"show", "keystone", "--type", "charm"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("typed show Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Type:") || !strings.Contains(out.String(), "charm") {
		t.Fatalf("typed show output = %q, want charm detail", out.String())
	}
}
