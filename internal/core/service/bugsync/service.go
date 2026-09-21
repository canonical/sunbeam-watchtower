// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package bugsync

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gboutry/sunbeam-watchtower/internal/core/port"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/gboutry/sunbeam-watchtower/pkg/forge/v1"
)

// ActionType describes the kind of sync action.
type ActionType = dto.BugSyncActionType

const (
	ActionStatusUpdate     ActionType = dto.BugSyncActionStatusUpdate
	ActionSeriesAssignment ActionType = dto.BugSyncActionSeriesAssignment
	ActionAddProjectTask   ActionType = dto.BugSyncActionAddProjectTask
)

// SyncAction represents a single action taken (or planned) during sync.
type SyncAction = dto.BugSyncAction

// SyncResult holds the outcome of a sync operation.
type SyncResult = dto.BugSyncResult

// SyncOptions controls the sync behavior.
type SyncOptions struct {
	Projects []string // filter to these watchtower project names (empty = all)
	BugIDs   []string // filter to these LP bug IDs (empty = all)
	DryRun   bool
	Since    *time.Time // only consider commits after this time
}

// BugBranch tracks which bug was found on which branch of which project.
type BugBranch struct {
	BugID   string
	Project string // watchtower project name
	Branch  string // "main", "stable/2024.1", etc.
	Commit  string
	RefType forge.BugRefType // strongest ref type for this occurrence
}

// ReleaseBoundary identifies one snap revision published to a stable series.
type ReleaseBoundary struct {
	Project  string
	Series   string
	Channel  string
	Revision int
	Tag      string
}

type releaseEvidence struct {
	ReleaseBoundary
	Commit string
}

// Service performs bug status synchronization from cached commits to LP.
type Service struct {
	commitSources map[string]port.CommitSource
	bugTracker    port.BugTracker
	lpProjects    []string // LP project names for searchTasks queries
	// Maps watchtower project name → LP bug project names.
	lpProjectMap     map[string][]string
	commonProjects   map[string]bool
	releases         []ReleaseBoundary
	configuredSeries map[string][]string
	logger           *slog.Logger

	// Caches to avoid redundant API calls.
	projectCache map[string]*forge.Project        // lpProject → Project
	seriesCache  map[string][]forge.ProjectSeries // lpProject → series list
}

// WithReleaseEvidence configures cached stable-channel boundaries used to
// prove that fixes have shipped.
func (s *Service) WithReleaseEvidence(boundaries []ReleaseBoundary) *Service {
	s.releases = append([]ReleaseBoundary(nil), boundaries...)
	return s
}

// WithProjectPolicy configures shared LP projects and their known series.
func (s *Service) WithProjectPolicy(common map[string]bool, series map[string][]string) *Service {
	s.commonProjects = common
	s.configuredSeries = series
	return s
}

// NewService creates a bug sync service.
// lpProjectMap maps watchtower project names to their associated LP bug project names.
func NewService(
	commitSources map[string]port.CommitSource,
	bugTracker port.BugTracker,
	lpProjects []string,
	lpProjectMap map[string][]string,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if lpProjectMap == nil {
		lpProjectMap = make(map[string][]string)
	}
	return &Service{
		commitSources: commitSources,
		bugTracker:    bugTracker,
		lpProjects:    lpProjects,
		lpProjectMap:  lpProjectMap,
		logger:        logger,
		projectCache:  make(map[string]*forge.Project),
		seriesCache:   make(map[string][]forge.ProjectSeries),
	}
}

