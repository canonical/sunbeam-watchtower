// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/service/sru"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

func TestSRUMigrationTargetsUsesCachedOrderAndConfiguredParents(t *testing.T) {
	releases := map[string]config.ReleaseConfig{
		"focal":  {Backports: map[string]config.BackportConfig{"yoga": {ParentRelease: "jammy"}}},
		"jammy":  {Backports: map[string]config.BackportConfig{"caracal": {ParentRelease: "noble", SRUParentRequired: true}}},
		"noble":  {Backports: map[string]config.BackportConfig{"epoxy": {ParentRelease: "plucky"}}},
		"plucky": {},
	}
	metadata := []dto.UpstreamSeries{{Name: "epoxy", ReleaseID: "2025.1", Status: "maintained"}, {Name: "caracal", ReleaseID: "2024.1", Status: "unmaintained"}, {Name: "yoga", Status: "end of life"}}
	got, err := sruMigrationTargets(releases, metadata, "caracal")
	if err != nil || len(got) != 6 || got[0].Series != "plucky" || got[1].Series != "epoxy" || got[3].Series != "caracal" || !got[3].ParentRequired || got[5].Series != "yoga" || got[5].ReleaseID != "" {
		t.Fatalf("targets = %+v, %v", got, err)
	}
	if _, err := sruMigrationTargets(releases, metadata, "missing"); err == nil {
		t.Fatal("unknown selected target accepted")
	}
	if _, err := sruMigrationTargets(releases, metadata[:1], "epoxy"); err == nil {
		t.Fatal("missing order accepted")
	}
	delete(releases, "plucky")
	if _, err := sruMigrationTargets(releases, metadata, "caracal"); err == nil {
		t.Fatal("missing parent accepted")
	}
}

func TestFetchSRUMigrationTargetRetainsObservedVersionsAndUnknowns(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "-proposed"):
			http.Error(w, "unavailable", http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "-staging") || strings.HasSuffix(r.URL.Path, "-updates"):
			if r.URL.Query().Get("status") != "Published" || r.URL.Query().Get("exact_match") != "true" {
				t.Errorf("query = %s", r.URL.RawQuery)
			}
			version, id := "2.0-1", "staging"
			if strings.HasSuffix(r.URL.Path, "-updates") {
				version, id = "1.0-1", "updates"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"entries": []lp.SourcePublishing{
				{Status: "Published", SourcePackageName: "nova", SourcePackageVersion: version, DistroSeriesLink: lp.APIBaseURL + "/ubuntu/noble", SelfLink: lp.APIBaseURL + "/" + id},
				{Status: "Superseded", SourcePackageName: "nova", SourcePackageVersion: "0.5-1", DistroSeriesLink: lp.APIBaseURL + "/ubuntu/noble", SelfLink: lp.APIBaseURL + "/ignored"},
			}})
		case r.URL.Path == "/devel/staging":
			_ = json.NewEncoder(w).Encode("https://launchpad.net/nova.changes")
		case r.URL.Path == "/devel/updates":
			_ = json.NewEncoder(w).Encode("")
		case r.URL.Path == "/nova.changes":
			fmt.Fprint(w, "Launchpad-Bugs-Fixed: 42 21\n")
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	transport := server.Client()
	transport.Transport = rewriteSRUHost{base: transport.Transport, target: server.URL}
	client := lp.NewClient(nil, nil, transport)
	got := fetchSRUMigrationTarget(context.Background(), client, transport, "nova", dto.SRUMigrationTarget{Archive: "uca", Series: "epoxy", UbuntuBase: "noble"})
	if len(got.Pockets) != 3 || !got.Pockets[0].Known || got.Pockets[1].Known || got.Pockets[1].Warning == "" || !got.Pockets[2].Known {
		t.Fatalf("pockets=%+v", got.Pockets)
	}
	staging := got.Pockets[0].Publications
	if len(staging) != 1 || !staging[0].BugsKnown || strings.Join(staging[0].BugIDs, ",") != "21,42" {
		t.Fatalf("staging=%+v", staging)
	}
	updates := got.Pockets[2].Publications
	if len(updates) != 1 || updates[0].Version != "1.0-1" || updates[0].BugsKnown || updates[0].Warning == "" {
		t.Fatalf("updates=%+v", updates)
	}
}

