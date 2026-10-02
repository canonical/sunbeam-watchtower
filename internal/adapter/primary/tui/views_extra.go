// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	runtimeadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/runtime"
	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type packageMode int

const (
	packageModeInventory packageMode = iota
	packageModeDiff
	packageModeExcuses
)

type commitMode int

const (
	commitModeLog commitMode = iota
	commitModeTrack
)

type packagesFilters struct {
	mode            packageMode
	set             string
	distro          string
	release         string
	suite           string
	component       string
	backport        string
	merge           bool
	upstreamRelease string
	behindUpstream  bool
	onlyIn          string
	constraints     string
	tracker         string
	name            string
	team            string
	ftbfs           bool
	autopkgtest     bool
	blockedBy       string
	bugged          bool
	minAge          string
	maxAge          string
	limit           string
	reverse         bool
	excuseSet       string
	blockedBySet    string
}

type packagesModel struct {
	filters         packagesFilters
	defaults        packagesFilters
	inventoryRows   []distro.SourcePackage
	diffRows        []dto.PackageDiffResult
	diffSources     []dto.PackageSource
	diffHasUpstream bool
	excuseRows      []dto.PackageExcuseSummary
	index           int
	inventoryDetail *distro.SourcePackageInfo
	diffDetail      *frontend.PackagesShowVersionResponse
	excuseDetail    *dto.PackageExcuse
	loaded          bool
	err             string
	prompt          string
}

type bugsFilters struct {
	query      string
	mode       string
	fields     string
	fuzzy      string
	closed     string
	sort       string
	project    string
	status     string
	importance string
	assignee   string
	tag        string
	since      string
	merge      bool
	limit      string
}

type bugsModel struct {
	filters       bugsFilters
	defaults      bugsFilters
	rows          []forge.BugTask
	searchResults []dto.BugSearchResult
	searchOutcome dto.BugSearchOutcome
	index         int
	detail        *forge.Bug
	warnings      []string
	loaded        bool
	err           string
}

type reviewsFilters struct {
	project string
	forge   string
	state   string
	author  string
	since   string
}

type reviewsModel struct {
	filters   reviewsFilters
	defaults  reviewsFilters
	rows      []forge.MergeRequest
	index     int
	detail    *forge.MergeRequest
	detailErr string
	warnings  []string
	loaded    bool
	err       string
}

type commitsFilters struct {
	mode       commitMode
	project    string
	forge      string
	branch     string
	author     string
	includeMRs bool
	bugID      string
}

type commitsModel struct {
	filters  commitsFilters
	defaults commitsFilters
	rows     []forge.Commit
	index    int
	warnings []string
	loaded   bool
	err      string
	prompt   string
}

type projectSummary struct {
	Name             string
	ArtifactType     string
	CodeForge        string
	CodeProject      string
	BugForges        []string
	BugProjects      []string
	BugGroups        []string
	Series           []string
	DevelopmentFocus string
	HasBuild         bool
	HasRelease       bool
	Config           dto.ProjectConfig
}

type projectsFilters struct {
	name         string
	artifactType string
	codeForge    string
	bugForge     string
	hasBuild     string
	hasRelease   string
}

type projectsModel struct {
	filters  projectsFilters
	defaults projectsFilters
	rows     []projectSummary
	index    int
	config   *dto.Config
	loaded   bool
	err      string
}

type packagesLoadedMsg struct {
	inventoryRows   []distro.SourcePackage
	diffRows        []dto.PackageDiffResult
	diffSources     []dto.PackageSource
	diffHasUpstream bool
	excuseRows      []dto.PackageExcuseSummary
	prompt          string
	err             error
}

type packageInventoryDetailLoadedMsg struct {
	key    string
	detail *distro.SourcePackageInfo
	err    error
}

type packageDiffDetailLoadedMsg struct {
	key    string
	detail *frontend.PackagesShowVersionResponse
	err    error
}

type packageExcuseDetailLoadedMsg struct {
	key    string
	detail *dto.PackageExcuse
	err    error
}

type bugsLoadedMsg struct {
	rows          []forge.BugTask
	searchResults []dto.BugSearchResult
	searchOutcome dto.BugSearchOutcome
	warnings      []string
	err           error
}

type bugDetailLoadedMsg struct {
	key    string
	detail *forge.Bug
	err    error
}

type reviewsLoadedMsg struct {
	rows     []forge.MergeRequest
	warnings []string
	err      error
}

type reviewDetailLoadedMsg struct {
	key    string
	detail *forge.MergeRequest
	err    error
}

type commitsLoadedMsg struct {
	rows     []forge.Commit
	warnings []string
	prompt   string
	err      error
}

type projectsLoadedMsg struct {
	config *dto.Config
	rows   []projectSummary
	err    error
}

type packageFilterOptions struct {
	sets       []string
	distros    []string
	releases   []string
	suites     []string
	components []string
	backports  []string
	trackers   []string
	names      []string
	teams      []string
}

type bugFilterOptions struct {
	projects    []string
	statuses    []string
	importances []string
	assignees   []string
	tags        []string
}

type reviewFilterOptions struct {
	projects []string
	forges   []string
	states   []string
	authors  []string
}

type commitFilterOptions struct {
	projects []string
	forges   []string
	branches []string
	authors  []string
}

type projectFilterOptions struct {
	names         []string
	artifactTypes []string
	codeForges    []string
	bugForges     []string
	bools         []string
}

func loadPackagesCmd(session *runtimeadapter.Session, filters packagesFilters) tea.Cmd {
	actionID := frontend.ActionPackagesList
	switch filters.mode {
	case packageModeDiff:
		actionID = frontend.ActionPackagesDiff
	case packageModeExcuses:
		actionID = frontend.ActionPackagesExcusesList
	}
	return guardSessionAction(session, actionID, func() tea.Msg {
		ctx := context.Background()
		switch filters.mode {
		case packageModeDiff:
			setName := strings.TrimSpace(filters.set)
			if setName == "" {
				if session != nil && session.Config != nil {
					if cfg := session.Config.LocalConfig(); cfg != nil && len(cfg.Packages.Sets)+len(cfg.Packages.LaunchpadSets) == 1 {
						for name := range cfg.Packages.Sets {
							setName = name
						}
						for name := range cfg.Packages.LaunchpadSets {
							setName = name
						}
					}
				}
			}
			if setName == "" {
				return packagesLoadedMsg{prompt: "Select a package set in filters to load diff results."}
			}
			result, err := session.Frontend.Packages().Diff(ctx, frontend.PackagesDiffRequest{
				Set:             setName,
				Distros:         firstNonEmptySlice(filters.distro),
				Releases:        firstNonEmptySlice(filters.release),
				Suites:          firstNonEmptySlice(filters.suite),
				Backports:       packageBackports(filters.backport),
				Merge:           filters.merge,
				UpstreamRelease: filters.upstreamRelease,
				BehindUpstream:  filters.behindUpstream,
				OnlyIn:          filters.onlyIn,
				Constraints:     filters.constraints,
			})
			if err != nil {
				return packagesLoadedMsg{err: err}
			}
			return packagesLoadedMsg{
				diffRows:        result.Results,
				diffSources:     result.Sources,
				diffHasUpstream: result.HasUpstream,
			}
		case packageModeExcuses:
			tracker := strings.TrimSpace(filters.tracker)
			if tracker == "" {
				trackers := configuredExcusesTrackers(session)
				if len(trackers) == 1 {
					tracker = trackers[0]
				}
			}
			if tracker == "" {
				return packagesLoadedMsg{prompt: "Select an excuses tracker in filters to load migration blockers."}
			}
			minAge, maxAge, limit, err := parseExcuseAges(filters)
			if err != nil {
				return packagesLoadedMsg{err: err}
			}
			rows, err := session.Frontend.Packages().ExcusesList(ctx, frontend.PackagesExcusesListRequest{
				Trackers:     []string{tracker},
				Name:         filters.name,
				Component:    filters.component,
				Team:         filters.team,
				FTBFS:        filters.ftbfs,
				Autopkgtest:  filters.autopkgtest,
				BlockedBy:    filters.blockedBy,
				Bugged:       filters.bugged,
				MinAge:       minAge,
				MaxAge:       maxAge,
				Limit:        limit,
				Reverse:      filters.reverse,
				Set:          filters.excuseSet,
				BlockedBySet: filters.blockedBySet,
			})
			return packagesLoadedMsg{excuseRows: rows, err: err}
		default:
			rows, err := session.Frontend.Packages().List(ctx, frontend.PackagesListRequest{
				Distros:    firstNonEmptySlice(filters.distro),
				Releases:   firstNonEmptySlice(filters.release),
				Suites:     firstNonEmptySlice(filters.suite),
				Components: firstNonEmptySlice(filters.component),
				Backports:  packageBackports(filters.backport),
			})
			return packagesLoadedMsg{inventoryRows: rows, err: err}
		}
	})
}

func loadPackageDetailCmd(session *runtimeadapter.Session, packages packagesModel) tea.Cmd {
	switch packages.filters.mode {
	case packageModeDiff:
		row := selectedPackageDiff(packages.diffRows, packages.index)
		if row == nil {
			return nil
		}
		key := packageDiffDetailKey(packages.filters, *row)
		return guardSessionAction(session, frontend.ActionPackagesShowVersion, func() tea.Msg {
			detail, err := session.Frontend.Packages().ShowVersion(context.Background(), frontend.PackagesShowVersionRequest{
				Package:         row.Package,
				Distros:         firstNonEmptySlice(packages.filters.distro),
				Releases:        firstNonEmptySlice(packages.filters.release),
				Backports:       packageBackports(packages.filters.backport),
				Merge:           packages.filters.merge,
				UpstreamRelease: packages.filters.upstreamRelease,
			})
			return packageDiffDetailLoadedMsg{key: key, detail: detail, err: err}
		})
	case packageModeExcuses:
		row := selectedPackageExcuse(packages.excuseRows, packages.index)
		if row == nil {
			return nil
		}
		key := packageExcuseDetailKey(*row)
		return guardSessionAction(session, frontend.ActionPackagesExcusesShow, func() tea.Msg {
			detail, err := session.Frontend.Packages().ExcusesShow(context.Background(), frontend.PackagesExcusesShowRequest{
				Package: row.Package,
				Tracker: row.Tracker,
				Version: row.Version,
			})
			return packageExcuseDetailLoadedMsg{key: key, detail: detail, err: err}
		})
	default:
		row := selectedPackageInventory(packages.inventoryRows, packages.index)
		if row == nil {
			return nil
		}
		key := packageInventoryDetailKey(packages.filters, *row)
		return guardSessionAction(session, frontend.ActionPackagesShowDetail, func() tea.Msg {
			detail, err := session.Frontend.Packages().Detail(context.Background(), frontend.PackagesDetailRequest{
				Package:   row.Package,
				Version:   row.Version,
				Distros:   firstNonEmptySlice(packages.filters.distro),
				Releases:  firstNonEmptySlice(packages.filters.release),
				Suites:    firstNonEmptySlice(packages.filters.suite),
				Backports: packageBackports(packages.filters.backport),
			})
			return packageInventoryDetailLoadedMsg{key: key, detail: detail, err: err}
		})
	}
}

