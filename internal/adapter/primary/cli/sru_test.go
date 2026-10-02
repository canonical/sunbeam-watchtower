// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderSRUUsesStyledTablesAndLinks(t *testing.T) {
	snapshot := &dto.SRUSnapshot{SyncedAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC), Rows: []dto.SRURow{{
		BugID: "2167438", BugURL: "https://bugs.launchpad.net/bugs/2167438", Title: "Neutron SRU",
		Package: "neutron", Archive: "uca", Series: "gazpacho", TaskStatus: "Fix Released",
		TaskURL: "https://bugs.launchpad.net/cloud-archive/gazpacho/+bug/2167438",
		Stage:   "updates", Verification: "done", Version: "2:28.0.2-0ubuntu1~cloud0",
		Warning: "example warning", Evidence: []dto.SRUArchiveEvidence{{
			Stage: "updates", Version: "2:28.0.2-0ubuntu1~cloud0", URL: "https://api.launchpad.net/devel/+sourcepub/1",
		}},
	}}}
	var out bytes.Buffer
	if err := renderSRUListTableWidth(&out, newOutputStyler(true), snapshot, 120, sruListOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"BUG", "PACKAGE", "TARGETS", "PROGRESS", "#2167438", "Neutron SRU", "updates/done", "\x1b[", "\x1b]8;;"} {
		if !strings.Contains(got, want) {
			t.Errorf("styled list missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "Warnings") || strings.Contains(got, "example warning") {
		t.Errorf("default list includes diagnostics: %q", got)
	}
	out.Reset()
	if err := renderSRUDetailTable(&out, newOutputStyler(false), snapshot); err != nil {
		t.Fatal(err)
	}
	got = out.String()
	for _, want := range []string{"Bug #2167438", "Archive evidence", "PUBLICATION", "example warning", "https://api.launchpad.net/devel/+sourcepub/1"} {
		if !strings.Contains(got, want) {
			t.Errorf("detail missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "\x1b[") || strings.Contains(got, "\x1b]8;;") {
		t.Fatalf("plain detail contains terminal escapes: %q", got)
	}
	out.Reset()
	if err := renderSRU(&Options{Out: &out, Output: "json"}, snapshot, false, sruListOptions{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b[") || !strings.Contains(out.String(), `"bug_id": "2167438"`) {
		t.Fatalf("JSON output changed: %q", out.String())
	}
}

func TestSRURenderingSanitizesUntrustedTerminalText(t *testing.T) {
	snapshot := &dto.SRUSnapshot{SyncedAt: time.Now(), Rows: []dto.SRURow{{
		BugID: "1", BugURL: "https://bugs.launchpad.net/bugs/1\x1b]8;;https://example.com", Title: "line\n\x1b[31mred",
		Package: "neutron", Archive: "ubuntu", Series: "resolute", Stage: "not observed", Verification: "none",
	}}}
	var out bytes.Buffer
	if err := renderSRUListTableWidth(&out, newOutputStyler(true), snapshot, 120, sruListOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "line\n") || strings.Contains(got, "\x1b[31mred") || strings.Contains(got, "\x1b]8;;https://example.com") {
		t.Fatalf("untrusted terminal sequence reached output: %q", got)
	}
}

func TestSRUListFitsCommonTerminalWidths(t *testing.T) {
	snapshot := &dto.SRUSnapshot{SyncedAt: time.Now(), Rows: []dto.SRURow{{
		BugID: "2167438", Title: strings.Repeat("長い title ", 12), Package: "python-openstack-long-package",
		Archive: "ubuntu", Series: "resolute", TaskStatus: "Fix Released",
		Stage: "not observed", Verification: "needed",
	}}}
	for _, width := range []int{60, 80, 100, 120} {
		var out bytes.Buffer
		if err := renderSRUListTableWidth(&out, newOutputStyler(false), snapshot, width, sruListOptions{}); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
			if lipgloss.Width(line) > width {
				t.Errorf("width %d produced %d-column line: %q", width, lipgloss.Width(line), line)
			}
		}
		out.Reset()
		if err := renderSRUTargetTableWidth(&out, newOutputStyler(false), snapshot, width, sruListOptions{}); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
			if lipgloss.Width(line) > width {
				t.Errorf("target width %d produced %d-column line: %q", width, lipgloss.Width(line), line)
			}
		}
	}
}

func TestSRUListGroupsAfterFilteringAndDiagnosticsAreOptional(t *testing.T) {
	rows := []dto.SRURow{
		{BugID: "2167438", BugURL: "https://bugs.launchpad.net/bugs/2167438", Package: "neutron", Title: "Shared bug",
			Archive: "uca", Series: "caracal", Stage: "proposed", Verification: "needed", Warning: "parent not ready"},
		{BugID: "2167438", BugURL: "https://bugs.launchpad.net/bugs/2167438", Package: "neutron", Title: "Shared bug",
			Archive: "uca", Series: "flamingo", Stage: "proposed", Verification: "needed"},
		{BugID: "2167438", BugURL: "https://bugs.launchpad.net/bugs/2167438", Package: "nova", Title: "Shared bug",
			Archive: "ubuntu", Series: "noble", Stage: "not observed", Verification: "none"},
	}
	snapshot := &dto.SRUSnapshot{SyncedAt: time.Now(), Rows: rows}
	var out bytes.Buffer
	if err := renderSRUListTableWidth(&out, newOutputStyler(false), snapshot, 120, sruListOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "2 SRUs / 3 targets") || strings.Count(got, "#2167438") != 2 ||
		!strings.Contains(got, "2×proposed/needed") || strings.Contains(got, "parent not ready") {
		t.Fatalf("grouped output is inaccurate: %q", got)
	}
	out.Reset()
	if err := renderSRUListTableWidth(&out, newOutputStyler(false), snapshot, 120, sruListOptions{diagnostics: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Warnings") || !strings.Contains(out.String(), "parent not ready") {
		t.Fatalf("diagnostics missing from opt-in output: %q", out.String())
	}
	out.Reset()
	if err := renderSRUTargetTableWidth(&out, newOutputStyler(false), snapshot, 120, sruListOptions{}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "#2167438") != 3 || strings.Contains(out.String(), "Warnings") {
		t.Fatalf("target output should contain three rows without warnings: %q", out.String())
	}
}

func TestSRUListCommandDefaultsToAttentionAndAllIncludesCompleted(t *testing.T) {
	snapshot := dto.SRUSnapshot{SyncedAt: time.Now(), Rows: []dto.SRURow{
		{BugID: "2167438", Package: "neutron", Title: "Needs verification", Stage: "proposed", Verification: "needed"},
		{BugID: "1881771", Package: "neutron", Title: "Already released", Stage: "not observed", TaskStatus: "Fix Released"},
	}}
	queries := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sru" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		queries <- r.URL.Query().Get("needs_attention")
		result := snapshot
		if r.URL.Query().Get("needs_attention") == "true" {
			result.Rows = result.Rows[:1]
		}
		if err := json.NewEncoder(w).Encode(result); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()
	for _, test := range []struct {
		args  []string
		query string
		want  string
	}{
		{args: []string{"list"}, query: "true", want: "1 SRU / 1 target"},
		{args: []string{"list", "--all"}, query: "", want: "2 SRUs / 2 targets"},
	} {
		var out bytes.Buffer
		options := &Options{Out: &out, Output: "table", Client: client.NewClient(server.URL)}
		command := newSRUCmd(options)
		command.SetArgs(test.args)
		if err := command.Execute(); err != nil {
			t.Fatalf("%v: %v", test.args, err)
		}
		if !strings.Contains(out.String(), test.want) {
			t.Errorf("%v: missing %q in %q", test.args, test.want, out.String())
		}
		if got := <-queries; got != test.query {
			t.Errorf("%v: needs_attention = %q, want %q", test.args, got, test.query)
		}
		listCommand, _, err := command.Find([]string{"list"})
		if err != nil {
			t.Fatal(err)
		}
		if listCommand.Flags().Lookup("attention") != nil {
			t.Fatal("obsolete --attention flag remains")
		}
	}
}

func TestSRUBugIDAcceptsLaunchpadURLs(t *testing.T) {
	for _, raw := range []string{
		"2167438", "https://bugs.launchpad.net/cloud-archive/+bug/2167438",
		"https://bugs.launchpad.net/ubuntu/+source/neutron/+bug/2167438",
	} {
		id, err := sruBugID(raw)
		if err != nil || id != "2167438" {
			t.Errorf("sruBugID(%q) = %q, %v", raw, id, err)
		}
	}
	for _, raw := range []string{"0", "https://example.com/+bug/2167438", "https://bugs.launchpad.net/other/2167438",
		"https://bugs.launchpad.net:8443/+bug/2167438", "https://user@bugs.launchpad.net/+bug/2167438"} {
		if _, err := sruBugID(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}
