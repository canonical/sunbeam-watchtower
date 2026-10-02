package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func TestRenderReleaseList_ColorizesTableOutput(t *testing.T) {
	var out bytes.Buffer
	err := renderReleaseList(&out, "table", newOutputStyler(true), []dto.ReleaseListEntry{{
		Project:      "sunbeam",
		ArtifactType: dto.ArtifactSnap,
		Name:         "snap-openstack",
		Track:        "2024.1",
		Risk:         dto.ReleaseRiskStable,
		Targets: []dto.ReleaseTargetSnapshot{{
			Architecture: "amd64",
			Base:         dto.ReleaseBase{Name: "ubuntu", Channel: "24.04"},
			Revision:     41,
			Version:      "1.2.3",
		}},
		ReleasedAt: time.Date(2026, 3, 8, 11, 0, 0, 0, time.UTC),
	}})
	if err != nil {
		t.Fatalf("renderReleaseList() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected ANSI color codes in output, got %q", got)
	}
	if !strings.Contains(got, "PROJECT") || !strings.Contains(got, "amd64@ubuntu/24.04:r41/1.2.3") {
		t.Fatalf("missing expected table content: %q", got)
	}
}

func TestRenderBugSyncTableIncludesReason(t *testing.T) {
	var out bytes.Buffer
	result := &dto.BugSyncResult{Actions: []dto.BugSyncAction{{
		BugID: "12345", TaskTitle: "snap-openstack/2024.1", OldStatus: "Fix Committed",
		NewStatus: "Fix Released", ActionType: dto.BugSyncActionStatusUpdate,
		Reason: "included in 2024.1/stable revision 1101 (rev1101)",
	}}}
	if err := renderBugSyncResult(&out, "table", newOutputStyler(false), result, true); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "would update:") || !strings.Contains(got, "reason: included in 2024.1/stable revision 1101") {
		t.Fatalf("output = %q", got)
	}
}

func TestRenderReleaseList_JSONRemainsPlain(t *testing.T) {
	var out bytes.Buffer
	err := renderReleaseList(&out, "json", newOutputStyler(true), []dto.ReleaseListEntry{{
		Project:      "sunbeam",
		ArtifactType: dto.ArtifactSnap,
		Name:         "snap-openstack",
		Track:        "2024.1",
		Risk:         dto.ReleaseRiskStable,
	}})
	if err != nil {
		t.Fatalf("renderReleaseList() error = %v", err)
	}

	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("json output should not contain ANSI color codes: %q", out.String())
	}
}

func TestRenderPackageInfoTable_ColorizesKeys(t *testing.T) {
	info := &distro.SourcePackageInfo{
		SourcePackage: distro.SourcePackage{
			Package:   "keystone",
			Version:   "1:27.0.0-0ubuntu1",
			Suite:     "noble",
			Component: "main",
		},
		Fields: []distro.FieldEntry{
			{Key: "Homepage", Value: "https://example.invalid"},
			{Key: "Description", Value: "line 1\nline 2"},
		},
	}

	var out bytes.Buffer
	if err := renderPackageInfoTable(&out, newOutputStyler(true), info); err != nil {
		t.Fatalf("renderPackageInfoTable() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected ANSI color codes in output, got %q", got)
	}
	if !strings.Contains(got, "Source:") || !strings.Contains(got, "Homepage:") {
		t.Fatalf("missing expected key labels: %q", got)
	}
}

func TestWriteWarningLine_PlainWhenDisabled(t *testing.T) {
	var out bytes.Buffer
	if err := writeWarningLine(&out, newOutputStyler(false), "partial tracker failure"); err != nil {
		t.Fatalf("writeWarningLine() error = %v", err)
	}

	got := out.String()
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("plain warning output should not contain ANSI color codes: %q", got)
	}
	if !strings.Contains(got, "warning: partial tracker failure") {
		t.Fatalf("unexpected warning output: %q", got)
	}
}

