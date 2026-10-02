// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"fmt"
	"strings"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/build"
	opsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/operation"
	projectsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/project"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// Facade exposes frontend-oriented wrappers on top of the generic operation service.
type Facade struct {
	application *app.App
	operations  *OperationWorkflow
}

// BuildTriggerOptions holds the inputs for starting an async build trigger operation.
type BuildTriggerOptions struct {
	Project   string
	Artifacts []string
	Trigger   build.TriggerOpts
}

// ProjectSyncOptions holds the inputs for starting an async project sync operation.
type ProjectSyncOptions struct {
	Projects []string
	DryRun   bool
}

// TeamSyncOptions holds the inputs for starting an async team sync operation.
type TeamSyncOptions struct {
	Projects []string
	DryRun   bool
}

// NewFacade creates a frontend-oriented facade on top of the application wiring.
func NewFacade(application *app.App) *Facade {
	return &Facade{
		application: application,
		operations:  NewOperationWorkflow(application),
	}
}

// StartBuildTrigger starts an async build-trigger workflow.
func (f *Facade) StartBuildTrigger(ctx context.Context, opts BuildTriggerOptions) (*dto.OperationJob, error) {
	buildService, err := f.application.BuildService()
	if err != nil {
		return nil, err
	}

	attributes := map[string]string{
		"project": opts.Project,
	}
	if len(opts.Artifacts) > 0 {
		attributes["artifacts"] = fmt.Sprintf("%d", len(opts.Artifacts))
	}
	if opts.Trigger.Owner != "" {
		attributes["owner"] = opts.Trigger.Owner
	}
	targetRef := opts.Trigger.TargetRef
	if prepared := opts.Trigger.Prepared.Normalize(); prepared != nil && prepared.TargetRef != "" {
		targetRef = prepared.TargetRef
	}
	if targetRef != "" {
		attributes["target_ref"] = targetRef
	}
	if prepared := opts.Trigger.Prepared.Normalize(); prepared != nil && prepared.Backend != "" {
		attributes["backend"] = string(prepared.Backend)
	}
	if opts.Trigger.Wait {
		attributes["wait"] = "true"
	}

	return f.operations.Start(ctx, dto.OperationKindBuildTrigger, attributes, func(runCtx context.Context, reporter *opsvc.Reporter) (string, error) {
		reporter.Event(fmt.Sprintf("starting build trigger for project %q", opts.Project))
		reporter.Progress(dto.OperationProgress{
			Phase:         "triggering",
			Message:       "submitting build requests",
			Indeterminate: true,
		})

		result, err := buildService.Trigger(runCtx, opts.Project, opts.Artifacts, opts.Trigger)
		if err != nil {
			return "", err
		}

		for _, recipe := range result.RecipeResults {
			switch {
			case recipe.ErrorMessage != "":
				reporter.Event(fmt.Sprintf("recipe %q failed: %s", recipe.Name, recipe.ErrorMessage))
			case recipe.BuildRequest != nil:
				reporter.Event(fmt.Sprintf("recipe %q submitted build request %q", recipe.Name, recipe.BuildRequest.Status))
			case len(recipe.Builds) > 0:
				reporter.Event(fmt.Sprintf("recipe %q returned %d builds", recipe.Name, len(recipe.Builds)))
			default:
				reporter.Event(fmt.Sprintf("recipe %q processed with no build records", recipe.Name))
			}
		}

		summary := buildTriggerSummary(result)
		reporter.Progress(dto.OperationProgress{
			Phase:   "completed",
			Message: summary,
			Current: len(result.RecipeResults),
			Total:   len(result.RecipeResults),
		})
		return summary, nil
	})
}

// StartTeamSync starts an async team-sync workflow.
func (f *Facade) StartTeamSync(ctx context.Context, opts TeamSyncOptions) (*dto.OperationJob, error) {
	// Resolve the server workflow to perform the sync inside the operation runner.
	sw := NewTeamServerWorkflow(f.application, f)

	attributes := map[string]string{}
	if opts.DryRun {
		attributes["dry_run"] = "true"
	}
	if len(opts.Projects) > 0 {
		attributes["projects"] = fmt.Sprintf("%d", len(opts.Projects))
	}

	return f.operations.Start(ctx, dto.OperationKindTeamSync, attributes, func(runCtx context.Context, reporter *opsvc.Reporter) (string, error) {
		reporter.Event("starting team collaborator sync")
		reporter.Progress(dto.OperationProgress{
			Phase:         "syncing",
			Message:       "checking team members against store collaborators",
			Indeterminate: true,
		})

		result, err := sw.Sync(runCtx, dto.TeamSyncRequest{
			Projects: opts.Projects,
			DryRun:   opts.DryRun,
		})
		if err != nil {
			return "", err
		}

		for _, art := range result.Artifacts {
			if art.Error != "" {
				reporter.Event(fmt.Sprintf("artifact %s/%s: %s", art.Project, art.StoreName, art.Error))
			} else if art.AlreadySync {
				reporter.Event(fmt.Sprintf("artifact %s/%s: already in sync", art.Project, art.StoreName))
			} else {
				reporter.Event(fmt.Sprintf("artifact %s/%s: invited=%d extra=%d pending=%d",
					art.Project, art.StoreName, len(art.Invited), len(art.Extra), len(art.Pending)))
			}
		}
		for _, w := range result.Warnings {
			reporter.Event(fmt.Sprintf("warning: %s", w))
		}

		summary := teamSyncSummary(result, opts.DryRun)
		reporter.Progress(dto.OperationProgress{
			Phase:   "completed",
			Message: summary,
			Current: len(result.Artifacts),
			Total:   len(result.Artifacts),
		})
		return summary, nil
	})
}

