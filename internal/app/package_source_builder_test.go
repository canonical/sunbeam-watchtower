// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// standardTestConfig returns a config with ubuntu distro, noble + resolute releases,
// and a "gazpacho" backport in noble targeting resolute as parent.
func standardTestConfig() *config.Config {
	return &config.Config{
		Packages: config.PackagesConfig{
			Distros: map[string]config.DistroConfig{
				"ubuntu": {
					Mirror:     "http://archive.ubuntu.com/ubuntu",
					Components: []string{"main"},
					Releases: map[string]config.ReleaseConfig{
						"noble": {
							Suites: []string{"release", "updates"},
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
							Suites: []string{"release", "updates"},
						},
					},
				},
			},
		},
	}
}

// findSource returns the PackageSource with the given name, or nil.
func findSource(sources []dto.PackageSource, name string) *dto.PackageSource {
	for i := range sources {
		if sources[i].Name == name {
			return &sources[i]
		}
	}
	return nil
}

// hasEntry returns true if entries contains one matching the given mirror/suite/component triple.
func hasEntry(entries []dto.SourceEntry, mirror, suite, component string) bool {
	for _, e := range entries {
		if e.Mirror == mirror && e.Suite == suite && e.Component == component {
			return true
		}
	}
	return false
}

// TestBuildPackageSources_NilBackportsIncludesAll locks down that passing nil for backports
// causes all configured backports to be included (cache sync path).
func TestBuildPackageSources_NilBackportsIncludesAll(t *testing.T) {
	a := NewApp(standardTestConfig(), nil)
	sources := a.BuildPackageSources(nil, nil, nil, nil)

	// The ubuntu distro source must be present.
	ubuntuSrc := findSource(sources, "ubuntu")
	if ubuntuSrc == nil {
		t.Fatal("expected 'ubuntu' source in results")
	}

	// The gazpacho backport source must be present.
	bpSrc := findSource(sources, "ubuntu/gazpacho")
	if bpSrc == nil {
		t.Fatal("expected 'ubuntu/gazpacho' backport source in results")
	}

	// The backport source entry should reference the ppa mirror with the expanded suite.
	wantSuite := "noble-updates/gazpacho"
	if !hasEntry(bpSrc.Entries, "http://ppa.example.com", wantSuite, "main") {
		t.Errorf("backport entries = %+v, want entry {Mirror: %q, Suite: %q, Component: %q}",
			bpSrc.Entries, "http://ppa.example.com", wantSuite, "main")
	}
}

// TestBuildPackageSources_NoneBackportsSkipsAll locks down that ["none"] skips all backports.
func TestBuildPackageSources_NoneBackportsSkipsAll(t *testing.T) {
	a := NewApp(standardTestConfig(), nil)
	sources := a.BuildPackageSources(nil, nil, nil, []string{"none"})

	if src := findSource(sources, "ubuntu/gazpacho"); src != nil {
		t.Errorf("expected no backport sources, got %+v", src)
	}

	// Main distro source should still be present.
	if src := findSource(sources, "ubuntu"); src == nil {
		t.Fatal("expected 'ubuntu' source to still be present when backports are excluded")
	}
}

// TestBuildPackageSources_NamedBackportInfersParentRelease locks down the 3-state logic for
// named backports without an explicit --release filter:
//   - The backport target release (noble) is included with backport pockets only.
//   - The parent release (resolute) is included with full main suites.
//   - A release that is both a backport target AND a parent gets full suites.
func TestBuildPackageSources_NamedBackportInfersParentRelease(t *testing.T) {
	a := NewApp(standardTestConfig(), nil)
	// Request only the "gazpacho" backport; no explicit release filter.
	sources := a.BuildPackageSources(nil, nil, nil, []string{"gazpacho"})

	// The backport pocket source must be present.
	bpSrc := findSource(sources, "ubuntu/gazpacho")
	if bpSrc == nil {
		t.Fatal("expected 'ubuntu/gazpacho' backport source in results")
	}

	// The ubuntu main source must be present (parent release resolute + backport target noble).
	ubuntuSrc := findSource(sources, "ubuntu")
	if ubuntuSrc == nil {
		t.Fatal("expected 'ubuntu' source in results (inferred releases)")
	}

	// resolute is the parent release: it must include its full suites.
	if !hasEntry(ubuntuSrc.Entries, "http://archive.ubuntu.com/ubuntu", "resolute", "main") {
		t.Error("expected 'resolute' (release suite) in ubuntu entries for parent release")
	}
	if !hasEntry(ubuntuSrc.Entries, "http://archive.ubuntu.com/ubuntu", "resolute-updates", "main") {
		t.Error("expected 'resolute-updates' suite in ubuntu entries for parent release")
	}

	// noble is the backport target only: its main suites must NOT be in the ubuntu source.
	// (They would only appear if noble were also a parent release.)
	if hasEntry(ubuntuSrc.Entries, "http://archive.ubuntu.com/ubuntu", "noble", "main") {
		t.Error("did not expect 'noble' (release suite) in ubuntu entries: noble is backport-only, not a parent")
	}
	if hasEntry(ubuntuSrc.Entries, "http://archive.ubuntu.com/ubuntu", "noble-updates", "main") {
		t.Error("did not expect 'noble-updates' suite in ubuntu entries: noble is backport-only")
	}
}