func TestRenderAutopkgtestSection_GroupedOutput(t *testing.T) {
	var out bytes.Buffer
	styler := newOutputStyler(false)
	tests := []dto.ExcuseAutopkgtest{
		{Package: "nova/1:29.0.0", Architecture: "amd64", Status: "pass", URL: "https://example.com/log1"},
		{Package: "nova/1:29.0.0", Architecture: "arm64", Status: "in-progress", URL: "https://example.com/log2"},
		{Package: "keystone/27.0.0", Architecture: "amd64", Status: "regression", URL: ""},
	}

	if err := renderAutopkgtestSection(&out, styler, tests); err != nil {
		t.Fatalf("renderAutopkgtestSection() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "nova/1:29.0.0:") {
		t.Fatalf("expected grouped package label: %q", got)
	}
	if !strings.Contains(got, "amd64=pass") && !strings.Contains(got, "\x1b]8;;") {
		t.Fatalf("expected arch=status pairs (possibly hyperlinked): %q", got)
	}
	if !strings.Contains(got, "keystone/27.0.0:") {
		t.Fatalf("expected second package group: %q", got)
	}
}

func TestRenderAutopkgtestSection_FallbackEntries(t *testing.T) {
	var out bytes.Buffer
	styler := newOutputStyler(false)
	tests := []dto.ExcuseAutopkgtest{
		{Package: "", Architecture: "", Status: "unknown", Message: "arch:armhf not built yet, autopkgtest delayed there"},
	}

	if err := renderAutopkgtestSection(&out, styler, tests); err != nil {
		t.Fatalf("renderAutopkgtestSection() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "arch:armhf not built yet") {
		t.Fatalf("expected fallback message: %q", got)
	}
}

func TestHyperlink_EmitsOSC8WhenEnabled(t *testing.T) {
	styler := newOutputStyler(true)
	got := styler.Hyperlink("click", "https://example.com")
	if !strings.Contains(got, "\x1b]8;;https://example.com\a") {
		t.Fatalf("expected OSC 8 escape sequence: %q", got)
	}
	if !strings.Contains(got, "click") {
		t.Fatalf("expected text content: %q", got)
	}
}

func TestHyperlink_PlainWhenDisabled(t *testing.T) {
	styler := newOutputStyler(false)
	got := styler.Hyperlink("click", "https://example.com")
	if got != "click" {
		t.Fatalf("expected plain text, got %q", got)
	}
}

func TestSemanticAutopkgtestStatuses(t *testing.T) {
	styler := newOutputStyler(true)
	tests := []struct {
		status string
		expect string // Should contain ANSI codes
	}{
		{"pass", "\x1b["},
		{"in-progress", "\x1b["},
		{"regression", "\x1b["},
		{"no-results", "\x1b["},
		{"autopkgtest", "\x1b["},
		{"ftbfs", "\x1b["},
		{"dependency", "\x1b["},
	}
	for _, tt := range tests {
		got := styler.semantic(tt.status)
		if !strings.Contains(got, tt.expect) {
			t.Errorf("semantic(%q) should contain ANSI codes, got %q", tt.status, got)
		}
	}
}

func TestRenderBugTasks_TableStripsLaunchpadPrefixAndQuotes(t *testing.T) {
	var out bytes.Buffer
	err := renderBugTasks(&out, "table", newOutputStyler(false), []forge.BugTask{{
		Project:    "snap-openstack",
		BugID:      "2134598",
		Status:     "Triaged",
		Importance: "Medium",
		Title:      `Bug #2134598 in OpenStack Snap: "Fix bootstrap race in update flow"`,
		URL:        "https://bugs.launchpad.net/bugs/2134598",
	}})
	if err != nil {
		t.Fatalf("renderBugTasks() error = %v", err)
	}

	got := out.String()
	if strings.Contains(got, `Bug #2134598 in OpenStack Snap:`) {
		t.Fatalf("expected Launchpad bug prefix to be stripped: %q", got)
	}
	if strings.Contains(got, `"Fix bootstrap race in update flow"`) {
		t.Fatalf("expected surrounding quotes to be stripped: %q", got)
	}
	if !strings.Contains(got, "Fix bootstrap race in update flow") {
		t.Fatalf("expected cleaned bug title in output: %q", got)
	}
}
