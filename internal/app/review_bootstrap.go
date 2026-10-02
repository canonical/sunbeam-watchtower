// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/google/go-github/v68/github"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/bugcache"
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/reviewcache"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/bug"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/review"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

// ReviewCacheSyncResult reports the result of one review-cache refresh.
type ReviewCacheSyncResult struct {
	ProjectsSynced  int
	SummariesSynced int
	DetailsSynced   int
	Warnings        []string
}

// NewLaunchpadClient creates an LP client with credentials from the credential store.
// Returns nil if no credentials are available.
func NewLaunchpadClient(store port.LaunchpadCredentialStore, logger *slog.Logger) *lp.Client {
	return newLaunchpadClient(store, logger, nil)
}

func newLaunchpadClient(store port.LaunchpadCredentialStore, logger *slog.Logger, httpClient *http.Client) *lp.Client {
	record, err := store.Load(context.Background())
	if err != nil {
		logger.Warn("failed to load LP credentials", "error", err)
		return nil
	}
	if record == nil || record.Credentials == nil {
		return nil
	}
	return lp.NewClient(record.Credentials, logger, httpClient)
}

func newGitHubClient(store port.GitHubCredentialStore, logger *slog.Logger, httpClient *http.Client) *github.Client {
	client := github.NewClient(httpClient)
	record, err := store.Load(context.Background())
	if err != nil {
		logger.Warn("failed to load GitHub credentials", "error", err)
		return client
	}
	if record == nil || record.Credentials == nil || record.Credentials.AccessToken == "" {
		return client
	}
	return client.WithAuthToken(record.Credentials.AccessToken)
}

// NewLaunchpadForge creates a LaunchpadForge client, or nil if no auth is available.
func NewLaunchpadForge(store port.LaunchpadCredentialStore, logger *slog.Logger) *forge.LaunchpadForge {
	client := newLaunchpadClient(store, logger, nil)
	if client == nil {
		return nil
	}
	return forge.NewLaunchpadForge(client)
}

// ForgeTypeFromConfig maps a config forge name string to a forge.ForgeType.
func ForgeTypeFromConfig(forgeName string) forge.ForgeType {
	switch forgeName {
	case "github":
		return forge.ForgeGitHub
	case "launchpad":
		return forge.ForgeLaunchpad
	case "gerrit":
		return forge.ForgeGerrit
	default:
		return forge.ForgeGitHub
	}
}

// MRRefSpecs returns the additional git refspecs needed to fetch MR refs for a given forge.
func MRRefSpecs(forgeName string) []string {
	switch forgeName {
	case "github":
		return []string{"+refs/pull/*/head:refs/pull/*/head"}
	case "gerrit":
		return []string{"+refs/changes/*:refs/changes/*"}
	default:
		return nil
	}
}

// MRGitRef computes the git ref path for a merge request ID on the given forge.
func MRGitRef(forgeName string, mrID string) string {
	switch forgeName {
	case "github":
		return fmt.Sprintf("refs/pull/%s/head", strings.TrimPrefix(mrID, "#"))
	default:
		return ""
	}
}

// ConvertToMRMetadata converts forge MergeRequests to dto.MRMetadata entries.
func ConvertToMRMetadata(mrs []forge.MergeRequest, forgeName string) []dto.MRMetadata {
	result := make([]dto.MRMetadata, 0, len(mrs))
	for _, mr := range mrs {
		result = append(result, dto.MRMetadata{
			ID:     mr.ID,
			State:  mr.State,
			URL:    mr.URL,
			GitRef: MRGitRef(forgeName, mr.ID),
		})
	}
	return result
}

// BuildForgeClients creates forge clients from config, caching one per forge type/host.
func (a *App) BuildForgeClients() (map[string]review.ProjectForge, error) {
	cfg := a.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("no configuration loaded")
	}

	a.Logger.Debug("building forge clients", "project_count", len(cfg.Projects))

	result := make(map[string]review.ProjectForge, len(cfg.Projects))

	var ghClient *forge.GitHubForge
	gerritClients := make(map[string]*forge.GerritForge)
	var lpClient *forge.LaunchpadForge

	for _, proj := range cfg.Projects {
		var pf review.ProjectForge
		code := proj.Code

		a.Logger.Debug("configuring forge client", "project", proj.Name, "forge", code.Forge)

		switch code.Forge {
		case "github":
			if ghClient == nil {
				ghClient = forge.NewGitHubForge(newGitHubClient(a.GitHubCredentialStore(), a.Logger, a.upstreamHTTPClient("github", 30*time.Second)))
			}
			pf = review.ProjectForge{
				Forge:     ghClient,
				ProjectID: code.Owner + "/" + code.Project,
			}

		case "gerrit":
			gc, ok := gerritClients[code.Host]
			if !ok {
				client, err := gerrit.NewClient(context.Background(), code.Host, a.upstreamHTTPClient("gerrit", 30*time.Second))
				if err != nil {
					return nil, fmt.Errorf("creating Gerrit client for %s: %w", code.Host, err)
				}
				gc = forge.NewGerritForge(client, code.Host)
				gerritClients[code.Host] = gc
			}
			pf = review.ProjectForge{
				Forge:     gc,
				ProjectID: code.Project,
			}

		case "launchpad":
			if lpClient == nil {
				raw := newLaunchpadClient(a.LaunchpadCredentialStore(), a.Logger, a.upstreamHTTPClient("launchpad", 2*time.Minute))
				if raw == nil {
					a.Logger.Warn("skipping Launchpad project (no auth configured)", "project", proj.Name)
					continue
				}
				lpClient = forge.NewLaunchpadForge(raw)
			}
			pf = review.ProjectForge{
				Forge:     lpClient,
				ProjectID: code.Project,
			}

		default:
			return nil, fmt.Errorf("unknown forge type %q for project %s", code.Forge, proj.Name)
		}

		result[proj.Name] = pf
	}

	return result, nil
}