func loadBugsCmd(session *runtimeadapter.Session, filters bugsFilters) tea.Cmd {
	if strings.TrimSpace(filters.query) != "" {
		return loadBugSearchCmd(session, filters)
	}
	return guardSessionAction(session, frontend.ActionBugList, func() tea.Msg {
		limit := 0
		if strings.TrimSpace(filters.limit) != "" {
			parsed, err := strconv.Atoi(strings.TrimSpace(filters.limit))
			if err != nil || parsed < 0 {
				return bugsLoadedMsg{err: fmt.Errorf("limit must be a non-negative integer")}
			}
			limit = parsed
		}
		result, err := session.Frontend.Bugs().List(context.Background(), frontend.BugListRequest{
			Projects:   firstNonEmptySlice(filters.project),
			Status:     firstNonEmptySlice(filters.status),
			Importance: firstNonEmptySlice(filters.importance),
			Assignee:   filters.assignee,
			Tags:       firstNonEmptySlice(filters.tag),
			Since:      filters.since,
			Merge:      filters.merge,
			Limit:      limit,
		})
		if err != nil {
			return bugsLoadedMsg{err: err}
		}
		sort.Slice(result.Tasks, func(i, j int) bool {
			return result.Tasks[i].UpdatedAt.After(result.Tasks[j].UpdatedAt)
		})
		return bugsLoadedMsg{rows: result.Tasks, warnings: result.Warnings}
	})
}

func loadBugSearchCmd(session *runtimeadapter.Session, filters bugsFilters) tea.Cmd {
	return guardSessionAction(session, frontend.ActionBugSearch, func() tea.Msg {
		limit, err := parseNonNegativeLimit(filters.limit, "limit")
		if err != nil {
			return bugsLoadedMsg{err: err}
		}
		var fuzzy *bool
		switch strings.ToLower(strings.TrimSpace(filters.fuzzy)) {
		case "", "auto":
		case "true":
			value := true
			fuzzy = &value
		case "false":
			value := false
			fuzzy = &value
		default:
			return bugsLoadedMsg{err: fmt.Errorf("fuzzy must be auto, true, or false")}
		}
		merge := filters.merge
		result, err := session.Frontend.Bugs().Search(context.Background(), frontend.BugSearchRequest{
			Query:         strings.TrimSpace(filters.query),
			Mode:          strings.TrimSpace(filters.mode),
			Fields:        splitCSV(filters.fields),
			Fuzzy:         fuzzy,
			Projects:      firstNonEmptySlice(filters.project),
			Status:        firstNonEmptySlice(filters.status),
			Importance:    firstNonEmptySlice(filters.importance),
			Assignee:      filters.assignee,
			Tags:          firstNonEmptySlice(filters.tag),
			Closed:        strings.TrimSpace(filters.closed),
			ModifiedAfter: strings.TrimSpace(filters.since),
			Merge:         &merge,
			Sort:          strings.TrimSpace(filters.sort),
			Limit:         limit,
		})
		if err != nil {
			return bugsLoadedMsg{err: err}
		}
		searchRows := result.Results
		if len(searchRows) == 0 {
			searchRows = result.Related
		}
		return bugsLoadedMsg{
			rows:          bugSearchTasks(searchRows),
			searchResults: searchRows,
			searchOutcome: result.Outcome,
			warnings:      result.Warnings,
		}
	})
}

func parseNonNegativeLimit(raw string, name string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return parsed, nil
}

func bugSearchTasks(results []dto.BugSearchResult) []forge.BugTask {
	rows := make([]forge.BugTask, 0, len(results))
	for _, result := range results {
		if len(result.Tasks) > 0 {
			task := result.Tasks[0]
			task.Title = result.Title
			rows = append(rows, task)
			continue
		}
		rows = append(rows, forge.BugTask{
			BugID:      result.ID,
			Title:      result.Title,
			Project:    strings.Join(result.Projects, ","),
			Status:     strings.Join(result.Status, ","),
			Importance: strings.Join(result.Importance, ","),
			URL:        result.URL,
			UpdatedAt:  result.UpdatedAt,
		})
	}
	return rows
}

func loadBugDetailCmd(session *runtimeadapter.Session, task forge.BugTask) tea.Cmd {
	key := task.BugID
	return guardSessionAction(session, frontend.ActionBugShow, func() tea.Msg {
		detail, err := session.Frontend.Bugs().Show(context.Background(), task.BugID)
		return bugDetailLoadedMsg{key: key, detail: detail, err: err}
	})
}

func loadBugDetailCmdIfSelected(session *runtimeadapter.Session, task *forge.BugTask) tea.Cmd {
	if task == nil {
		return nil
	}
	return loadBugDetailCmd(session, *task)
}

func loadReviewsCmd(session *runtimeadapter.Session, filters reviewsFilters) tea.Cmd {
	return guardSessionAction(session, frontend.ActionReviewList, func() tea.Msg {
		result, err := session.Frontend.Reviews().List(context.Background(), frontend.ReviewListRequest{
			Projects: firstNonEmptySlice(filters.project),
			Forges:   firstNonEmptySlice(filters.forge),
			State:    filters.state,
			Author:   filters.author,
			Since:    filters.since,
		})
		if err != nil {
			return reviewsLoadedMsg{err: err}
		}
		sort.Slice(result.MergeRequests, func(i, j int) bool {
			return result.MergeRequests[i].UpdatedAt.After(result.MergeRequests[j].UpdatedAt)
		})
		return reviewsLoadedMsg{rows: result.MergeRequests, warnings: result.Warnings}
	})
}

func loadReviewDetailCmd(session *runtimeadapter.Session, mr forge.MergeRequest) tea.Cmd {
	key := reviewDetailKey(mr)
	return guardSessionAction(session, frontend.ActionReviewShow, func() tea.Msg {
		detail, err := session.Frontend.Reviews().Show(context.Background(), mr.Repo, mr.ID)
		return reviewDetailLoadedMsg{key: key, detail: detail, err: err}
	})
}

func loadReviewDetailCmdIfSelected(session *runtimeadapter.Session, mr *forge.MergeRequest) tea.Cmd {
	if mr == nil {
		return nil
	}
	return loadReviewDetailCmd(session, *mr)
}

func loadCommitsCmd(session *runtimeadapter.Session, filters commitsFilters) tea.Cmd {
	actionID := frontend.ActionCommitLog
	if filters.mode == commitModeTrack {
		actionID = frontend.ActionCommitTrack
	}
	return guardSessionAction(session, actionID, func() tea.Msg {
		ctx := context.Background()
		if filters.mode == commitModeTrack && strings.TrimSpace(filters.bugID) == "" {
			return commitsLoadedMsg{prompt: "Enter a bug ID in filters to track matching commits."}
		}
		var (
			result *frontend.CommitListResponse
			err    error
		)
		if filters.mode == commitModeTrack {
			result, err = session.Frontend.Commits().Track(ctx, frontend.CommitTrackRequest{
				BugID:      filters.bugID,
				Projects:   firstNonEmptySlice(filters.project),
				Forges:     firstNonEmptySlice(filters.forge),
				Branch:     filters.branch,
				IncludeMRs: filters.includeMRs,
			})
		} else {
			result, err = session.Frontend.Commits().Log(ctx, frontend.CommitLogRequest{
				Projects:   firstNonEmptySlice(filters.project),
				Forges:     firstNonEmptySlice(filters.forge),
				Branch:     filters.branch,
				Author:     filters.author,
				IncludeMRs: filters.includeMRs,
			})
		}
		if err != nil {
			return commitsLoadedMsg{err: err}
		}
		sort.Slice(result.Commits, func(i, j int) bool {
			return result.Commits[i].Date.After(result.Commits[j].Date)
		})
		return commitsLoadedMsg{rows: result.Commits, warnings: result.Warnings}
	})
}

func loadProjectsCmd(session *runtimeadapter.Session, filters projectsFilters) tea.Cmd {
	return guardSessionAction(session, frontend.ActionConfigShow, func() tea.Msg {
		cfg, err := session.Frontend.Config().Show(context.Background())
		if err != nil {
			return projectsLoadedMsg{err: err}
		}
		return projectsLoadedMsg{
			config: cfg,
			rows:   summarizeProjects(cfg, filters),
		}
	})
}

