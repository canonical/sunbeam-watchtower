// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

func TestMapPackageName(t *testing.T) {
	p := NewProvider("", "")

	tests := []struct {
		name        string
		deliverable string
		dtype       dto.DeliverableType
		want        string
	}{
		{"service stays unchanged", "nova", dto.DeliverableService, "nova"},
		{"client stays unchanged", "python-novaclient", dto.DeliverableClient, "python-novaclient"},
		{"library gets python- prefix", "stevedore", dto.DeliverableLibrary, "python-stevedore"},
		{"oslo.db dots to hyphens", "oslo.db", dto.DeliverableLibrary, "python-oslo-db"},
		{"oslo.messaging dots to hyphens", "oslo.messaging", dto.DeliverableLibrary, "python-oslo-messaging"},
		{"explicit mapping override", "keystoneauth1", dto.DeliverableLibrary, "python-keystoneauth1"},
		{"explicit mapping for osc-lib", "osc-lib", dto.DeliverableLibrary, "python-osc-lib"},
		{"explicit mapping for os-brick", "os-brick", dto.DeliverableLibrary, "python-os-brick"},
		{"service with dots", "oslo.service", dto.DeliverableService, "python-oslo-service"},
		{"other type", "unknown-thing", dto.DeliverableOther, "unknown-thing"},
		{"openstacksdk mapping", "openstacksdk", dto.DeliverableLibrary, "python-openstacksdk"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.MapPackageName(tt.deliverable, tt.dtype)
			if got != tt.want {
				t.Errorf("MapPackageName(%q, %v) = %q, want %q", tt.deliverable, tt.dtype, got, tt.want)
			}
		})
	}
}

