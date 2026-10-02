// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func newEmptyPackagesApp(t *testing.T) *app.App {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return newEphemeralTestApp(t, &config.Config{})
}

func TestPackagesDiff_UnknownSetReturns404(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEmptyPackagesApp(t)
	RegisterPackagesAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/packages/diff/openstack")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPackagesDiff_UnsyncedLaunchpadSetReturns409(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	application := newEphemeralTestApp(t, &config.Config{Packages: config.PackagesConfig{
		LaunchpadSets: map[string]config.LaunchpadSetConfig{
			"openstack": {Series: "development"},
		},
	}})
	RegisterPackagesAPI(srv.API(), application)
	resp, err := http.Get(base + "/api/v1/packages/diff/openstack")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestEffectivePackagesUpstreamReleaseUsesProviderDefault(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	application := app.NewApp(&config.Config{
		Launchpad: config.LaunchpadConfig{DevelopmentFocus: "2025.1"},
		Packages: config.PackagesConfig{
			Upstream: &config.UpstreamConfig{Provider: "openstack"},
		},
	}, nil)

	got := effectivePackagesUpstreamRelease(context.Background(), application, "", "", nil)
	if got != "" {
		t.Fatalf("effectivePackagesUpstreamRelease() = %q, want provider default", got)
	}
}

func TestEffectivePackagesUpstreamReleaseNeedsUpstreamProvider(t *testing.T) {
	application := app.NewApp(&config.Config{
		Launchpad: config.LaunchpadConfig{DevelopmentFocus: "2025.1"},
	}, nil)

	got := effectivePackagesUpstreamRelease(context.Background(), application, "", "", nil)
	if got != "" {
		t.Fatalf("effectivePackagesUpstreamRelease() = %q, want empty", got)
	}
}

func TestEffectivePackagesUpstreamReleaseUsesSelectedBackport(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	application := app.NewApp(&config.Config{
		Packages: config.PackagesConfig{
			Upstream: &config.UpstreamConfig{Provider: "openstack"},
		},
	}, nil)
	ctx := context.Background()
	if got := effectivePackagesUpstreamRelease(ctx, application, "", "", []string{"gazpacho"}); got != "gazpacho" {
		t.Fatalf("selected backport upstream release = %q, want gazpacho", got)
	}
	if got := effectivePackagesUpstreamRelease(ctx, application, "2025.1", "", []string{"gazpacho"}); got != "2025.1" {
		t.Fatalf("explicit upstream release = %q, want 2025.1", got)
	}
	if got := effectivePackagesUpstreamRelease(ctx, application, "", "", []string{"gazpacho", "flamingo"}); got != "" {
		t.Fatalf("multiple backports upstream release = %q, want provider default", got)
	}
}

func TestFilterBehindUpstreamTreatsEquivalentRCTagsAsCurrent(t *testing.T) {
	sources := []dto.PackageSource{{Name: "ubuntu/gazpacho"}}
	results := []dto.PackageDiffResult{
		{
			Package:  "ovn-bgp-agent",
			Upstream: "7.0.0.0rc1",
			Versions: map[string][]distro.SourcePackage{
				"ubuntu/gazpacho": {
					{Version: "6.0.0-0ubuntu1"},
					{Version: "7.0.0~rc1-0ubuntu1~cloud0"},
				},
			},
		},
		{
			Package:  "nova",
			Upstream: "32.0.0",
			Versions: map[string][]distro.SourcePackage{
				"ubuntu/gazpacho": {{Version: "31.0.0-0ubuntu1"}},
			},
		},
	}
	got := filterBehindUpstreamResults(results, sources, true)
	if len(got) != 1 || got[0].Package != "nova" {
		t.Fatalf("merged filtered results = %+v, want only nova", got)
	}
	got = filterBehindUpstreamResults(results, sources, false)
	if len(got) != 2 {
		t.Fatalf("unmerged filtered results = %+v, want both packages", got)
	}
}

func TestPackagesList_NoConfiguredSourcesReturns400(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEmptyPackagesApp(t)
	RegisterPackagesAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/packages/list?distro=ubuntu")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPackagesShow_NoConfiguredSourcesReturns200(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEmptyPackagesApp(t)
	RegisterPackagesAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/packages/show/nova?distro=ubuntu")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestPackagesDsc_InvalidPackageFormatReturns400(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEmptyPackagesApp(t)
	RegisterPackagesAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/packages/dsc?packages=invalid-format")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPackagesExcuses_InvalidTrackerReturns400(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEmptyPackagesApp(t)
	RegisterPackagesAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/packages/excuses?tracker=not-valid")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPackagesCacheStatus_EmptyConfigReturns200(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEmptyPackagesApp(t)
	RegisterPackagesAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/packages/cache/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
}

func TestPackagesRdepends_NoConfiguredSourcesReturns400(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEmptyPackagesApp(t)
	RegisterPackagesAPI(srv.API(), application)

	resp, err := http.Get(base + "/api/v1/packages/rdepends/nova?distro=ubuntu")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPackagesCacheSync_RejectsBackportAsRelease(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())

	application := newEphemeralTestApp(t, packageSyncFilterTestConfig())
	RegisterPackagesAPI(srv.API(), application)

	resp, err := http.Post(
		base+"/api/v1/packages/cache/sync",
		"application/json",
		bytes.NewBufferString(`{"distros":["ubuntu"],"releases":["gazpacho"]}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var body struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Detail, `unknown distro release "gazpacho"`) || !strings.Contains(body.Detail, "--backport") {
		t.Fatalf("detail = %q, want release/backport guidance", body.Detail)
	}
}

func packageSyncFilterTestConfig() *config.Config {
	return &config.Config{
		Packages: config.PackagesConfig{
			Distros: map[string]config.DistroConfig{
				"ubuntu": {
					Mirror:     "http://archive.ubuntu.com/ubuntu",
					Components: []string{"main"},
					Releases: map[string]config.ReleaseConfig{
						"noble": {
							Suites: []string{"release"},
							Backports: map[string]config.BackportConfig{
								"gazpacho": {
									ParentRelease: "resolute",
									Sources: []config.DistroSourceConfig{{
										Mirror:     "http://ppa.example.com",
										Suites:     []string{"updates"},
										Components: []string{"main"},
									}},
								},
							},
						},
						"resolute": {
							Suites: []string{"release"},
						},
					},
				},
			},
		},
	}
}
