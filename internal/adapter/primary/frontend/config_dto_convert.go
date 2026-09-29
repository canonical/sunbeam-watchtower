// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"github.com/gboutry/sunbeam-watchtower/internal/config"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
)

// DTOToConfig converts a public DTO back to an internal config. It is the
// reverse of ConfigToDTO.
func DTOToConfig(d *dto.Config) *config.Config {
	if d == nil {
		return nil
	}

	out := &config.Config{
		Launchpad: config.LaunchpadConfig{
			DefaultOwner:     d.Launchpad.DefaultOwner,
			UseKeyring:       d.Launchpad.UseKeyring,
			Series:           append([]string(nil), d.Launchpad.Series...),
			DevelopmentFocus: d.Launchpad.DevelopmentFocus,
		},
		GitHub: config.GitHubConfig{
			UseKeyring: d.GitHub.UseKeyring,
			ClientID:   d.GitHub.ClientID,
		},
		Build: config.BuildConfig{
			DefaultPrefix:  d.Build.DefaultPrefix,
			TimeoutMinutes: d.Build.TimeoutMinutes,
			ArtifactsDir:   d.Build.ArtifactsDir,
		},
		Releases: config.ReleasesConfig{
			DefaultTargetProfile: d.Releases.DefaultTargetProfile,
			TargetProfiles:       make(map[string]config.ReleaseTargetProfileConfig, len(d.Releases.TargetProfiles)),
		},
		TUI: config.TUIConfig{
			DefaultPane: d.TUI.DefaultPane,
		},
		BugGroups: make(map[string]config.BugGroupConfig, len(d.BugGroups)),
		OTel: config.OTelConfig{
			ServiceName:        d.OTel.ServiceName,
			ServiceNamespace:   d.OTel.ServiceNamespace,
			ResourceAttributes: copyStringMap(d.OTel.ResourceAttributes),
			Metrics: config.OTelMetricsConfig{
				Self: config.OTelMetricsListenerConfig{
					Enabled:                d.OTel.Metrics.Self.Enabled,
					ListenAddr:             d.OTel.Metrics.Self.ListenAddr,
					Path:                   d.OTel.Metrics.Self.Path,
					Runtime:                d.OTel.Metrics.Self.Runtime,
					Process:                d.OTel.Metrics.Self.Process,
					DefaultRefreshInterval: d.OTel.Metrics.Self.DefaultRefreshInterval,
				},
				Domain: config.OTelMetricsListenerConfig{
					Enabled:                d.OTel.Metrics.Domain.Enabled,
					ListenAddr:             d.OTel.Metrics.Domain.ListenAddr,
					Path:                   d.OTel.Metrics.Domain.Path,
					Runtime:                d.OTel.Metrics.Domain.Runtime,
					Process:                d.OTel.Metrics.Domain.Process,
					DefaultRefreshInterval: d.OTel.Metrics.Domain.DefaultRefreshInterval,
					LiveSystems:            append([]string(nil), d.OTel.Metrics.Domain.LiveSystems...),
				},
				Collectors: config.OTelDomainCollectorsConfig{
					Auth:       collectorDTOToConfig(d.OTel.Metrics.Collectors.Auth),
					Operations: collectorDTOToConfig(d.OTel.Metrics.Collectors.Operations),
					Projects:   collectorDTOToConfig(d.OTel.Metrics.Collectors.Projects),
					Builds:     collectorDTOToConfig(d.OTel.Metrics.Collectors.Builds),
					Releases:   collectorDTOToConfig(d.OTel.Metrics.Collectors.Releases),
					Reviews:    collectorDTOToConfig(d.OTel.Metrics.Collectors.Reviews),
					Commits:    collectorDTOToConfig(d.OTel.Metrics.Collectors.Commits),
					Bugs:       collectorDTOToConfig(d.OTel.Metrics.Collectors.Bugs),
					Packages:   collectorDTOToConfig(d.OTel.Metrics.Collectors.Packages),
					Excuses:    collectorDTOToConfig(d.OTel.Metrics.Collectors.Excuses),
					Cache:      collectorDTOToConfig(d.OTel.Metrics.Collectors.Cache),
				},
			},
			Traces: signalDTOToConfig(d.OTel.Traces),
			Logs:   signalDTOToConfig(d.OTel.Logs),
		},
		ServerAddress: d.ServerAddress,
		ServerToken:   d.ServerToken,
		AuthToken:     d.AuthToken,
	}

	out.Gerrit.Hosts = make([]config.GerritHost, len(d.Gerrit.Hosts))
	for i, host := range d.Gerrit.Hosts {
		out.Gerrit.Hosts[i] = config.GerritHost{URL: host.URL}
	}

	out.Projects = make([]config.ProjectConfig, len(d.Projects))
	for i, project := range d.Projects {
		outProject := config.ProjectConfig{
			Name:             project.Name,
			ArtifactType:     project.ArtifactType,
			Series:           append([]string(nil), project.Series...),
			DevelopmentFocus: project.DevelopmentFocus,
			Code: config.CodeConfig{
				Forge:   project.Code.Forge,
				Owner:   project.Code.Owner,
				Host:    project.Code.Host,
				Project: project.Code.Project,
				GitURL:  project.Code.GitURL,
			},
		}

		if project.Build != nil {
			outProject.Build = &config.ProjectBuildConfig{
				Owner:          project.Build.Owner,
				Artifacts:      append([]string(nil), project.Build.Artifacts...),
				PrepareCommand: project.Build.PrepareCommand,
			}
		}

		outProject.Bugs = make([]config.BugTrackerConfig, len(project.Bugs))
		for j, bug := range project.Bugs {
			outProject.Bugs[j] = config.BugTrackerConfig{
				Forge:   bug.Forge,
				Owner:   bug.Owner,
				Host:    bug.Host,
				Project: bug.Project,
				Group:   bug.Group,
			}
		}

		if project.Release != nil {
			outProject.Release = &config.ProjectReleaseConfig{
				Tracks:        append([]string(nil), project.Release.Tracks...),
				TrackMap:      make(map[string]string, len(project.Release.TrackMap)),
				Branches:      make([]config.ProjectReleaseBranchConfig, len(project.Release.Branches)),
				SkipArtifacts: append([]string(nil), project.Release.SkipArtifacts...),
				TargetProfile: project.Release.TargetProfile,
			}
			if project.Release.TargetProfileOverrides != nil {
				outProject.Release.TargetProfileOverrides = profileDTOToConfig(project.Release.TargetProfileOverrides)
			}
			for series, track := range project.Release.TrackMap {
				outProject.Release.TrackMap[series] = track
			}
			for j, branch := range project.Release.Branches {
				outProject.Release.Branches[j] = config.ProjectReleaseBranchConfig{
					Series: branch.Series,
					Track:  branch.Track,
					Branch: branch.Branch,
					Risks:  append([]string(nil), branch.Risks...),
				}
			}
		}

		out.Projects[i] = outProject
	}

	for name, profile := range d.Releases.TargetProfiles {
		out.Releases.TargetProfiles[name] = *profileDTOToConfig(&profile)
	}

	out.TUI.Panes = config.TUIPanesConfig{
		Builds:   tuiBuildsPaneDTOToConfig(d.TUI.Panes.Builds),
		Releases: tuiReleasesPaneDTOToConfig(d.TUI.Panes.Releases),
		Packages: tuiPackagesPaneDTOToConfig(d.TUI.Panes.Packages),
		Bugs:     tuiBugsPaneDTOToConfig(d.TUI.Panes.Bugs),
		Reviews:  tuiReviewsPaneDTOToConfig(d.TUI.Panes.Reviews),
		Commits:  tuiCommitsPaneDTOToConfig(d.TUI.Panes.Commits),
		Projects: tuiProjectsPaneDTOToConfig(d.TUI.Panes.Projects),
	}

	for name, group := range d.BugGroups {
		out.BugGroups[name] = config.BugGroupConfig{
			CommonProject: group.CommonProject,
		}
	}

	if len(d.Packages.Distros) > 0 {
		out.Packages.Distros = make(map[string]config.DistroConfig, len(d.Packages.Distros))
		for name, distro := range d.Packages.Distros {
			outDistro := config.DistroConfig{
				Mirror:     distro.Mirror,
				Components: append([]string(nil), distro.Components...),
				Releases:   make(map[string]config.ReleaseConfig, len(distro.Releases)),
			}
			for releaseName, release := range distro.Releases {
				outRelease := config.ReleaseConfig{
					Suites:    append([]string(nil), release.Suites...),
					Backports: make(map[string]config.BackportConfig, len(release.Backports)),
				}
				if len(release.Backports) > 0 {
					for backportName, backport := range release.Backports {
						outBackport := config.BackportConfig{
							ParentRelease: backport.ParentRelease,
							Sources:       make([]config.DistroSourceConfig, len(backport.Sources)),
						}
						for i, source := range backport.Sources {
							outBackport.Sources[i] = config.DistroSourceConfig{
								Mirror:     source.Mirror,
								Suites:     append([]string(nil), source.Suites...),
								Components: append([]string(nil), source.Components...),
							}
						}
						outRelease.Backports[backportName] = outBackport
					}
				}
				outDistro.Releases[releaseName] = outRelease
			}
			out.Packages.Distros[name] = outDistro
		}
	}

	if len(d.Packages.Sets) > 0 {
		out.Packages.Sets = make(map[string][]string, len(d.Packages.Sets))
		for setName, packages := range d.Packages.Sets {
			out.Packages.Sets[setName] = append([]string(nil), packages...)
		}
	}
	if len(d.Packages.LaunchpadSets) > 0 {
		out.Packages.LaunchpadSets = make(map[string]config.LaunchpadSetConfig, len(d.Packages.LaunchpadSets))
		for name, set := range d.Packages.LaunchpadSets {
			out.Packages.LaunchpadSets[name] = config.LaunchpadSetConfig{Series: set.Series}
		}
	}

	if d.Packages.Upstream != nil {
		out.Packages.Upstream = &config.UpstreamConfig{
			Provider:         d.Packages.Upstream.Provider,
			ReleasesRepo:     d.Packages.Upstream.ReleasesRepo,
			RequirementsRepo: d.Packages.Upstream.RequirementsRepo,
		}
	}

	return out
}

