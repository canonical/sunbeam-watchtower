// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/core/service/artifactdiscovery"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// helper to sort and compare string slices
func sortedEqual(t *testing.T, got, want []string) {
	t.Helper()
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// --- RockStrategy tests ---

func TestRockStrategy_ArtifactType(t *testing.T) {
	s := &RockStrategy{}
	if s.ArtifactType() != dto.ArtifactRock {
		t.Fatalf("expected ArtifactRock")
	}
}

func TestRockStrategy_MetadataFileName(t *testing.T) {
	s := &RockStrategy{}
	if s.MetadataFileName() != "rockcraft.yaml" {
		t.Fatalf("expected rockcraft.yaml")
	}
}

func TestRockStrategy_BuildPath(t *testing.T) {
	s := &RockStrategy{}
	if got := s.BuildPath("keystone"); got != "rocks/keystone" {
		t.Fatalf("got %q, want %q", got, "rocks/keystone")
	}
}

func TestRockStrategy_ParsePlatforms_Simple(t *testing.T) {
	yaml := []byte(`
platforms:
  amd64:
  arm64:
`)
	s := &RockStrategy{}
	got, err := s.ParsePlatforms(yaml)
	if err != nil {
		t.Fatal(err)
	}
	sortedEqual(t, got, []string{"amd64", "arm64"})
}

func TestRockStrategy_ParsePlatforms_Complex(t *testing.T) {
	yaml := []byte(`
platforms:
  multi-arch:
    build-on: [arm64]
    build-for: [amd64]
  native:
    build-on: [s390x]
    build-for: [s390x]
`)
	s := &RockStrategy{}
	got, err := s.ParsePlatforms(yaml)
	if err != nil {
		t.Fatal(err)
	}
	sortedEqual(t, got, []string{"amd64", "s390x"})
}

func TestRockStrategy_ParsePlatforms_Empty(t *testing.T) {
	yaml := []byte(`name: test`)
	s := &RockStrategy{}
	got, err := s.ParsePlatforms(yaml)
	if err != nil {
		t.Fatal(err)
	}
	sortedEqual(t, got, []string{"amd64"})
}

// --- CharmStrategy tests ---

func TestCharmStrategy_ArtifactType(t *testing.T) {
	s := &CharmStrategy{}
	if s.ArtifactType() != dto.ArtifactCharm {
		t.Fatalf("expected ArtifactCharm")
	}
}

func TestCharmStrategy_MetadataFileName(t *testing.T) {
	s := &CharmStrategy{}
	if s.MetadataFileName() != "charmcraft.yaml" {
		t.Fatalf("expected charmcraft.yaml")
	}
}

func TestCharmStrategy_BuildPath(t *testing.T) {
	s := &CharmStrategy{}
	if got := s.BuildPath("mysql-k8s"); got != "charms/mysql-k8s" {
		t.Fatalf("got %q, want %q", got, "charms/mysql-k8s")
	}
}

func TestCharmStrategy_ParsePlatforms_NewSyntax(t *testing.T) {
	yaml := []byte(`
platforms:
  amd64:
  arm64:
`)
	s := &CharmStrategy{}
	got, err := s.ParsePlatforms(yaml)
	if err != nil {
		t.Fatal(err)
	}
	sortedEqual(t, got, []string{"amd64", "arm64"})
}

func TestCharmStrategy_ParsePlatforms_OldBases(t *testing.T) {
	yaml := []byte(`
bases:
  - build-on:
      - name: ubuntu
        channel: "22.04"
        architectures: [amd64]
    run-on:
      - name: ubuntu
        channel: "22.04"
        architectures: [amd64, arm64]
`)
	s := &CharmStrategy{}
	got, err := s.ParsePlatforms(yaml)
	if err != nil {
		t.Fatal(err)
	}
	sortedEqual(t, got, []string{"amd64", "arm64"})
}

func TestCharmStrategy_ParsePlatforms_OldBasesBuildOnFallback(t *testing.T) {
	yaml := []byte(`
bases:
  - build-on:
      - name: ubuntu
        channel: "22.04"
        architectures: [amd64, s390x]
`)
	s := &CharmStrategy{}
	got, err := s.ParsePlatforms(yaml)
	if err != nil {
		t.Fatal(err)
	}
	sortedEqual(t, got, []string{"amd64", "s390x"})
}

func TestCharmStrategy_ParsePlatforms_Empty(t *testing.T) {
	yaml := []byte(`name: test`)
	s := &CharmStrategy{}
	got, err := s.ParsePlatforms(yaml)
	if err != nil {
		t.Fatal(err)
	}
	sortedEqual(t, got, []string{"amd64"})
}

func TestCharmStrategy_DiscoverRecipes_NestedLayout(t *testing.T) {
	repo := t.TempDir()
	layout := map[string]string{
		"charms/foo/charmcraft.yaml":         "name: foo\n",
		"charms/storage/bar/charmcraft.yaml": "name: bar\n",
	}
	for rel, body := range layout {
		full := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	s := &CharmStrategy{}
	got, err := s.DiscoverRecipes(repo)
	if err != nil {
		t.Fatalf("DiscoverRecipes: %v", err)
	}

	want := map[string]string{
		"foo": "charms/foo",
		"bar": "charms/storage/bar",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d recipes, want %d: %+v", len(got), len(want), got)
	}
	for _, r := range got {
		wantRel, ok := want[r.Name]
		if !ok {
			t.Errorf("unexpected recipe name %q", r.Name)
			continue
		}
		if r.RelPath != wantRel {
			t.Errorf("recipe %q: got RelPath %q, want %q", r.Name, r.RelPath, wantRel)
		}
	}
}

// TestCharmStrategy_DiscoverRecipes_ManifestNameWins verifies the YAML-declared
// name wins over the directory base name.
func TestCharmStrategy_DiscoverRecipes_ManifestNameWins(t *testing.T) {
	repo := t.TempDir()
	full := filepath.Join(repo, "charms", "dirname", "charmcraft.yaml")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("name: manifest-wins\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := (&CharmStrategy{}).DiscoverRecipes(repo)
	if err != nil {
		t.Fatalf("DiscoverRecipes: %v", err)
	}
	if len(got) != 1 || got[0].Name != "manifest-wins" {
		t.Fatalf("got %+v, want single recipe named manifest-wins", got)
	}
	if got[0].RelPath != "charms/dirname" {
		t.Fatalf("RelPath = %q, want charms/dirname", got[0].RelPath)
	}
}

// TestCharmStrategy_DiscoverRecipes_AgreesWithSharedParser verifies the
// build walker and the shared artifactdiscovery.ParseManifestName agree on
// the extracted name — the refactor's core invariant.
func TestCharmStrategy_DiscoverRecipes_AgreesWithSharedParser(t *testing.T) {
	manifest := []byte("name: shared-parser-name\nsummary: test\n")
	repo := t.TempDir()
	full := filepath.Join(repo, "charms", "somewhere", "charmcraft.yaml")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, manifest, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	walkResult, err := (&CharmStrategy{}).DiscoverRecipes(repo)
	if err != nil {
		t.Fatalf("DiscoverRecipes: %v", err)
	}
	if len(walkResult) != 1 {
		t.Fatalf("got %d recipes, want 1", len(walkResult))
	}
	parsed, err := artifactdiscovery.ParseManifestName(manifest, "charmcraft.yaml")
	if err != nil {
		t.Fatalf("ParseManifestName: %v", err)
	}
	if walkResult[0].Name != parsed {
		t.Fatalf("walker name %q != shared parser name %q", walkResult[0].Name, parsed)
	}
}

// TestCharmStrategy_DiscoverRecipes_FallbackToDirName verifies the directory
// base name is used when the manifest omits a name.
func TestCharmStrategy_DiscoverRecipes_FallbackToDirName(t *testing.T) {
	repo := t.TempDir()
	full := filepath.Join(repo, "charms", "fallbackname", "charmcraft.yaml")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("summary: no name declared\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := (&CharmStrategy{}).DiscoverRecipes(repo)
	if err != nil {
		t.Fatalf("DiscoverRecipes: %v", err)
	}
	if len(got) != 1 || got[0].Name != "fallbackname" {
		t.Fatalf("got %+v, want single recipe named fallbackname", got)
	}
}

// --- SnapStrategy tests ---

func TestSnapStrategy_ArtifactType(t *testing.T) {
	s := &SnapStrategy{}
	if s.ArtifactType() != dto.ArtifactSnap {
		t.Fatalf("expected ArtifactSnap")
	}
}

func TestSnapStrategy_MetadataFileName(t *testing.T) {
	s := &SnapStrategy{}
	if s.MetadataFileName() != "snapcraft.yaml" {
		t.Fatalf("expected snapcraft.yaml")
	}
}

func TestSnapStrategy_BuildPath(t *testing.T) {
	s := &SnapStrategy{}
	if got := s.BuildPath("mysnap"); got != "" {
		t.Fatalf("got %q, want %q", got, "")
	}
}

func TestSnapStrategy_ParsePlatforms_Architectures(t *testing.T) {
	yaml := []byte(`
architectures:
  - build-on: [amd64]
    build-for: [amd64]
  - build-on: [arm64]
    build-for: [arm64]
`)
	s := &SnapStrategy{}
	got, err := s.ParsePlatforms(yaml)
	if err != nil {
		t.Fatal(err)
	}
	sortedEqual(t, got, []string{"amd64", "arm64"})
}

func TestSnapStrategy_ParsePlatforms_Platforms(t *testing.T) {
	yaml := []byte(`
platforms:
  amd64:
  arm64:
`)
	s := &SnapStrategy{}
	got, err := s.ParsePlatforms(yaml)
	if err != nil {
		t.Fatal(err)
	}
	sortedEqual(t, got, []string{"amd64", "arm64"})
}

func TestSnapStrategy_ParsePlatforms_Default(t *testing.T) {
	yaml := []byte(`name: test`)
	s := &SnapStrategy{}
	got, err := s.ParsePlatforms(yaml)
	if err != nil {
		t.Fatal(err)
	}
	sortedEqual(t, got, []string{"amd64"})
}

// --- OfficialRecipeName tests ---

func TestRockStrategy_OfficialRecipeName(t *testing.T) {
	s := &RockStrategy{}
	// Dev focus returns artifact name only
	if got := s.OfficialRecipeName("nova-consolidated", "2025.1", "2025.1"); got != "nova-consolidated" {
		t.Errorf("OfficialRecipeName(dev focus) = %q, want %q", got, "nova-consolidated")
	}
	// Non-dev-focus appends series
	if got := s.OfficialRecipeName("nova-consolidated", "2024.1", "2025.1"); got != "nova-consolidated-2024.1" {
		t.Errorf("OfficialRecipeName(non-dev) = %q, want %q", got, "nova-consolidated-2024.1")
	}
}

func TestCharmStrategy_OfficialRecipeName(t *testing.T) {
	s := &CharmStrategy{}
	if got := s.OfficialRecipeName("mysql-k8s", "2025.1", "2025.1"); got != "mysql-k8s" {
		t.Errorf("OfficialRecipeName(dev focus) = %q, want %q", got, "mysql-k8s")
	}
	if got := s.OfficialRecipeName("mysql-k8s", "2024.1", "2025.1"); got != "mysql-k8s-2024.1" {
		t.Errorf("OfficialRecipeName(non-dev) = %q, want %q", got, "mysql-k8s-2024.1")
	}
}

func TestSnapStrategy_OfficialRecipeName(t *testing.T) {
	s := &SnapStrategy{}
	if got := s.OfficialRecipeName("mysnap", "2025.1", "2025.1"); got != "mysnap" {
		t.Errorf("OfficialRecipeName(dev focus) = %q, want %q", got, "mysnap")
	}
	if got := s.OfficialRecipeName("mysnap", "2024.1", "2025.1"); got != "mysnap-2024.1" {
		t.Errorf("OfficialRecipeName(non-dev) = %q, want %q", got, "mysnap-2024.1")
	}
}

// --- BranchForSeries tests ---

func TestRockStrategy_BranchForSeries(t *testing.T) {
	s := &RockStrategy{}
	// Dev focus returns default branch
	if got := s.BranchForSeries("2025.1", "2025.1", "main"); got != "main" {
		t.Errorf("BranchForSeries(dev focus) = %q, want %q", got, "main")
	}
	// Non-dev-focus returns stable/<series>
	if got := s.BranchForSeries("2024.1", "2025.1", "main"); got != "stable/2024.1" {
		t.Errorf("BranchForSeries(non-dev) = %q, want %q", got, "stable/2024.1")
	}
}

func TestCharmStrategy_BranchForSeries(t *testing.T) {
	s := &CharmStrategy{}
	if got := s.BranchForSeries("2025.1", "2025.1", "main"); got != "main" {
		t.Errorf("BranchForSeries(dev focus) = %q, want %q", got, "main")
	}
	if got := s.BranchForSeries("2024.1", "2025.1", "main"); got != "stable/2024.1" {
		t.Errorf("BranchForSeries(non-dev) = %q, want %q", got, "stable/2024.1")
	}
}

func TestSnapStrategy_BranchForSeries(t *testing.T) {
	s := &SnapStrategy{}
	if got := s.BranchForSeries("2025.1", "2025.1", "master"); got != "master" {
		t.Errorf("BranchForSeries(dev focus) = %q, want %q", got, "master")
	}
	if got := s.BranchForSeries("2024.1", "2025.1", "master"); got != "stable/2024.1" {
		t.Errorf("BranchForSeries(non-dev) = %q, want %q", got, "stable/2024.1")
	}
}

// --- TempRecipeName tests ---

func TestTempRecipeName(t *testing.T) {
	tests := []struct {
		strategy ArtifactStrategy
		name     string
		sha      string
		prefix   string
		want     string
	}{
		{&RockStrategy{}, "keystone", "abcdef1234567890", "temp", "temp-abcdef12-keystone"},
		{&CharmStrategy{}, "mysql-k8s", "12345678", "pr", "pr-12345678-mysql-k8s"},
		{&SnapStrategy{}, "mysnap", "short", "test", "test-short-mysnap"},
	}
	for _, tt := range tests {
		got := tt.strategy.TempRecipeName(tt.name, tt.sha, tt.prefix)
		if got != tt.want {
			t.Errorf("TempRecipeName(%q, %q, %q) = %q, want %q", tt.name, tt.sha, tt.prefix, got, tt.want)
		}
	}
}