// BuildReviewProjects returns cache-backed review clients keyed by config project name.
func (a *App) BuildReviewProjects() (map[string]review.ProjectForge, error) {
	live, err := a.BuildForgeClients()
	if err != nil {
		return nil, err
	}
	cache, err := a.ReviewCache()
	if err != nil {
		return nil, err
	}
	result := make(map[string]review.ProjectForge, len(live))
	for project, pf := range live {
		result[project] = review.ProjectForge{
			Forge:     reviewcache.NewCachedForge(pf.Forge, cache, project, a.Logger),
			ProjectID: pf.ProjectID,
		}
	}
	return result, nil
}

// SyncReviewCache refreshes cached review summaries and details for configured projects.
func (a *App) SyncReviewCache(ctx context.Context, projects []string, since *time.Time) (*ReviewCacheSyncResult, error) {
	liveClients, err := a.BuildForgeClients()
	if err != nil {
		return nil, err
	}
	cache, err := a.ReviewCache()
	if err != nil {
		return nil, err
	}

	result := &ReviewCacheSyncResult{}
	selected := stringSet(projects)
	for name, pf := range liveClients {
		if len(selected) > 0 && !selected[name] {
			continue
		}
		cachedForge := reviewcache.NewCachedForge(pf.Forge, cache, name, a.Logger)
		syncResult, err := cachedForge.Sync(ctx, pf.ProjectID, since)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		result.ProjectsSynced++
		result.SummariesSynced += syncResult.Summaries
		result.DetailsSynced += syncResult.Details
		result.Warnings = append(result.Warnings, syncResult.Warnings...)
	}

	return result, nil
}

// BuildBugTrackers creates bug tracker clients from config, deduplicating by (forge, project).
func (a *App) BuildBugTrackers() (map[string]bug.ProjectBugTracker, map[string][]bug.ProjectBinding, error) {
	cfg := a.GetConfig()
	if cfg == nil {
		return nil, nil, fmt.Errorf("no configuration loaded")
	}

	trackers := make(map[string]bug.ProjectBugTracker)
	bindings := make(map[string][]bug.ProjectBinding)

	var lpBugTracker *forge.LaunchpadBugTracker

	cache, cacheErr := a.BugCache()
	if cacheErr != nil {
		a.Logger.Warn("bug cache unavailable, using live trackers only", "error", cacheErr)
	}

	for _, proj := range cfg.Projects {
		for _, b := range proj.Bugs {
			switch b.Forge {
			case "launchpad":
				if lpBugTracker == nil {
					lpBugTracker = a.newLaunchpadBugTrackerForReads(proj.Name)
				}

				key := "launchpad:" + b.Project
				if _, ok := trackers[key]; !ok {
					var tracker port.BugTracker = lpBugTracker
					if cache != nil {
						tracker = bugcache.NewCachedBugTracker(lpBugTracker, cache, b.Project, a.Logger)
					}
					trackers[key] = bug.ProjectBugTracker{
						Tracker:   tracker,
						ProjectID: b.Project,
					}
				}
				bindings[key] = append(bindings[key], bug.ProjectBinding{
					ProjectName:   proj.Name,
					Group:         b.Group,
					CommonProject: cfg.BugGroups[b.Group].CommonProject,
				})

			default:
				return nil, nil, fmt.Errorf("unsupported bug tracker forge %q for project %s", b.Forge, proj.Name)
			}
		}
	}

	if len(trackers) == 0 && cache != nil {
		statuses, err := cache.Status(context.Background())
		if err != nil {
			return nil, nil, fmt.Errorf("listing bug cache status: %w", err)
		}
		if len(statuses) > 0 {
			a.Logger.Info("using cached bug tracker metadata because no bug trackers are configured", "entries", len(statuses))
		}
		for _, status := range statuses {
			if status.ForgeType != forge.ForgeLaunchpad.String() {
				continue
			}
			if lpBugTracker == nil {
				lpBugTracker = a.newLaunchpadBugTrackerForReads(status.Project)
			}
			key := "launchpad:" + status.Project
			if _, ok := trackers[key]; ok {
				continue
			}
			trackers[key] = bug.ProjectBugTracker{
				Tracker:   bugcache.NewCachedBugTracker(lpBugTracker, cache, status.Project, a.Logger),
				ProjectID: status.Project,
			}
			bindings[key] = []bug.ProjectBinding{{
				ProjectName: status.Project,
			}}
		}
	}

	return trackers, bindings, nil
}

func (a *App) newLaunchpadBugTrackerForReads(projectName string) *forge.LaunchpadBugTracker {
	lpClient := newLaunchpadClient(a.LaunchpadCredentialStore(), a.Logger, a.upstreamHTTPClient("launchpad", 2*time.Minute))
	if lpClient == nil {
		a.Logger.Info("using unauthenticated Launchpad client for bug tracker reads", "project", projectName)
		lpClient = lp.NewClient(nil, a.Logger, a.upstreamHTTPClient("launchpad", 2*time.Minute))
	}
	return forge.NewLaunchpadBugTracker(lpClient)
}