func TestFetchSRUMigrationUbuntuEmptyPocketsRemainKnown(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/devel/ubuntu/+archive/primary" || r.URL.Query().Get("distro_series") != lp.APIBaseURL+"/ubuntu/noble" || (r.URL.Query().Get("pocket") != "Proposed" && r.URL.Query().Get("pocket") != "Updates") {
			t.Errorf("unexpected Ubuntu publication request %s", r.URL)
		}
		fmt.Fprint(w, `{"entries":[]}`)
	}))
	defer server.Close()
	transport := server.Client()
	transport.Transport = rewriteSRUHost{base: transport.Transport, target: server.URL}
	got := fetchSRUMigrationTarget(context.Background(), lp.NewClient(nil, nil, transport), transport, "nova", dto.SRUMigrationTarget{Archive: "ubuntu", Series: "noble", UbuntuBase: "noble"})
	if requests != 2 || len(got.Pockets) != 2 {
		t.Fatalf("requests=%d, pockets=%+v", requests, got.Pockets)
	}
	for _, pocket := range got.Pockets {
		if !pocket.Known || len(pocket.Publications) != 0 {
			t.Fatalf("empty pocket became unavailable: %+v", pocket)
		}
	}
}

func TestSRUMigrationTargetsRejectsAmbiguousMetadata(t *testing.T) {
	releases := map[string]config.ReleaseConfig{"jammy": {Backports: map[string]config.BackportConfig{"caracal": {}}}}
	legacy, err := sruMigrationTargets(releases, []dto.UpstreamSeries{{Name: "caracal"}}, "caracal")
	if err != nil || len(legacy) != 1 || legacy[0].ReleaseID != "" {
		t.Fatalf("name-only chronological metadata rejected: %+v, %v", legacy, err)
	}

	for _, metadata := range [][]dto.UpstreamSeries{
		{{Name: "caracal", ReleaseID: "2024.1"}, {Name: "caracal", ReleaseID: "2024.1"}},
		{{Name: "caracal", ReleaseID: "2024.1"}, {Name: "other", ReleaseID: "2024.1"}},
	} {
		if _, err := sruMigrationTargets(releases, metadata, "caracal"); err == nil {
			t.Fatalf("ambiguous metadata accepted: %+v", metadata)
		}
	}
	releases["focal"] = config.ReleaseConfig{Backports: map[string]config.BackportConfig{"caracal": {}}}
	if _, err := sruMigrationTargets(releases, []dto.UpstreamSeries{{Name: "caracal", ReleaseID: "2024.1"}}, "caracal"); err == nil {
		t.Fatal("duplicate UCA target accepted")
	}
}

