// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func TestForgeTypeFromConfig(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  forge.ForgeType
	}{
		{"github", "github", forge.ForgeGitHub},
		{"launchpad", "launchpad", forge.ForgeLaunchpad},
		{"gerrit", "gerrit", forge.ForgeGerrit},
		{"unknown defaults to github", "unknown", forge.ForgeGitHub},
		{"empty defaults to github", "", forge.ForgeGitHub},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ForgeTypeFromConfig(tc.input)
			if got != tc.want {
				t.Errorf("ForgeTypeFromConfig(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestMRRefSpecs(t *testing.T) {
	tests := []struct {
		name  string
		forge string
		want  []string
	}{
		{"github", "github", []string{"+refs/pull/*/head:refs/pull/*/head"}},
		{"gerrit", "gerrit", []string{"+refs/changes/*:refs/changes/*"}},
		{"launchpad", "launchpad", nil},
		{"unknown", "unknown", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MRRefSpecs(tc.forge)
			if len(got) != len(tc.want) {
				t.Fatalf("MRRefSpecs(%q) returned %d items, want %d", tc.forge, len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("MRRefSpecs(%q)[%d] = %q, want %q", tc.forge, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestMRGitRef(t *testing.T) {
	tests := []struct {
		name  string
		forge string
		mrID  string
		want  string
	}{
		{"github with hash prefix", "github", "#42", "refs/pull/42/head"},
		{"github without hash prefix", "github", "42", "refs/pull/42/head"},
		{"gerrit returns empty", "gerrit", "123", ""},
		{"unknown returns empty", "unknown", "1", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MRGitRef(tc.forge, tc.mrID)
			if got != tc.want {
				t.Errorf("MRGitRef(%q, %q) = %q, want %q", tc.forge, tc.mrID, got, tc.want)
			}
		})
	}
}

func TestConvertToMRMetadata(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		got := ConvertToMRMetadata(nil, "github")
		if len(got) != 0 {
			t.Fatalf("expected empty result, got %d items", len(got))
		}
	})

	t.Run("converts items", func(t *testing.T) {
		mrs := []forge.MergeRequest{
			{ID: "#10", State: forge.MergeStateOpen, URL: "https://github.com/o/r/pull/10"},
			{ID: "#20", State: forge.MergeStateMerged, URL: "https://github.com/o/r/pull/20"},
		}
		got := ConvertToMRMetadata(mrs, "github")
		if len(got) != 2 {
			t.Fatalf("expected 2 items, got %d", len(got))
		}
		for i, mr := range mrs {
			if got[i].ID != mr.ID {
				t.Errorf("[%d] ID = %q, want %q", i, got[i].ID, mr.ID)
			}
			if got[i].State != mr.State {
				t.Errorf("[%d] State = %v, want %v", i, got[i].State, mr.State)
			}
			if got[i].URL != mr.URL {
				t.Errorf("[%d] URL = %q, want %q", i, got[i].URL, mr.URL)
			}
			wantRef := MRGitRef("github", mr.ID)
			if got[i].GitRef != wantRef {
				t.Errorf("[%d] GitRef = %q, want %q", i, got[i].GitRef, wantRef)
			}
		}
	})
}

func TestUpstreamRepoPath(t *testing.T) {
	tests := []struct {
		name     string
		cacheDir string
		repoURL  string
		want     string
	}{
		{"with .git suffix", "/cache", "https://github.com/foo/bar.git", "/cache/bar"},
		{"without .git suffix", "/cache", "https://github.com/foo/bar", "/cache/bar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := UpstreamRepoPath(tc.cacheDir, tc.repoURL)
			if got != tc.want {
				t.Errorf("UpstreamRepoPath(%q, %q) = %q, want %q", tc.cacheDir, tc.repoURL, got, tc.want)
			}
		})
	}
}

func TestNewApp(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.Default()
	a := NewApp(cfg, logger)
	if a.GetConfig() != cfg {
		t.Error("NewApp did not set Config")
	}
	if a.Logger != logger {
		t.Error("NewApp did not set Logger")
	}
	if a.runtimeMode != RuntimeModePersistent {
		t.Errorf("NewApp runtimeMode = %q, want %q", a.runtimeMode, RuntimeModePersistent)
	}
}

func TestNewAppWithOptions(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.Default()
	a := NewAppWithOptions(cfg, logger, Options{RuntimeMode: RuntimeModeEphemeral})
	if a.runtimeMode != RuntimeModeEphemeral {
		t.Errorf("NewAppWithOptions runtimeMode = %q, want %q", a.runtimeMode, RuntimeModeEphemeral)
	}
}

func TestOperationService(t *testing.T) {
	a := NewApp(&config.Config{}, slog.Default())

	service, err := a.OperationService()
	if err != nil {
		t.Fatalf("OperationService() error = %v", err)
	}
	if service == nil {
		t.Fatal("OperationService() returned nil")
	}

	second, err := a.OperationService()
	if err != nil {
		t.Fatalf("OperationService() second call error = %v", err)
	}
	if second != service {
		t.Fatal("OperationService() did not return shared service instance")
	}
}

func TestExcusesSources(t *testing.T) {
	a := NewApp(&config.Config{}, slog.Default())
	sources := a.ExcusesSources()
	if len(sources) != 2 {
		t.Fatalf("len(ExcusesSources()) = %d, want 2", len(sources))
	}
	if sources[0].Tracker != "ubuntu" || sources[1].Tracker != "debian" {
		t.Fatalf("unexpected trackers: %+v", sources)
	}
}

func TestDefaultExcusesTracker(t *testing.T) {
	a := NewApp(&config.Config{}, slog.Default())
	if got := a.DefaultExcusesTracker(); got != "ubuntu" {
		t.Fatalf("DefaultExcusesTracker() = %q, want ubuntu", got)
	}
}

func TestResolveCacheDir(t *testing.T) {
	t.Run("uses XDG_CACHE_HOME when set", func(t *testing.T) {
		xdgDir := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", xdgDir)
		got, err := ResolveCacheDir()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(xdgDir, "sunbeam-watchtower")
		if got != want {
			t.Errorf("ResolveCacheDir() = %q, want %q", got, want)
		}
	})
}