// Sync scans cached commits for bug references and updates LP bug tasks.
func (s *Service) Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	result := &SyncResult{}
	projFilter := make(map[string]bool, len(opts.Projects))
	for _, p := range opts.Projects {
		projFilter[p] = true
	}
	bugFilter := make(map[string]bool, len(opts.BugIDs))
	for _, id := range opts.BugIDs {
		bugFilter[id] = true
	}

	// Phase 1: Collect bug references from all branches of all projects.
	bugBranches := make(map[string][]BugBranch) // bugID → []BugBranch

	for name, ps := range s.commitSources {
		if len(projFilter) > 0 && !projFilter[name] {
			continue
		}

		branches, err := ps.ListBranches(ctx)
		if err != nil {
			s.logger.Warn("failed to list branches", "project", name, "error", err)
			continue
		}

		for _, branch := range branches {
			if !isRelevantBranch(branch) {
				continue
			}

			commits, err := ps.ListCommits(ctx, forge.ListCommitsOpts{Branch: branch})
			if err != nil {
				s.logger.Warn("failed to list commits", "project", name, "branch", branch, "error", err)
				continue
			}

			for _, c := range commits {
				for _, bugRef := range c.BugRefs {
					if len(bugFilter) > 0 && !bugFilter[bugRef.ID] {
						continue
					}
					bugBranches[bugRef.ID] = appendUnique(bugBranches[bugRef.ID], BugBranch{
						BugID:   bugRef.ID,
						Project: name,
						Branch:  branch,
						Commit:  c.SHA,
						RefType: bugRef.Type,
					})
				}
			}
		}
	}

	// Explicit bug IDs plus --project declare affected project scope, allowing
	// project tasks to be created before a corresponding commit lands.
	if len(bugFilter) > 0 && len(projFilter) > 0 {
		for bugID := range bugFilter {
			for project := range projFilter {
				bugBranches[bugID] = appendUnique(bugBranches[bugID], BugBranch{BugID: bugID, Project: project})
			}
		}
	}
	for bugID := range bugFilter {
		if _, ok := bugBranches[bugID]; !ok {
			result.Errors = append(result.Errors, fmt.Errorf("bug %s: no matching cached commit reference; use --project to declare affected projects", bugID))
		}
	}

	s.logger.Debug("bug references collected", "unique_bugs", len(bugBranches))

	// Phase 1.5: If Since is set, use searchTasks to restrict to recent bugs.
	if opts.Since != nil {
		eligible, err := s.fetchRecentBugIDs(ctx, *opts.Since)
		if err != nil {
			return nil, fmt.Errorf("fetching recent bugs: %w", err)
		}
		for bugID := range bugBranches {
			if !eligible[bugID] {
				s.logger.Debug("skipping bug (not in recent search)", "bug_id", bugID)
				delete(bugBranches, bugID)
			}
		}
		s.logger.Debug("filtered to recent bugs", "remaining", len(bugBranches))
	}

	released := s.collectReleasedBugs(ctx, result, bugBranches)
	finalSeriesStates := make(map[string]string)

	bugIDs := make([]string, 0, len(bugBranches))
	for bugID := range bugBranches {
		bugIDs = append(bugIDs, bugID)
	}
	sort.Strings(bugIDs)
	for _, bugID := range bugIDs {
		branches := bugBranches[bugID]
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Related-Bug references are informational and do not declare an affected
		// project. Explicitly selected project scope is represented by an empty
		// branch and remains eligible for task creation.
		if !hasActionableEvidence(branches) && !hasDeclaredProject(branches) {
			s.logger.Debug("skipping bug (Related-Bug only)", "bug_id", bugID)
			result.Skipped++
			continue
		}

		bug, err := s.bugTracker.GetBug(ctx, bugID)
		if err != nil {
			s.logger.Warn("failed to fetch bug", "bug_id", bugID, "error", err)
			result.Errors = append(result.Errors, fmt.Errorf("bug %s: %w", bugID, err))
			continue
		}

		// Ensure bug has tasks for all LP projects associated with the watchtower projects.
		actionStart := len(result.Actions)
		if err := s.ensureProjectTasks(ctx, bugID, bug, branches, opts.DryRun, result); err != nil {
			s.logger.Warn("failed to ensure project tasks", "bug_id", bugID, "error", err)
			result.Errors = append(result.Errors, err)
		}

		// Re-fetch bug after potential task additions to get updated task list.
		if !opts.DryRun {
			bug, err = s.bugTracker.GetBug(ctx, bugID)
			if err != nil {
				s.logger.Warn("failed to re-fetch bug after task addition", "bug_id", bugID, "error", err)
				result.Errors = append(result.Errors, fmt.Errorf("bug %s: %w", bugID, err))
				continue
			}
		} else {
			appendPlannedTasks(bug, result.Actions[actionStart:])
		}

		// Ensure series tasks before statuses so newly-created tasks can be
		// updated in the same apply run.
		actionStart = len(result.Actions)
		if err := s.assignToSeries(ctx, bugID, bug, branches, opts.DryRun, result); err != nil {
			s.logger.Warn("series assignment failed", "bug_id", bugID, "error", err)
			result.Errors = append(result.Errors, err)
		}
		if !opts.DryRun {
			bug, err = s.bugTracker.GetBug(ctx, bugID)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("bug %s: re-fetching after series assignment: %w", bugID, err))
				continue
			}
		} else {
			appendPlannedTasks(bug, result.Actions[actionStart:])
		}

		// Update task statuses.
		for _, task := range bug.Tasks {
			targetStatus, reason, evidence := s.targetStatus(task, branches, released)
			taskProject, taskSeries := taskTarget(task)
			if taskSeries != "" {
				finalSeriesStates[bugID+"\x00"+taskProject+"\x00"+taskSeries] = task.Status
			}
			if targetStatus == "" {
				continue
			}
			if task.Status == "Fix Released" {
				s.logger.Debug("skipping task (already Fix Released)", "bug_id", bugID, "task", task.Title)
				result.Skipped++
				continue
			}
			if task.Status == "Fix Committed" && targetStatus == "Fix Committed" {
				s.logger.Debug("skipping task (already Fix Committed)", "bug_id", bugID, "task", task.Title)
				result.Skipped++
				continue
			}
			// Don't downgrade: Fix Committed is stronger than In Progress.
			if task.Status == "Fix Committed" && targetStatus == "In Progress" {
				s.logger.Debug("skipping task (Fix Committed > In Progress)", "bug_id", bugID, "task", task.Title)
				result.Skipped++
				continue
			}
			// Don't update if already at target status.
			if task.Status == targetStatus {
				s.logger.Debug("skipping task (already at target)", "bug_id", bugID, "task", task.Title, "status", targetStatus)
				result.Skipped++
				continue
			}

			action := SyncAction{
				BugID:      bugID,
				TaskTitle:  task.Title,
				OldStatus:  task.Status,
				NewStatus:  targetStatus,
				SelfLink:   task.SelfLink,
				URL:        task.URL,
				ActionType: ActionStatusUpdate,
				Reason:     reason,
				Project:    taskProject,
				Series:     taskSeries,
			}
			if evidence != nil {
				action.Channel = evidence.Channel
				action.Revision = evidence.Revision
				action.Tag = evidence.Tag
				action.Commit = evidence.Commit
			}

			if !opts.DryRun {
				if err := s.bugTracker.UpdateBugTaskStatus(ctx, task.SelfLink, targetStatus); err != nil {
					s.logger.Warn("failed to update task status", "bug_id", bugID, "task", task.Title, "error", err)
					result.Errors = append(result.Errors, fmt.Errorf("bug %s task %q: %w", bugID, task.Title, err))
					continue
				}
			}

			result.Actions = append(result.Actions, action)
			if taskSeries != "" {
				finalSeriesStates[bugID+"\x00"+taskProject+"\x00"+taskSeries] = targetStatus
			}
		}

	}
	s.sortResult(result)
	s.appendReleaseOrderWarnings(result, finalSeriesStates)
	sort.SliceStable(result.Errors, func(i, j int) bool { return result.Errors[i].Error() < result.Errors[j].Error() })

	return result, nil
}