func profileDTOToConfig(profile *dto.ReleaseTargetProfileConfig) *config.ReleaseTargetProfileConfig {
	if profile == nil {
		return nil
	}
	out := &config.ReleaseTargetProfileConfig{
		Include: make([]config.ReleaseTargetMatcherConfig, len(profile.Include)),
		Exclude: make([]config.ReleaseTargetMatcherConfig, len(profile.Exclude)),
	}
	for i, matcher := range profile.Include {
		out.Include[i] = matcherDTOToConfig(matcher)
	}
	for i, matcher := range profile.Exclude {
		out.Exclude[i] = matcherDTOToConfig(matcher)
	}
	return out
}

func matcherDTOToConfig(matcher dto.ReleaseTargetMatcherConfig) config.ReleaseTargetMatcherConfig {
	return config.ReleaseTargetMatcherConfig{
		BaseNames:      append([]string(nil), matcher.BaseNames...),
		BaseChannels:   append([]string(nil), matcher.BaseChannels...),
		MinBaseChannel: matcher.MinBaseChannel,
		Architectures:  append([]string(nil), matcher.Architectures...),
	}
}

func collectorDTOToConfig(d dto.OTelCollectorConfig) config.OTelCollectorConfig {
	return config.OTelCollectorConfig{
		Enabled:         d.Enabled,
		RefreshInterval: d.RefreshInterval,
	}
}