func summarizeProjects(cfg *dto.Config, filters projectsFilters) []projectSummary {
	if cfg == nil {
		return nil
	}
	rows := make([]projectSummary, 0, len(cfg.Projects))
	for _, project := range cfg.Projects {
		effectiveSeries, effectiveFocus := projectSeriesAndFocus(cfg, project)
		row := projectSummary{
			Name:             project.Name,
			ArtifactType:     project.ArtifactType,
			CodeForge:        project.Code.Forge,
			CodeProject:      project.Code.Project,
			Series:           effectiveSeries,
			DevelopmentFocus: effectiveFocus,
			HasBuild:         project.Build != nil,
			HasRelease:       project.Release != nil,
			Config:           project,
		}
		for _, bug := range project.Bugs {
			row.BugForges = append(row.BugForges, bug.Forge)
			row.BugProjects = append(row.BugProjects, bug.Project)
			if bug.Group != "" {
				row.BugGroups = append(row.BugGroups, bug.Group)
			}
		}
		if !projectMatchesFilters(row, filters) {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

func projectSeriesAndFocus(cfg *dto.Config, project dto.ProjectConfig) ([]string, string) {
	series := append([]string(nil), project.Series...)
	if len(series) == 0 {
		series = append([]string(nil), cfg.Launchpad.Series...)
	}
	focus := project.DevelopmentFocus
	if focus == "" {
		focus = cfg.Launchpad.DevelopmentFocus
	}
	return series, focus
}

func projectMatchesFilters(row projectSummary, filters projectsFilters) bool {
	if filters.name != "" && !strings.Contains(row.Name, filters.name) {
		return false
	}
	if filters.artifactType != "" && row.ArtifactType != filters.artifactType {
		return false
	}
	if filters.codeForge != "" && row.CodeForge != filters.codeForge {
		return false
	}
	if filters.bugForge != "" && !containsString(row.BugForges, filters.bugForge) {
		return false
	}
	if !matchesBoolFilter(row.HasBuild, filters.hasBuild) {
		return false
	}
	if !matchesBoolFilter(row.HasRelease, filters.hasRelease) {
		return false
	}
	return true
}

func matchesBoolFilter(value bool, raw string) bool {
	switch strings.TrimSpace(raw) {
	case "", "any":
		return true
	case "true":
		return value
	case "false":
		return !value
	default:
		return true
	}
}

func parseExcuseAges(filters packagesFilters) (int, int, int, error) {
	minAge := 0
	maxAge := 0
	limit := 0
	var err error
	if strings.TrimSpace(filters.minAge) != "" {
		minAge, err = strconv.Atoi(strings.TrimSpace(filters.minAge))
		if err != nil {
			return 0, 0, 0, fmt.Errorf("min age must be an integer")
		}
	}
	if strings.TrimSpace(filters.maxAge) != "" {
		maxAge, err = strconv.Atoi(strings.TrimSpace(filters.maxAge))
		if err != nil {
			return 0, 0, 0, fmt.Errorf("max age must be an integer")
		}
	}
	if strings.TrimSpace(filters.limit) != "" {
		limit, err = strconv.Atoi(strings.TrimSpace(filters.limit))
		if err != nil {
			return 0, 0, 0, fmt.Errorf("limit must be an integer")
		}
	}
	return minAge, maxAge, limit, nil
}

func configuredExcusesTrackers(_ *runtimeadapter.Session) []string {
	return dto.KnownExcusesTrackers()
}

func packageBackports(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []string{value}
}

func selectedPackageInventory(rows []distro.SourcePackage, idx int) *distro.SourcePackage {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	row := rows[idx]
	return &row
}

func selectedPackageDiff(rows []dto.PackageDiffResult, idx int) *dto.PackageDiffResult {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	row := rows[idx]
	return &row
}

func selectedPackageExcuse(rows []dto.PackageExcuseSummary, idx int) *dto.PackageExcuseSummary {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	row := rows[idx]
	return &row
}

func selectedBug(rows []forge.BugTask, idx int) *forge.BugTask {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	row := rows[idx]
	return &row
}

func selectedReview(rows []forge.MergeRequest, idx int) *forge.MergeRequest {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	row := rows[idx]
	return &row
}

func selectedCommit(rows []forge.Commit, idx int) *forge.Commit {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	row := rows[idx]
	return &row
}

func selectedProject(rows []projectSummary, idx int) *projectSummary {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	row := rows[idx]
	return &row
}

func packageInventoryDetailKey(filters packagesFilters, row distro.SourcePackage) string {
	return strings.Join([]string{"inventory", filters.distro, filters.release, row.Package, row.Version, row.Suite}, "|")
}

func packageDiffDetailKey(filters packagesFilters, row dto.PackageDiffResult) string {
	return strings.Join([]string{"diff", filters.set, row.Package}, "|")
}

func packageExcuseDetailKey(row dto.PackageExcuseSummary) string {
	return strings.Join([]string{"excuse", row.Tracker, row.Package, row.Version}, "|")
}

func reviewDetailKey(mr forge.MergeRequest) string {
	return mr.Repo + "|" + mr.ID
}

func renderWarningsInline(t theme, warnings []string, width int) string {
	if len(warnings) == 0 {
		return ""
	}
	limit := minInt(3, len(warnings))
	lines := make([]string, 0, limit+1)
	for _, warning := range warnings[:limit] {
		lines = append(lines, t.errorText.Render("warning: "+fitLine(warning, width)))
	}
	if len(warnings) > limit {
		lines = append(lines, t.subtle.Render(fmt.Sprintf("(+%d more warnings)", len(warnings)-limit)))
	}
	return strings.Join(lines, "\n")
}

func renderPackages(theme theme, width int, model packagesModel) string {
	const gap = 1
	listWidth, detailWidth := splitColumns(width, gap)
	header := renderPackageModeTabs(theme, model.filters.mode) + "\n\n" + theme.panelTitle.Render("Filters") + "\n" + renderPackagesFilterSummary(model.filters)
	if warnings := renderPackagesPrompt(theme, model, innerPanelWidth(theme.panel, listWidth)); warnings != "" {
		header += "\n\n" + warnings
	}
	list := renderPackagesRows(theme, model, innerPanelWidth(theme.panel, listWidth))
	detail := renderPackagesDetail(theme, model, innerPanelWidth(theme.panel, detailWidth))
	if width >= 120 {
		left := renderPanel(theme.panel, listWidth, theme.panelTitle.Render("Packages"), header+"\n\n"+list)
		right := renderPanel(theme.panel, detailWidth, theme.panelTitle.Render("Detail"), detail)
		return lipJoin(left, right, gap)
	}
	return lipVertical(theme, width, header, list, detail, "Packages", "Detail")
}

func renderBugs(theme theme, width int, model bugsModel) string {
	const gap = 1
	listWidth, detailWidth := splitColumns(width, gap)
	header := theme.panelTitle.Render("Filters") + "\n"
	if strings.TrimSpace(model.filters.query) != "" {
		header += fmt.Sprintf("query=%q  mode=%s  fields=%s  fuzzy=%s  closed=%s  sort=%s\n",
			model.filters.query, defaultString(model.filters.mode, "text"),
			defaultString(model.filters.fields, "all"), defaultString(model.filters.fuzzy, "auto"),
			defaultString(model.filters.closed, "include"), defaultString(model.filters.sort, "relevance"))
	}
	header += fmt.Sprintf("project=%s  status=%s  importance=%s  assignee=%s  tag=%s  since=%s  merge=%t",
		emptyAsAny(model.filters.project), emptyAsAny(model.filters.status), emptyAsAny(model.filters.importance),
		emptyAsAny(model.filters.assignee), emptyAsAny(model.filters.tag), emptyAsAny(model.filters.since), model.filters.merge)
	if len(model.searchResults) > 0 || strings.TrimSpace(model.filters.query) != "" {
		header += fmt.Sprintf("\noutcome=%s  displayed=%d", model.searchOutcome, len(model.searchResults))
	}
	if warnings := renderWarningsInline(theme, model.warnings, innerPanelWidth(theme.panel, listWidth)); warnings != "" {
		header += "\n\n" + warnings
	}
	list := renderBugRows(theme, model.rows, model.index, innerPanelWidth(theme.panel, listWidth), model.loaded)
	detail := renderBugPane(theme, model.detail, innerPanelWidth(theme.panel, detailWidth))
	if model.index >= 0 && model.index < len(model.searchResults) {
		detail = renderBugSearchEvidence(theme, model.searchResults[model.index], innerPanelWidth(theme.panel, detailWidth)) + "\n\n" + detail
	}
	if width >= 120 {
		left := renderPanel(theme.panel, listWidth, theme.panelTitle.Render("Bugs"), header+"\n\n"+list)
		right := renderPanel(theme.panel, detailWidth, theme.panelTitle.Render("Detail"), detail)
		return lipJoin(left, right, gap)
	}
	return lipVertical(theme, width, header, list, detail, "Bugs", "Detail")
}

func renderBugSearchEvidence(t theme, result dto.BugSearchResult, width int) string {
	lines := []string{
		fmt.Sprintf("Search: %s  score=%.3f", result.Classification, result.Score),
		fmt.Sprintf("Coverage: %.0f%% (%d/%d clauses)",
			result.Coverage.Ratio*100,
			result.Coverage.MatchedClauses,
			result.Coverage.TotalClauses),
		"Reference: " + result.Reference,
	}
	if len(result.MatchedTasks) > 0 {
		lines = append(lines, "Matched tasks: "+strings.Join(result.MatchedTasks, ", "))
	}
	if len(result.Evidence) > 0 {
		lines = append(lines, "Evidence:")
		for _, evidence := range result.Evidence {
			excerpt := evidence.Excerpt
			if evidence.TruncatedBefore {
				excerpt = "…" + excerpt
			}
			if evidence.TruncatedAfter {
				excerpt += "…"
			}
			var matches []string
			for _, match := range evidence.Matches {
				matches = append(matches, fmt.Sprintf(
					"%s/%s", match.QueryConcept, match.MatchType))
			}
			lines = append(lines, fmt.Sprintf("- %s [%s]: %s",
				evidence.Field, strings.Join(matches, ", "), excerpt))
		}
		if result.EvidenceTruncated {
			lines = append(lines, fmt.Sprintf(
				"Showing %d of %d evidence excerpts.",
				len(result.Evidence), result.EvidenceTotal))
		}
	}
	return fitBlock(strings.Join(lines, "\n"), width)
}

func renderReviews(theme theme, width int, model reviewsModel) string {
	const gap = 1
	listWidth, detailWidth := splitColumns(width, gap)
	header := theme.panelTitle.Render("Filters") + "\n" +
		fmt.Sprintf("project=%s  forge=%s  state=%s  author=%s  since=%s",
			emptyAsAny(model.filters.project), emptyAsAny(model.filters.forge), emptyAsAny(model.filters.state),
			emptyAsAny(model.filters.author), emptyAsAny(model.filters.since))
	if warnings := renderWarningsInline(theme, model.warnings, innerPanelWidth(theme.panel, listWidth)); warnings != "" {
		header += "\n\n" + warnings
	}
	list := renderReviewRows(theme, model.rows, model.index, innerPanelWidth(theme.panel, listWidth), model.loaded)
	detail := renderReviewPane(theme, model.detail, model.detailErr, innerPanelWidth(theme.panel, detailWidth))
	if width >= 120 {
		left := renderPanel(theme.panel, listWidth, theme.panelTitle.Render("Reviews"), header+"\n\n"+list)
		right := renderPanel(theme.panel, detailWidth, theme.panelTitle.Render("Detail"), detail)
		return lipJoin(left, right, gap)
	}
	return lipVertical(theme, width, header, list, detail, "Reviews", "Detail")
}

func renderCommits(theme theme, width int, model commitsModel) string {
	const gap = 1
	listWidth, detailWidth := splitColumns(width, gap)
	header := theme.panelTitle.Render("Filters") + "\n" +
		fmt.Sprintf("mode=%s  project=%s  forge=%s  branch=%s  author=%s  include mrs=%t  bug=%s",
			commitModeName(model.filters.mode), emptyAsAny(model.filters.project), emptyAsAny(model.filters.forge),
			emptyAsAny(model.filters.branch), emptyAsAny(model.filters.author), model.filters.includeMRs, emptyAsAny(model.filters.bugID))
	if model.prompt != "" {
		header += "\n\n" + theme.subtle.Render(model.prompt)
	}
	if warnings := renderWarningsInline(theme, model.warnings, innerPanelWidth(theme.panel, listWidth)); warnings != "" {
		header += "\n\n" + warnings
	}
	list := renderCommitRows(theme, model.rows, model.index, innerPanelWidth(theme.panel, listWidth), model.loaded)
	detail := renderCommitPane(theme, selectedCommit(model.rows, model.index), innerPanelWidth(theme.panel, detailWidth))
	if width >= 120 {
		left := renderPanel(theme.panel, listWidth, theme.panelTitle.Render("Commits / "+commitModeName(model.filters.mode)), header+"\n\n"+list)
		right := renderPanel(theme.panel, detailWidth, theme.panelTitle.Render("Detail"), detail)
		return lipJoin(left, right, gap)
	}
	return lipVertical(theme, width, header, list, detail, "Commits", "Detail")
}

func renderProjects(theme theme, width int, model projectsModel) string {
	const gap = 1
	listWidth, detailWidth := splitColumns(width, gap)
	header := theme.panelTitle.Render("Filters") + "\n" +
		fmt.Sprintf("name=%s  type=%s  code forge=%s  bug forge=%s  has build=%s  has release=%s",
			emptyAsAny(model.filters.name), emptyAsAny(model.filters.artifactType), emptyAsAny(model.filters.codeForge),
			emptyAsAny(model.filters.bugForge), emptyAsAny(model.filters.hasBuild), emptyAsAny(model.filters.hasRelease))
	list := renderProjectRows(theme, model.rows, model.index, innerPanelWidth(theme.panel, listWidth), model.loaded)
	detail := renderProjectPane(theme, selectedProject(model.rows, model.index), innerPanelWidth(theme.panel, detailWidth))
	if width >= 120 {
		left := renderPanel(theme.panel, listWidth, theme.panelTitle.Render("Projects"), header+"\n\n"+list)
		right := renderPanel(theme.panel, detailWidth, theme.panelTitle.Render("Detail"), detail)
		return lipJoin(left, right, gap)
	}
	return lipVertical(theme, width, header, list, detail, "Projects", "Detail")
}

func renderPackagesFilterSummary(filters packagesFilters) string {
	switch filters.mode {
	case packageModeDiff:
		return fmt.Sprintf("mode=Diff  set=%s  distro=%s  release=%s  suite=%s  backport=%s  merge=%t  upstream=%s  behind=%t  only-in=%s  constraints=%s",
			emptyAsAny(filters.set), emptyAsAny(filters.distro), emptyAsAny(filters.release), emptyAsAny(filters.suite),
			emptyAsAny(filters.backport), filters.merge, emptyAsAny(filters.upstreamRelease), filters.behindUpstream,
			emptyAsAny(filters.onlyIn), emptyAsAny(filters.constraints))
	case packageModeExcuses:
		return fmt.Sprintf("mode=Excuses  tracker=%s  name=%s  component=%s  team=%s  set=%s  blocked-by-set=%s  ftbfs=%t  autopkgtest=%t  blocked-by=%s  bugged=%t",
			emptyAsAny(filters.tracker), emptyAsAny(filters.name), emptyAsAny(filters.component), emptyAsAny(filters.team),
			emptyAsAny(filters.excuseSet), emptyAsAny(filters.blockedBySet),
			filters.ftbfs, filters.autopkgtest, emptyAsAny(filters.blockedBy), filters.bugged)
	default:
		return fmt.Sprintf("mode=Inventory  distro=%s  release=%s  suite=%s  component=%s  backport=%s",
			emptyAsAny(filters.distro), emptyAsAny(filters.release), emptyAsAny(filters.suite), emptyAsAny(filters.component), emptyAsAny(filters.backport))
	}
}

func renderPackageModeTabs(t theme, active packageMode) string {
	tabs := []string{
		renderInlineModeTab(t, "Inventory", active == packageModeInventory),
		renderInlineModeTab(t, "Diff", active == packageModeDiff),
		renderInlineModeTab(t, "Excuses", active == packageModeExcuses),
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func renderInlineModeTab(t theme, label string, active bool) string {
	if active {
		return t.tabActive.Render(label)
	}
	return t.tab.Render(label)
}

func renderPackagesPrompt(t theme, model packagesModel, width int) string {
	if model.prompt == "" {
		return ""
	}
	return t.subtle.Render(fitLine(model.prompt, width))
}

func renderPackagesRows(t theme, model packagesModel, width int) string {
	switch model.filters.mode {
	case packageModeDiff:
		if len(model.diffRows) == 0 {
			if !model.loaded {
				return t.subtle.Render("Loading packages...")
			}
			return t.subtle.Render("No package diff rows.")
		}
		lines := make([]string, 0, len(model.diffRows))
		for i, row := range model.diffRows {
			line := fitLine(fmt.Sprintf("%-24s  %s", row.Package, compactDiffSummary(row)), width)
			if i == model.index {
				line = t.selectedRow.Render(line)
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")
	case packageModeExcuses:
		if len(model.excuseRows) == 0 {
			if !model.loaded {
				return t.subtle.Render("Loading excuses...")
			}
			return t.subtle.Render("No excuses.")
		}
		lines := make([]string, 0, len(model.excuseRows))
		for i, row := range model.excuseRows {
			line := fitLine(fmt.Sprintf("%-10s  %-22s  %-12s  %s", row.Tracker, row.Package, emptyAsDash(row.Verdict), emptyAsDash(row.PrimaryReason)), width)
			if i == model.index {
				line = t.selectedRow.Render(line)
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")
	default:
		if len(model.inventoryRows) == 0 {
			if !model.loaded {
				return t.subtle.Render("Loading packages...")
			}
			return t.subtle.Render("No packages.")
		}
		lines := make([]string, 0, len(model.inventoryRows))
		for i, row := range model.inventoryRows {
			line := fitLine(fmt.Sprintf("%-24s  %-18s  %-18s  %s", row.Package, row.Version, row.Suite, row.Component), width)
			if i == model.index {
				line = t.selectedRow.Render(line)
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")
	}
}

func renderPackagesDetail(t theme, model packagesModel, width int) string {
	switch model.filters.mode {
	case packageModeDiff:
		if model.diffDetail == nil {
			return t.subtle.Render("Select a package to load its version matrix.")
		}
		return fitBlock(renderDiffDetailText(model.diffDetail), width)
	case packageModeExcuses:
		if model.excuseDetail == nil {
			return t.subtle.Render("Select an excuse to load its blockers.")
		}
		return fitBlock(renderExcuseDetailText(t, model.excuseDetail), width)
	default:
		if model.inventoryDetail == nil {
			return t.subtle.Render("Select a package to load metadata.")
		}
		return fitBlock(renderPackageInfoText(model.inventoryDetail), width)
	}
}

func renderBugRows(t theme, rows []forge.BugTask, selected int, width int, loaded bool) string {
	if len(rows) == 0 {
		if !loaded {
			return t.subtle.Render("Loading bugs...")
		}
		return t.subtle.Render("No bug tasks.")
	}
	const (
		projectColWidth    = 18
		bugIDColWidth      = 9
		statusColWidth     = 14
		importanceColWidth = 10
	)
	lines := make([]string, 0, len(rows))
	for i, row := range rows {
		rowWidth := width
		if rowWidth > 2 {
			rowWidth -= 2
		} else if rowWidth > 1 {
			rowWidth--
		}
		prefix := padRight(truncateToWidth(row.Project, projectColWidth), projectColWidth) +
			"  " + padRight(truncateToWidth("#"+row.BugID, bugIDColWidth), bugIDColWidth) +
			"  " + padRight(truncateToWidth(row.Status, statusColWidth), statusColWidth) +
			"  " + padRight(truncateToWidth(emptyAsDash(row.Importance), importanceColWidth), importanceColWidth)
		titleWidth := maxInt(1, rowWidth-lipgloss.Width(prefix)-2)
		title := truncateToWidth(cleanBugListTitle(firstLine(row.Title), row.BugID, row.Project), titleWidth)
		line := prefix + "  " + title
		line = fitLine(line, rowWidth)
		line = strings.TrimRight(line, " ")
		if i == selected {
			line = t.selectedRow.Width(rowWidth).MaxWidth(rowWidth).Render(line)
		} else {
			line = lipgloss.NewStyle().Width(rowWidth).MaxWidth(rowWidth).Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func stripBugListTitlePrefix(title, bugID, project string) string {
	title = strings.TrimSpace(title)
	if title == "" || bugID == "" {
		return title
	}

	genericPrefix := fmt.Sprintf("Bug #%s in ", bugID)
	if strings.HasPrefix(title, genericPrefix) {
		if idx := strings.Index(title[len(genericPrefix):], ": "); idx >= 0 {
			return strings.TrimSpace(title[len(genericPrefix)+idx+2:])
		}
	}

	prefixes := []string{
		fmt.Sprintf("Bug #%s in %s: ", bugID, project),
		fmt.Sprintf("Bug #%s in %s: ", bugID, humanizeBugProject(project)),
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(title, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(title, prefix))
		}
	}
	return title
}

func cleanBugListTitle(title, bugID, project string) string {
	title = stripBugListTitlePrefix(title, bugID, project)
	title = strings.TrimSpace(title)
	if len(title) >= 2 && strings.HasPrefix(title, "\"") && strings.HasSuffix(title, "\"") {
		title = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(title, "\""), "\""))
	}
	return title
}

func humanizeBugProject(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return project
	}
	parts := strings.FieldsFunc(project, func(r rune) bool {
		return r == '-' || r == '_' || r == '/'
	})
	if len(parts) == 0 {
		return project
	}
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func renderBugPane(t theme, bug *forge.Bug, width int) string {
	if bug == nil {
		return t.subtle.Render("Select a bug to load its details.")
	}
	lines := []string{
		"Bug: #" + bug.ID,
		"Title: " + bug.Title,
		"Owner: " + emptyAsDash(bug.Owner),
		"Created: " + emptyTime(bug.CreatedAt),
		"Updated: " + emptyTime(bug.UpdatedAt),
		"URL: " + emptyAsDash(bug.URL),
	}
	if len(bug.Tags) > 0 {
		lines = append(lines, "Tags: "+strings.Join(bug.Tags, ", "))
	}
	if bug.Description != "" {
		lines = append(lines, "", "Description:", bug.Description)
	}
	if len(bug.Tasks) > 0 {
		lines = append(lines, "", "Tasks:")
		for _, task := range bug.Tasks {
			lines = append(lines, fmt.Sprintf("- %s  %s  %s  %s", task.Project, task.Status, emptyAsDash(task.Importance), emptyAsDash(task.Assignee)))
		}
	}
	return fitBlock(strings.Join(lines, "\n"), width)
}

func renderReviewRows(t theme, rows []forge.MergeRequest, selected int, width int, loaded bool) string {
	if len(rows) == 0 {
		if !loaded {
			return t.subtle.Render("Loading reviews...")
		}
		return t.subtle.Render("No reviews.")
	}
	const (
		repoColWidth   = 22
		forgeColWidth  = 10
		stateColWidth  = 10
		authorColWidth = 12
	)
	lines := make([]string, 0, len(rows))
	for i, row := range rows {
		rowWidth := width
		if rowWidth > 2 {
			rowWidth -= 2
		} else if rowWidth > 1 {
			rowWidth--
		}
		prefix := padRight(truncateToWidth(row.Repo, repoColWidth), repoColWidth) +
			"  " + padRight(truncateToWidth(row.Forge.String(), forgeColWidth), forgeColWidth) +
			"  " + padRight(truncateToWidth(row.State.String(), stateColWidth), stateColWidth) +
			"  " + padRight(truncateToWidth(row.Author, authorColWidth), authorColWidth)
		titleWidth := maxInt(1, rowWidth-lipgloss.Width(prefix)-2)
		title := truncateToWidth(firstLine(row.Title), titleWidth)
		line := prefix + "  " + title
		line = fitLine(line, rowWidth)
		line = strings.TrimRight(line, " ")
		if i == selected {
			line = t.selectedRow.Width(rowWidth).MaxWidth(rowWidth).Render(line)
		} else {
			line = lipgloss.NewStyle().Width(rowWidth).MaxWidth(rowWidth).Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderReviewPane(t theme, mr *forge.MergeRequest, detailErr string, width int) string {
	if detailErr != "" {
		return t.errorText.Render(fitLine(detailErr, width))
	}
	if mr == nil {
		return t.subtle.Render("Select a review to load its details.")
	}
	lines := []string{
		"Project: " + mr.Repo,
		"Forge: " + mr.Forge.String(),
		"ID: " + mr.ID,
		"Title: " + mr.Title,
		"Author: " + mr.Author,
		"State: " + mr.State.String(),
		"Review: " + mr.ReviewState.String(),
		"Source: " + mr.SourceBranch,
		"Target: " + mr.TargetBranch,
		"Created: " + emptyTime(mr.CreatedAt),
		"Updated: " + emptyTime(mr.UpdatedAt),
		"URL: " + emptyAsDash(mr.URL),
	}
	if len(mr.Checks) > 0 {
		lines = append(lines, "", "Checks:")
		for _, check := range mr.Checks {
			lines = append(lines, fmt.Sprintf("- %s  %s  %s", check.State.String(), check.Name, emptyAsDash(check.URL)))
		}
	}
	if mr.Description != "" {
		lines = append(lines, "", "Description:", mr.Description)
	}
	if len(mr.Comments) > 0 {
		lines = append(lines, "", "Comments:")
		for _, comment := range mr.Comments {
			header := fmt.Sprintf("- [%s] %s", comment.Kind, emptyAsDash(comment.Author))
			if comment.File != "" {
				header += " " + comment.File
				if comment.Line > 0 {
					header += fmt.Sprintf(":%d", comment.Line)
				}
			}
			lines = append(lines, header)
			if comment.Body != "" {
				lines = append(lines, "  "+strings.ReplaceAll(comment.Body, "\n", "\n  "))
			}
		}
	}
	if len(mr.Files) > 0 {
		lines = append(lines, "", "Files:")
		for _, file := range mr.Files {
			lines = append(lines, fmt.Sprintf("- %s  %s  +%d -%d", file.Path, emptyAsDash(file.Status), file.Additions, file.Deletions))
		}
	}
	if mr.DiffText != "" {
		lines = append(lines, "", "Diff:", mr.DiffText)
	}
	return fitBlock(strings.Join(lines, "\n"), width)
}

func renderCommitRows(t theme, rows []forge.Commit, selected int, width int, loaded bool) string {
	if len(rows) == 0 {
		if !loaded {
			return t.subtle.Render("Loading commits...")
		}
		return t.subtle.Render("No commits.")
	}
	const (
		repoColWidth   = 22
		shaColWidth    = 10
		dateColWidth   = 12
		authorColWidth = 16
	)
	lines := make([]string, 0, len(rows))
	for i, row := range rows {
		rowWidth := width
		if rowWidth > 2 {
			rowWidth -= 2
		} else if rowWidth > 1 {
			rowWidth--
		}
		prefix := padRight(truncateToWidth(row.Repo, repoColWidth), repoColWidth) +
			"  " + padRight(truncateToWidth(shortSHA(row.SHA), shaColWidth), shaColWidth) +
			"  " + padRight(truncateToWidth(formatListTime(row.Date), dateColWidth), dateColWidth) +
			"  " + padRight(truncateToWidth(row.Author, authorColWidth), authorColWidth)
		messageWidth := maxInt(1, rowWidth-lipgloss.Width(prefix)-2)
		message := truncateToWidth(firstLine(row.Message), messageWidth)
		line := prefix + "  " + message
		line = fitLine(line, rowWidth)
		line = strings.TrimRight(line, " ")
		if i == selected {
			line = t.selectedRow.Width(rowWidth).MaxWidth(rowWidth).Render(line)
		} else {
			line = lipgloss.NewStyle().Width(rowWidth).MaxWidth(rowWidth).Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderCommitPane(t theme, commit *forge.Commit, width int) string {
	if commit == nil {
		return t.subtle.Render("Select a commit to inspect it.")
	}
	lines := []string{
		"Project: " + commit.Repo,
		"Forge: " + commit.Forge.String(),
		"SHA: " + commit.SHA,
		"Author: " + commit.Author,
		"Date: " + emptyTime(commit.Date),
		"URL: " + emptyAsDash(commit.URL),
		"",
		"Message:",
		commit.Message,
	}
	if len(commit.BugRefs) > 0 {
		refs := make([]string, 0, len(commit.BugRefs))
		for _, ref := range commit.BugRefs {
			refs = append(refs, ref.ID)
		}
		lines = append(lines, "", "Bug refs: "+strings.Join(refs, ", "))
	}
	if commit.MergeRequest != nil {
		lines = append(lines, "", "Merge request:", fmt.Sprintf("%s  %s", commit.MergeRequest.ID, commit.MergeRequest.State.String()), emptyAsDash(commit.MergeRequest.URL))
	}
	return fitBlock(strings.Join(lines, "\n"), width)
}

func renderProjectRows(t theme, rows []projectSummary, selected int, width int, loaded bool) string {
	if len(rows) == 0 {
		if !loaded {
			return t.subtle.Render("Loading projects...")
		}
		return t.subtle.Render("No configured projects.")
	}
	lines := make([]string, 0, len(rows))
	for i, row := range rows {
		series := "-"
		if len(row.Series) > 0 {
			series = strings.Join(row.Series, ",")
		}
		line := fitLine(fmt.Sprintf("%-24s  %-8s  %-10s  %s", row.Name, emptyAsDash(row.ArtifactType), row.CodeForge, seriesSummary(series, row.DevelopmentFocus)), width)
		if i == selected {
			line = t.selectedRow.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderProjectPane(t theme, project *projectSummary, width int) string {
	if project == nil {
		return t.subtle.Render("Select a project to inspect its config.")
	}
	lines := []string{
		"Name: " + project.Name,
		"Artifact type: " + emptyAsDash(project.ArtifactType),
		"Code forge: " + project.CodeForge,
		"Code project: " + project.CodeProject,
		"Code owner: " + emptyAsDash(project.Config.Code.Owner),
		"Code host: " + emptyAsDash(project.Config.Code.Host),
		"Git URL: " + emptyAsDash(project.Config.Code.GitURL),
		"Series: " + emptyAsDash(strings.Join(project.Series, ", ")),
		"Development focus: " + emptyAsDash(project.DevelopmentFocus),
		fmt.Sprintf("Build configured: %t", project.HasBuild),
		fmt.Sprintf("Release configured: %t", project.HasRelease),
	}
	if len(project.Config.Bugs) > 0 {
		lines = append(lines, "", "Bug trackers:")
		for _, bug := range project.Config.Bugs {
			line := fmt.Sprintf("- %s  %s", bug.Forge, bug.Project)
			if bug.Group != "" {
				line += "  group=" + bug.Group
			}
			lines = append(lines, line)
		}
	}
	if project.Config.Build != nil {
		lines = append(lines, "", "Build:")
		lines = append(lines,
			"  owner="+emptyAsDash(project.Config.Build.Owner),
			"  artifacts="+emptyAsDash(strings.Join(project.Config.Build.Artifacts, ", ")),
			"  prepare_command="+emptyAsDash(project.Config.Build.PrepareCommand),
		)
	}
	if project.Config.Release != nil {
		lines = append(lines, "", "Release:")
		lines = append(lines,
			"  tracks="+emptyAsDash(strings.Join(project.Config.Release.Tracks, ", ")),
			"  target_profile="+emptyAsDash(project.Config.Release.TargetProfile),
		)
		if len(project.Config.Release.SkipArtifacts) > 0 {
			lines = append(lines, "  skip_artifacts="+strings.Join(project.Config.Release.SkipArtifacts, ", "))
		}
	}
	return fitBlock(strings.Join(lines, "\n"), width)
}

func renderPackageInfoText(info *distro.SourcePackageInfo) string {
	lines := []string{
		"Package: " + info.Package,
		"Version: " + info.Version,
		"Suite: " + info.Suite,
		"Component: " + info.Component,
	}
	if len(info.Fields) > 0 {
		lines = append(lines, "", "Fields:")
		for _, field := range info.Fields {
			lines = append(lines, fmt.Sprintf("- %s: %s", field.Key, field.Value))
		}
	}
	return strings.Join(lines, "\n")
}

func renderDiffDetailText(detail *frontend.PackagesShowVersionResponse) string {
	lines := []string{"Package: " + detail.Result.Package}
	if detail.Result.Upstream != "" {
		lines = append(lines, "Upstream: "+detail.Result.Upstream)
	}
	lines = append(lines, "", "Versions:")
	sources := sortedMapKeys(detail.Result.Versions)
	for _, source := range sources {
		for _, pkg := range detail.Result.Versions[source] {
			lines = append(lines, fmt.Sprintf("- %s  %s  %s", source, pkg.Suite, pkg.Version))
		}
	}
	return strings.Join(lines, "\n")
}

func renderExcuseDetailText(t theme, detail *dto.PackageExcuse) string {
	lines := []string{
		t.key.Render("Tracker:") + " " + detail.Tracker,
		t.key.Render("Package:") + " " + t.project.Render(detail.Package),
		t.key.Render("Version:") + " " + detail.Version,
		t.key.Render("Verdict:") + " " + t.semantic(emptyAsDash(detail.Verdict)),
		t.key.Render("Reason:") + " " + t.semantic(emptyAsDash(detail.PrimaryReason)),
	}
	if len(detail.Reasons) > 0 {
		lines = append(lines, "", t.panelTitle.Render("Reasons:"))
		for _, reason := range detail.Reasons {
			lines = append(lines, fmt.Sprintf("- %s  %s", t.semantic(reason.Code), emptyAsDash(reason.Message)))
		}
	}
	if len(detail.Dependencies) > 0 {
		lines = append(lines, "", t.panelTitle.Render("Dependencies:"))
		for _, dep := range detail.Dependencies {
			lines = append(lines, fmt.Sprintf("- %s  %s", t.metadata.Render(dep.Kind), dep.Package))
		}
	}
	if len(detail.Autopkgtests) > 0 {
		lines = append(lines, "", t.panelTitle.Render("Autopkgtests:"))
		lines = append(lines, renderExcuseAutopkgtests(t, detail.Autopkgtests)...)
	}
	if len(detail.Messages) > 0 {
		lines = append(lines, "", t.panelTitle.Render("Messages:"))
		for _, message := range detail.Messages {
			lines = append(lines, "- "+t.subtle.Render(message))
		}
	}
	return strings.Join(lines, "\n")
}

func renderExcuseAutopkgtests(t theme, tests []dto.ExcuseAutopkgtest) []string {
	// Group by Package, preserving insertion order.
	var pkgOrder []string
	pkgMap := map[string][]dto.ExcuseAutopkgtest{}
	for _, test := range tests {
		pkg := test.Package
		if _, seen := pkgMap[pkg]; !seen {
			pkgOrder = append(pkgOrder, pkg)
		}
		pkgMap[pkg] = append(pkgMap[pkg], test)
	}

	var lines []string
	for _, pkg := range pkgOrder {
		entries := pkgMap[pkg]

		// Fallback entries: plain-text messages without structured fields.
		if len(entries) > 0 && entries[0].Architecture == "" {
			for _, e := range entries {
				lines = append(lines, "  - "+t.subtle.Render(e.Message))
			}
			continue
		}

		parts := make([]string, 0, len(entries))
		for _, e := range entries {
			arch := emptyAsDash(e.Architecture)
			status := t.autopkgtestStatus(e.Status)
			parts = append(parts, arch+"="+status)
		}
		label := pkg
		if label == "" {
			label = "(unknown)"
		}
		lines = append(lines, "  "+t.project.Render(label)+": "+strings.Join(parts, "  "))
	}
	return lines
}

func compactDiffSummary(row dto.PackageDiffResult) string {
	if len(row.Versions) == 0 {
		return emptyAsDash(row.Upstream)
	}
	source := sortedMapKeys(row.Versions)[0]
	pkgs := row.Versions[source]
	if len(pkgs) == 0 {
		return source
	}
	return fmt.Sprintf("%s %s", source, pkgs[0].Version)
}

func sortedMapKeys[M ~map[string]V, V any](m M) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func shortSHA(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}

func firstLine(text string) string {
	text = strings.ReplaceAll(text, "\t", " ")
	if idx := strings.Index(text, "\n"); idx >= 0 {
		text = text[:idx]
	}
	return text
}

func seriesSummary(series, focus string) string {
	if focus == "" {
		return series
	}
	return series + "  focus=" + focus
}

func lipJoin(left, right string, gap int) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer(gap), right)
}

func lipVertical(theme theme, width int, header, list, detail, listTitle, detailTitle string) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		renderPanel(theme.panel, width, "", header),
		renderPanel(theme.panel, width, theme.panelTitle.Render(listTitle), list),
		renderPanel(theme.panel, width, theme.panelTitle.Render(detailTitle), detail),
	)
}

func packageModeName(mode packageMode) string {
	switch mode {
	case packageModeDiff:
		return "Diff"
	case packageModeExcuses:
		return "Excuses"
	default:
		return "Inventory"
	}
}

func commitModeName(mode commitMode) string {
	if mode == commitModeTrack {
		return "Track"
	}
	return "Log"
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func packageFilterSuggestions(session *runtimeadapter.Session, model packagesModel) packageFilterOptions {
	opts := packageFilterOptions{
		backports: []string{"none"},
	}
	if session != nil && session.Config != nil {
		if cfg := session.Config.LocalConfig(); cfg != nil {
			for name := range cfg.Packages.Sets {
				opts.sets = append(opts.sets, name)
			}
			for name := range cfg.Packages.LaunchpadSets {
				opts.sets = append(opts.sets, name)
			}
			for distroName, distroCfg := range cfg.Packages.Distros {
				opts.distros = append(opts.distros, distroName)
				opts.components = append(opts.components, distroCfg.Components...)
				for releaseName, releaseCfg := range distroCfg.Releases {
					opts.releases = append(opts.releases, releaseName)
					opts.suites = append(opts.suites, releaseCfg.Suites...)
					for backport := range releaseCfg.Backports {
						opts.backports = append(opts.backports, backport)
					}
				}
			}
		}
		opts.trackers = append(opts.trackers, configuredExcusesTrackers(session)...)
	}
	for _, row := range model.inventoryRows {
		opts.names = append(opts.names, row.Package)
		opts.components = append(opts.components, row.Component)
		opts.suites = append(opts.suites, row.Suite)
	}
	for _, row := range model.diffRows {
		opts.names = append(opts.names, row.Package)
	}
	for _, row := range model.excuseRows {
		opts.names = append(opts.names, row.Package)
		opts.trackers = append(opts.trackers, row.Tracker)
		opts.teams = append(opts.teams, row.Team)
		opts.components = append(opts.components, row.Component)
	}
	opts.sets = uniqueSortedStrings(opts.sets...)
	opts.distros = uniqueSortedStrings(opts.distros...)
	opts.releases = uniqueSortedStrings(opts.releases...)
	opts.suites = uniqueSortedStrings(opts.suites...)
	opts.components = uniqueSortedStrings(opts.components...)
	opts.backports = uniqueSortedStrings(opts.backports...)
	opts.trackers = uniqueSortedStrings(opts.trackers...)
	opts.names = uniqueSortedStrings(opts.names...)
	opts.teams = uniqueSortedStrings(opts.teams...)
	return opts
}

func bugFilterSuggestions(session *runtimeadapter.Session, model bugsModel) bugFilterOptions {
	opts := bugFilterOptions{
		statuses:    []string{"New", "Incomplete", "Opinion", "Invalid", "Won't Fix", "Expired", "Confirmed", "Triaged", "In Progress", "Fix Committed", "Fix Released", "Does Not Exist", "Deferred"},
		importances: []string{"Critical", "High", "Medium", "Low", "Wishlist", "Undecided"},
	}
	if session != nil && session.Config != nil {
		if cfg := session.Config.LocalConfig(); cfg != nil {
			for _, project := range cfg.Projects {
				opts.projects = append(opts.projects, project.Name)
			}
		}
	}
	for _, row := range model.rows {
		opts.projects = append(opts.projects, row.Project)
		opts.statuses = append(opts.statuses, row.Status)
		opts.importances = append(opts.importances, row.Importance)
		opts.assignees = append(opts.assignees, row.Assignee)
		opts.tags = append(opts.tags, row.Tags...)
	}
	opts.projects = uniqueSortedStrings(opts.projects...)
	opts.statuses = uniqueSortedStrings(opts.statuses...)
	opts.importances = uniqueSortedStrings(opts.importances...)
	opts.assignees = uniqueSortedStrings(opts.assignees...)
	opts.tags = uniqueSortedStrings(opts.tags...)
	return opts
}

func reviewFilterSuggestions(session *runtimeadapter.Session, model reviewsModel) reviewFilterOptions {
	opts := reviewFilterOptions{forges: []string{"github", "launchpad", "gerrit"}, states: []string{"open", "merged", "closed", "wip", "abandoned"}}
	if session != nil && session.Config != nil {
		if cfg := session.Config.LocalConfig(); cfg != nil {
			for _, project := range cfg.Projects {
				opts.projects = append(opts.projects, project.Name)
			}
		}
	}
	for _, row := range model.rows {
		opts.projects = append(opts.projects, row.Repo)
		opts.forges = append(opts.forges, row.Forge.String())
		opts.authors = append(opts.authors, row.Author)
		opts.states = append(opts.states, row.State.String())
	}
	opts.projects = uniqueSortedStrings(opts.projects...)
	opts.forges = uniqueSortedStrings(opts.forges...)
	opts.authors = uniqueSortedStrings(opts.authors...)
	opts.states = uniqueSortedStrings(opts.states...)
	return opts
}

func commitFilterSuggestions(session *runtimeadapter.Session, model commitsModel) commitFilterOptions {
	opts := commitFilterOptions{forges: []string{"github", "launchpad", "gerrit"}}
	if session != nil && session.Config != nil {
		if cfg := session.Config.LocalConfig(); cfg != nil {
			for _, project := range cfg.Projects {
				opts.projects = append(opts.projects, project.Name)
			}
		}
	}
	for _, row := range model.rows {
		opts.projects = append(opts.projects, row.Repo)
		opts.forges = append(opts.forges, row.Forge.String())
		opts.authors = append(opts.authors, row.Author)
	}
	opts.projects = uniqueSortedStrings(opts.projects...)
	opts.forges = uniqueSortedStrings(opts.forges...)
	opts.authors = uniqueSortedStrings(opts.authors...)
	opts.branches = uniqueSortedStrings(opts.branches...)
	return opts
}

func projectFilterSuggestions(model projectsModel) projectFilterOptions {
	opts := projectFilterOptions{bools: []string{"any", "true", "false"}}
	for _, row := range model.rows {
		opts.names = append(opts.names, row.Name)
		opts.artifactTypes = append(opts.artifactTypes, row.ArtifactType)
		opts.codeForges = append(opts.codeForges, row.CodeForge)
		opts.bugForges = append(opts.bugForges, row.BugForges...)
	}
	opts.names = uniqueSortedStrings(opts.names...)
	opts.artifactTypes = uniqueSortedStrings(opts.artifactTypes...)
	opts.codeForges = uniqueSortedStrings(opts.codeForges...)
	opts.bugForges = uniqueSortedStrings(opts.bugForges...)
	return opts
}

func (m rootModel) renderPackages() string { return renderPackages(m.theme, m.width, m.packages) }
func (m rootModel) renderBugs() string     { return renderBugs(m.theme, m.width, m.bugs) }
func (m rootModel) renderReviews() string  { return renderReviews(m.theme, m.width, m.reviews) }
func (m rootModel) renderCommits() string  { return renderCommits(m.theme, m.width, m.commits) }
func (m rootModel) renderProjects() string { return renderProjects(m.theme, m.width, m.projects) }

func (m packagesModel) rowCount() int {
	switch m.filters.mode {
	case packageModeDiff:
		return len(m.diffRows)
	case packageModeExcuses:
		return len(m.excuseRows)
	default:
		return len(m.inventoryRows)
	}
}

func (m *packagesModel) clearDetails() {
	m.inventoryDetail = nil
	m.diffDetail = nil
	m.excuseDetail = nil
}

func (m *packagesModel) clearStaleDetail() {
	switch m.filters.mode {
	case packageModeDiff:
		if selectedPackageDiff(m.diffRows, m.index) == nil {
			m.diffDetail = nil
		}
		m.inventoryDetail = nil
		m.excuseDetail = nil
	case packageModeExcuses:
		if selectedPackageExcuse(m.excuseRows, m.index) == nil {
			m.excuseDetail = nil
		}
		m.inventoryDetail = nil
		m.diffDetail = nil
	default:
		if selectedPackageInventory(m.inventoryRows, m.index) == nil {
			m.inventoryDetail = nil
		}
		m.diffDetail = nil
		m.excuseDetail = nil
	}
}

func (m rootModel) packageDetailKey() string {
	switch m.packages.filters.mode {
	case packageModeDiff:
		if row := selectedPackageDiff(m.packages.diffRows, m.packages.index); row != nil {
			return packageDiffDetailKey(m.packages.filters, *row)
		}
	case packageModeExcuses:
		if row := selectedPackageExcuse(m.packages.excuseRows, m.packages.index); row != nil {
			return packageExcuseDetailKey(*row)
		}
	default:
		if row := selectedPackageInventory(m.packages.inventoryRows, m.packages.index); row != nil {
			return packageInventoryDetailKey(m.packages.filters, *row)
		}
	}
	return ""
}

func (m rootModel) bugDetailKey() string {
	if task := selectedBug(m.bugs.rows, m.bugs.index); task != nil {
		return task.BugID
	}
	return ""
}

func (m rootModel) reviewDetailKey() string {
	if review := selectedReview(m.reviews.rows, m.reviews.index); review != nil {
		return reviewDetailKey(*review)
	}
	return ""
}

func clampIndex(idx, size int) int {
	if size <= 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= size {
		return size - 1
	}
	return idx
}

func newPackageFilterForm(session *runtimeadapter.Session, model packagesModel) formModalModel {
	s := packageFilterSuggestions(session, model)
	switch model.filters.mode {
	case packageModeDiff:
		return newFormModal("Package Filters / Diff", []fieldDef{
			{placeholder: "set", value: model.filters.set, resetValue: model.defaults.set, suggestions: s.sets},
			{placeholder: "distro", value: model.filters.distro, resetValue: model.defaults.distro, suggestions: s.distros},
			{placeholder: "release", value: model.filters.release, resetValue: model.defaults.release, suggestions: s.releases},
			{placeholder: "suite", value: model.filters.suite, resetValue: model.defaults.suite, suggestions: s.suites},
			{placeholder: "backport", value: defaultString(model.filters.backport, "none"), resetValue: defaultString(model.defaults.backport, "none"), suggestions: s.backports},
			{placeholder: "merge", value: fmt.Sprintf("%t", model.filters.merge), resetValue: fmt.Sprintf("%t", model.defaults.merge), suggestions: []string{"false", "true"}, kind: fieldKindEnum},
			{placeholder: "upstream release", value: model.filters.upstreamRelease, resetValue: model.defaults.upstreamRelease},
			{placeholder: "behind upstream", value: fmt.Sprintf("%t", model.filters.behindUpstream), resetValue: fmt.Sprintf("%t", model.defaults.behindUpstream), suggestions: []string{"false", "true"}, kind: fieldKindEnum},
			{placeholder: "only in", value: model.filters.onlyIn, resetValue: model.defaults.onlyIn},
			{placeholder: "constraints", value: model.filters.constraints, resetValue: model.defaults.constraints},
		})
	case packageModeExcuses:
		return newFormModal("Package Filters / Excuses", []fieldDef{
			{placeholder: "tracker", value: model.filters.tracker, resetValue: model.defaults.tracker, suggestions: s.trackers, kind: fieldKindEnum},
			{placeholder: "name", value: model.filters.name, resetValue: model.defaults.name, suggestions: s.names},
			{placeholder: "component", value: model.filters.component, resetValue: model.defaults.component, suggestions: s.components},
			{placeholder: "team", value: model.filters.team, resetValue: model.defaults.team, suggestions: s.teams},
			{placeholder: "set", value: model.filters.excuseSet, resetValue: model.defaults.excuseSet, suggestions: s.sets},
			{placeholder: "blocked by set", value: model.filters.blockedBySet, resetValue: model.defaults.blockedBySet, suggestions: s.sets},
			{placeholder: "ftbfs", value: fmt.Sprintf("%t", model.filters.ftbfs), resetValue: fmt.Sprintf("%t", model.defaults.ftbfs), suggestions: []string{"false", "true"}, kind: fieldKindEnum},
			{placeholder: "autopkgtest", value: fmt.Sprintf("%t", model.filters.autopkgtest), resetValue: fmt.Sprintf("%t", model.defaults.autopkgtest), suggestions: []string{"false", "true"}, kind: fieldKindEnum},
			{placeholder: "blocked by", value: model.filters.blockedBy, resetValue: model.defaults.blockedBy},
			{placeholder: "bugged", value: fmt.Sprintf("%t", model.filters.bugged), resetValue: fmt.Sprintf("%t", model.defaults.bugged), suggestions: []string{"false", "true"}, kind: fieldKindEnum},
			{placeholder: "min age", value: model.filters.minAge, resetValue: model.defaults.minAge},
			{placeholder: "max age", value: model.filters.maxAge, resetValue: model.defaults.maxAge},
			{placeholder: "limit", value: model.filters.limit, resetValue: model.defaults.limit},
			{placeholder: "reverse", value: fmt.Sprintf("%t", model.filters.reverse), resetValue: fmt.Sprintf("%t", model.defaults.reverse), suggestions: []string{"false", "true"}, kind: fieldKindEnum},
		})
	default:
		return newFormModal("Package Filters / Inventory", []fieldDef{
			{placeholder: "distro", value: model.filters.distro, resetValue: model.defaults.distro, suggestions: s.distros},
			{placeholder: "release", value: model.filters.release, resetValue: model.defaults.release, suggestions: s.releases},
			{placeholder: "suite", value: model.filters.suite, resetValue: model.defaults.suite, suggestions: s.suites},
			{placeholder: "component", value: model.filters.component, resetValue: model.defaults.component, suggestions: s.components},
			{placeholder: "backport", value: defaultString(model.filters.backport, "none"), resetValue: defaultString(model.defaults.backport, "none"), suggestions: s.backports},
		})
	}
}

func newBugFilterForm(session *runtimeadapter.Session, model bugsModel) formModalModel {
	s := bugFilterSuggestions(session, model)
	return newFormModal("Bug List / Search", []fieldDef{
		{placeholder: "query (empty lists)", value: model.filters.query, resetValue: model.defaults.query},
		{placeholder: "mode", value: defaultString(model.filters.mode, "text"), resetValue: defaultString(model.defaults.mode, "text"), suggestions: []string{"text", "regex"}, kind: fieldKindEnum},
		{placeholder: "fields (comma separated)", value: model.filters.fields, resetValue: model.defaults.fields},
		{placeholder: "fuzzy", value: defaultString(model.filters.fuzzy, "auto"), resetValue: defaultString(model.defaults.fuzzy, "auto"), suggestions: []string{"auto", "true", "false"}, kind: fieldKindEnum},
		{placeholder: "closed", value: defaultString(model.filters.closed, "include"), resetValue: defaultString(model.defaults.closed, "include"), suggestions: []string{"include", "exclude", "only"}, kind: fieldKindEnum},
		{placeholder: "sort", value: defaultString(model.filters.sort, "relevance"), resetValue: defaultString(model.defaults.sort, "relevance"), suggestions: []string{"relevance", "updated", "created"}, kind: fieldKindEnum},
		{placeholder: "project", value: model.filters.project, resetValue: model.defaults.project, suggestions: s.projects},
		{placeholder: "status", value: model.filters.status, resetValue: model.defaults.status, suggestions: s.statuses, kind: fieldKindEnum},
		{placeholder: "importance", value: model.filters.importance, resetValue: model.defaults.importance, suggestions: s.importances, kind: fieldKindEnum},
		{placeholder: "assignee", value: model.filters.assignee, resetValue: model.defaults.assignee, suggestions: s.assignees},
		{placeholder: "tag", value: model.filters.tag, resetValue: model.defaults.tag, suggestions: s.tags},
		{placeholder: "since", value: model.filters.since, resetValue: model.defaults.since},
		{placeholder: "merge", value: fmt.Sprintf("%t", model.filters.merge), resetValue: fmt.Sprintf("%t", model.defaults.merge), suggestions: []string{"false", "true"}, kind: fieldKindEnum},
		{placeholder: "limit", value: model.filters.limit, resetValue: model.defaults.limit},
	})
}

func newReviewFilterForm(session *runtimeadapter.Session, model reviewsModel) formModalModel {
	s := reviewFilterSuggestions(session, model)
	return newFormModal("Review Filters", []fieldDef{
		{placeholder: "project", value: model.filters.project, resetValue: model.defaults.project, suggestions: s.projects},
		{placeholder: "forge", value: model.filters.forge, resetValue: model.defaults.forge, suggestions: s.forges, kind: fieldKindEnum},
		{placeholder: "state", value: model.filters.state, resetValue: model.defaults.state, suggestions: s.states, kind: fieldKindEnum},
		{placeholder: "author", value: model.filters.author, resetValue: model.defaults.author, suggestions: s.authors},
		{placeholder: "since", value: model.filters.since, resetValue: model.defaults.since},
	})
}

func newCommitFilterForm(session *runtimeadapter.Session, model commitsModel) formModalModel {
	s := commitFilterSuggestions(session, model)
	return newFormModal("Commit Filters", []fieldDef{
		{placeholder: "mode", value: strings.ToLower(commitModeName(model.filters.mode)), resetValue: strings.ToLower(commitModeName(model.defaults.mode)), suggestions: []string{"log", "track"}, kind: fieldKindEnum},
		{placeholder: "project", value: model.filters.project, resetValue: model.defaults.project, suggestions: s.projects},
		{placeholder: "forge", value: model.filters.forge, resetValue: model.defaults.forge, suggestions: s.forges, kind: fieldKindEnum},
		{placeholder: "branch", value: model.filters.branch, resetValue: model.defaults.branch, suggestions: s.branches},
		{placeholder: "author", value: model.filters.author, resetValue: model.defaults.author, suggestions: s.authors},
		{placeholder: "include mrs", value: fmt.Sprintf("%t", model.filters.includeMRs), resetValue: fmt.Sprintf("%t", model.defaults.includeMRs), suggestions: []string{"false", "true"}, kind: fieldKindEnum},
		{placeholder: "bug id", value: model.filters.bugID, resetValue: model.defaults.bugID},
	})
}

func newProjectFilterForm(model projectsModel) formModalModel {
	s := projectFilterSuggestions(model)
	return newFormModal("Project Filters", []fieldDef{
		{placeholder: "name", value: model.filters.name, resetValue: model.defaults.name, suggestions: s.names},
		{placeholder: "artifact type", value: model.filters.artifactType, resetValue: model.defaults.artifactType, suggestions: s.artifactTypes, kind: fieldKindEnum},
		{placeholder: "code forge", value: model.filters.codeForge, resetValue: model.defaults.codeForge, suggestions: s.codeForges, kind: fieldKindEnum},
		{placeholder: "bug forge", value: model.filters.bugForge, resetValue: model.defaults.bugForge, suggestions: s.bugForges, kind: fieldKindEnum},
		{placeholder: "has build", value: defaultString(model.filters.hasBuild, "any"), resetValue: defaultString(model.defaults.hasBuild, "any"), suggestions: s.bools, kind: fieldKindEnum},
		{placeholder: "has release", value: defaultString(model.filters.hasRelease, "any"), resetValue: defaultString(model.defaults.hasRelease, "any"), suggestions: s.bools, kind: fieldKindEnum},
	})
}

func (m rootModel) updatePackageFilterForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.packageFilterForm, func(values []string) tea.Cmd {
		filters := m.packages.filters
		switch m.packages.filters.mode {
		case packageModeDiff:
			merge, err := strconv.ParseBool(strings.TrimSpace(values[5]))
			if err != nil {
				m.packageFilterForm.errorMsg = "merge must be true or false"
				return nil
			}
			behind, err := strconv.ParseBool(strings.TrimSpace(values[7]))
			if err != nil {
				m.packageFilterForm.errorMsg = "behind upstream must be true or false"
				return nil
			}
			filters.set = strings.TrimSpace(values[0])
			filters.distro = strings.TrimSpace(values[1])
			filters.release = strings.TrimSpace(values[2])
			filters.suite = strings.TrimSpace(values[3])
			filters.backport = strings.TrimSpace(values[4])
			filters.merge = merge
			filters.upstreamRelease = strings.TrimSpace(values[6])
			filters.behindUpstream = behind
			filters.onlyIn = strings.TrimSpace(values[8])
			filters.constraints = strings.TrimSpace(values[9])
		case packageModeExcuses:
			ftbfs, err := strconv.ParseBool(strings.TrimSpace(values[6]))
			if err != nil {
				m.packageFilterForm.errorMsg = "ftbfs must be true or false"
				return nil
			}
			autopkgtest, err := strconv.ParseBool(strings.TrimSpace(values[7]))
			if err != nil {
				m.packageFilterForm.errorMsg = "autopkgtest must be true or false"
				return nil
			}
			bugged, err := strconv.ParseBool(strings.TrimSpace(values[9]))
			if err != nil {
				m.packageFilterForm.errorMsg = "bugged must be true or false"
				return nil
			}
			reverse, err := strconv.ParseBool(strings.TrimSpace(values[13]))
			if err != nil {
				m.packageFilterForm.errorMsg = "reverse must be true or false"
				return nil
			}
			filters.tracker = strings.TrimSpace(values[0])
			filters.name = strings.TrimSpace(values[1])
			filters.component = strings.TrimSpace(values[2])
			filters.team = strings.TrimSpace(values[3])
			filters.excuseSet = strings.TrimSpace(values[4])
			filters.blockedBySet = strings.TrimSpace(values[5])
			filters.ftbfs = ftbfs
			filters.autopkgtest = autopkgtest
			filters.blockedBy = strings.TrimSpace(values[8])
			filters.bugged = bugged
			filters.minAge = strings.TrimSpace(values[10])
			filters.maxAge = strings.TrimSpace(values[11])
			filters.limit = strings.TrimSpace(values[12])
			filters.reverse = reverse
		default:
			filters.distro = strings.TrimSpace(values[0])
			filters.release = strings.TrimSpace(values[1])
			filters.suite = strings.TrimSpace(values[2])
			filters.component = strings.TrimSpace(values[3])
			filters.backport = strings.TrimSpace(values[4])
		}
		m.packages.filters = filters
		m.packages.index = 0
		m.packages.clearDetails()
		m.overlay = overlayNone
		return loadPackagesCmd(m.session, m.packages.filters)
	}, func() { m.overlay = overlayNone })
	return m, cmd
}

func (m rootModel) updateBugFilterForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.bugFilterForm, func(values []string) tea.Cmd {
		merge, err := strconv.ParseBool(strings.TrimSpace(values[12]))
		if err != nil {
			m.bugFilterForm.errorMsg = "merge must be true or false"
			return nil
		}
		m.bugs.filters = bugsFilters{
			query:      strings.TrimSpace(values[0]),
			mode:       strings.TrimSpace(values[1]),
			fields:     strings.TrimSpace(values[2]),
			fuzzy:      strings.TrimSpace(values[3]),
			closed:     strings.TrimSpace(values[4]),
			sort:       strings.TrimSpace(values[5]),
			project:    strings.TrimSpace(values[6]),
			status:     strings.TrimSpace(values[7]),
			importance: strings.TrimSpace(values[8]),
			assignee:   strings.TrimSpace(values[9]),
			tag:        strings.TrimSpace(values[10]),
			since:      strings.TrimSpace(values[11]),
			merge:      merge,
			limit:      strings.TrimSpace(values[13]),
		}
		m.bugs.index = 0
		m.bugs.detail = nil
		m.overlay = overlayNone
		return loadBugsCmd(m.session, m.bugs.filters)
	}, func() { m.overlay = overlayNone })
	return m, cmd
}

func (m rootModel) updateReviewFilterForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.reviewFilterForm, func(values []string) tea.Cmd {
		m.reviews.filters = reviewsFilters{
			project: strings.TrimSpace(values[0]),
			forge:   strings.TrimSpace(values[1]),
			state:   strings.TrimSpace(values[2]),
			author:  strings.TrimSpace(values[3]),
			since:   strings.TrimSpace(values[4]),
		}
		m.reviews.index = 0
		m.reviews.detail = nil
		m.overlay = overlayNone
		return loadReviewsCmd(m.session, m.reviews.filters)
	}, func() { m.overlay = overlayNone })
	return m, cmd
}

func (m rootModel) updateCommitFilterForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.commitFilterForm, func(values []string) tea.Cmd {
		mode, err := parseCommitMode(values[0])
		if err != nil {
			m.commitFilterForm.errorMsg = err.Error()
			return nil
		}
		includeMRs, err := strconv.ParseBool(strings.TrimSpace(values[5]))
		if err != nil {
			m.commitFilterForm.errorMsg = "include mrs must be true or false"
			return nil
		}
		m.commits.filters = commitsFilters{
			mode:       mode,
			project:    strings.TrimSpace(values[1]),
			forge:      strings.TrimSpace(values[2]),
			branch:     strings.TrimSpace(values[3]),
			author:     strings.TrimSpace(values[4]),
			includeMRs: includeMRs,
			bugID:      strings.TrimSpace(values[6]),
		}
		m.commits.index = 0
		m.overlay = overlayNone
		return loadCommitsCmd(m.session, m.commits.filters)
	}, func() { m.overlay = overlayNone })
	return m, cmd
}

func (m rootModel) updateProjectFilterForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.projectFilterForm, func(values []string) tea.Cmd {
		m.projects.filters = projectsFilters{
			name:         strings.TrimSpace(values[0]),
			artifactType: strings.TrimSpace(values[1]),
			codeForge:    strings.TrimSpace(values[2]),
			bugForge:     strings.TrimSpace(values[3]),
			hasBuild:     strings.TrimSpace(values[4]),
			hasRelease:   strings.TrimSpace(values[5]),
		}
		m.projects.index = 0
		m.overlay = overlayNone
		return loadProjectsCmd(m.session, m.projects.filters)
	}, func() { m.overlay = overlayNone })
	return m, cmd
}

func parsePackageMode(raw string) (packageMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "inventory":
		return packageModeInventory, nil
	case "diff":
		return packageModeDiff, nil
	case "excuses":
		return packageModeExcuses, nil
	default:
		return packageModeInventory, fmt.Errorf("mode must be inventory, diff, or excuses")
	}
}

func parseCommitMode(raw string) (commitMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "log":
		return commitModeLog, nil
	case "track":
		return commitModeTrack, nil
	default:
		return commitModeLog, fmt.Errorf("mode must be log or track")
	}
}