func appendPlannedTasks(bug *forge.Bug, actions []SyncAction) {
	for _, action := range actions {
		var target string
		switch action.ActionType {
		case ActionAddProjectTask:
			target = action.Project
		case ActionSeriesAssignment:
			target = action.Project + "/" + action.Series
		default:
			continue
		}
		bug.Tasks = append(bug.Tasks, forge.BugTask{
			BugID: action.BugID, TargetName: target, Title: target, Status: "New",
		})
	}
}

func hasActionableEvidence(branches []BugBranch) bool {
	for _, branch := range branches {
		if branch.Branch != "" && branch.RefType != forge.BugRefRelated {
			return true
		}
	}
	return false
}

func hasDeclaredProject(branches []BugBranch) bool {
	for _, branch := range branches {
		if branch.Branch == "" && branch.Project != "" {
			return true
		}
	}
	return false
}

func (s *Service) collectReleasedBugs(ctx context.Context, result *SyncResult, bugBranches map[string][]BugBranch) map[string]map[string]map[string]releaseEvidence {
	type releaseGroup struct {
		project string
		series  string
		items   []ReleaseBoundary
	}
	groups := make(map[string]*releaseGroup)
	neededSeries := make(map[string]map[string]bool)
	neededBugs := make(map[string]map[string]bool)
	for bugID, branches := range bugBranches {
		for _, branch := range branches {
			series := branchToSeriesName(branch.Branch)
			if series != "" && series != "development" && branch.RefType != forge.BugRefRelated && s.sourceSupportsSeries(branch.Project, series) {
				if neededSeries[branch.Project] == nil {
					neededSeries[branch.Project] = make(map[string]bool)
				}
				neededSeries[branch.Project][series] = true
				key := branch.Project + "\x00" + series
				if neededBugs[key] == nil {
					neededBugs[key] = make(map[string]bool)
				}
				neededBugs[key][bugID] = true
			}
		}
	}
	for _, boundary := range s.releases {
		if !neededSeries[boundary.Project][boundary.Series] {
			continue
		}
		key := boundary.Project + "\x00" + boundary.Series
		if groups[key] == nil {
			groups[key] = &releaseGroup{project: boundary.Project, series: boundary.Series}
		}
		groups[key].items = append(groups[key].items, boundary)
	}
	for project, seriesSet := range neededSeries {
		for series := range seriesSet {
			if groups[project+"\x00"+series] == nil {
				result.Errors = append(result.Errors, fmt.Errorf("release %s/%s: no cached stable snap revision; Fix Released detection is unavailable", project, series))
			}
		}
	}

	out := make(map[string]map[string]map[string]releaseEvidence)
	for _, group := range groups {
		source := s.commitSources[group.project]
		if source == nil {
			result.Errors = append(result.Errors, fmt.Errorf("release %s/%s: commit source is not configured", group.project, group.series))
			continue
		}
		var intersection map[string]releaseEvidence
		union := make(map[string]bool)
		valid := true
		for _, boundary := range group.items {
			tag := boundary.Tag
			if tag == "" {
				tag = fmt.Sprintf("rev%d", boundary.Revision)
			}
			commits, err := source.ListCommits(ctx, forge.ListCommitsOpts{Revision: "refs/tags/" + tag + "^{}"})
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("release %s %s revision %d (%s): %w", group.project, boundary.Channel, boundary.Revision, tag, err))
				valid = false
				break
			}
			bugs := make(map[string]releaseEvidence)
			rootCommit := ""
			if len(commits) > 0 {
				rootCommit = commits[0].SHA
			}
			for _, commit := range commits {
				for _, ref := range commit.BugRefs {
					if ref.Type != forge.BugRefCloses {
						continue
					}
					boundaryCopy := boundary
					boundaryCopy.Tag = tag
					bugs[ref.ID] = releaseEvidence{ReleaseBoundary: boundaryCopy, Commit: rootCommit}
					union[ref.ID] = true
				}
			}
			if intersection == nil {
				intersection = bugs
				continue
			}
			for bugID := range intersection {
				if _, ok := bugs[bugID]; !ok {
					delete(intersection, bugID)
				}
			}
		}
		if !valid || intersection == nil {
			continue
		}
		for bugID := range neededBugs[group.project+"\x00"+group.series] {
			if union[bugID] {
				if _, inEveryTarget := intersection[bugID]; !inEveryTarget {
					result.Errors = append(result.Errors, fmt.Errorf("bug %s: release %s/%s targets disagree on fix inclusion; keeping Fix Committed", bugID, group.project, group.series))
				}
			}
		}
		if out[group.project] == nil {
			out[group.project] = make(map[string]map[string]releaseEvidence)
		}
		out[group.project][group.series] = intersection
	}
	return out
}

