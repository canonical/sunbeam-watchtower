// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"

	"github.com/gboutry/sunbeam-watchtower/internal/app"
	"github.com/gboutry/sunbeam-watchtower/internal/config"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
)

// ConfigServerWorkflow exposes reusable server-side config workflows for the HTTP API.
type ConfigServerWorkflow struct {
	application *app.App
}

// NewConfigServerWorkflow creates a server-side config workflow.
func NewConfigServerWorkflow(application *app.App) *ConfigServerWorkflow {
	return &ConfigServerWorkflow{application: application}
}

// Show returns the loaded configuration as a public DTO.
func (w *ConfigServerWorkflow) Show(context.Context) (*dto.Config, error) {
	if w.application == nil || w.application.GetConfig() == nil {
		return nil, errors.New("no configuration loaded")
	}
	return ConfigToDTO(w.application.GetConfig()), nil
}

// Reload reloads the configuration from the file it was originally loaded from.
func (w *ConfigServerWorkflow) Reload(context.Context) error {
	if w.application == nil {
		return errors.New("no application available")
	}
	return w.application.ReloadConfig(w.application.ConfigPath())
}

// ConfigToDTO converts an internal config to a public DTO. Exported so the
// TUI can use the locally-loaded config without going through the server API.
func ConfigToDTO(cfg *config.Config) *dto.Config {
	if cfg == nil {
		return nil
	}

	out := &dto.Config{
		Launchpad: dto.LaunchpadConfig{
			DefaultOwner:     cfg.Launchpad.DefaultOwner,
			UseKeyring:       cfg.Launchpad.UseKeyring,
			Series:           append([]string(nil), cfg.Launchpad.Series...),
			DevelopmentFocus: cfg.Launchpad.DevelopmentFocus,
		},
		GitHub: dto.GitHubConfig{
			UseKeyring: cfg.GitHub.UseKeyring,
			ClientID:   cfg.GitHub.ClientID,
		},
		Build: dto.BuildConfig{
			DefaultPrefix:  cfg.Build.DefaultPrefix,
			TimeoutMinutes: cfg.Build.TimeoutMinutes,
			ArtifactsDir:   cfg.Build.ArtifactsDir,
		},
		Releases: dto.ReleasesConfig{
			DefaultTargetProfile: cfg.Releases.DefaultTargetProfile,
			TargetProfiles:       make(map[string]dto.ReleaseTargetProfileConfig, len(cfg.Releases.TargetProfiles)),
		},
		TUI: dto.TUIConfig{
			DefaultPane: cfg.TUI.DefaultPane,
		},
		BugGroups: make(map[string]dto.BugGroupConfig, len(cfg.BugGroups)),
		OTel: dto.OTelConfig{
			ServiceName:        cfg.OTel.ServiceName,
			ServiceNamespace:   cfg.OTel.ServiceNamespace,
			ResourceAttributes: copyStringMap(cfg.OTel.ResourceAttributes),
			Metrics: dto.OTelMetricsConfig{
				Self: dto.OTelMetricsListenerConfig{
					Enabled:                cfg.OTel.Metrics.Self.Enabled,
					ListenAddr:             cfg.OTel.Metrics.Self.ListenAddr,
					Path:                   cfg.OTel.Metrics.Self.Path,
					Runtime:                cfg.OTel.Metrics.Self.Runtime,
					Process:                cfg.OTel.Metrics.Self.Process,
					DefaultRefreshInterval: cfg.OTel.Metrics.Self.DefaultRefreshInterval,
				},
				Domain: dto.OTelMetricsListenerConfig{
					Enabled:                cfg.OTel.Metrics.Domain.Enabled,
					ListenAddr:             cfg.OTel.Metrics.Domain.ListenAddr,
					Path:                   cfg.OTel.Metrics.Domain.Path,
					Runtime:                cfg.OTel.Metrics.Domain.Runtime,
					Process:                cfg.OTel.Metrics.Domain.Process,
					DefaultRefreshInterval: cfg.OTel.Metrics.Domain.DefaultRefreshInterval,
					LiveSystems:            append([]string(nil), cfg.OTel.Metrics.Domain.LiveSystems...),
				},
				Collectors: dto.OTelDomainCollectorsConfig{
					Auth:       collectorConfigToDTO(cfg.OTel.Metrics.Collectors.Auth),
					Operations: collectorConfigToDTO(cfg.OTel.Metrics.Collectors.Operations),
					Projects:   collectorConfigToDTO(cfg.OTel.Metrics.Collectors.Projects),
					Builds:     collectorConfigToDTO(cfg.OTel.Metrics.Collectors.Builds),
					Releases:   collectorConfigToDTO(cfg.OTel.Metrics.Collectors.Releases),
					Reviews:    collectorConfigToDTO(cfg.OTel.Metrics.Collectors.Reviews),
					Commits:    collectorConfigToDTO(cfg.OTel.Metrics.Collectors.Commits),
					Bugs:       collectorConfigToDTO(cfg.OTel.Metrics.Collectors.Bugs),
					Packages:   collectorConfigToDTO(cfg.OTel.Metrics.Collectors.Packages),
					Excuses:    collectorConfigToDTO(cfg.OTel.Metrics.Collectors.Excuses),
					Cache:      collectorConfigToDTO(cfg.OTel.Metrics.Collectors.Cache),
				},
			},
			Traces: signalConfigToDTO(cfg.OTel.Traces),
			Logs:   signalConfigToDTO(cfg.OTel.Logs),
		},
		ServerAddress: cfg.ServerAddress,
		// ServerToken and AuthToken are intentionally omitted — secrets must
		// not be exposed via the config API endpoint.
	}

	out.Gerrit.Hosts = make([]dto.GerritHost, len(cfg.Gerrit.Hosts))
	for i, host := range cfg.Gerrit.Hosts {
		out.Gerrit.Hosts[i] = dto.GerritHost{URL: host.URL}
	}

	out.Projects = make([]dto.ProjectConfig, len(cfg.Projects))
	for i, project := range cfg.Projects {
		outProject := dto.ProjectConfig{
			Name:             project.Name,
			ArtifactType:     project.ArtifactType,
			Series:           append([]string(nil), project.Series...),
			DevelopmentFocus: project.DevelopmentFocus,
			Code: dto.CodeConfig{
				Forge:   project.Code.Forge,
				Owner:   project.Code.Owner,
				Host:    project.Code.Host,
				Project: project.Code.Project,
				GitURL:  project.Code.GitURL,
			},
		}

		if project.Build != nil {
			outProject.Build = &dto.ProjectBuildConfig{
				Owner:          project.Build.Owner,
				Artifacts:      append([]string(nil), project.Build.Artifacts...),
				PrepareCommand: project.Build.PrepareCommand,
			}
		}

		outProject.Bugs = make([]dto.BugTrackerConfig, len(project.Bugs))
		for j, bug := range project.Bugs {
			outProject.Bugs[j] = dto.BugTrackerConfig{
				Forge:   bug.Forge,
				Owner:   bug.Owner,
				Host:    bug.Host,
				Project: bug.Project,
				Group:   bug.Group,
			}
		}
		if project.Release != nil {
			outProject.Release = &dto.ProjectReleaseConfig{
				Tracks:        append([]string(nil), project.Release.Tracks...),
				TrackMap:      make(map[string]string, len(project.Release.TrackMap)),
				Branches:      make([]dto.ProjectReleaseBranchConfig, len(project.Release.Branches)),
				SkipArtifacts: append([]string(nil), project.Release.SkipArtifacts...),
				TargetProfile: project.Release.TargetProfile,
			}
			if project.Release.TargetProfileOverrides != nil {
				outProject.Release.TargetProfileOverrides = profileConfigToDTO(project.Release.TargetProfileOverrides)
			}
			for series, track := range project.Release.TrackMap {
				outProject.Release.TrackMap[series] = track
			}
			for j, branch := range project.Release.Branches {
				outProject.Release.Branches[j] = dto.ProjectReleaseBranchConfig{
					Series: branch.Series,
					Track:  branch.Track,
					Branch: branch.Branch,
					Risks:  append([]string(nil), branch.Risks...),
				}
			}
		}

		out.Projects[i] = outProject
	}

	for name, profile := range cfg.Releases.TargetProfiles {
		out.Releases.TargetProfiles[name] = *profileConfigToDTO(&profile)
	}
	out.TUI.Panes = dto.TUIPanesConfig{
		Builds:   tuiBuildsPaneConfigToDTO(cfg.TUI.Panes.Builds),
		Releases: tuiReleasesPaneConfigToDTO(cfg.TUI.Panes.Releases),
		Packages: tuiPackagesPaneConfigToDTO(cfg.TUI.Panes.Packages),
		Bugs:     tuiBugsPaneConfigToDTO(cfg.TUI.Panes.Bugs),
		Reviews:  tuiReviewsPaneConfigToDTO(cfg.TUI.Panes.Reviews),
		Commits:  tuiCommitsPaneConfigToDTO(cfg.TUI.Panes.Commits),
		Projects: tuiProjectsPaneConfigToDTO(cfg.TUI.Panes.Projects),
	}
	for name, group := range cfg.BugGroups {
		out.BugGroups[name] = dto.BugGroupConfig{
			CommonProject: group.CommonProject,
		}
	}

	if len(cfg.Packages.Distros) > 0 {
		out.Packages.Distros = make(map[string]dto.DistroConfig, len(cfg.Packages.Distros))
		for name, distro := range cfg.Packages.Distros {
			outDistro := dto.DistroConfig{
				Mirror:     distro.Mirror,
				Components: append([]string(nil), distro.Components...),
				Releases:   make(map[string]dto.ReleaseConfig, len(distro.Releases)),
			}
			for releaseName, release := range distro.Releases {
				outRelease := dto.ReleaseConfig{
					Suites:    append([]string(nil), release.Suites...),
					Backports: make(map[string]dto.BackportConfig, len(release.Backports)),
				}
				if len(release.Backports) > 0 {
					for backportName, backport := range release.Backports {
						outBackport := dto.BackportConfig{
							ParentRelease: backport.ParentRelease,
							Sources:       make([]dto.DistroSourceConfig, len(backport.Sources)),
						}
						for i, source := range backport.Sources {
							outBackport.Sources[i] = dto.DistroSourceConfig{
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

	if len(cfg.Packages.Sets) > 0 {
		out.Packages.Sets = make(map[string][]string, len(cfg.Packages.Sets))
		for setName, packages := range cfg.Packages.Sets {
			out.Packages.Sets[setName] = append([]string(nil), packages...)
		}
	}
	if len(cfg.Packages.LaunchpadSets) > 0 {
		out.Packages.LaunchpadSets = make(map[string]dto.LaunchpadSetConfig, len(cfg.Packages.LaunchpadSets))
		for name, set := range cfg.Packages.LaunchpadSets {
			out.Packages.LaunchpadSets[name] = dto.LaunchpadSetConfig{Series: set.Series}
		}
	}

	if cfg.Packages.Upstream != nil {
		out.Packages.Upstream = &dto.UpstreamConfig{
			Provider:         cfg.Packages.Upstream.Provider,
			ReleasesRepo:     cfg.Packages.Upstream.ReleasesRepo,
			RequirementsRepo: cfg.Packages.Upstream.RequirementsRepo,
		}
	}

	return out
}

func profileConfigToDTO(profile *config.ReleaseTargetProfileConfig) *dto.ReleaseTargetProfileConfig {
	if profile == nil {
		return nil
	}
	out := &dto.ReleaseTargetProfileConfig{
		Include: make([]dto.ReleaseTargetMatcherConfig, len(profile.Include)),
		Exclude: make([]dto.ReleaseTargetMatcherConfig, len(profile.Exclude)),
	}
	for i, matcher := range profile.Include {
		out.Include[i] = matcherConfigToDTO(matcher)
	}
	for i, matcher := range profile.Exclude {
		out.Exclude[i] = matcherConfigToDTO(matcher)
	}
	return out
}

func matcherConfigToDTO(matcher config.ReleaseTargetMatcherConfig) dto.ReleaseTargetMatcherConfig {
	return dto.ReleaseTargetMatcherConfig{
		BaseNames:      append([]string(nil), matcher.BaseNames...),
		BaseChannels:   append([]string(nil), matcher.BaseChannels...),
		MinBaseChannel: matcher.MinBaseChannel,
		Architectures:  append([]string(nil), matcher.Architectures...),
	}
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func collectorConfigToDTO(cfg config.OTelCollectorConfig) dto.OTelCollectorConfig {
	return dto.OTelCollectorConfig{
		Enabled:         cfg.Enabled,
		RefreshInterval: cfg.RefreshInterval,
	}
}

func signalConfigToDTO(cfg config.OTelSignalConfig) dto.OTelSignalConfig {
	return dto.OTelSignalConfig{
		Enabled:       cfg.Enabled,
		Endpoint:      cfg.Endpoint,
		Protocol:      cfg.Protocol,
		Insecure:      cfg.Insecure,
		Headers:       copyStringMap(cfg.Headers),
		SamplingRatio: cfg.SamplingRatio,
		MinLevel:      cfg.MinLevel,
		MirrorStderr:  cfg.MirrorStderr,
	}
}

func tuiBuildsPaneConfigToDTO(cfg *config.TUIBuildsPaneConfig) *dto.TUIBuildsPaneConfig {
	if cfg == nil {
		return nil
	}
	return &dto.TUIBuildsPaneConfig{
		Filters: dto.TUIBuildsFiltersConfig{
			Project: cfg.Filters.Project,
			State:   cfg.Filters.State,
			Active:  cfg.Filters.Active,
			Source:  cfg.Filters.Source,
		},
	}
}

func tuiReleasesPaneConfigToDTO(cfg *config.TUIReleasesPaneConfig) *dto.TUIReleasesPaneConfig {
	if cfg == nil {
		return nil
	}
	return &dto.TUIReleasesPaneConfig{
		Filters: dto.TUIReleasesFiltersConfig{
			Project:       cfg.Filters.Project,
			ArtifactType:  cfg.Filters.ArtifactType,
			Risk:          cfg.Filters.Risk,
			Track:         cfg.Filters.Track,
			Branch:        cfg.Filters.Branch,
			TargetProfile: cfg.Filters.TargetProfile,
			AllTargets:    cfg.Filters.AllTargets,
		},
	}
}

func tuiPackagesPaneConfigToDTO(cfg *config.TUIPackagesPaneConfig) *dto.TUIPackagesPaneConfig {
	if cfg == nil {
		return nil
	}
	return &dto.TUIPackagesPaneConfig{
		Mode: cfg.Mode,
		Filters: dto.TUIPackagesFiltersConfig{
			Set:             cfg.Filters.Set,
			Distro:          cfg.Filters.Distro,
			Release:         cfg.Filters.Release,
			Suite:           cfg.Filters.Suite,
			Component:       cfg.Filters.Component,
			Backport:        cfg.Filters.Backport,
			Merge:           cfg.Filters.Merge,
			UpstreamRelease: cfg.Filters.UpstreamRelease,
			BehindUpstream:  cfg.Filters.BehindUpstream,
			OnlyIn:          cfg.Filters.OnlyIn,
			Constraints:     cfg.Filters.Constraints,
			Tracker:         cfg.Filters.Tracker,
			Name:            cfg.Filters.Name,
			Team:            cfg.Filters.Team,
			FTBFS:           cfg.Filters.FTBFS,
			Autopkgtest:     cfg.Filters.Autopkgtest,
			BlockedBy:       cfg.Filters.BlockedBy,
			Bugged:          cfg.Filters.Bugged,
			MinAge:          cfg.Filters.MinAge,
			MaxAge:          cfg.Filters.MaxAge,
			Limit:           cfg.Filters.Limit,
			Reverse:         cfg.Filters.Reverse,
		},
	}
}

func tuiBugsPaneConfigToDTO(cfg *config.TUIBugsPaneConfig) *dto.TUIBugsPaneConfig {
	if cfg == nil {
		return nil
	}
	return &dto.TUIBugsPaneConfig{
		Filters: dto.TUIBugsFiltersConfig{
			Project:    cfg.Filters.Project,
			Status:     cfg.Filters.Status,
			Importance: cfg.Filters.Importance,
			Assignee:   cfg.Filters.Assignee,
			Tag:        cfg.Filters.Tag,
			Since:      cfg.Filters.Since,
			Merge:      cfg.Filters.Merge,
		},
	}
}

func tuiReviewsPaneConfigToDTO(cfg *config.TUIReviewsPaneConfig) *dto.TUIReviewsPaneConfig {
	if cfg == nil {
		return nil
	}
	return &dto.TUIReviewsPaneConfig{
		Filters: dto.TUIReviewsFiltersConfig{
			Project: cfg.Filters.Project,
			Forge:   cfg.Filters.Forge,
			State:   cfg.Filters.State,
			Author:  cfg.Filters.Author,
			Since:   cfg.Filters.Since,
		},
	}
}

func tuiCommitsPaneConfigToDTO(cfg *config.TUICommitsPaneConfig) *dto.TUICommitsPaneConfig {
	if cfg == nil {
		return nil
	}
	return &dto.TUICommitsPaneConfig{
		Mode: cfg.Mode,
		Filters: dto.TUICommitsFiltersConfig{
			Project:    cfg.Filters.Project,
			Forge:      cfg.Filters.Forge,
			Branch:     cfg.Filters.Branch,
			Author:     cfg.Filters.Author,
			IncludeMRs: cfg.Filters.IncludeMRs,
			BugID:      cfg.Filters.BugID,
		},
	}
}

func tuiProjectsPaneConfigToDTO(cfg *config.TUIProjectsPaneConfig) *dto.TUIProjectsPaneConfig {
	if cfg == nil {
		return nil
	}
	return &dto.TUIProjectsPaneConfig{
		Filters: dto.TUIProjectsFiltersConfig{
			Name:         cfg.Filters.Name,
			ArtifactType: cfg.Filters.ArtifactType,
			CodeForge:    cfg.Filters.CodeForge,
			BugForge:     cfg.Filters.BugForge,
			HasBuild:     cfg.Filters.HasBuild,
			HasRelease:   cfg.Filters.HasRelease,
		},
	}
}
