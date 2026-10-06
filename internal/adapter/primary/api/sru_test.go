// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/testsupport"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestSRUAPIReportsMissingSnapshotAndFiltersRows(t *testing.T) {
	cacheDir := t.TempDir()
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())
	application := newEphemeralTestApp(t, &config.Config{})
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	RegisterSRUAPI(srv.API(), application)
	RegisterCacheAPI(srv.API(), application)

	response, err := http.Get(base + "/api/v1/sru")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("missing snapshot status = %d", response.StatusCode)
	}

	path := filepath.Join(cacheDir, "sunbeam-watchtower", "sru", "snapshot.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	snapshot := dto.SRUSnapshot{SyncedAt: time.Now().UTC(), Rows: []dto.SRURow{
		{BugID: "2167438", Package: "neutron", Archive: "uca", Series: "gazpacho", Stage: "updates"},
		{BugID: "2167438", Package: "neutron", Archive: "ubuntu", Series: "resolute", Stage: "updates"},
	}}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	response, err = http.Get(base + "/api/v1/sru?archive=uca")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", response.StatusCode)
	}
	var listed dto.SRUSnapshot
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Rows) != 1 || listed.Rows[0].Archive != "uca" {
		t.Fatalf("filtered rows: %+v", listed.Rows)
	}

	response, err = http.Get(base + "/api/v1/sru/2167438")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("show status = %d", response.StatusCode)
	}
	response, err = http.Get(base + "/api/v1/sru/9999999")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown bug status = %d", response.StatusCode)
	}

	request, err := http.NewRequest(http.MethodDelete, base+"/api/v1/cache/sru", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("cache clear status = %d", response.StatusCode)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("SRU snapshot remains after API clear: %v", err)
	}
	request, err = http.NewRequest(http.MethodPost, base+"/api/v1/cache/sync/sru", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unconfigured cache sync status = %d", response.StatusCode)
	}
}

func TestSRUMigrationAPISeparatesValidationMissingTargetsAndUnavailableOrder(t *testing.T) {
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())
	cfg := &config.Config{Packages: config.PackagesConfig{
		Sets:    map[string][]string{"openstack": {"nova"}},
		Distros: map[string]config.DistroConfig{"ubuntu": {Releases: map[string]config.ReleaseConfig{"jammy": {Backports: map[string]config.BackportConfig{"caracal": {}}}}}},
	}}
	RegisterSRUAPI(srv.API(), newEphemeralTestApp(t, cfg))
	for _, test := range []struct {
		path   string
		status int
	}{
		{"nova/caracal/0", http.StatusUnprocessableEntity},
		{"Nova/caracal/42", http.StatusUnprocessableEntity},
		{"missing/caracal/42", http.StatusConflict},
		{"nova/missing/42", http.StatusNotFound},
		{"nova/caracal/42", http.StatusConflict},
	} {
		response, err := http.Get(base + "/api/v1/sru/migration/" + test.path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != test.status {
			t.Errorf("%s: status=%d, want %d", test.path, response.StatusCode, test.status)
		}
	}
}

func TestSRUMigrationAPICollectsBuglessOccupantAndIndirectDependencies(t *testing.T) {
	testsupport.ClearGitEnvironment(t)
	srv, base := startTestServer(t)
	defer srv.Shutdown(context.Background())
	cfg := &config.Config{Packages: config.PackagesConfig{
		Upstream: &config.UpstreamConfig{Provider: "openstack", ReleasesRepo: "https://example.invalid/releases.git"},
		Distros: map[string]config.DistroConfig{"ubuntu": {Releases: map[string]config.ReleaseConfig{
			"jammy": {Backports: map[string]config.BackportConfig{"caracal": {}}},
			"noble": {Backports: map[string]config.BackportConfig{"epoxy": {}}},
		}}},
	}}
	application := newEphemeralTestApp(t, cfg)
	cacheDir, err := app.UpstreamCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	repo := app.UpstreamRepoPath(cacheDir, cfg.Packages.Upstream.ReleasesRepo)
	if err := os.MkdirAll(filepath.Join(repo, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "data", "series_status.yaml"), []byte("- name: epoxy\n  release-id: 2025.1\n  status: maintained\n- name: caracal\n  release-id: 2024.1\n  status: unmaintained\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"}, {"add", "data/series_status.yaml"},
		{"-c", "user.name=Watchtower Test", "-c", "user.email=watchtower@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "test release metadata"},
	} {
		command := testsupport.GitCommand(args...)
		command.Dir = repo
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v: %s", err, output)
		}
	}
	original := http.DefaultTransport
	withDefaultTransport(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() != "api.launchpad.net" && r.URL.Hostname() != "launchpad.net" {
			return original.RoundTrip(r)
		}
		if r.URL.Path == "/devel/bugs/42" {
			return jsonResponse(http.StatusOK, `{"id":42,"information_type":"Public"}`), nil
		}
		if r.URL.Query().Get("ws.op") == "getPublishedSources" {
			archive := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			series, pocket, _ := strings.Cut(archive, "-")
			ubuntuBase, version := "noble", map[string]string{"staging": "4", "proposed": "3", "updates": "2"}[pocket]
			if series == "caracal" {
				ubuntuBase = "jammy"
				version = map[string]string{"staging": "6", "proposed": "5", "updates": "5"}[pocket]
			}
			publication := lp.SourcePublishing{Status: "Published", SourcePackageName: "nova", SourcePackageVersion: version, DistroSeriesLink: lp.APIBaseURL + "/ubuntu/" + ubuntuBase, SelfLink: lp.APIBaseURL + "/source/" + archive}
			data, _ := json.Marshal(map[string]any{"entries": []lp.SourcePublishing{publication}})
			return jsonResponse(http.StatusOK, string(data)), nil
		}
		if r.URL.Query().Get("ws.op") == "changesFileUrl" {
			archive := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			data, _ := json.Marshal("https://launchpad.net/" + archive + ".changes")
			return jsonResponse(http.StatusOK, string(data)), nil
		}
		body := "Launchpad-Bugs-Fixed: 9\n"
		if strings.HasSuffix(r.URL.Path, "-staging.changes") {
			body = "Launchpad-Bugs-Fixed: 42\n"
		}
		if r.URL.Path == "/epoxy-proposed.changes" {
			body = "Changes:\n  Security update without Launchpad references\n"
		}
		return jsonResponse(http.StatusOK, body), nil
	}))
	RegisterSRUAPI(srv.API(), application)
	response, err := http.Get(base + "/api/v1/sru/migration/nova/caracal/42")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("status=%d, body=%s", response.StatusCode, data)
	}
	var result dto.SRUMigrationChain
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	steps := map[string]dto.SRUMigrationTransition{}
	for _, step := range result.Transitions {
		steps[step.ID] = step
	}
	if steps["uca/epoxy/occupant/updates"].Version != "3" || !slices.Contains(steps["uca/caracal/proposed"].DependsOn, "uca/epoxy/proposed") || !slices.Contains(result.FirstOutstanding, "uca/epoxy/occupant/updates") {
		t.Fatalf("chain=%+v", result)
	}
	if len(result.Targets) != 2 || result.Targets[0].Pockets[1].Publications[0].BugsKnown {
		t.Fatalf("bugless inventory=%+v", result.Targets)
	}
}