func signalDTOToConfig(d dto.OTelSignalConfig) config.OTelSignalConfig {
	return config.OTelSignalConfig{
		Enabled:       d.Enabled,
		Endpoint:      d.Endpoint,
		Protocol:      d.Protocol,
		Insecure:      d.Insecure,
		Headers:       copyStringMap(d.Headers),
		SamplingRatio: d.SamplingRatio,
		MinLevel:      d.MinLevel,
		MirrorStderr:  d.MirrorStderr,
	}
}

func tuiBuildsPaneDTOToConfig(d *dto.TUIBuildsPaneConfig) *config.TUIBuildsPaneConfig {
	if d == nil {
		return nil
	}
	return &config.TUIBuildsPaneConfig{
		Filters: config.TUIBuildsFiltersConfig{
			Project: d.Filters.Project,
			State:   d.Filters.State,
			Active:  d.Filters.Active,
			Source:  d.Filters.Source,
		},
	}
}

func tuiReleasesPaneDTOToConfig(d *dto.TUIReleasesPaneConfig) *config.TUIReleasesPaneConfig {
	if d == nil {
		return nil
	}
	return &config.TUIReleasesPaneConfig{
		Filters: config.TUIReleasesFiltersConfig{
			Project:       d.Filters.Project,
			ArtifactType:  d.Filters.ArtifactType,
			Risk:          d.Filters.Risk,
			Track:         d.Filters.Track,
			Branch:        d.Filters.Branch,
			TargetProfile: d.Filters.TargetProfile,
			AllTargets:    d.Filters.AllTargets,
		},
	}
}

