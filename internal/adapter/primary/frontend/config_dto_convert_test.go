// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"testing"

	"github.com/gboutry/sunbeam-watchtower/internal/config"
)

func boolPtr(b bool) *bool { return &b }

func TestConfigToDTO_IncludesServerAddressButRedactsSecrets(t *testing.T) {
	cfg := &config.Config{
		ServerAddress: "http://remote:8472",
		ServerToken:   "token",
		AuthToken:     "auth",
	}
	d := ConfigToDTO(cfg)
	if d.ServerAddress != "http://remote:8472" {
		t.Fatalf("ServerAddress = %q, want %q", d.ServerAddress, "http://remote:8472")
	}
	if d.ServerToken != "" {
		t.Fatalf("ServerToken should be redacted, got %q", d.ServerToken)
	}
	if d.AuthToken != "" {
		t.Fatalf("AuthToken should be redacted, got %q", d.AuthToken)
	}
}

func TestDTOToConfigNil(t *testing.T) {
	if DTOToConfig(nil) != nil {
		t.Fatal("DTOToConfig(nil) should return nil")
	}
}

func TestLaunchpadPackageSetsSurviveConfigRoundTrip(t *testing.T) {
	cfg := &config.Config{Packages: config.PackagesConfig{
		LaunchpadSets: map[string]config.LaunchpadSetConfig{
			"openstack": {Series: "development"},
		},
	}}
	got := DTOToConfig(ConfigToDTO(cfg))
	if got.Packages.LaunchpadSets["openstack"].Series != "development" {
		t.Fatalf("LaunchpadSets = %+v, want openstack development", got.Packages.LaunchpadSets)
	}
}