func TestBuglessAndUnassociatedPublicationsStillOccupyProposed(t *testing.T) {
	for _, mode := range []string{"no bug references", "missing changes file", "failed changes fetch"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pocket := ""
				for _, candidate := range []string{"staging", "proposed", "updates"} {
					if strings.HasSuffix(r.URL.Path, "-"+candidate) {
						pocket = candidate
					}
				}
				if pocket != "" {
					version := map[string]string{"staging": "3", "proposed": "2", "updates": "1"}[pocket]
					_ = json.NewEncoder(w).Encode(map[string]any{"entries": []lp.SourcePublishing{{Status: "Published", SourcePackageName: "nova", SourcePackageVersion: version, DistroSeriesLink: lp.APIBaseURL + "/ubuntu/noble", SelfLink: lp.APIBaseURL + "/" + pocket}}})
					return
				}
				if r.URL.Query().Get("ws.op") == "changesFileUrl" {
					pocket = strings.TrimPrefix(r.URL.Path, "/devel/")
					changes := "https://launchpad.net/" + pocket + ".changes"
					if pocket == "proposed" && mode == "missing changes file" {
						changes = ""
					}
					_ = json.NewEncoder(w).Encode(changes)
					return
				}
				switch r.URL.Path {
				case "/staging.changes":
					fmt.Fprint(w, "Launchpad-Bugs-Fixed: 42\n")
				case "/updates.changes":
					fmt.Fprint(w, "Launchpad-Bugs-Fixed: 9\n")
				case "/proposed.changes":
					if mode == "failed changes fetch" {
						http.NotFound(w, r)
					} else {
						fmt.Fprint(w, "Changes:\n  Security update without Launchpad references\n")
					}
				default:
					t.Errorf("unexpected request %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			transport := server.Client()
			transport.Transport = rewriteSRUHost{base: transport.Transport, target: server.URL}
			higher := fetchSRUMigrationTarget(context.Background(), lp.NewClient(nil, nil, transport), transport, "nova", dto.SRUMigrationTarget{Archive: "uca", Series: "epoxy", UbuntuBase: "noble", ReleaseID: "2025.1"})
			proposed := higher.Pockets[1]
			if !proposed.Known || len(proposed.Publications) != 1 || proposed.Publications[0].Version != "2" || proposed.Publications[0].BugsKnown || proposed.Publications[0].Warning == "" {
				t.Fatalf("unassociated publication disappeared: %+v", proposed)
			}
			selected := dto.SRUMigrationTarget{Archive: "uca", Series: "caracal", UbuntuBase: "jammy", ReleaseID: "2024.1", Pockets: []dto.SRUPocketObservation{
				{Pocket: "staging", Known: true, Publications: []dto.SRUPublication{{Version: "8", BugsKnown: true, BugIDs: []string{"42"}}}},
				{Pocket: "proposed", Known: true, Publications: []dto.SRUPublication{{Version: "7", BugsKnown: true, BugIDs: []string{"9"}}}},
				{Pocket: "updates", Known: true, Publications: []dto.SRUPublication{{Version: "7", BugsKnown: true, BugIDs: []string{"9"}}}},
			}}
			result := sru.MigrationChain(dto.SRUMigrationQuery{Package: "nova", Series: "caracal", BugID: "42"}, []dto.SRUMigrationTarget{higher, selected}, time.Now())
			steps := map[string]dto.SRUMigrationTransition{}
			for _, step := range result.Transitions {
				steps[step.ID] = step
			}
			occupant := steps["uca/epoxy/occupant/updates"]
			if occupant.Version != "2" || occupant.Kind != "occupant" || !slices.Contains(steps["uca/epoxy/proposed"].DependsOn, occupant.ID) || !slices.Contains(steps["uca/caracal/proposed"].DependsOn, "uca/epoxy/proposed") {
				t.Fatalf("missing indirect wait chain: %+v", result)
			}
			if steps["uca/epoxy/proposed"].State != "unknown" {
				t.Fatalf("missing fix association treated as certainty: %+v", result)
			}
		})
	}
}

func TestSRUMigrationValidatesQueriesBeforeUpstreamAccess(t *testing.T) {
	application := NewApp(&config.Config{}, nil)
	for _, query := range []dto.SRUMigrationQuery{
		{Package: "nova", Series: "caracal", BugID: "0"},
		{Package: "../../nova", Series: "caracal", BugID: "42"},
		{Package: "nova", Series: "../caracal", BugID: "42"},
	} {
		if _, err := application.SRUMigration(context.Background(), query); !errors.Is(err, ErrSRUMigrationQuery) {
			t.Fatalf("query=%+v, error=%v", query, err)
		}
	}
}

func TestSRUMigrationExplicitPackageDoesNotRequireMonitoringSets(t *testing.T) {
	for _, sets := range []map[string][]string{nil, {"openstack": {"nova"}}} {
		application := NewApp(&config.Config{Packages: config.PackagesConfig{
			Sets: sets,
			Distros: map[string]config.DistroConfig{"ubuntu": {Releases: map[string]config.ReleaseConfig{
				"focal": {Backports: map[string]config.BackportConfig{"yoga": {}}},
			}}},
		}}, nil)
		_, err := application.SRUMigration(context.Background(), dto.SRUMigrationQuery{Package: "openvswitch", Series: "yoga", BugID: "2154006"})
		if !errors.Is(err, ErrSRUMigrationUnavailable) {
			t.Fatalf("explicit package rejected before ordering lookup: %v", err)
		}
	}
}