func tuiPackagesPaneDTOToConfig(d *dto.TUIPackagesPaneConfig) *config.TUIPackagesPaneConfig {
	if d == nil {
		return nil
	}
	return &config.TUIPackagesPaneConfig{
		Mode: d.Mode,
		Filters: config.TUIPackagesFiltersConfig{
			Set:             d.Filters.Set,
			Distro:          d.Filters.Distro,
			Release:         d.Filters.Release,
			Suite:           d.Filters.Suite,
			Component:       d.Filters.Component,
			Backport:        d.Filters.Backport,
			Merge:           d.Filters.Merge,
			UpstreamRelease: d.Filters.UpstreamRelease,
			BehindUpstream:  d.Filters.BehindUpstream,
			OnlyIn:          d.Filters.OnlyIn,
			Constraints:     d.Filters.Constraints,
			Tracker:         d.Filters.Tracker,
			Name:            d.Filters.Name,
			Team:            d.Filters.Team,
			FTBFS:           d.Filters.FTBFS,
			Autopkgtest:     d.Filters.Autopkgtest,
			BlockedBy:       d.Filters.BlockedBy,
			Bugged:          d.Filters.Bugged,
			MinAge:          d.Filters.MinAge,
			MaxAge:          d.Filters.MaxAge,
			Limit:           d.Filters.Limit,
			Reverse:         d.Filters.Reverse,
		},
	}
}

func tuiBugsPaneDTOToConfig(d *dto.TUIBugsPaneConfig) *config.TUIBugsPaneConfig {
	if d == nil {
		return nil
	}
	return &config.TUIBugsPaneConfig{
		Filters: config.TUIBugsFiltersConfig{
			Project:    d.Filters.Project,
			Status:     d.Filters.Status,
			Importance: d.Filters.Importance,
			Assignee:   d.Filters.Assignee,
			Tag:        d.Filters.Tag,
			Since:      d.Filters.Since,
			Merge:      d.Filters.Merge,
		},
	}
}

func tuiReviewsPaneDTOToConfig(d *dto.TUIReviewsPaneConfig) *config.TUIReviewsPaneConfig {
	if d == nil {
		return nil
	}
	return &config.TUIReviewsPaneConfig{
		Filters: config.TUIReviewsFiltersConfig{
			Project: d.Filters.Project,
			Forge:   d.Filters.Forge,
			State:   d.Filters.State,
			Author:  d.Filters.Author,
			Since:   d.Filters.Since,
		},
	}
}

func tuiCommitsPaneDTOToConfig(d *dto.TUICommitsPaneConfig) *config.TUICommitsPaneConfig {
	if d == nil {
		return nil
	}
	return &config.TUICommitsPaneConfig{
		Mode: d.Mode,
		Filters: config.TUICommitsFiltersConfig{
			Project:    d.Filters.Project,
			Forge:      d.Filters.Forge,
			Branch:     d.Filters.Branch,
			Author:     d.Filters.Author,
			IncludeMRs: d.Filters.IncludeMRs,
			BugID:      d.Filters.BugID,
		},
	}
}

func tuiProjectsPaneDTOToConfig(d *dto.TUIProjectsPaneConfig) *config.TUIProjectsPaneConfig {
	if d == nil {
		return nil
	}
	return &config.TUIProjectsPaneConfig{
		Filters: config.TUIProjectsFiltersConfig{
			Name:         d.Filters.Name,
			ArtifactType: d.Filters.ArtifactType,
			CodeForge:    d.Filters.CodeForge,
			BugForge:     d.Filters.BugForge,
			HasBuild:     d.Filters.HasBuild,
			HasRelease:   d.Filters.HasRelease,
		},
	}
}