func TestDTOToConfigRoundTrip(t *testing.T) {
	trueVal := true
	original := &config.Config{
		Launchpad: config.LaunchpadConfig{
			DefaultOwner:     "~myteam",
			UseKeyring:       true,
			Series:           []string{"noble", "jammy"},
			DevelopmentFocus: "noble",
		},
		GitHub: config.GitHubConfig{
			UseKeyring: false,
			ClientID:   "gh-client-id",
		},
		Gerrit: config.GerritConfig{
			Hosts: []config.GerritHost{
				{URL: "https://review.example.com"},
			},
		},
		Build: config.BuildConfig{
			DefaultPrefix:  "tmp-build",
			TimeoutMinutes: 45,
			ArtifactsDir:   "dist",
		},
		Releases: config.ReleasesConfig{
			DefaultTargetProfile: "default",
			TargetProfiles: map[string]config.ReleaseTargetProfileConfig{
				"default": {
					Include: []config.ReleaseTargetMatcherConfig{
						{
							BaseNames:      []string{"ubuntu"},
							BaseChannels:   []string{"22.04/stable"},
							MinBaseChannel: "20.04/stable",
							Architectures:  []string{"amd64", "arm64"},
						},
					},
					Exclude: []config.ReleaseTargetMatcherConfig{
						{
							BaseNames: []string{"centos"},
						},
					},
				},
			},
		},
		TUI: config.TUIConfig{
			DefaultPane: "builds",
			Panes: config.TUIPanesConfig{
				Builds: &config.TUIBuildsPaneConfig{
					Filters: config.TUIBuildsFiltersConfig{
						Project: "myproject",
						State:   "running",
						Active:  &trueVal,
						Source:  "lp",
					},
				},
				Releases: &config.TUIReleasesPaneConfig{
					Filters: config.TUIReleasesFiltersConfig{
						Project:       "myproject",
						ArtifactType:  "charm",
						Risk:          "stable",
						Track:         "2.0",
						Branch:        "main",
						TargetProfile: "default",
						AllTargets:    &trueVal,
					},
				},
				Packages: &config.TUIPackagesPaneConfig{
					Mode: "excuses",
					Filters: config.TUIPackagesFiltersConfig{
						Set:             "myset",
						Distro:          "ubuntu",
						Release:         "noble",
						Suite:           "proposed",
						Component:       "main",
						Backport:        "uca",
						Merge:           &trueVal,
						UpstreamRelease: "2024.1",
						BehindUpstream:  &trueVal,
						OnlyIn:          "ubuntu",
						Constraints:     "amd64",
						Tracker:         "lp",
						Name:            "neutron",
						Team:            "openstack",
						FTBFS:           boolPtr(false),
						Autopkgtest:     &trueVal,
						BlockedBy:       "libssl",
						Bugged:          boolPtr(false),
						MinAge:          "3d",
						MaxAge:          "30d",
						Limit:           "50",
						Reverse:         &trueVal,
					},
				},
				Bugs: &config.TUIBugsPaneConfig{
					Filters: config.TUIBugsFiltersConfig{
						Project:    "myproject",
						Status:     "New",
						Importance: "Critical",
						Assignee:   "nobody",
						Tag:        "regression",
						Since:      "2024-01-01",
						Merge:      &trueVal,
					},
				},
				Reviews: &config.TUIReviewsPaneConfig{
					Filters: config.TUIReviewsFiltersConfig{
						Project: "myproject",
						Forge:   "github",
						State:   "open",
						Author:  "alice",
						Since:   "2024-01-01",
					},
				},
				Commits: &config.TUICommitsPaneConfig{
					Mode: "diff",
					Filters: config.TUICommitsFiltersConfig{
						Project:    "myproject",
						Forge:      "github",
						Branch:     "main",
						Author:     "alice",
						IncludeMRs: &trueVal,
						BugID:      "1234567",
					},
				},
				Projects: &config.TUIProjectsPaneConfig{
					Filters: config.TUIProjectsFiltersConfig{
						Name:         "neutron",
						ArtifactType: "charm",
						CodeForge:    "github",
						BugForge:     "launchpad",
						HasBuild:     "true",
						HasRelease:   "true",
					},
				},
			},
		},
		BugGroups: map[string]config.BugGroupConfig{
			"openstack": {CommonProject: "openstack"},
		},
		OTel: config.OTelConfig{
			ServiceName:        "watchtower",
			ServiceNamespace:   "canonical",
			ResourceAttributes: map[string]string{"env": "prod"},
			Metrics: config.OTelMetricsConfig{
				Self: config.OTelMetricsListenerConfig{
					Enabled:                true,
					ListenAddr:             ":9090",
					Path:                   "/metrics",
					Runtime:                true,
					Process:                true,
					DefaultRefreshInterval: "5m",
				},
				Domain: config.OTelMetricsListenerConfig{
					Enabled:                true,
					ListenAddr:             ":9091",
					Path:                   "/metrics",
					Runtime:                false,
					Process:                false,
					DefaultRefreshInterval: "10m",
					LiveSystems:            []string{"builds", "releases"},
				},
				Collectors: config.OTelDomainCollectorsConfig{
					Auth:       config.OTelCollectorConfig{Enabled: true, RefreshInterval: "1m"},
					Operations: config.OTelCollectorConfig{Enabled: true, RefreshInterval: "2m"},
					Projects:   config.OTelCollectorConfig{Enabled: true, RefreshInterval: "3m"},
					Builds:     config.OTelCollectorConfig{Enabled: true, RefreshInterval: "4m"},
					Releases:   config.OTelCollectorConfig{Enabled: true, RefreshInterval: "5m"},
					Reviews:    config.OTelCollectorConfig{Enabled: true, RefreshInterval: "6m"},
					Commits:    config.OTelCollectorConfig{Enabled: true, RefreshInterval: "7m"},
					Bugs:       config.OTelCollectorConfig{Enabled: true, RefreshInterval: "8m"},
					Packages:   config.OTelCollectorConfig{Enabled: true, RefreshInterval: "9m"},
					Excuses:    config.OTelCollectorConfig{Enabled: true, RefreshInterval: "10m"},
					Cache:      config.OTelCollectorConfig{Enabled: true, RefreshInterval: "11m"},
				},
			},
			Traces: config.OTelSignalConfig{
				Enabled:       true,
				Endpoint:      "localhost:4317",
				Protocol:      "grpc",
				Insecure:      true,
				Headers:       map[string]string{"x-token": "abc"},
				SamplingRatio: 0.5,
				MinLevel:      "info",
				MirrorStderr:  true,
			},
			Logs: config.OTelSignalConfig{
				Enabled:  true,
				Endpoint: "localhost:4318",
				Protocol: "http",
			},
		},
		Projects: []config.ProjectConfig{
			{
				Name:             "neutron",
				ArtifactType:     "charm",
				Series:           []string{"noble"},
				DevelopmentFocus: "noble",
				Code: config.CodeConfig{
					Forge:   "github",
					Owner:   "openstack",
					Host:    "github.com",
					Project: "neutron",
					GitURL:  "https://github.com/openstack/neutron.git",
				},
				Build: &config.ProjectBuildConfig{
					Owner:          "~myteam",
					Artifacts:      []string{"neutron.charm"},
					PrepareCommand: "make build",
				},
				Bugs: []config.BugTrackerConfig{
					{
						Forge:   "launchpad",
						Owner:   "~myteam",
						Host:    "",
						Project: "neutron",
						Group:   "openstack",
					},
				},
				Release: &config.ProjectReleaseConfig{
					Tracks:        []string{"2.0", "3.0"},
					TrackMap:      map[string]string{"noble": "2.0"},
					SkipArtifacts: []string{"old.charm"},
					TargetProfile: "default",
					TargetProfileOverrides: &config.ReleaseTargetProfileConfig{
						Include: []config.ReleaseTargetMatcherConfig{
							{BaseNames: []string{"ubuntu"}, Architectures: []string{"amd64"}},
						},
						Exclude: []config.ReleaseTargetMatcherConfig{},
					},
					Branches: []config.ProjectReleaseBranchConfig{
						{
							Series: "noble",
							Track:  "2.0",
							Branch: "stable/2.0",
							Risks:  []string{"stable", "candidate"},
						},
					},
				},
			},
		},
		Packages: config.PackagesConfig{
			Distros: map[string]config.DistroConfig{
				"ubuntu": {
					Mirror:     "http://archive.ubuntu.com/ubuntu",
					Components: []string{"main", "universe"},
					Releases: map[string]config.ReleaseConfig{
						"noble": {
							Suites: []string{"release", "updates", "proposed"},
							Backports: map[string]config.BackportConfig{
								"uca": {
									ParentRelease: "noble",
									Sources: []config.DistroSourceConfig{
										{
											Mirror:     "http://ubuntu-cloud.archive.canonical.com/ubuntu",
											Suites:     []string{"release", "updates"},
											Components: []string{"main"},
										},
									},
								},
							},
						},
					},
				},
			},
			Sets: map[string][]string{
				"networking": {"neutron", "nova"},
			},
			Upstream: &config.UpstreamConfig{
				Provider:         "opendev",
				ReleasesRepo:     "openstack/releases",
				RequirementsRepo: "openstack/requirements",
			},
		},
		ServerAddress: "https://watchtower.example.com",
		ServerToken:   "srv-token-abc",
		AuthToken:     "auth-token-xyz",
	}

	d := ConfigToDTO(original)
	// ServerAddress/ServerToken/AuthToken are not yet emitted by ConfigToDTO
	// (that is Task 14). Set them directly on the DTO to verify DTOToConfig
	// reads them correctly regardless.
	d.ServerAddress = original.ServerAddress
	d.ServerToken = original.ServerToken
	d.AuthToken = original.AuthToken
	got := DTOToConfig(d)

	// Top-level scalar fields
	if got.Launchpad.DefaultOwner != original.Launchpad.DefaultOwner {
		t.Errorf("Launchpad.DefaultOwner = %q, want %q", got.Launchpad.DefaultOwner, original.Launchpad.DefaultOwner)
	}
	if got.Launchpad.UseKeyring != original.Launchpad.UseKeyring {
		t.Errorf("Launchpad.UseKeyring = %v, want %v", got.Launchpad.UseKeyring, original.Launchpad.UseKeyring)
	}
	if len(got.Launchpad.Series) != len(original.Launchpad.Series) {
		t.Errorf("Launchpad.Series len = %d, want %d", len(got.Launchpad.Series), len(original.Launchpad.Series))
	}
	if got.Launchpad.DevelopmentFocus != original.Launchpad.DevelopmentFocus {
		t.Errorf("Launchpad.DevelopmentFocus = %q, want %q", got.Launchpad.DevelopmentFocus, original.Launchpad.DevelopmentFocus)
	}

	// GitHub
	if got.GitHub.ClientID != original.GitHub.ClientID {
		t.Errorf("GitHub.ClientID = %q, want %q", got.GitHub.ClientID, original.GitHub.ClientID)
	}

	// Gerrit
	if len(got.Gerrit.Hosts) != 1 || got.Gerrit.Hosts[0].URL != "https://review.example.com" {
		t.Errorf("Gerrit.Hosts = %+v, want one host with url https://review.example.com", got.Gerrit.Hosts)
	}

	// Build
	if got.Build.DefaultPrefix != original.Build.DefaultPrefix {
		t.Errorf("Build.DefaultPrefix = %q, want %q", got.Build.DefaultPrefix, original.Build.DefaultPrefix)
	}
	if got.Build.TimeoutMinutes != original.Build.TimeoutMinutes {
		t.Errorf("Build.TimeoutMinutes = %d, want %d", got.Build.TimeoutMinutes, original.Build.TimeoutMinutes)
	}
	if got.Build.ArtifactsDir != original.Build.ArtifactsDir {
		t.Errorf("Build.ArtifactsDir = %q, want %q", got.Build.ArtifactsDir, original.Build.ArtifactsDir)
	}

	// Releases
	if got.Releases.DefaultTargetProfile != original.Releases.DefaultTargetProfile {
		t.Errorf("Releases.DefaultTargetProfile = %q, want %q", got.Releases.DefaultTargetProfile, original.Releases.DefaultTargetProfile)
	}
	if len(got.Releases.TargetProfiles) != 1 {
		t.Errorf("Releases.TargetProfiles len = %d, want 1", len(got.Releases.TargetProfiles))
	}
	defaultProfile := got.Releases.TargetProfiles["default"]
	if len(defaultProfile.Include) != 1 {
		t.Errorf("default profile Include len = %d, want 1", len(defaultProfile.Include))
	} else {
		inc := defaultProfile.Include[0]
		if len(inc.BaseNames) != 1 || inc.BaseNames[0] != "ubuntu" {
			t.Errorf("Include[0].BaseNames = %v, want [ubuntu]", inc.BaseNames)
		}
		if inc.MinBaseChannel != "20.04/stable" {
			t.Errorf("Include[0].MinBaseChannel = %q, want 20.04/stable", inc.MinBaseChannel)
		}
	}
	if len(defaultProfile.Exclude) != 1 {
		t.Errorf("default profile Exclude len = %d, want 1", len(defaultProfile.Exclude))
	}

	// TUI
	if got.TUI.DefaultPane != "builds" {
		t.Errorf("TUI.DefaultPane = %q, want builds", got.TUI.DefaultPane)
	}
	if got.TUI.Panes.Builds == nil {
		t.Fatal("TUI.Panes.Builds is nil")
	}
	if got.TUI.Panes.Builds.Filters.Project != "myproject" {
		t.Errorf("TUI.Panes.Builds.Filters.Project = %q, want myproject", got.TUI.Panes.Builds.Filters.Project)
	}
	if got.TUI.Panes.Builds.Filters.Active == nil || !*got.TUI.Panes.Builds.Filters.Active {
		t.Error("TUI.Panes.Builds.Filters.Active should be true")
	}
	if got.TUI.Panes.Releases == nil {
		t.Fatal("TUI.Panes.Releases is nil")
	}
	if got.TUI.Panes.Releases.Filters.Risk != "stable" {
		t.Errorf("TUI.Panes.Releases.Filters.Risk = %q, want stable", got.TUI.Panes.Releases.Filters.Risk)
	}
	if got.TUI.Panes.Packages == nil {
		t.Fatal("TUI.Panes.Packages is nil")
	}
	if got.TUI.Panes.Packages.Mode != "excuses" {
		t.Errorf("TUI.Panes.Packages.Mode = %q, want excuses", got.TUI.Panes.Packages.Mode)
	}
	if got.TUI.Panes.Packages.Filters.Name != "neutron" {
		t.Errorf("TUI.Panes.Packages.Filters.Name = %q, want neutron", got.TUI.Panes.Packages.Filters.Name)
	}
	if got.TUI.Panes.Bugs == nil {
		t.Fatal("TUI.Panes.Bugs is nil")
	}
	if got.TUI.Panes.Bugs.Filters.Tag != "regression" {
		t.Errorf("TUI.Panes.Bugs.Filters.Tag = %q, want regression", got.TUI.Panes.Bugs.Filters.Tag)
	}
	if got.TUI.Panes.Reviews == nil {
		t.Fatal("TUI.Panes.Reviews is nil")
	}
	if got.TUI.Panes.Reviews.Filters.Forge != "github" {
		t.Errorf("TUI.Panes.Reviews.Filters.Forge = %q, want github", got.TUI.Panes.Reviews.Filters.Forge)
	}
	if got.TUI.Panes.Commits == nil {
		t.Fatal("TUI.Panes.Commits is nil")
	}
	if got.TUI.Panes.Commits.Mode != "diff" {
		t.Errorf("TUI.Panes.Commits.Mode = %q, want diff", got.TUI.Panes.Commits.Mode)
	}
	if got.TUI.Panes.Commits.Filters.BugID != "1234567" {
		t.Errorf("TUI.Panes.Commits.Filters.BugID = %q, want 1234567", got.TUI.Panes.Commits.Filters.BugID)
	}
	if got.TUI.Panes.Projects == nil {
		t.Fatal("TUI.Panes.Projects is nil")
	}
	if got.TUI.Panes.Projects.Filters.CodeForge != "github" {
		t.Errorf("TUI.Panes.Projects.Filters.CodeForge = %q, want github", got.TUI.Panes.Projects.Filters.CodeForge)
	}

	// BugGroups
	if len(got.BugGroups) != 1 {
		t.Errorf("BugGroups len = %d, want 1", len(got.BugGroups))
	}
	if got.BugGroups["openstack"].CommonProject != "openstack" {
		t.Errorf("BugGroups[openstack].CommonProject = %q, want openstack", got.BugGroups["openstack"].CommonProject)
	}

	// OTel
	if got.OTel.ServiceName != "watchtower" {
		t.Errorf("OTel.ServiceName = %q, want watchtower", got.OTel.ServiceName)
	}
	if got.OTel.ResourceAttributes["env"] != "prod" {
		t.Errorf("OTel.ResourceAttributes[env] = %q, want prod", got.OTel.ResourceAttributes["env"])
	}
	if !got.OTel.Metrics.Self.Enabled {
		t.Error("OTel.Metrics.Self.Enabled should be true")
	}
	if got.OTel.Metrics.Self.ListenAddr != ":9090" {
		t.Errorf("OTel.Metrics.Self.ListenAddr = %q, want :9090", got.OTel.Metrics.Self.ListenAddr)
	}
	if len(got.OTel.Metrics.Domain.LiveSystems) != 2 {
		t.Errorf("OTel.Metrics.Domain.LiveSystems len = %d, want 2", len(got.OTel.Metrics.Domain.LiveSystems))
	}
	if got.OTel.Metrics.Collectors.Auth.RefreshInterval != "1m" {
		t.Errorf("Collectors.Auth.RefreshInterval = %q, want 1m", got.OTel.Metrics.Collectors.Auth.RefreshInterval)
	}
	if got.OTel.Metrics.Collectors.Cache.RefreshInterval != "11m" {
		t.Errorf("Collectors.Cache.RefreshInterval = %q, want 11m", got.OTel.Metrics.Collectors.Cache.RefreshInterval)
	}
	if got.OTel.Traces.Endpoint != "localhost:4317" {
		t.Errorf("OTel.Traces.Endpoint = %q, want localhost:4317", got.OTel.Traces.Endpoint)
	}
	if got.OTel.Traces.SamplingRatio != 0.5 {
		t.Errorf("OTel.Traces.SamplingRatio = %v, want 0.5", got.OTel.Traces.SamplingRatio)
	}
	if got.OTel.Traces.Headers["x-token"] != "abc" {
		t.Errorf("OTel.Traces.Headers[x-token] = %q, want abc", got.OTel.Traces.Headers["x-token"])
	}
	if got.OTel.Logs.Protocol != "http" {
		t.Errorf("OTel.Logs.Protocol = %q, want http", got.OTel.Logs.Protocol)
	}

	// Projects
	if len(got.Projects) != 1 {
		t.Fatalf("Projects len = %d, want 1", len(got.Projects))
	}
	p := got.Projects[0]
	if p.Name != "neutron" {
		t.Errorf("Projects[0].Name = %q, want neutron", p.Name)
	}
	if p.Code.Forge != "github" {
		t.Errorf("Projects[0].Code.Forge = %q, want github", p.Code.Forge)
	}
	if p.Code.GitURL != "https://github.com/openstack/neutron.git" {
		t.Errorf("Projects[0].Code.GitURL = %q, want https://github.com/openstack/neutron.git", p.Code.GitURL)
	}
	if p.Build == nil {
		t.Fatal("Projects[0].Build is nil")
	}
	if p.Build.PrepareCommand != "make build" {
		t.Errorf("Projects[0].Build.PrepareCommand = %q, want make build", p.Build.PrepareCommand)
	}
	if len(p.Bugs) != 1 || p.Bugs[0].Group != "openstack" {
		t.Errorf("Projects[0].Bugs = %+v, want one bug with group openstack", p.Bugs)
	}
	if p.Release == nil {
		t.Fatal("Projects[0].Release is nil")
	}
	if len(p.Release.Tracks) != 2 {
		t.Errorf("Projects[0].Release.Tracks len = %d, want 2", len(p.Release.Tracks))
	}
	if p.Release.TrackMap["noble"] != "2.0" {
		t.Errorf("Projects[0].Release.TrackMap[noble] = %q, want 2.0", p.Release.TrackMap["noble"])
	}
	if len(p.Release.Branches) != 1 {
		t.Errorf("Projects[0].Release.Branches len = %d, want 1", len(p.Release.Branches))
	} else {
		b := p.Release.Branches[0]
		if b.Branch != "stable/2.0" {
			t.Errorf("Branch.Branch = %q, want stable/2.0", b.Branch)
		}
		if len(b.Risks) != 2 {
			t.Errorf("Branch.Risks len = %d, want 2", len(b.Risks))
		}
	}
	if p.Release.TargetProfileOverrides == nil {
		t.Fatal("Projects[0].Release.TargetProfileOverrides is nil")
	}
	if len(p.Release.TargetProfileOverrides.Include) != 1 {
		t.Errorf("TargetProfileOverrides.Include len = %d, want 1", len(p.Release.TargetProfileOverrides.Include))
	}
	if len(p.Release.SkipArtifacts) != 1 || p.Release.SkipArtifacts[0] != "old.charm" {
		t.Errorf("Release.SkipArtifacts = %v, want [old.charm]", p.Release.SkipArtifacts)
	}

	// Packages
	if len(got.Packages.Distros) != 1 {
		t.Errorf("Packages.Distros len = %d, want 1", len(got.Packages.Distros))
	}
	ubuntuDistro := got.Packages.Distros["ubuntu"]
	if ubuntuDistro.Mirror != "http://archive.ubuntu.com/ubuntu" {
		t.Errorf("Distros[ubuntu].Mirror = %q, want http://archive.ubuntu.com/ubuntu", ubuntuDistro.Mirror)
	}
	nobleRelease := ubuntuDistro.Releases["noble"]
	if len(nobleRelease.Suites) != 3 {
		t.Errorf("Releases[noble].Suites len = %d, want 3", len(nobleRelease.Suites))
	}
	ucaBackport := nobleRelease.Backports["uca"]
	if ucaBackport.ParentRelease != "noble" {
		t.Errorf("Backports[uca].ParentRelease = %q, want noble", ucaBackport.ParentRelease)
	}
	if len(ucaBackport.Sources) != 1 {
		t.Errorf("Backports[uca].Sources len = %d, want 1", len(ucaBackport.Sources))
	}
	if got.Packages.Sets["networking"][0] != "neutron" {
		t.Errorf("Packages.Sets[networking][0] = %q, want neutron", got.Packages.Sets["networking"][0])
	}
	if got.Packages.Upstream == nil {
		t.Fatal("Packages.Upstream is nil")
	}
	if got.Packages.Upstream.Provider != "opendev" {
		t.Errorf("Packages.Upstream.Provider = %q, want opendev", got.Packages.Upstream.Provider)
	}
	if got.Packages.Upstream.ReleasesRepo != "openstack/releases" {
		t.Errorf("Packages.Upstream.ReleasesRepo = %q, want openstack/releases", got.Packages.Upstream.ReleasesRepo)
	}

	// New fields
	if got.ServerAddress != "https://watchtower.example.com" {
		t.Errorf("ServerAddress = %q, want https://watchtower.example.com", got.ServerAddress)
	}
	if got.ServerToken != "srv-token-abc" {
		t.Errorf("ServerToken = %q, want srv-token-abc", got.ServerToken)
	}
	if got.AuthToken != "auth-token-xyz" {
		t.Errorf("AuthToken = %q, want auth-token-xyz", got.AuthToken)
	}
}
