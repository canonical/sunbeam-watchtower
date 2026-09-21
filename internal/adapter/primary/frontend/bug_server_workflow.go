// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gboutry/sunbeam-watchtower/internal/app"
	"github.com/gboutry/sunbeam-watchtower/internal/config"
	"github.com/gboutry/sunbeam-watchtower/internal/core/port"
	"github.com/gboutry/sunbeam-watchtower/internal/core/service/bug"
	"github.com/gboutry/sunbeam-watchtower/internal/core/service/bugsearch"
	"github.com/gboutry/sunbeam-watchtower/internal/core/service/bugsync"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/gboutry/sunbeam-watchtower/pkg/forge/v1"
)

// BugServerWorkflow exposes reusable server-side bug workflows for the HTTP API.
type BugServerWorkflow struct {
	application *app.App
}

// NewBugServerWorkflow creates a server-side bug workflow.
func NewBugServerWorkflow(application *app.App) *BugServerWorkflow {
	return &BugServerWorkflow{application: application}
}

// Show returns one bug with its associated tasks.
func (w *BugServerWorkflow) Show(ctx context.Context, id string) (*forge.Bug, error) {
	if reference, ok := bugsearch.DirectReference(id); ok {
		_, parsedID, err := bugsearch.SplitReference(reference)
		if err != nil {
			return nil, err
		}
		id = parsedID
	}
	trackers, projectMap, err := w.application.BuildBugTrackers()
	if err != nil {
		return nil, err
	}
	return bug.NewService(trackers, projectMap, w.application.Logger).Get(ctx, id)
}

// List returns bug tasks matching the requested filters.
func (w *BugServerWorkflow) List(ctx context.Context, req BugListRequest) (*BugListResponse, error) {
	trackers, projectMap, err := w.application.BuildBugTrackers()
	if err != nil {
		return nil, err
	}

	tasks, results, err := bug.NewService(trackers, projectMap, w.application.Logger).List(ctx, bug.ListOptions{
		Projects:   req.Projects,
		Status:     req.Status,
		Importance: req.Importance,
		Assignee:   req.Assignee,
		Tags:       req.Tags,
		Since:      req.Since,
		Merge:      req.Merge,
		Limit:      req.Limit,
	})
	if err != nil {
		return nil, err
	}

	response := &BugListResponse{Tasks: tasks}
	for _, result := range results {
		if result.Err != nil {
			response.Warnings = append(response.Warnings, result.Err.Error())
		}
	}
	return response, nil
}

// Search returns ranked, explained bug candidates from the complete bug cache.
func (w *BugServerWorkflow) Search(ctx context.Context, req BugSearchRequest) (*BugSearchResponse, error) {
	cache, err := w.application.BugCache()
	if err != nil {
		return nil, fmt.Errorf("opening bug cache: %w", err)
	}
	snapshot, err := cache.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading bug cache snapshot: %w", err)
	}
	bindings := searchProjectBindings(w.application)

	statusByKey := make(map[string]dto.BugCacheStatus)
	for _, status := range snapshot.Status {
		if status.NeedsRefresh {
			return nil, fmt.Errorf(
				"%w: run `watchtower cache sync bugs` to rebuild %s/%s",
				bugsearch.ErrCacheIncompatible,
				status.ForgeType,
				status.Project,
			)
		}
		statusByKey[strings.ToLower(status.ForgeType)+":"+status.Project] = status
	}

	var documents []bugsearch.Document
	var warnings []string
	for _, cached := range snapshot.Documents {
		if cached.Bug == nil {
			continue
		}
		document := bugsearch.Document{Bug: cached.Bug}
		for _, scoped := range cached.Tasks {
			key := strings.ToLower(cached.Bug.Forge.String()) + ":" + scoped.TrackerProject
			projectBindings := bindings[key]
			if len(projectBindings) == 0 {
				task := scoped.Task
				task.Project = scoped.TrackerProject
				document.Tasks = append(document.Tasks, task)
			} else {
				for _, projectName := range projectBindings {
					task := scoped.Task
					task.Project = projectName
					document.Tasks = append(document.Tasks, task)
				}
			}
			status := statusByKey[key]
			document.Provenance = appendUniqueSearchSource(document.Provenance, dto.BugSearchSource{
				Forge:    strings.ToLower(cached.Bug.Forge.String()),
				Project:  scoped.TrackerProject,
				Source:   "cache",
				SyncedAt: status.LastSync,
			})
		}
		if cached.Bug.Sensitive() {
			warnings = append(warnings, "one or more sensitive cached bugs were omitted from offline search")
			continue
		}
		documents = append(documents, document)
	}

	result, err := bugsearch.NewService(documents).Search(req)
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, uniqueStrings(warnings)...)
	return result, nil
}

func searchProjectBindings(application *app.App) map[string][]string {
	bindings := make(map[string][]string)
	cfg := application.GetConfig()
	if cfg == nil {
		return bindings
	}
	for _, project := range cfg.Projects {
		for _, tracker := range project.Bugs {
			key := strings.ToLower(tracker.Forge) + ":" + tracker.Project
			bindings[key] = append(bindings[key], project.Name)
		}
	}
	return bindings
}