func (s *Service) targetStatus(task forge.BugTask, branches []BugBranch, released map[string]map[string]map[string]releaseEvidence) (string, string, *releaseEvidence) {
	target, series := taskTarget(task)
	if series != "" && series != "development" && !s.supportsSeries(target, series) {
		return "", "", nil
	}
	type component struct {
		project  string
		status   string
		branch   BugBranch
		evidence *releaseEvidence
	}
	byProject := make(map[string][]BugBranch)
	for _, branch := range branches {
		mappedTargets := s.lpProjectMap[branch.Project]
		if branch.Branch == "" || branch.RefType == forge.BugRefRelated || (len(mappedTargets) > 0 && !containsString(mappedTargets, target)) {
			continue
		}
		if series != "" {
			branchSeries := branchToSeriesName(branch.Branch)
			if branchSeries != series {
				continue
			}
		}
		byProject[branch.Project] = append(byProject[branch.Project], branch)
	}

	components := make([]component, 0, len(byProject))
	for project, projectBranches := range byProject {
		strongest := strongestRefType(projectBranches)
		status := refTypeToStatus(strongest)
		var evidence *releaseEvidence
		if status == "Fix Committed" && series != "" {
			if found, ok := released[project][series][task.BugID]; ok {
				status = "Fix Released"
				evidenceCopy := found
				evidence = &evidenceCopy
			}
		}
		if status != "" {
			components = append(components, component{project: project, status: status, branch: projectBranches[0], evidence: evidence})
		}
	}
	if len(components) == 0 {
		return "", "", nil
	}
	sort.Slice(components, func(i, j int) bool { return components[i].project < components[j].project })
	selected := components[0]
	for _, component := range components[1:] {
		if statusRank(component.status) < statusRank(selected.status) {
			selected = component
		}
	}

	if len(components) > 1 || s.commonProjects[target] {
		parts := make([]string, 0, len(components))
		for _, component := range components {
			parts = append(parts, component.project+"="+component.status)
		}
		reason := "shared task aggregates affected components: " + strings.Join(parts, ", ")
		if selected.status == "Fix Released" && selected.evidence != nil {
			reason += fmt.Sprintf("; all are included in stable release evidence (including %s revision %d, tag %s)", selected.evidence.Channel, selected.evidence.Revision, selected.evidence.Tag)
		}
		return selected.status, reason, selected.evidence
	}

	if selected.status == "Fix Released" && selected.evidence != nil {
		return selected.status, fmt.Sprintf("Closes-Bug is included in %s revision %d (%s)", selected.evidence.Channel, selected.evidence.Revision, selected.evidence.Tag), selected.evidence
	}
	return selected.status, fmt.Sprintf("%s reference found in project %s on branch %s at %s", selected.status, selected.project, selected.branch.Branch, shortSHA(selected.branch.Commit)), nil
}

