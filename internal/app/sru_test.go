// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

func TestMakeSRURowsAssociatesEachSeriesWithItsPackage(t *testing.T) {
	bug := lp.Bug{ID: 2167438, Title: "[SRU] neutron", WebLink: "https://bugs.launchpad.net/bugs/2167438",
		Tags: []string{"verification-done-resolute", "verification-gazpacho-done"}, InformationType: "Public"}
	tasks := []lp.BugTask{
		{TargetLink: lp.APIBaseURL + "/ubuntu/+source/neutron", Status: "Invalid"},
		{TargetLink: lp.APIBaseURL + "/ubuntu/resolute/+source/neutron", Status: "Fix Released"},
		{TargetLink: lp.APIBaseURL + "/cloud-archive/gazpacho", Status: "Fix Released"},
		{TargetLink: lp.APIBaseURL + "/cloud-archive/hibiscus", Status: "Invalid"},
	}
	releases := map[string]config.ReleaseConfig{
		"resolute": {},
		"noble":    {Backports: map[string]config.BackportConfig{"gazpacho": {ParentRelease: "resolute", SRUParentRequired: true}}},
	}
	rows := makeSRURows([]sruBug{{Bug: bug, Tasks: tasks}}, map[string][]string{"neutron": {"openstack"}}, releases)
	if len(rows) != 2 || rows[0].Verification != "done" || rows[1].Verification != "done" ||
		rows[1].ParentSeries != "resolute" || !rows[1].ParentRequired {
		t.Fatalf("unexpected SRU rows: %+v", rows)
	}
}

func TestSRUCacheStatusAndClear(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	a := &App{}
	status, err := a.SRUCacheStatus()
	if err != nil || !status.SyncedAt.IsZero() {
		t.Fatalf("missing snapshot status = %+v, %v", status, err)
	}
	snapshot := dto.SRUSnapshot{SyncedAt: time.Now().UTC(), Rows: []dto.SRURow{{BugID: "1"}}}
	if err := writeSRUSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	status, err = a.SRUCacheStatus()
	if err != nil || status.Targets != 1 || !status.SyncedAt.Equal(snapshot.SyncedAt) {
		t.Fatalf("saved snapshot status = %+v, %v", status, err)
	}
	if err := a.ClearSRUCache(); err != nil {
		t.Fatal(err)
	}
	if err := a.ClearSRUCache(); err != nil {
		t.Fatalf("clearing missing snapshot: %v", err)
	}
	path := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "sunbeam-watchtower", "sru", "snapshot.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("snapshot remains after clear: %v", err)
	}
}

func TestReadFixedBugsParsesChangesAndRejectsUntrustedURLs(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "Format: 1.8\nLaunchpad-Bugs-Fixed: 2167438\n 2150000\nChanges:\n  unrelated 999999\n")
	}))
	defer server.Close()
	// The URL validator deliberately disallows test-server hosts, so exercise
	// the parser through the same HTTPS transport with an allowed Host header.
	client := server.Client()
	client.Transport = rewriteSRUHost{base: client.Transport, target: server.URL}
	ids, err := readFixedBugs(context.Background(), client, "https://launchpad.net/test.changes")
	if err != nil || !ids["2167438"] || !ids["2150000"] || ids["999999"] {
		t.Fatalf("readFixedBugs = %v, %v", ids, err)
	}
	if _, err := readFixedBugs(context.Background(), client, "https://example.com/test.changes"); err == nil {
		t.Fatal("untrusted changes-file URL accepted")
	}
}

func TestReadFixedBugsRejectsTruncatedChanges(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "Launchpad-Bugs-Fixed: 2167438\n")
		fmt.Fprint(w, strings.Repeat("x", 2<<20))
	}))
	defer server.Close()
	client := server.Client()
	client.Transport = rewriteSRUHost{base: client.Transport, target: server.URL}
	if _, err := readFixedBugs(context.Background(), client, "https://launchpad.net/test.changes"); err == nil {
		t.Fatal("accepted a changes file beyond the size limit")
	}
}

func TestMakeSRURowsWarnsOnAmbiguousCloudArchivePackage(t *testing.T) {
	bug := lp.Bug{ID: 1, InformationType: "Public"}
	tasks := []lp.BugTask{
		{TargetLink: lp.APIBaseURL + "/ubuntu/resolute/+source/neutron"},
		{TargetLink: lp.APIBaseURL + "/ubuntu/resolute/+source/nova"},
		{TargetLink: lp.APIBaseURL + "/cloud-archive/gazpacho"},
	}
	releases := map[string]config.ReleaseConfig{
		"resolute": {},
		"noble":    {Backports: map[string]config.BackportConfig{"gazpacho": {}}},
	}
	rows := makeSRURows([]sruBug{{Bug: bug, Tasks: tasks}}, map[string][]string{
		"neutron": {"openstack"}, "nova": {"openstack"},
	}, releases)
	var warnings int
	for _, row := range rows {
		if row.Archive == "uca" && strings.Contains(row.Warning, "inferred") {
			warnings++
		}
	}
	if warnings != 2 {
		t.Fatalf("expected two flagged UCA rows, got %+v", rows)
	}
}

type rewriteSRUHost struct {
	base   http.RoundTripper
	target string
}

func (r rewriteSRUHost) RoundTrip(req *http.Request) (*http.Response, error) {
	request := req.Clone(req.Context())
	u, _ := url.Parse(r.target)
	request.URL.Scheme, request.URL.Host = u.Scheme, u.Host
	request.Host = u.Host
	return r.base.RoundTrip(request)
}

func TestSRUChangesURLValidation(t *testing.T) {
	for _, raw := range []string{"http://launchpad.net/a", "https://example.com/a", "https://launchpad.net:8443/a", "https://user@launchpad.net/a"} {
		u, _ := url.Parse(raw)
		if validSRUChangesURL(u) {
			t.Errorf("accepted %s", raw)
		}
	}
	u, _ := url.Parse("https://launchpad.net/+files/example_source.changes")
	if !validSRUChangesURL(u) || !strings.Contains(u.Path, ".changes") {
		t.Fatal("real changes URL rejected")
	}
}