// TestBuildPackageSources_EmptyConfig locks down that an empty PackagesConfig produces no sources.
func TestBuildPackageSources_EmptyConfig(t *testing.T) {
	a := NewApp(&config.Config{}, nil)
	sources := a.BuildPackageSources(nil, nil, nil, nil)
	if len(sources) != 0 {
		t.Errorf("expected empty sources for empty config, got %d: %+v", len(sources), sources)
	}
}

// TestBuildPackageSources_DistroAndSuiteFilters locks down that distro and suite-type filters work:
//   - A distro filter for "ubuntu" excludes any other distros.
//   - A suite-type filter restricts which suites are expanded per release.
func TestBuildPackageSources_DistroAndSuiteFilters(t *testing.T) {
	cfg := &config.Config{
		Packages: config.PackagesConfig{
			Distros: map[string]config.DistroConfig{
				"ubuntu": {
					Mirror:     "http://archive.ubuntu.com/ubuntu",
					Components: []string{"main"},
					Releases: map[string]config.ReleaseConfig{
						"noble": {
							Suites: []string{"release", "updates"},
						},
					},
				},
				"debian": {
					Mirror:     "http://deb.debian.org/debian",
					Components: []string{"main"},
					Releases: map[string]config.ReleaseConfig{
						"trixie": {
							Suites: []string{"release", "updates"},
						},
					},
				},
			},
		},
	}
	a := NewApp(cfg, nil)

	t.Run("distro filter excludes other distros", func(t *testing.T) {
		sources := a.BuildPackageSources([]string{"ubuntu"}, nil, nil, []string{"none"})
		if src := findSource(sources, "debian"); src != nil {
			t.Errorf("expected debian to be excluded, but found it: %+v", src)
		}
		if src := findSource(sources, "ubuntu"); src == nil {
			t.Fatal("expected ubuntu to be present")
		}
	})

	t.Run("suite-type filter restricts suites", func(t *testing.T) {
		// Only request the "updates" suite type; "release" suite should be excluded.
		sources := a.BuildPackageSources([]string{"ubuntu"}, nil, []string{"updates"}, []string{"none"})
		ubuntuSrc := findSource(sources, "ubuntu")
		if ubuntuSrc == nil {
			t.Fatal("expected 'ubuntu' source")
		}
		// "noble" (the expanded "release" suite) should be absent.
		if hasEntry(ubuntuSrc.Entries, "http://archive.ubuntu.com/ubuntu", "noble", "main") {
			t.Error("did not expect 'noble' entry when suite filter is 'updates' only")
		}
		// "noble-updates" should be present.
		if !hasEntry(ubuntuSrc.Entries, "http://archive.ubuntu.com/ubuntu", "noble-updates", "main") {
			t.Error("expected 'noble-updates' entry when suite filter includes 'updates'")
		}
	})
}

func TestBuildPackageCacheSyncSourcesRejectsBackportAsRelease(t *testing.T) {
	a := NewApp(standardTestConfig(), nil)

	_, err := a.BuildPackageCacheSyncSources([]string{"ubuntu"}, []string{"gazpacho"}, nil, nil)
	if err == nil {
		t.Fatal("expected error for backport name passed as release")
	}
	if got := err.Error(); !strings.Contains(got, `unknown distro release "gazpacho"`) || !strings.Contains(got, "--backport") {
		t.Fatalf("error = %q, want release/backport guidance", got)
	}
}

func TestBuildPackageCacheSyncSourcesAcceptsNamedBackport(t *testing.T) {
	a := NewApp(standardTestConfig(), nil)

	sources, err := a.BuildPackageCacheSyncSources([]string{"ubuntu"}, nil, nil, []string{"gazpacho"})
	if err != nil {
		t.Fatalf("BuildPackageCacheSyncSources() error = %v", err)
	}
	if src := findSource(sources, "ubuntu/gazpacho"); src == nil {
		t.Fatal("expected 'ubuntu/gazpacho' backport source")
	}
	if src := findSource(sources, "ubuntu"); src == nil {
		t.Fatal("expected 'ubuntu' parent release source")
	}
}

func TestBuildPackageCacheSyncSourcesAcceptsDistroRelease(t *testing.T) {
	a := NewApp(standardTestConfig(), nil)

	sources, err := a.BuildPackageCacheSyncSources([]string{"ubuntu"}, []string{"noble"}, nil, []string{"none"})
	if err != nil {
		t.Fatalf("BuildPackageCacheSyncSources() error = %v", err)
	}
	ubuntuSrc := findSource(sources, "ubuntu")
	if ubuntuSrc == nil {
		t.Fatal("expected 'ubuntu' source")
	}
	if !hasEntry(ubuntuSrc.Entries, "http://archive.ubuntu.com/ubuntu", "noble", "main") {
		t.Fatal("expected noble release entry")
	}
	if src := findSource(sources, "ubuntu/gazpacho"); src != nil {
		t.Fatal("did not expect backport source when backports are explicitly disabled")
	}
}

func TestBuildPackageCacheSyncSourcesRejectsUnknownFilters(t *testing.T) {
	a := NewApp(standardTestConfig(), nil)

	tests := []struct {
		name      string
		distros   []string
		releases  []string
		backports []string
		want      string
	}{
		{
			name:    "unknown distro",
			distros: []string{"notubuntu"},
			want:    `unknown distro "notubuntu"`,
		},
		{
			name:     "unknown release",
			releases: []string{"notnoble"},
			want:     `unknown distro release "notnoble"`,
		},
		{
			name:      "unknown backport",
			backports: []string{"notgazpacho"},
			want:      `unknown backport "notgazpacho"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := a.BuildPackageCacheSyncSources(tt.distros, tt.releases, nil, tt.backports)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}