func TestParseConstraints(t *testing.T) {
	input := []byte(`# This is a comment
alembic===1.13.1
amqp===5.2.0;python_version>='3.0'
aodhclient===3.6.0

# Another comment
numpy===1.26.4
`)

	got, err := parseConstraints(input)
	if err != nil {
		t.Fatalf("parseConstraints() error: %v", err)
	}

	expected := map[string]string{
		"alembic":    "1.13.1",
		"amqp":       "5.2.0",
		"aodhclient": "3.6.0",
		"numpy":      "1.26.4",
	}

	if len(got) != len(expected) {
		t.Fatalf("parseConstraints() returned %d entries, want %d", len(got), len(expected))
	}

	for k, v := range expected {
		if got[k] != v {
			t.Errorf("parseConstraints()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseConstraintsEmpty(t *testing.T) {
	got, err := parseConstraints([]byte(""))
	if err != nil {
		t.Fatalf("parseConstraints() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("parseConstraints() returned %d entries for empty input, want 0", len(got))
	}
}

func TestParseSeriesStatus(t *testing.T) {
	input := []byte(`---
- name: hibiscus
  release-id: 2026.2
  status: development
- name: gazpacho
  release-id: 2026.1
  status: maintained
- name: epoxy
  release-id: 2025.1
  status: maintained
`)

	got, err := parseSeriesStatus(input)
	if err != nil {
		t.Fatalf("parseSeriesStatus() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("parseSeriesStatus() returned %d entries, want 3", len(got))
	}
	if got[1].Name != "gazpacho" || got[1].ReleaseID != "2026.1" || got[1].Status != "maintained" {
		t.Fatalf("second series = %+v, want gazpacho/2026.1/maintained", got[1])
	}
}

func TestListDeliverablesDefaultUsesLatestAvailablePerPackage(t *testing.T) {
	repo := initTestGitRepo(t)
	writeTestFile(t, repo, "data/series_status.yaml", `---
- name: hibiscus
  release-id: 2026.2
  status: development
- name: gazpacho
  release-id: 2026.1
  status: maintained
- name: flamingo
  release-id: 2025.2
  status: maintained
- name: stein
  release-id: 2019.1
  status: end of life
`)
	writeTestFile(t, repo, "deliverables/gazpacho/nova.yaml", `---
team: nova
type: service
releases:
  - version: 32.0.0
`)
	writeTestFile(t, repo, "deliverables/flamingo/nova.yaml", `---
team: nova
type: service
releases:
  - version: 31.0.0
`)
	writeTestFile(t, repo, "deliverables/flamingo/vitrage.yaml", `---
team: vitrage
type: service
releases:
  - version: 15.0.1
`)
	writeTestFile(t, repo, "deliverables/stein/os-traits.yaml", `---
team: nova
type: library
releases:
  - version: 0.11.0
  - version: stein-em
`)
	writeTestFile(t, repo, "deliverables/_independent/os-traits.yaml", `---
team: nova
type: library
releases:
  - version: 3.6.0
  - version: 3.9.0
`)
	commitTestRepo(t, repo)

	provider := NewProvider(repo, "")
	got, err := provider.ListDeliverables(context.Background(), "")
	if err != nil {
		t.Fatalf("ListDeliverables() error: %v", err)
	}

	versions := map[string]string{}
	for _, d := range got {
		versions[d.Name] = d.Version
	}
	if versions["nova"] != "32.0.0" {
		t.Fatalf("nova version = %q, want latest gazpacho version 32.0.0", versions["nova"])
	}
	if versions["vitrage"] != "15.0.1" {
		t.Fatalf("vitrage version = %q, want latest available flamingo version 15.0.1", versions["vitrage"])
	}
	if versions["os-traits"] != "3.9.0" {
		t.Fatalf("os-traits version = %q, want latest independent version 3.9.0", versions["os-traits"])
	}
	if len(versions) != 3 {
		t.Fatalf("versions = %+v, want nova, vitrage and os-traits", versions)
	}
}

func TestGetConstraintsUsesSeriesReleaseID(t *testing.T) {
	releases := initTestGitRepo(t)
	writeTestFile(t, releases, "data/series_status.yaml", `---
- name: gazpacho
  release-id: 2026.1
  status: maintained
- name: flamingo
  release-id: 2025.2
  status: maintained
`)
	commitTestRepo(t, releases)

	requirements := initTestGitRepo(t)
	writeTestFile(t, requirements, "upper-constraints.txt", "os-traits===3.9.0\n")
	commitTestRepo(t, requirements)
	runTestGit(t, requirements, "branch", "test-base")
	runTestGit(t, requirements, "checkout", "-b", "stable/2026.1")
	writeTestFile(t, requirements, "upper-constraints.txt", "os-traits===3.6.0\n")
	commitTestRepo(t, requirements)
	runTestGit(t, requirements, "checkout", "test-base")
	runTestGit(t, requirements, "checkout", "-b", "stable/2025.2")
	writeTestFile(t, requirements, "upper-constraints.txt", "os-traits===3.5.0\n")
	commitTestRepo(t, requirements)
	runTestGit(t, requirements, "checkout", "test-base")

	provider := NewProvider(releases, requirements)
	for _, tt := range []struct{ release, version string }{
		{"gazpacho", "3.6.0"}, {"2026.1", "3.6.0"},
		{"flamingo", "3.5.0"}, {"2025.2", "3.5.0"},
	} {
		got, err := provider.GetConstraints(context.Background(), tt.release)
		if err != nil {
			t.Fatalf("GetConstraints(%q): %v", tt.release, err)
		}
		if got["os-traits"] != tt.version {
			t.Fatalf("GetConstraints(%q) = %q, want %s", tt.release, got["os-traits"], tt.version)
		}
	}
	got, err := provider.GetConstraints(context.Background(), "")
	if err != nil || got["os-traits"] != "3.9.0" {
		t.Fatalf("GetConstraints(default) = %v, %v, want 3.9.0", got, err)
	}
}

func TestParseDeliverable(t *testing.T) {
	yamlContent := []byte(`---
launchpad: nova
team: Nova
type: service
releases:
  - version: '28.0.0'
    projects:
      - repo: openstack/nova
        hash: abc123
  - version: '28.1.0'
    projects:
      - repo: openstack/nova
        hash: def456
`)

	d, err := parseDeliverable("nova.yaml", yamlContent)
	if err != nil {
		t.Fatalf("parseDeliverable() error: %v", err)
	}

	if d.Name != "nova" {
		t.Errorf("Name = %q, want %q", d.Name, "nova")
	}
	if d.Type != dto.DeliverableService {
		t.Errorf("Type = %v, want DeliverableService", d.Type)
	}
	if d.Version != "28.1.0" {
		t.Errorf("Version = %q, want %q", d.Version, "28.1.0")
	}
	if d.Team != "Nova" {
		t.Errorf("Team = %q, want %q", d.Team, "Nova")
	}
}

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runTestGit(t, dir, "init")
	runTestGit(t, dir, "config", "user.email", "watchtower@example.invalid")
	runTestGit(t, dir, "config", "user.name", "Watchtower Test")
	runTestGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func writeTestFile(t *testing.T, repo, name, content string) {
	t.Helper()
	path := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating parent directory for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func commitTestRepo(t *testing.T, repo string) {
	t.Helper()
	runTestGit(t, repo, "add", ".")
	runTestGit(t, repo, "commit", "-m", "test data")
}

func runTestGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestParseDeliverableLibrary(t *testing.T) {
	yamlContent := []byte(`---
team: Oslo
type: library
releases:
  - version: '5.0.0'
  - version: '5.1.0'
`)

	d, err := parseDeliverable("oslo.db.yaml", yamlContent)
	if err != nil {
		t.Fatalf("parseDeliverable() error: %v", err)
	}

	if d.Name != "oslo.db" {
		t.Errorf("Name = %q, want %q", d.Name, "oslo.db")
	}
	if d.Type != dto.DeliverableLibrary {
		t.Errorf("Type = %v, want DeliverableLibrary", d.Type)
	}
	if d.Version != "5.1.0" {
		t.Errorf("Version = %q, want %q", d.Version, "5.1.0")
	}
}

func TestParseDeliverableClientLibrary(t *testing.T) {
	yamlContent := []byte(`---
team: Nova
type: client-library
releases:
  - version: '18.0.0'
  - version: '18.1.0'
`)

	d, err := parseDeliverable("python-novaclient.yaml", yamlContent)
	if err != nil {
		t.Fatalf("parseDeliverable() error: %v", err)
	}

	if d.Type != dto.DeliverableClient {
		t.Errorf("Type = %v, want DeliverableClient", d.Type)
	}
}

func TestParseDeliverableSkipsLifecycleVersions(t *testing.T) {
	yamlContent := []byte(`---
team: SomeTeam
type: service
releases:
  - version: '1.0.0'
  - version: '2.0.0'
  - version: '2.0.0-eol'
`)

	d, err := parseDeliverable("old-project.yaml", yamlContent)
	if err != nil {
		t.Fatalf("parseDeliverable() error: %v", err)
	}

	if d.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q (should skip -eol)", d.Version, "2.0.0")
	}
}

func TestParseDeliverableSkipsEomVersions(t *testing.T) {
	yamlContent := []byte(`---
team: SomeTeam
type: service
releases:
  - version: '1.0.0'
  - version: '1.0.0-eom'
`)

	d, err := parseDeliverable("eom-project.yaml", yamlContent)
	if err != nil {
		t.Fatalf("parseDeliverable() error: %v", err)
	}

	if d.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q (should skip -eom)", d.Version, "1.0.0")
	}
}

func TestParseDeliverableAllLifecycleVersions(t *testing.T) {
	yamlContent := []byte(`---
team: SomeTeam
type: service
releases:
  - version: '1.0.0-eol'
  - version: '2.0.0-eom'
`)

	d, err := parseDeliverable("dead-project.yaml", yamlContent)
	if err != nil {
		t.Fatalf("parseDeliverable() error: %v", err)
	}

	if d.Version != "" {
		t.Errorf("Version = %q, want empty (all versions are lifecycle)", d.Version)
	}
}

func TestMapDeliverableType(t *testing.T) {
	tests := []struct {
		input string
		want  dto.DeliverableType
	}{
		{"service", dto.DeliverableService},
		{"library", dto.DeliverableLibrary},
		{"client-library", dto.DeliverableClient},
		{"other", dto.DeliverableOther},
		{"Service", dto.DeliverableService},
		{"unknown", dto.DeliverableOther},
		{"", dto.DeliverableOther},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapDeliverableType(tt.input)
			if got != tt.want {
				t.Errorf("mapDeliverableType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsLifecycleVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"1.0.0", false},
		{"1.0.0-eol", true},
		{"1.0.0-eom", true},
		{"stein-em", true},
		{"1.0.0-rc1", false},
		{"eol", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := isLifecycleVersion(tt.version)
			if got != tt.want {
				t.Errorf("isLifecycleVersion(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