// StartProjectSync starts an async project-sync workflow.
func (f *Facade) StartProjectSync(ctx context.Context, opts ProjectSyncOptions) (*dto.OperationJob, error) {
	projectService, err := f.application.ProjectService()
	if err != nil {
		return nil, err
	}

	attributes := map[string]string{}
	if opts.DryRun {
		attributes["dry_run"] = "true"
	}
	if len(opts.Projects) > 0 {
		attributes["projects"] = fmt.Sprintf("%d", len(opts.Projects))
	}

	return f.operations.Start(ctx, dto.OperationKindProjectSync, attributes, func(runCtx context.Context, reporter *opsvc.Reporter) (string, error) {
		reporter.Event("starting Launchpad project sync")
		reporter.Progress(dto.OperationProgress{
			Phase:         "syncing",
			Message:       "checking Launchpad project metadata",
			Indeterminate: true,
		})

		result, err := projectService.Sync(runCtx, projectsvc.SyncOptions{
			Projects: opts.Projects,
			DryRun:   opts.DryRun,
		})
		if err != nil {
			return "", err
		}

		for _, action := range result.Actions {
			reporter.Event(projectSyncActionMessage(action))
		}
		for _, syncErr := range result.Errors {
			reporter.Event(fmt.Sprintf("project sync error: %s", syncErr.Error()))
		}

		summary := projectSyncSummary(result, opts.DryRun)
		reporter.Progress(dto.OperationProgress{
			Phase:   "completed",
			Message: summary,
			Current: len(result.Actions),
			Total:   len(result.Actions) + len(result.Errors),
		})
		return summary, nil
	})
}

func buildTriggerSummary(result *dto.BuildTriggerResult) string {
	requestCount := 0
	buildCount := 0
	errorCount := 0
	for _, recipe := range result.RecipeResults {
		if recipe.BuildRequest != nil {
			requestCount++
		}
		buildCount += len(recipe.Builds)
		if recipe.ErrorMessage != "" {
			errorCount++
		}
	}

	parts := []string{fmt.Sprintf("%d recipes", len(result.RecipeResults))}
	if requestCount > 0 {
		parts = append(parts, fmt.Sprintf("%d requests", requestCount))
	}
	if buildCount > 0 {
		parts = append(parts, fmt.Sprintf("%d builds", buildCount))
	}
	if errorCount > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", errorCount))
	}

	summary := fmt.Sprintf("processed %s", strings.Join(parts, ", "))
	if result.Project != "" {
		summary = fmt.Sprintf("%s for %s", summary, result.Project)
	}
	return summary
}

func projectSyncSummary(result *projectsvc.SyncResult, dryRun bool) string {
	mode := "applied"
	if dryRun {
		mode = "planned"
	}

	summary := fmt.Sprintf("%s %d project sync actions", mode, len(result.Actions))
	if len(result.Errors) > 0 {
		summary = fmt.Sprintf("%s with %d non-fatal errors", summary, len(result.Errors))
	}
	return summary
}

func projectSyncActionMessage(action dto.ProjectSyncAction) string {
	switch action.ActionType {
	case dto.ProjectSyncActionCreateSeries:
		return fmt.Sprintf("project %q: create series %q", action.Project, action.Series)
	case dto.ProjectSyncActionSetDevFocus:
		return fmt.Sprintf("project %q: set development focus to %q", action.Project, action.Series)
	case dto.ProjectSyncActionDevFocusUnchanged:
		return fmt.Sprintf("project %q: development focus already %q", action.Project, action.Series)
	default:
		return fmt.Sprintf("project %q: completed %q", action.Project, action.ActionType)
	}
}

func teamSyncSummary(result *dto.TeamSyncResult, dryRun bool) string {
	mode := "applied"
	if dryRun {
		mode = "planned"
	}

	invited := 0
	extra := 0
	synced := 0
	for _, art := range result.Artifacts {
		invited += len(art.Invited)
		extra += len(art.Extra)
		if art.AlreadySync {
			synced++
		}
	}

	summary := fmt.Sprintf("%s team sync across %d artifacts: %d invited, %d extra, %d already in sync",
		mode, len(result.Artifacts), invited, extra, synced)
	if len(result.Warnings) > 0 {
		summary = fmt.Sprintf("%s (%d warnings)", summary, len(result.Warnings))
	}
	return summary
}
