// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gboutry/sunbeam-watchtower/internal/app"
	"github.com/gboutry/sunbeam-watchtower/internal/config"
	"github.com/gboutry/sunbeam-watchtower/pkg/client"
	distro "github.com/gboutry/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
)

func TestPackagesClientWorkflowDiff(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/packages/diff/openstack" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		query := r.URL.Query()
		if got := query["distro"]; len(got) != 1 || got[0] != "ubuntu" {
			t.Fatalf("distro query = %+v, want ubuntu", got)
		}
		if got := query["release"]; len(got) != 1 || got[0] != "noble" {
			t.Fatalf("release query = %+v, want noble", got)
		}
		if query.Get("constraints") != "2025.1" {
			t.Fatalf("constraints = %q, want 2025.1", query.Get("constraints"))
		}
		_ = json.NewEncoder(w).Encode([]dto.PackageDiffResult{{
			Package: "keystone",
			Versions: map[string][]distro.SourcePackage{
				"ubuntu": {{
					Package: "keystone",
					Version: "2:27.0.0-0ubuntu1",
					Suite:   "noble",
				}},
			},
			Upstream: "2025.1",
		}})
	}))
	defer ts.Close()

	workflow := NewPackagesClientWorkflow(NewClientTransport(client.NewClient(ts.URL)), testPackagesApp())
	got, err := workflow.Diff(context.Background(), PackagesDiffRequest{
		Set:         "openstack",
		Distros:     []string{"ubuntu"},
		Releases:    []string{"noble"},
		Backports:   []string{"none"},
		Constraints: "2025.1",
	})
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Package != "keystone" {
		t.Fatalf("Diff() results = %+v, want keystone", got.Results)
	}
	if !got.HasUpstream {
		t.Fatal("HasUpstream = false, want true")
	}
	if len(got.Sources) != 1 || got.Sources[0].Name != "ubuntu" {
		t.Fatalf("Sources = %+v, want ubuntu source", got.Sources)
	}
	if len(got.Sources[0].Entries) != 2 || got.Sources[0].Entries[0].Suite != "noble" || got.Sources[0].Entries[1].Suite != "noble-updates" {
		t.Fatalf("Entries = %+v, want noble and noble-updates suites", got.Sources[0].Entries)
	}
}

func TestPackagesClientWorkflowEffectiveUpstreamReleaseDefaultsFromConfig(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	workflow := NewPackagesClientWorkflow(nil, testPackagesAppWithUpstream())

	got := workflow.effectiveUpstreamRelease(context.Background(), "", "", nil)
	if got != "" {
		t.Fatalf("effectiveUpstreamRelease() = %q, want provider default", got)
	}
}

func TestPackagesClientWorkflowDiffUsesSelectedBackportForUpstream(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("upstream_release"); got != "gazpacho" {
			t.Errorf("upstream_release = %q, want gazpacho", got)
		}
		_ = json.NewEncoder(w).Encode([]dto.PackageDiffResult{})
	}))
	defer ts.Close()

	workflow := NewPackagesClientWorkflow(NewClientTransport(client.NewClient(ts.URL)), testPackagesAppWithUpstream())
	_, err := workflow.Diff(context.Background(), PackagesDiffRequest{
		Set: "openstack", Backports: []string{"gazpacho"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPackagesClientWorkflowShowVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/packages/show/keystone" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query()["backport"]; len(got) != 1 || got[0] != "none" {
			t.Fatalf("backport query = %+v, want none", got)
		}
		_ = json.NewEncoder(w).Encode(dto.PackageDiffResult{
			Package: "keystone",
			Versions: map[string][]distro.SourcePackage{
				"ubuntu": {{
					Package: "keystone",
					Version: "2:27.0.0-0ubuntu1",
					Suite:   "noble",
				}},
			},
			Upstream: "2025.1",
		})
	}))
	defer ts.Close()

	workflow := NewPackagesClientWorkflow(NewClientTransport(client.NewClient(ts.URL)), testPackagesApp())
	got, err := workflow.ShowVersion(context.Background(), PackagesShowVersionRequest{
		Package:         "keystone",
		Distros:         []string{"ubuntu"},
		Releases:        []string{"noble"},
		Backports:       []string{"none"},
		UpstreamRelease: "2025.1",
	})
	if err != nil {
		t.Fatalf("ShowVersion() error = %v", err)
	}
	if got.Result.Package != "keystone" || !got.HasUpstream {
		t.Fatalf("ShowVersion() = %+v, want keystone with upstream", got)
	}
	if len(got.Sources) != 1 || got.Sources[0].Name != "ubuntu" {
		t.Fatalf("Sources = %+v, want ubuntu source", got.Sources)
	}
}

func TestPackagesClientWorkflowEffectiveUpstreamReleaseNeedsUpstreamProvider(t *testing.T) {
	workflow := NewPackagesClientWorkflow(nil, testPackagesApp())

	got := workflow.effectiveUpstreamRelease(context.Background(), "", "", nil)
	if got != "" {
		t.Fatalf("effectiveUpstreamRelease() = %q, want empty", got)
	}
}

func TestHasUpstreamResults(t *testing.T) {
	results := []dto.PackageDiffResult{
		{Package: "nova"},
		{Package: "keystone", Upstream: "27.0.0"},
	}

	if !hasUpstreamResults(results) {
		t.Fatal("hasUpstreamResults() = false, want true")
	}
}

func TestPackagesClientWorkflowExcusesList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/packages/excuses" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		query := r.URL.Query()
		if got := query["tracker"]; len(got) != 1 || got[0] != "ubuntu-devel" {
			t.Fatalf("tracker query = %+v, want ubuntu-devel", got)
		}
		if query.Get("team") != "server" || query.Get("autopkgtest") != "true" {
			t.Fatalf("query = %s, want team/server and autopkgtest true", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]dto.PackageExcuseSummary{{
			Package:       "keystone",
			Tracker:       "ubuntu-devel",
			PrimaryReason: "autopkgtest",
		}})
	}))
	defer ts.Close()

	workflow := NewPackagesClientWorkflow(NewClientTransport(client.NewClient(ts.URL)), nil)
	got, err := workflow.ExcusesList(context.Background(), PackagesExcusesListRequest{
		Trackers:    []string{"ubuntu-devel"},
		Team:        "server",
		Autopkgtest: true,
	})
	if err != nil {
		t.Fatalf("ExcusesList() error = %v", err)
	}
	if len(got) != 1 || got[0].Package != "keystone" {
		t.Fatalf("ExcusesList() = %+v, want keystone", got)
	}
}

func testPackagesApp() *app.App {
	return app.NewApp(&config.Config{
		Packages: config.PackagesConfig{
			Distros: map[string]config.DistroConfig{
				"ubuntu": {
					Mirror:     "http://archive.ubuntu.com/ubuntu",
					Components: []string{"main"},
					Releases: map[string]config.ReleaseConfig{
						"noble": {Suites: []string{"release", "updates"}},
					},
				},
			},
		},
	}, discardFrontendLogger())
}

func testPackagesAppWithUpstream() *app.App {
	cfg := testPackagesApp().GetConfig()
	cfg.Launchpad.DevelopmentFocus = "2025.1"
	cfg.Packages.Upstream = &config.UpstreamConfig{
		Provider:     "openstack",
		ReleasesRepo: "https://opendev.org/openstack/releases",
	}
	return app.NewApp(cfg, discardFrontendLogger())
}
