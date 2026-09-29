package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FullConfig(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	artifactsDir := filepath.Join(dir, "artifacts")

	yaml := `
launchpad:
  default_owner: my-team
  use_keyring: true
github:
  use_keyring: false
gerrit:
  hosts:
    - url: https://review.opendev.org
bug_groups:
  sunbeam:
    common_project: snap-openstack
projects:
  - name: snap-openstack
    code:
      forge: github
      owner: canonical
      project: snap-openstack
    bugs:
      - forge: launchpad
        project: snap-openstack
        group: sunbeam
  - name: sunbeam-charms
    code:
      forge: gerrit
      host: https://review.opendev.org
      project: openstack/sunbeam-charms
    bugs:
      - forge: launchpad
        project: snap-openstack
        group: sunbeam
      - forge: launchpad
        project: sunbeam-charms
        group: sunbeam
  - name: charm-keystone
    code:
      forge: launchpad
      project: charm-keystone-k8s
build:
  default_prefix: sunbeam
  timeout_minutes: 45
  artifacts_dir: ` + artifactsDir + `
packages:
  distros:
    ubuntu:
      mirror: http://archive.ubuntu.com/ubuntu
      components: [main, universe]
      excuses:
        provider: ubuntu
        url: https://ubuntu-archive-team.ubuntu.com/proposed-migration/update_excuses.yaml.xz
        team_url: https://ubuntu-archive-team.ubuntu.com/proposed-migration/update_excuses_by_team.yaml
      releases:
        noble:
          suites: [release, updates, proposed]
tui:
  default_pane: packages
  panes:
    packages:
      mode: excuses
      filters:
        tracker: ubuntu
        team: ubuntu-openstack
    bugs:
      filters:
        merge: false
`
	if err := os.WriteFile(cfgFile, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Launchpad.DefaultOwner != "my-team" {
		t.Errorf("Launchpad.DefaultOwner = %q, want %q", cfg.Launchpad.DefaultOwner, "my-team")
	}
	if !cfg.Launchpad.UseKeyring {
		t.Error("Launchpad.UseKeyring = false, want true")
	}
	if cfg.GitHub.UseKeyring {
		t.Error("GitHub.UseKeyring = true, want false")
	}
	if len(cfg.Gerrit.Hosts) != 1 || cfg.Gerrit.Hosts[0].URL != "https://review.opendev.org" {
		t.Errorf("Gerrit.Hosts = %+v, want one host with URL https://review.opendev.org", cfg.Gerrit.Hosts)
	}
	if len(cfg.Projects) != 3 {
		t.Fatalf("len(Projects) = %d, want 3", len(cfg.Projects))
	}

	// snap-openstack: github code + 1 LP bug tracker
	p0 := cfg.Projects[0]
	if p0.Name != "snap-openstack" {
		t.Errorf("Projects[0].Name = %q, want %q", p0.Name, "snap-openstack")
	}
	if p0.Code.Forge != "github" || p0.Code.Owner != "canonical" || p0.Code.Project != "snap-openstack" {
		t.Errorf("Projects[0].Code = %+v", p0.Code)
	}
	if len(p0.Bugs) != 1 || p0.Bugs[0].Forge != "launchpad" || p0.Bugs[0].Project != "snap-openstack" {
		t.Errorf("Projects[0].Bugs = %+v", p0.Bugs)
	}
	if cfg.BugGroups["sunbeam"].CommonProject != "snap-openstack" {
		t.Fatalf("BugGroups[sunbeam] = %+v", cfg.BugGroups["sunbeam"])
	}

	// sunbeam-charms: gerrit code + 2 LP bug trackers
	p1 := cfg.Projects[1]
	if p1.Code.Forge != "gerrit" || p1.Code.Host != "https://review.opendev.org" {
		t.Errorf("Projects[1].Code = %+v", p1.Code)
	}
	if len(p1.Bugs) != 2 {
		t.Fatalf("len(Projects[1].Bugs) = %d, want 2", len(p1.Bugs))
	}

	// charm-keystone: launchpad code, no bug trackers
	p2 := cfg.Projects[2]
	if p2.Code.Forge != "launchpad" || p2.Code.Project != "charm-keystone-k8s" {
		t.Errorf("Projects[2].Code = %+v", p2.Code)
	}
	if len(p2.Bugs) != 0 {
		t.Errorf("Projects[2].Bugs should be empty, got %+v", p2.Bugs)
	}

	if cfg.Build.TimeoutMinutes != 45 {
		t.Errorf("Build.TimeoutMinutes = %d, want 45", cfg.Build.TimeoutMinutes)
	}
	if cfg.Build.ArtifactsDir != artifactsDir {
		t.Errorf("Build.ArtifactsDir = %q, want %q", cfg.Build.ArtifactsDir, artifactsDir)
	}
	if cfg.TUI.DefaultPane != "packages" {
		t.Fatalf("TUI.DefaultPane = %q, want packages", cfg.TUI.DefaultPane)
	}
	if cfg.TUI.Panes.Packages == nil || cfg.TUI.Panes.Packages.Mode != "excuses" {
		t.Fatalf("TUI.Panes.Packages = %+v, want excuses preset", cfg.TUI.Panes.Packages)
	}
	if cfg.TUI.Panes.Bugs == nil || cfg.TUI.Panes.Bugs.Filters.Merge == nil || *cfg.TUI.Panes.Bugs.Filters.Merge {
		t.Fatalf("TUI.Panes.Bugs = %+v, want merge=false preset", cfg.TUI.Panes.Bugs)
	}
}

func TestValidateSRUParentGateRequiresConfiguredUbuntuParent(t *testing.T) {
	cfg := Config{Packages: PackagesConfig{Distros: map[string]DistroConfig{
		"ubuntu": {Releases: map[string]ReleaseConfig{
			"resolute": {},
			"noble": {Backports: map[string]BackportConfig{
				"gazpacho": {ParentRelease: "resolute", SRUParentRequired: true},
			}},
		}},
	}}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid gate rejected: %v", err)
	}
	backport := cfg.Packages.Distros["ubuntu"].Releases["noble"].Backports["gazpacho"]
	backport.ParentRelease = "plucky"
	release := cfg.Packages.Distros["ubuntu"].Releases["noble"]
	release.Backports["gazpacho"] = backport
	cfg.Packages.Distros["ubuntu"].Releases["noble"] = release
	if err := cfg.Validate(); err == nil {
		t.Fatal("unconfigured SRU parent accepted")
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Build.TimeoutMinutes != 30 {
		t.Errorf("default TimeoutMinutes = %d, want 30", cfg.Build.TimeoutMinutes)
	}
	if cfg.Build.ArtifactsDir != "artifacts" {
		t.Errorf("default ArtifactsDir = %q, want %q", cfg.Build.ArtifactsDir, "artifacts")
	}
}

func TestLoad_MissingExplicitFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("Load() should error for missing explicit config path")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("key:\n\t- bad indent"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgFile)
	if err == nil {
		t.Fatal("Load() should error for invalid YAML")
	}
}

func TestLoad_TUIUnknownPaneFilterFails(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")

	yaml := `
projects: []
build:
  default_prefix: sunbeam
tui:
  panes:
    packages:
      filters:
        not_a_filter: nope
`
	if err := os.WriteFile(cfgFile, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgFile)
	if err == nil {
		t.Fatal("Load() should error for unknown tui filter key")
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Packages: PackagesConfig{
			Distros: map[string]DistroConfig{
				"ubuntu": {},
			},
		},
		Projects: []ProjectConfig{
			{
				Name: "p1",
				Code: CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
				Bugs: []BugTrackerConfig{
					{Forge: "launchpad", Project: "p1"},
				},
			},
			{
				Name: "p2",
				Code: CodeConfig{Forge: "gerrit", Host: "https://review.opendev.org", Project: "openstack/nova"},
			},
			{
				Name: "p3",
				Code: CodeConfig{Forge: "launchpad", Project: "proj"},
			},
		},
		TUI: TUIConfig{
			DefaultPane: "packages",
			Panes: TUIPanesConfig{
				Packages: &TUIPackagesPaneConfig{
					Mode: "excuses",
					Filters: TUIPackagesFiltersConfig{
						Tracker: "ubuntu",
					},
				},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestValidate_InvalidTUIDefaultPane(t *testing.T) {
	cfg := &Config{TUI: TUIConfig{DefaultPane: "not-a-pane"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error for invalid tui default pane")
	}
}

func TestValidate_MissingName(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{Code: CodeConfig{Forge: "github", Owner: "org", Project: "repo"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for missing name")
	}
}

func TestValidate_InvalidCodeForge(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{Name: "p1", Code: CodeConfig{Forge: "gitlab", Project: "repo"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for invalid code forge")
	}
}

func TestValidate_GitHubMissingOwner(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{Name: "p1", Code: CodeConfig{Forge: "github", Project: "repo"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for github code missing owner")
	}
}

func TestValidate_GerritMissingHost(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{Name: "p1", Code: CodeConfig{Forge: "gerrit", Project: "openstack/nova"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for gerrit code missing host")
	}
}

func TestValidate_InvalidBugForge(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name: "p1",
				Code: CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
				Bugs: []BugTrackerConfig{{Forge: "jira", Project: "proj"}},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for invalid bug tracker forge")
	}
}

func TestValidate_BugMissingProject(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name: "p1",
				Code: CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
				Bugs: []BugTrackerConfig{{Forge: "launchpad"}},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for bug tracker missing project")
	}
}

func TestValidate_BugGroupUnknown(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name: "p1",
			Code: CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
			Bugs: []BugTrackerConfig{{Forge: "launchpad", Project: "lp-proj", Group: "missing"}},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error for unknown bug group")
	}
}

func TestValidate_BugGroupCommonProjectMustMatchTrackedProject(t *testing.T) {
	cfg := &Config{
		BugGroups: map[string]BugGroupConfig{
			"sunbeam": {CommonProject: "snap-openstack"},
		},
		Projects: []ProjectConfig{{
			Name: "p1",
			Code: CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
			Bugs: []BugTrackerConfig{{Forge: "launchpad", Project: "sunbeam-charms", Group: "sunbeam"}},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error when common_project is not among grouped tracker projects")
	}
}

func TestValidate_BugGroupRejectsMixedForges(t *testing.T) {
	cfg := &Config{
		BugGroups: map[string]BugGroupConfig{
			"mixed": {CommonProject: "demo"},
		},
		Projects: []ProjectConfig{
			{
				Name: "p1",
				Code: CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
				Bugs: []BugTrackerConfig{{Forge: "launchpad", Project: "demo", Group: "mixed"}},
			},
			{
				Name: "p2",
				Code: CodeConfig{Forge: "github", Owner: "org", Project: "repo2"},
				Bugs: []BugTrackerConfig{{Forge: "github", Owner: "org", Project: "repo2", Group: "mixed"}},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error for mixed-forge bug group")
	}
}

func TestValidate_ValidArtifactTypeAndBuild(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:         "p1",
				ArtifactType: "rock",
				Code:         CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
				Build:        &ProjectBuildConfig{Owner: "team", Artifacts: []string{"recipe1"}},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestValidate_InvalidArtifactType(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:         "p1",
				ArtifactType: "docker",
				Code:         CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for invalid artifact_type")
	}
}

func TestValidate_BuildWithoutArtifactType(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:  "p1",
				Code:  CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
				Build: &ProjectBuildConfig{Owner: "team"},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for build without artifact_type")
	}
}

func TestValidate_BuildWithoutOwner(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:         "p1",
				ArtifactType: "charm",
				Code:         CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
				Build:        &ProjectBuildConfig{OfficialCodehosting: true},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for build without owner when official_codehosting is true")
	}

	// No error when official_codehosting is false and owner is empty
	cfg2 := &Config{
		Projects: []ProjectConfig{
			{
				Name:         "p2",
				ArtifactType: "charm",
				Code:         CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
				Build:        &ProjectBuildConfig{},
			},
		},
	}
	if err := cfg2.Validate(); err != nil {
		t.Errorf("Validate() unexpected error for build without owner when official_codehosting is false: %v", err)
	}
}

func TestValidate_BuildWithPrepareCommand(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:         "p1",
				ArtifactType: "snap",
				Code:         CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
				Build:        &ProjectBuildConfig{Owner: "team", PrepareCommand: "make prepare"},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestValidate_DevelopmentFocusNotInSeries(t *testing.T) {
	cfg := &Config{
		Launchpad: LaunchpadConfig{
			Series:           []string{"2024.1", "2024.2"},
			DevelopmentFocus: "2025.1",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error when development_focus is not in series")
	}
}

func TestValidate_DevelopmentFocusInSeries(t *testing.T) {
	cfg := &Config{
		Launchpad: LaunchpadConfig{
			Series:           []string{"2024.1", "2024.2", "2025.1"},
			DevelopmentFocus: "2025.1",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestValidate_DevelopmentFocusWithoutSeries(t *testing.T) {
	cfg := &Config{
		Launchpad: LaunchpadConfig{
			DevelopmentFocus: "2025.1",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should pass when series is empty: %v", err)
	}
}

func TestValidate_ProjectDevelopmentFocusNotInSeries(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:             "p1",
				Code:             CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
				Series:           []string{"1.0", "2.0"},
				DevelopmentFocus: "3.0",
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error when project development_focus is not in project series")
	}
}

func TestValidate_ProjectDevelopmentFocusInSeries(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:             "p1",
				Code:             CodeConfig{Forge: "github", Owner: "org", Project: "repo"},
				Series:           []string{"1.0", "2.0"},
				DevelopmentFocus: "2.0",
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestLoad_UpstreamConfig(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")

	yaml := `
packages:
  upstream:
    provider: openstack
    releases_repo: https://opendev.org/openstack/releases
    requirements_repo: https://opendev.org/openstack/requirements
  distros:
    ubuntu:
      mirror: http://archive.ubuntu.com/ubuntu
      suites: [noble]
      components: [main]
`
	if err := os.WriteFile(cfgFile, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Packages.Upstream == nil {
		t.Fatal("Packages.Upstream should not be nil")
	}
	if cfg.Packages.Upstream.Provider != "openstack" {
		t.Errorf("Upstream.Provider = %q, want %q", cfg.Packages.Upstream.Provider, "openstack")
	}
	if cfg.Packages.Upstream.ReleasesRepo != "https://opendev.org/openstack/releases" {
		t.Errorf("Upstream.ReleasesRepo = %q, want %q", cfg.Packages.Upstream.ReleasesRepo, "https://opendev.org/openstack/releases")
	}
	if cfg.Packages.Upstream.RequirementsRepo != "https://opendev.org/openstack/requirements" {
		t.Errorf("Upstream.RequirementsRepo = %q, want %q", cfg.Packages.Upstream.RequirementsRepo, "https://opendev.org/openstack/requirements")
	}
	if len(cfg.Packages.Distros) != 1 {
		t.Errorf("len(Packages.Distros) = %d, want 1", len(cfg.Packages.Distros))
	}
}

func TestValidate_UpstreamMissingProvider(t *testing.T) {
	cfg := &Config{
		Packages: PackagesConfig{
			Upstream: &UpstreamConfig{},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for upstream missing provider")
	}
}

func TestValidate_UpstreamOpenstackMissingReleasesRepo(t *testing.T) {
	cfg := &Config{
		Packages: PackagesConfig{
			Upstream: &UpstreamConfig{Provider: "openstack"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error for openstack provider missing releases_repo")
	}
}

func TestValidate_UpstreamValid(t *testing.T) {
	cfg := &Config{
		Packages: PackagesConfig{
			Upstream: &UpstreamConfig{
				Provider:     "openstack",
				ReleasesRepo: "https://opendev.org/openstack/releases",
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestValidate_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should pass for empty config: %v", err)
	}
}

func TestValidate_ReleaseConfigRejectsTracksAndTrackMapTogether(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name:         "sunbeam",
			Code:         CodeConfig{Forge: "github", Owner: "canonical", Project: "snap-openstack"},
			ArtifactType: "snap",
			Release: &ProjectReleaseConfig{
				Tracks:   []string{"2024.1"},
				TrackMap: map[string]string{"2024.1": "latest"},
			},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error when release.tracks and release.track_map are both set")
	}
}

func TestValidate_ReleaseConfigAcceptsTrackMapAndBranches(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name:         "sunbeam",
			Code:         CodeConfig{Forge: "github", Owner: "canonical", Project: "snap-openstack"},
			ArtifactType: "snap",
			Series:       []string{"2024.1", "2025.1"},
			Release: &ProjectReleaseConfig{
				TrackMap: map[string]string{"2025.1": "latest"},
				Branches: []ProjectReleaseBranchConfig{{
					Series: "2024.1",
					Branch: "risc-v",
					Risks:  []string{"edge", "stable"},
				}, {
					Track:  "latest",
					Branch: "hotfix",
				}},
			},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() should accept release overrides: %v", err)
	}
}

func TestValidate_ReleaseConfigRequiresSnapOrCharmArtifactType(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name:         "sunbeam",
			Code:         CodeConfig{Forge: "github", Owner: "canonical", Project: "snap-openstack"},
			ArtifactType: "rock",
			Release: &ProjectReleaseConfig{
				Tracks: []string{"2024.1"},
			},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject release overrides for non snap/charm projects")
	}
}

func TestValidate_ReleaseBranchRequiresExactlyOneSeriesOrTrack(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name:         "sunbeam",
			Code:         CodeConfig{Forge: "github", Owner: "canonical", Project: "snap-openstack"},
			ArtifactType: "snap",
			Series:       []string{"2024.1"},
			Release: &ProjectReleaseConfig{
				Branches: []ProjectReleaseBranchConfig{{
					Series: "2024.1",
					Track:  "latest",
					Branch: "risc-v",
				}},
			},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject branches with both series and track")
	}
}

func TestValidate_ReleaseTrackMapRequiresKnownSeries(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name:         "sunbeam",
			Code:         CodeConfig{Forge: "github", Owner: "canonical", Project: "snap-openstack"},
			ArtifactType: "snap",
			Series:       []string{"2024.1"},
			Release: &ProjectReleaseConfig{
				TrackMap: map[string]string{"2025.1": "latest"},
			},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject track_map keys outside declared series")
	}
}

func TestValidate_ReleaseTrackMapAcceptsLaunchpadSeriesFallback(t *testing.T) {
	cfg := &Config{
		Launchpad: LaunchpadConfig{Series: []string{"2024.1", "2025.1"}},
		Projects: []ProjectConfig{{
			Name:         "sunbeam",
			Code:         CodeConfig{Forge: "github", Owner: "canonical", Project: "snap-openstack"},
			ArtifactType: "snap",
			Release: &ProjectReleaseConfig{
				TrackMap: map[string]string{"2025.1": "latest"},
			},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() should accept launchpad.series fallback for release.track_map: %v", err)
	}
}

func TestValidate_ReleaseBranchRejectsUnknownSeriesAndInvalidRisk(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name:         "sunbeam",
			Code:         CodeConfig{Forge: "github", Owner: "canonical", Project: "snap-openstack"},
			ArtifactType: "snap",
			Series:       []string{"2024.1"},
			Release: &ProjectReleaseConfig{
				Branches: []ProjectReleaseBranchConfig{{
					Series: "2025.1",
					Branch: "risc-v",
					Risks:  []string{"weird"},
				}},
			},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject unknown branch series and invalid risk")
	}
}

func TestValidate_ReleaseTracksRejectDuplicateValues(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name:         "sunbeam",
			Code:         CodeConfig{Forge: "github", Owner: "canonical", Project: "snap-openstack"},
			ArtifactType: "snap",
			Release: &ProjectReleaseConfig{
				Tracks: []string{"2024.1", "2024.1"},
			},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject duplicate release tracks")
	}
}

func TestValidate_ReleaseSkipArtifactsRejectsEmptyAndDuplicateValues(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name:         "sunbeam-charms",
			Code:         CodeConfig{Forge: "gerrit", Host: "https://review.opendev.org", Project: "openstack/sunbeam-charms"},
			ArtifactType: "charm",
			Release: &ProjectReleaseConfig{
				SkipArtifacts: []string{"sunbeam-libs", ""},
			},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject empty release.skip_artifacts values")
	}

	cfg.Projects[0].Release.SkipArtifacts = []string{"sunbeam-libs", "sunbeam-libs"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject duplicate release.skip_artifacts values")
	}
}

func TestValidate_TeamSkipArtifactsRejectsEmptyAndDuplicateValues(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name:         "sunbeam-charms",
			Code:         CodeConfig{Forge: "gerrit", Host: "https://review.opendev.org", Project: "openstack/sunbeam-charms"},
			ArtifactType: "charm",
			Team: &ProjectTeamConfig{
				SkipArtifacts: []string{"sunbeam-libs", ""},
			},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject empty team.skip_artifacts values")
	}

	cfg.Projects[0].Team.SkipArtifacts = []string{"sunbeam-libs", "sunbeam-libs"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject duplicate team.skip_artifacts values")
	}
}

func TestLoad_ProjectTeamSkipArtifacts(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")

	yaml := `
projects:
  - name: sunbeam-charms
    artifact_type: charm
    code:
      forge: gerrit
      host: https://review.opendev.org
      project: openstack/sunbeam-charms
    team:
      skip_artifacts:
        - foo
        - bar
`
	if err := os.WriteFile(cfgFile, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(cfg.Projects))
	}
	team := cfg.Projects[0].Team
	if team == nil {
		t.Fatal("Projects[0].Team = nil, want populated ProjectTeamConfig")
	}
	want := []string{"foo", "bar"}
	if len(team.SkipArtifacts) != len(want) {
		t.Fatalf("Team.SkipArtifacts = %v, want %v", team.SkipArtifacts, want)
	}
	for i, v := range want {
		if team.SkipArtifacts[i] != v {
			t.Errorf("Team.SkipArtifacts[%d] = %q, want %q", i, team.SkipArtifacts[i], v)
		}
	}
}

func TestValidate_OTelMetricsListenerRequiresAddressWhenEnabled(t *testing.T) {
	cfg := &Config{OTel: OTelConfig{Metrics: OTelMetricsConfig{Self: OTelMetricsListenerConfig{Enabled: true}}}}
	if err := cfg.Validate(); err == nil {
		fatalf := t.Fatalf
		fatalf("Validate() expected error for enabled self metrics without listen_addr")
	}
}

func TestValidate_OTelTraceSignalRequiresEndpointWhenEnabled(t *testing.T) {
	cfg := &Config{OTel: OTelConfig{Traces: OTelSignalConfig{Enabled: true}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for enabled traces without endpoint")
	}
}

func TestValidate_OTelListenerCollision(t *testing.T) {
	cfg := &Config{OTel: OTelConfig{Metrics: OTelMetricsConfig{
		Self:   OTelMetricsListenerConfig{Enabled: true, ListenAddr: "127.0.0.1:9464"},
		Domain: OTelMetricsListenerConfig{Enabled: true, ListenAddr: "127.0.0.1:9464"},
	}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for listener collision")
	}
}

func TestValidate_OTelDomainLiveSystemsRejectsUnknownAndDuplicateValues(t *testing.T) {
	cfg := &Config{OTel: OTelConfig{Metrics: OTelMetricsConfig{
		Domain: OTelMetricsListenerConfig{LiveSystems: []string{"reviews", "unknown"}},
	}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for unknown live system")
	}

	cfg.OTel.Metrics.Domain.LiveSystems = []string{"reviews", "reviews"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for duplicate live system")
	}
}

func TestValidate_OTelSelfLiveSystemsRejected(t *testing.T) {
	cfg := &Config{OTel: OTelConfig{Metrics: OTelMetricsConfig{
		Self: OTelMetricsListenerConfig{LiveSystems: []string{"reviews"}},
	}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for self live_systems")
	}
}

func TestValidate_ReleasesTargetProfiles(t *testing.T) {
	cfg := &Config{
		Releases: ReleasesConfig{
			DefaultTargetProfile: "noble-and-newer",
			TargetProfiles: map[string]ReleaseTargetProfileConfig{
				"noble-and-newer": {
					Include: []ReleaseTargetMatcherConfig{{
						BaseNames:      []string{"ubuntu"},
						MinBaseChannel: "24.04",
					}},
				},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() should accept valid release target profiles: %v", err)
	}
}

func TestValidate_ReleasesTargetProfileRejectsInvalidMinBaseChannel(t *testing.T) {
	cfg := &Config{
		Releases: ReleasesConfig{
			TargetProfiles: map[string]ReleaseTargetProfileConfig{
				"broken": {
					Include: []ReleaseTargetMatcherConfig{{
						MinBaseChannel: "noble",
					}},
				},
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject invalid min_base_channel")
	}
}

func TestValidate_ProjectReleaseTargetProfileMustExist(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name:         "openstack",
			ArtifactType: "snap",
			Code:         CodeConfig{Forge: "github", Owner: "canonical", Project: "snap-openstack"},
			Release: &ProjectReleaseConfig{
				TargetProfile: "missing",
			},
		}},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject unknown release.target_profile")
	}
}

func TestValidate_ProjectReleaseTargetProfileOverrides(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{{
			Name:         "openstack",
			ArtifactType: "snap",
			Code:         CodeConfig{Forge: "github", Owner: "canonical", Project: "snap-openstack"},
			Release: &ProjectReleaseConfig{
				TargetProfileOverrides: &ReleaseTargetProfileConfig{
					Exclude: []ReleaseTargetMatcherConfig{{
						Architectures: []string{"s390x"},
					}},
				},
			},
		}},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() should accept release.target_profile_overrides without a named profile: %v", err)
	}
}

func TestExpandSuiteHelpers(t *testing.T) {
	if got := ExpandSuiteType("noble", "release"); got != "noble" {
		t.Fatalf("ExpandSuiteType() = %q, want noble", got)
	}
	if got := ExpandSuiteType("noble", "updates"); got != "noble-updates" {
		t.Fatalf("ExpandSuiteType() = %q, want noble-updates", got)
	}
	if got := ExpandBackportSuiteType("noble", "gazpacho", "release"); got != "noble" {
		t.Fatalf("ExpandBackportSuiteType(release) = %q, want noble", got)
	}
	if got := ExpandBackportSuiteType("noble", "gazpacho", "updates"); got != "noble-updates/gazpacho" {
		t.Fatalf("ExpandBackportSuiteType(updates) = %q", got)
	}
	if got := ExpandBackportSuiteType("noble", "gazpacho", "trixie-gazpacho-backports"); got != "trixie-gazpacho-backports" {
		t.Fatalf("ExpandBackportSuiteType(literal) = %q", got)
	}
}

func TestValidateTUIConfig_AllPaneEnumsAccepted(t *testing.T) {
	active := true
	allTargets := true
	merge := false
	behind := true
	ftbfs := true
	autopkgtest := true
	bugged := true
	reverse := true
	includeMRs := true
	cfg := &Config{
		Releases: ReleasesConfig{
			TargetProfiles: map[string]ReleaseTargetProfileConfig{
				"noble-and-newer": {Include: []ReleaseTargetMatcherConfig{{BaseNames: []string{"ubuntu"}}}},
			},
		},
		Packages: PackagesConfig{
			Distros: map[string]DistroConfig{
				"ubuntu": {

					Releases: map[string]ReleaseConfig{
						"noble": {
							Backports: map[string]BackportConfig{
								"gazpacho": {},
							},
						},
					},
				},
			},
		},
		TUI: TUIConfig{
			DefaultPane: "packages",
			Panes: TUIPanesConfig{
				Builds: &TUIBuildsPaneConfig{Filters: TUIBuildsFiltersConfig{Active: &active, Source: "local"}},
				Releases: &TUIReleasesPaneConfig{Filters: TUIReleasesFiltersConfig{
					ArtifactType:  "snap",
					Risk:          "stable",
					TargetProfile: "noble-and-newer",
					AllTargets:    &allTargets,
				}},
				Packages: &TUIPackagesPaneConfig{
					Mode: "excuses",
					Filters: TUIPackagesFiltersConfig{
						Backport:       "gazpacho",
						Tracker:        "ubuntu",
						Merge:          &merge,
						BehindUpstream: &behind,
						FTBFS:          &ftbfs,
						Autopkgtest:    &autopkgtest,
						Bugged:         &bugged,
						Reverse:        &reverse,
					},
				},
				Bugs:    &TUIBugsPaneConfig{Filters: TUIBugsFiltersConfig{Status: "Fix Released", Importance: "Critical", Merge: &merge}},
				Reviews: &TUIReviewsPaneConfig{Filters: TUIReviewsFiltersConfig{Forge: "github", State: "wip"}},
				Commits: &TUICommitsPaneConfig{Mode: "track", Filters: TUICommitsFiltersConfig{Forge: "gerrit", IncludeMRs: &includeMRs}},
				Projects: &TUIProjectsPaneConfig{Filters: TUIProjectsFiltersConfig{
					ArtifactType: "rock",
					CodeForge:    "github",
					BugForge:     "launchpad",
					HasBuild:     "true",
					HasRelease:   "any",
				}},
			},
		},
	}

	if err := validateTUIConfig(cfg.TUI, cfg); err != nil {
		t.Fatalf("validateTUIConfig() error = %v", err)
	}
}

func TestValidateTUIConfig_InvalidValues(t *testing.T) {
	base := &Config{
		Packages: PackagesConfig{
			Distros: map[string]DistroConfig{
				"ubuntu": {},
			},
		},
	}

	cases := []struct {
		name string
		tui  TUIConfig
	}{
		{name: "invalid build source", tui: TUIConfig{Panes: TUIPanesConfig{Builds: &TUIBuildsPaneConfig{Filters: TUIBuildsFiltersConfig{Source: "weird"}}}}},
		{name: "invalid release artifact", tui: TUIConfig{Panes: TUIPanesConfig{Releases: &TUIReleasesPaneConfig{Filters: TUIReleasesFiltersConfig{ArtifactType: "deb"}}}}},
		{name: "invalid release risk", tui: TUIConfig{Panes: TUIPanesConfig{Releases: &TUIReleasesPaneConfig{Filters: TUIReleasesFiltersConfig{Risk: "daily"}}}}},
		{name: "invalid package mode", tui: TUIConfig{Panes: TUIPanesConfig{Packages: &TUIPackagesPaneConfig{Mode: "browse"}}}},
		{name: "invalid package tracker", tui: TUIConfig{Panes: TUIPanesConfig{Packages: &TUIPackagesPaneConfig{Filters: TUIPackagesFiltersConfig{Tracker: "nonexistent"}}}}},
		{name: "invalid bug status", tui: TUIConfig{Panes: TUIPanesConfig{Bugs: &TUIBugsPaneConfig{Filters: TUIBugsFiltersConfig{Status: "Done"}}}}},
		{name: "invalid review forge", tui: TUIConfig{Panes: TUIPanesConfig{Reviews: &TUIReviewsPaneConfig{Filters: TUIReviewsFiltersConfig{Forge: "gitlab"}}}}},
		{name: "invalid commit mode", tui: TUIConfig{Panes: TUIPanesConfig{Commits: &TUICommitsPaneConfig{Mode: "browse"}}}},
		{name: "invalid project bool", tui: TUIConfig{Panes: TUIPanesConfig{Projects: &TUIProjectsPaneConfig{Filters: TUIProjectsFiltersConfig{HasBuild: "sometimes"}}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := *base
			cfg.TUI = tc.tui
			if err := validateTUIConfig(cfg.TUI, &cfg); err == nil {
				t.Fatal("validateTUIConfig() expected error")
			}
		})
	}
}

func TestTUIHelperFunctions(t *testing.T) {
	cfg := &Config{
		Packages: PackagesConfig{
			Distros: map[string]DistroConfig{
				"ubuntu": {

					Releases: map[string]ReleaseConfig{
						"noble": {
							Backports: map[string]BackportConfig{
								"gazpacho": {},
							},
						},
					},
				},
			},
		},
	}

	if !isConfiguredBackport(cfg, "gazpacho") {
		t.Fatal("isConfiguredBackport() = false, want true")
	}
	if isConfiguredBackport(cfg, "epoxy") {
		t.Fatal("isConfiguredBackport() = true, want false")
	}
	if !isConfiguredExcusesTracker(cfg, "ubuntu") {
		t.Fatal("isConfiguredExcusesTracker() = false, want true")
	}
	if containsOneOf("gamma", []string{"alpha", "beta"}) {
		t.Fatal("containsOneOf() = true, want false")
	}
}

func TestValidateSignalConfig_SamplingRatioIgnoredWhenNotAllowed(t *testing.T) {
	cfg := OTelSignalConfig{SamplingRatio: 2.0}
	if err := validateSignalConfig("test", cfg, false); err != nil {
		t.Fatalf("validateSignalConfig() with allowSampling=false and SamplingRatio=2.0 should return nil, got: %v", err)
	}
}

func TestValidate_CollaboratorsEmptyTeam(t *testing.T) {
	cfg := &Config{
		Collaborators: &CollaboratorsConfig{LaunchpadTeam: ""},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should error for empty launchpad_team")
	}
}

func TestValidate_CollaboratorsValid(t *testing.T) {
	cfg := &Config{
		Collaborators: &CollaboratorsConfig{LaunchpadTeam: "ubuntu-openstack"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestValidate_CollaboratorsNil(t *testing.T) {
	cfg := &Config{}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should pass with nil collaborators: %v", err)
	}
}

func TestLoad_ClientConfigFields(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")

	yaml := `
server_address: "https://watchtower.example.com:8080"
server_token: "s3cr3t-server-token"
auth_token: "b3ar3r-auth-token"
`
	if err := os.WriteFile(cfgFile, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.ServerAddress != "https://watchtower.example.com:8080" {
		t.Errorf("ServerAddress = %q, want %q", cfg.ServerAddress, "https://watchtower.example.com:8080")
	}
	if cfg.ServerToken != "s3cr3t-server-token" {
		t.Errorf("ServerToken = %q, want %q", cfg.ServerToken, "s3cr3t-server-token")
	}
	if cfg.AuthToken != "b3ar3r-auth-token" {
		t.Errorf("AuthToken = %q, want %q", cfg.AuthToken, "b3ar3r-auth-token")
	}
}