func splitTaskTarget(target string) (string, string) {
	parts := strings.SplitN(target, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return target, ""
}

func taskTarget(task forge.BugTask) (string, string) {
	project, series := splitTaskTarget(task.TargetName)
	if series != "" || task.TargetLink == "" {
		return project, series
	}
	u, err := url.Parse(task.TargetLink)
	if err != nil {
		return project, ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, part := range parts {
		if part == project && i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "+") {
			return project, parts[i+1]
		}
	}
	return project, ""
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (s *Service) supportsSeries(project, series string) bool {
	if series == "development" || len(s.configuredSeries) == 0 {
		return true
	}
	return containsString(s.configuredSeries[project], series)
}

func (s *Service) sourceSupportsSeries(project, series string) bool {
	if len(s.configuredSeries) == 0 {
		return true
	}
	for _, lpProject := range s.lpProjectMap[project] {
		if s.supportsSeries(lpProject, series) {
			return true
		}
	}
	return false
}

func statusRank(status string) int {
	switch status {
	case "In Progress":
		return 1
	case "Fix Committed":
		return 2
	case "Fix Released":
		return 3
	default:
		return 0
	}
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func (s *Service) sortResult(result *SyncResult) {
	sort.SliceStable(result.Actions, func(i, j int) bool {
		a, b := result.Actions[i], result.Actions[j]
		if a.BugID != b.BugID {
			return a.BugID < b.BugID
		}
		if a.Project != b.Project {
			return a.Project < b.Project
		}
		if a.Series != b.Series {
			return a.Series < b.Series
		}
		if a.TaskTitle != b.TaskTitle {
			return a.TaskTitle < b.TaskTitle
		}
		return a.ActionType < b.ActionType
	})
}

// appendReleaseOrderWarnings is populated by the planner once all status
// actions are known. It deliberately warns without blocking authoritative
// older-series release updates.
func (s *Service) appendReleaseOrderWarnings(result *SyncResult, finalSeriesStates map[string]string) {
	type releaseAction struct{ bugID, project, series string }
	var releases []releaseAction
	for _, action := range result.Actions {
		if action.ActionType != ActionStatusUpdate || action.NewStatus != "Fix Released" {
			continue
		}
		project, series := splitTaskTarget(actionTarget(action))
		if series == "" {
			continue
		}
		key := action.BugID + "\x00" + project + "\x00" + series
		finalSeriesStates[key] = "Fix Released"
		releases = append(releases, releaseAction{action.BugID, project, series})
	}
	for _, action := range releases {
		current, ok := parseSeries(action.series)
		if !ok {
			continue
		}
		var behind []string
		for _, newer := range s.configuredSeries[action.project] {
			parsed, valid := parseSeries(newer)
			if !valid || parsed <= current {
				continue
			}
			status := finalSeriesStates[action.bugID+"\x00"+action.project+"\x00"+newer]
			if status != "Fix Released" {
				if status == "" {
					status = "missing task"
				}
				behind = append(behind, newer+"="+status)
			}
		}
		if len(behind) > 0 {
			sort.Strings(behind)
			result.Errors = append(result.Errors, fmt.Errorf("bug %s: marking %s/%s Fix Released before newer series %s", action.bugID, action.project, action.series, strings.Join(behind, ", ")))
		}
	}
}

func actionTarget(action SyncAction) string {
	if action.Project != "" && action.Series != "" {
		return action.Project + "/" + action.Series
	}
	// Status actions carry the Launchpad task title but not a normalized target
	// in older API responses. New actions set Project and Series below.
	return action.Project
}

func parseSeries(value string) (int, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return 0, false
	}
	year, err1 := strconv.Atoi(parts[0])
	cycle, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return year*100 + cycle, true
}

// strongestRefType returns the strongest (lowest enum value) ref type from branches.
func strongestRefType(branches []BugBranch) forge.BugRefType {
	strongest := forge.BugRefRelated
	for _, bb := range branches {
		if bb.RefType < strongest {
			strongest = bb.RefType
		}
	}
	return strongest
}

// refTypeToStatus maps a BugRefType to the target LP bug status.
func refTypeToStatus(rt forge.BugRefType) string {
	switch rt {
	case forge.BugRefCloses:
		return "Fix Committed"
	case forge.BugRefPartial:
		return "In Progress"
	default:
		return ""
	}
}

// ensureProjectTasks adds bug tasks for LP projects associated with the watchtower
// projects where the bug was found, if those tasks don't already exist.
func (s *Service) ensureProjectTasks(ctx context.Context, bugID string, bug *forge.Bug, branches []BugBranch, dryRun bool, result *SyncResult) error {
	// Collect LP projects that should have tasks.
	neededProjects := make(map[string]map[string]bool)
	unmappedProjects := make(map[string]bool)
	for _, bb := range branches {
		if bb.Branch != "" && bb.RefType == forge.BugRefRelated {
			continue
		}
		if len(s.lpProjectMap[bb.Project]) == 0 {
			if !unmappedProjects[bb.Project] {
				result.Errors = append(result.Errors, fmt.Errorf("bug %s: affected project %s has no Launchpad bug-task mapping", bugID, bb.Project))
				unmappedProjects[bb.Project] = true
			}
			continue
		}
		for _, lpProj := range s.lpProjectMap[bb.Project] {
			if neededProjects[lpProj] == nil {
				neededProjects[lpProj] = make(map[string]bool)
			}
			neededProjects[lpProj][bb.Project] = true
		}
	}

	// Check which LP projects already have tasks on this bug.
	existingProjects := make(map[string]bool)
	for _, task := range bug.Tasks {
		proj := task.TargetName
		if idx := strings.Index(proj, "/"); idx != -1 {
			proj = proj[:idx]
		}
		existingProjects[proj] = true
	}

	bugIDInt, err := strconv.Atoi(bugID)
	if err != nil {
		return fmt.Errorf("invalid bug ID %q: %w", bugID, err)
	}

	orderedProjects := make([]string, 0, len(neededProjects))
	for lpProj := range neededProjects {
		orderedProjects = append(orderedProjects, lpProj)
	}
	sort.Strings(orderedProjects)
	for _, lpProj := range orderedProjects {
		if existingProjects[lpProj] {
			continue
		}

		// Get project self_link for AddBugTask.
		proj, err := s.getCachedProject(ctx, lpProj)
		if err != nil {
			s.logger.Warn("failed to get project for task addition", "project", lpProj, "error", err)
			result.Errors = append(result.Errors, fmt.Errorf("bug %s project %s: %w", bugID, lpProj, err))
			continue
		}

		action := SyncAction{
			BugID:      bugID,
			Project:    lpProj,
			ActionType: ActionAddProjectTask,
			Reason:     fmt.Sprintf("affected source projects %s map to Launchpad project %s", strings.Join(sortedSet(neededProjects[lpProj]), ", "), lpProj),
		}

		if !dryRun {
			if err := s.bugTracker.AddBugTask(ctx, bugIDInt, proj.SelfLink); err != nil {
				s.logger.Warn("failed to add project task", "bug_id", bugID, "project", lpProj, "error", err)
				result.Errors = append(result.Errors, fmt.Errorf("bug %s add task %s: %w", bugID, lpProj, err))
				continue
			}
		}

		result.Actions = append(result.Actions, action)
	}

	return nil
}

func sortedSet(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// assignToSeries handles series task assignment for a bug based on which branches it appears on.
func (s *Service) assignToSeries(ctx context.Context, bugID string, bug *forge.Bug, branches []BugBranch, dryRun bool, result *SyncResult) error {
	existing := make(map[string]bool)
	for _, task := range bug.Tasks {
		project, series := taskTarget(task)
		if project != "" && series != "" {
			existing[project+"/"+series] = true
		}
	}

	bugIDInt, err := strconv.Atoi(bugID)
	if err != nil {
		return fmt.Errorf("invalid bug ID %q: %w", bugID, err)
	}

	type target struct{ project, series, source string }
	needed := make(map[string]target)
	legacyProjects := make([]string, 0)
	for _, task := range bug.Tasks {
		project, _ := taskTarget(task)
		if project != "" && !containsString(legacyProjects, project) {
			legacyProjects = append(legacyProjects, project)
		}
	}
	for _, bb := range branches {
		if bb.Branch == "" || bb.RefType == forge.BugRefRelated {
			continue
		}
		seriesName := branchToSeriesName(bb.Branch)
		if seriesName == "" {
			continue
		}
		mappedProjects := s.lpProjectMap[bb.Project]
		if len(mappedProjects) == 0 {
			mappedProjects = legacyProjects
		}
		for _, lpProject := range mappedProjects {
			if seriesName != "development" && !s.supportsSeries(lpProject, seriesName) {
				continue
			}
			key := lpProject + "/" + seriesName
			needed[key] = target{project: lpProject, series: seriesName, source: bb.Project}
		}
	}
	keys := make([]string, 0, len(needed))
	for key := range needed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		item := needed[key]
		if existing[key] {
			continue
		}

		seriesLink, err := s.resolveSeriesLink(ctx, item.project, item.series)
		if err != nil {
			s.logger.Warn("failed to resolve series", "project", item.project, "series", item.series, "error", err)
			continue
		}
		if seriesLink == "" {
			s.logger.Debug("series not found", "project", item.project, "series", item.series)
			continue
		}

		action := SyncAction{
			BugID:      bugID,
			Series:     item.series,
			Project:    item.project,
			ActionType: ActionSeriesAssignment,
			Reason:     fmt.Sprintf("fix evidence in source project %s requires series task %s/%s", item.source, item.project, item.series),
		}

		if !dryRun {
			if err := s.bugTracker.AddBugTask(ctx, bugIDInt, seriesLink); err != nil {
				// Assignment may fail if task already exists — log and continue.
				s.logger.Warn("series assignment failed (may already exist)", "bug_id", bugID, "series", item.series, "error", err)
				continue
			}
		}

		result.Actions = append(result.Actions, action)
	}

	return nil
}

// resolveSeriesLink finds the LP API self_link for a series, using cached results.
func (s *Service) resolveSeriesLink(ctx context.Context, lpProject, seriesName string) (string, error) {
	if seriesName == "development" {
		proj, err := s.getCachedProject(ctx, lpProject)
		if err != nil {
			return "", err
		}
		return proj.DevelopmentFocusLink, nil
	}

	series, err := s.getCachedSeries(ctx, lpProject)
	if err != nil {
		return "", err
	}
	for _, ps := range series {
		if ps.Name == seriesName {
			return ps.SelfLink, nil
		}
	}
	return "", nil
}

// getCachedProject returns a cached project or fetches it.
func (s *Service) getCachedProject(ctx context.Context, lpProject string) (*forge.Project, error) {
	if proj, ok := s.projectCache[lpProject]; ok {
		return proj, nil
	}
	proj, err := s.bugTracker.GetProject(ctx, lpProject)
	if err != nil {
		return nil, err
	}
	s.projectCache[lpProject] = proj
	return proj, nil
}

// getCachedSeries returns cached series or fetches them.
func (s *Service) getCachedSeries(ctx context.Context, lpProject string) ([]forge.ProjectSeries, error) {
	if series, ok := s.seriesCache[lpProject]; ok {
		return series, nil
	}
	series, err := s.bugTracker.GetProjectSeries(ctx, lpProject)
	if err != nil {
		return nil, err
	}
	s.seriesCache[lpProject] = series
	return series, nil
}

// fetchRecentBugIDs uses searchTasks with created_since to build a set of bug IDs.
func (s *Service) fetchRecentBugIDs(ctx context.Context, since time.Time) (map[string]bool, error) {
	sinceStr := since.UTC().Format(time.RFC3339)
	eligible := make(map[string]bool)

	for _, proj := range s.lpProjects {
		tasks, err := s.bugTracker.ListBugTasks(ctx, proj, forge.ListBugTasksOpts{
			CreatedSince: sinceStr,
		})
		if err != nil {
			s.logger.Warn("failed to search recent bugs", "project", proj, "error", err)
			continue
		}
		for _, t := range tasks {
			eligible[t.BugID] = true
		}
		s.logger.Debug("fetched recent bugs for project", "project", proj, "count", len(tasks))
	}

	return eligible, nil
}

// branchToSeriesName maps a git branch to an LP series name.
// Returns "" for branches that don't map to a series.
func branchToSeriesName(branch string) string {
	switch branch {
	case "main", "master":
		return "development" // sentinel for development focus
	}
	if strings.HasPrefix(branch, "stable/") {
		return strings.TrimPrefix(branch, "stable/")
	}
	return ""
}

// isRelevantBranch returns true for branches we should scan (main, master, stable/*).
func isRelevantBranch(branch string) bool {
	if branch == "main" || branch == "master" {
		return true
	}
	return strings.HasPrefix(branch, "stable/")
}

// appendUnique appends a BugBranch if not already present for the same project+branch.
// If already present, promotes to the stronger ref type.
func appendUnique(slice []BugBranch, bb BugBranch) []BugBranch {
	for i, existing := range slice {
		if existing.Project == bb.Project && existing.Branch == bb.Branch {
			if bb.RefType < existing.RefType {
				slice[i].RefType = bb.RefType
				slice[i].Commit = bb.Commit
			}
			return slice
		}
	}
	return append(slice, bb)
}