func appendUniqueSearchSource(values []dto.BugSearchSource, candidate dto.BugSearchSource) []dto.BugSearchSource {
	for _, value := range values {
		if value.Forge == candidate.Forge && value.Project == candidate.Project {
			return values
		}
	}
	return append(values, candidate)
}

func uniqueStrings(values []string) []string {
	sort.Strings(values)
	return slicesCompact(values)
}

func slicesCompact(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

// Sync triggers one bug correlation/sync run.
func (w *BugServerWorkflow) Sync(ctx context.Context, req BugSyncRequest) (*BugSyncResponse, error) {
	bugIDs, err := normalizeBugIDs(req.BugIDs)
	if err != nil {
		return nil, err
	}
	sources, err := w.application.BuildCommitSources()
	if err != nil {
		return nil, err
	}

	trackers, _, err := w.application.BuildBugTrackers()
	if err != nil {
		return nil, err
	}

	var tracker port.BugTracker
	var lpProjects []string
	for _, pt := range trackers {
		if tracker == nil {
			tracker = pt.Tracker
		}
		lpProjects = append(lpProjects, pt.ProjectID)
	}
	if tracker == nil {
		return nil, ErrNoBugTrackerConfigured
	}

	lpProjectMap := make(map[string][]string)
	commonProjects := make(map[string]bool)
	configuredSeries := make(map[string][]string)
	developmentFocus := make(map[string]string)
	cfg := w.application.GetConfig()
	for _, group := range cfg.BugGroups {
		commonProjects[group.CommonProject] = true
	}
	for _, proj := range cfg.Projects {
		series := proj.Series
		if len(series) == 0 {
			series = cfg.Launchpad.Series
		}
		for _, bugConfig := range proj.Bugs {
			if bugConfig.Forge == "launchpad" {
				lpProjectMap[proj.Name] = append(lpProjectMap[proj.Name], bugConfig.Project)
				configuredSeries[bugConfig.Project] = append(configuredSeries[bugConfig.Project], series...)
				focus := proj.DevelopmentFocus
				if focus == "" {
					focus = cfg.Launchpad.DevelopmentFocus
				}
				if focus != "" {
					developmentFocus[bugConfig.Project] = focus
				}
			}
		}
	}

	var boundaries []bugsync.ReleaseBoundary
	var releaseWarnings []string
	if releaseCache, cacheErr := w.application.ReleaseCache(); cacheErr != nil {
		releaseWarnings = append(releaseWarnings, "release cache unavailable: "+cacheErr.Error())
	} else if snapshots, listErr := releaseCache.List(ctx); listErr != nil {
		releaseWarnings = append(releaseWarnings, "release cache unavailable: "+listErr.Error())
	} else {
		projects := make(map[string]config.ProjectConfig, len(cfg.Projects))
		for _, project := range cfg.Projects {
			projects[project.Name] = project
		}
		for _, snapshot := range snapshots {
			if snapshot.ArtifactType != dto.ArtifactSnap {
				continue
			}
			project, ok := projects[snapshot.Project]
			if !ok {
				continue
			}
			for _, channel := range snapshot.Channels {
				if channel.Risk != dto.ReleaseRiskStable || channel.Branch != "" {
					continue
				}
				series := releaseSeriesForTrack(project, channel.Track)
				if series == "" {
					continue
				}
				seen := make(map[int]bool)
				for _, target := range channel.Targets {
					if target.Revision <= 0 || seen[target.Revision] {
						continue
					}
					seen[target.Revision] = true
					boundaries = append(boundaries, bugsync.ReleaseBoundary{
						Project: snapshot.Project, Series: series, Channel: channel.Channel,
						Revision: target.Revision, Tag: fmt.Sprintf("rev%d", target.Revision),
					})
				}
			}
		}
		if len(boundaries) == 0 {
			releaseWarnings = append(releaseWarnings, "release cache contains no stable snap revisions; Fix Released detection is unavailable")
		}
	}

	opts := bugsync.SyncOptions{
		Projects: req.Projects,
		BugIDs:   bugIDs,
		DryRun:   req.DryRun,
	}
	if req.Since != "" {
		since, err := time.Parse(time.RFC3339, req.Since)
		if err != nil {
			return nil, ErrInvalidBugSyncSince
		}
		opts.Since = &since
	}

	service := bugsync.NewService(sources, tracker, lpProjects, lpProjectMap, w.application.Logger).
		WithProjectPolicy(commonProjects, configuredSeries).
		WithDevelopmentFocus(developmentFocus).
		WithReleaseEvidence(boundaries)
	result, err := service.Sync(ctx, opts)
	if err != nil {
		return nil, err
	}

	response := &BugSyncResponse{
		Result:   result,
		Warnings: releaseWarnings,
	}
	for _, syncErr := range result.Errors {
		response.Warnings = append(response.Warnings, syncErr.Error())
	}
	return response, nil
}

func releaseSeriesForTrack(project config.ProjectConfig, track string) string {
	if project.Release != nil {
		for series, mappedTrack := range project.Release.TrackMap {
			if mappedTrack == track {
				return series
			}
		}
	}
	for _, series := range project.Series {
		if series == track {
			return series
		}
	}
	return track
}
