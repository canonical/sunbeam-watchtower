// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	frontend "github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/frontend"
	runtimeadapter "github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/runtime"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
)

// cancelHolder stores a context.CancelFunc behind a pointer so it survives
// Bubble Tea model copies.
type cancelHolder struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

func (h *cancelHolder) Set(fn context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cancel = fn
}

func (h *cancelHolder) Cancel() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

type viewID int

const (
	viewDashboard viewID = iota
	viewBuilds
	viewReleases
	viewPackages
	viewBugs
	viewReviews
	viewCommits
	viewProjects
	viewSRU
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
	overlayAuth
	overlayOperations
	overlayCache
	overlaySync
	overlayLogs
	overlayServer
	overlayPrompt
	overlayBuildFilters
	overlayReleaseFilters
	overlayBuildTrigger
	overlayPackageFilters
	overlayBugFilters
	overlaySRUFilters
	overlayReviewFilters
	overlayCommitFilters
	overlayProjectFilters
	overlayProjectSync
	overlayBugSync
	overlayTeamSync
	overlayCacheSync
	overlayCacheClear
	overlayBuildCleanup
)

type deferredActionKind int

const (
	deferredNone deferredActionKind = iota
	deferredLaunchpadAuthLogin
	deferredLaunchpadAuthLogout
	deferredGitHubAuthLogin
	deferredGitHubAuthLogout
	deferredOperationCancel
	deferredBuildTrigger
	deferredSwitchServer
)

type deferredAction struct {
	kind        deferredActionKind
	operationID string
	buildReq    frontend.BuildTriggerRequest
}

type toastState struct {
	message string
	level   string
}

type dashboardModel struct {
	section int
	auth    *dto.AuthStatus
	ops     []dto.OperationJob
	builds  []dto.Build
	cache   *frontend.CacheStatusResponse
	loaded  bool
	err     string
}

type buildsFilters struct {
	project string
	state   string
	active  bool
	source  string
}

type buildsModel struct {
	filters  buildsFilters
	defaults buildsFilters
	rows     []dto.Build
	index    int
	loaded   bool
	err      string
}

type releasesFilters struct {
	project       string
	artifactType  string
	risk          string
	track         string
	branch        string
	targetProfile string
	allTargets    bool
}

type releasesModel struct {
	filters   releasesFilters
	defaults  releasesFilters
	rows      []dto.ReleaseListEntry
	artifacts []releaseArtifactSummary
	index     int
	detail    *dto.ReleaseShowResult
	loaded    bool
	err       string
}

type releaseArtifactSummary struct {
	Project             string
	Name                string
	ArtifactType        dto.ArtifactType
	ReleasedAt          time.Time
	ChannelCount        int
	ResourceCount       int
	LatestVisibleTarget string
	VisibleTargetCount  int
}

type operationsDrawerModel struct {
	rows   []dto.OperationJob
	events []dto.OperationEvent
	index  int
	loaded bool
	err    string
}

type authModalModel struct {
	status         *dto.AuthStatus
	launchpadBegin *dto.LaunchpadAuthBeginResult
	githubBegin    *dto.GitHubAuthBeginResult
	githubCancel   *cancelHolder
	loaded         bool
	err            string
}

type cacheModalModel struct {
	status      *frontend.CacheStatusResponse
	selected    cacheActionTarget
	lastAction  string
	lastSummary []string
	loaded      bool
	err         string
}

type syncModalModel struct {
	selected    syncActionTarget
	lastAction  string
	lastSummary []string
	err         string
}

type serverModalModel struct {
	local  *runtimeadapter.LocalServerStatus
	err    string
	loaded bool
}

type logsModalModel struct {
	sessionLines []string
	daemonLines  []string
	daemonNote   string
	loaded       bool
	err          string
}

type formModalModel struct {
	title         string
	fields        []textinput.Model
	kinds         []fieldKind
	options       [][]string
	optionIndices []int
	resetValues   []string
	selected      [][]string
	selectedSets  []map[string]bool
	active        int
	errorMsg      string
	scroll        int
	pendingG      bool
	visualAnchor  int
	visualMode    bool
}

type promptModel struct {
	title  string
	body   string
	accept string
	reject string
}

type syncActionTarget int

const (
	syncActionProject syncActionTarget = iota
	syncActionBug
	syncActionTeam
)

type cacheActionTarget int

const (
	cacheActionGit cacheActionTarget = iota
	cacheActionPackages
	cacheActionUpstream
	cacheActionBugs
	cacheActionExcuses
	cacheActionReleases
	cacheActionReviews
	cacheActionSRU
)

type releaseFilterOptions struct {
	projects       []string
	artifactTypes  []string
	risks          []string
	tracks         []string
	branches       []string
	targetProfiles []string
}

type dashboardLoadedMsg struct {
	auth   *dto.AuthStatus
	ops    []dto.OperationJob
	builds []dto.Build
	cache  *frontend.CacheStatusResponse
	err    error
}

type buildsLoadedMsg struct {
	rows []dto.Build
	err  error
}

type releasesLoadedMsg struct {
	rows []dto.ReleaseListEntry
	err  error
}

type tuiBootstrapLoadedMsg struct {
	config *dto.Config
	err    error
}

type releaseDetailLoadedMsg struct {
	key    string
	detail *dto.ReleaseShowResult
	err    error
}

type opsLoadedMsg struct {
	rows   []dto.OperationJob
	events []dto.OperationEvent
	err    error
}

type authStatusLoadedMsg struct {
	status *dto.AuthStatus
	err    error
}

type authBeginMsg struct {
	begin *dto.LaunchpadAuthBeginResult
	err   error
}

type authFinalizeMsg struct {
	result *dto.LaunchpadAuthFinalizeResult
	err    error
}

type authLogoutMsg struct {
	result *dto.LaunchpadAuthLogoutResult
	err    error
}

type authGitHubBeginMsg struct {
	begin *dto.GitHubAuthBeginResult
	err   error
}

type authGitHubFinalizeMsg struct {
	result *dto.GitHubAuthFinalizeResult
	err    error
}

type authGitHubLogoutMsg struct {
	result *dto.GitHubAuthLogoutResult
	err    error
}

type cacheLoadedMsg struct {
	status *frontend.CacheStatusResponse
	err    error
}

type projectSyncFinishedMsg struct {
	req    frontend.ProjectSyncRequest
	result *frontend.ProjectSyncResponse
	err    error
}

type bugSyncFinishedMsg struct {
	req    frontend.BugSyncRequest
	result *frontend.BugSyncResponse
	err    error
}

type sruLoadedMsg struct {
	snapshot *dto.SRUSnapshot
	err      error
}

type teamSyncFinishedMsg struct {
	req    dto.TeamSyncRequest
	result *frontend.TeamSyncResponse
	err    error
}

type cacheMutationFinishedMsg struct {
	action  string
	summary []string
	err     error
}

type localServerStatusMsg struct {
	status runtimeadapter.LocalServerStatus
	err    error
}

type logsLoadedMsg struct {
	sessionLines []string
	daemonLines  []string
	daemonNote   string
	err          error
}

type buildTriggeredMsg struct {
	job *dto.OperationJob
	err error
}

type buildRetryMsg struct{ err error }
type buildCancelMsg struct{ err error }

type buildCleanupMsg struct {
	deletedRecipes  []string
	deletedBranches []string
	dryRun          bool
	err             error
}

type operationCancelledMsg struct {
	job *dto.OperationJob
	err error
}

type upgradedMsg struct {
	err error
}

type browserOpenedMsg struct {
	err error
}

type tickDashboardMsg time.Time
type tickOperationsMsg time.Time
type tickBuildsMsg time.Time
type tickLogsMsg time.Time
type clearToastMsg struct{}

type rootModel struct {
	session *runtimeadapter.Session
	logs    *logBuffer
	theme   theme

	width  int
	height int

	activeView viewID
	overlay    overlayKind

	lastRefresh   time.Time
	toast         toastState
	viewScrolls   [9]int
	overlayScroll int
	pendingG      bool

	dashboard dashboardModel
	builds    buildsModel
	releases  releasesModel
	packages  packagesModel
	bugs      bugsModel
	sru       sruModel
	reviews   reviewsModel
	commits   commitsModel
	projects  projectsModel
	ops       operationsDrawerModel
	auth      authModalModel
	cache     cacheModalModel
	syncModal syncModalModel
	logsModal logsModalModel
	server    serverModalModel

	tickOpsActive    bool
	tickBuildsActive bool
	tickLogsActive   bool

	buildFilterForm   formModalModel
	releaseFilterForm formModalModel
	buildTriggerForm  formModalModel
	buildCleanupForm  formModalModel
	packageFilterForm formModalModel
	bugFilterForm     formModalModel
	sruFilterForm     formModalModel
	reviewFilterForm  formModalModel
	commitFilterForm  formModalModel
	projectFilterForm formModalModel
	projectSyncForm   formModalModel
	bugSyncForm       formModalModel
	teamSyncForm      formModalModel
	cacheSyncForm     formModalModel
	cacheClearForm    formModalModel
	prompt            promptModel
	deferred          deferredAction
}

func newRootModel(session *runtimeadapter.Session, noColor bool) rootModel {
	return newRootModelWithLogs(session, noColor, nil)
}

func newRootModelWithLogs(session *runtimeadapter.Session, noColor bool, logs *logBuffer) rootModel {
	t := newTheme()
	if noColor {
		lipgloss.SetColorProfile(0)
	}
	m := rootModel{
		session:    session,
		logs:       logs,
		theme:      t,
		activeView: viewDashboard,
		builds: buildsModel{
			filters:  buildsFilters{active: true, source: "remote"},
			defaults: buildsFilters{active: true, source: "remote"},
		},
		releases: releasesModel{
			filters:  releasesFilters{},
			defaults: releasesFilters{},
		},
		packages: packagesModel{
			filters:  packagesFilters{mode: packageModeInventory, backport: "none"},
			defaults: packagesFilters{mode: packageModeInventory, backport: "none"},
		},
		bugs: bugsModel{
			filters:  bugsFilters{mode: "text", fuzzy: "auto", closed: "include", sort: "relevance", merge: true},
			defaults: bugsFilters{mode: "text", fuzzy: "auto", closed: "include", sort: "relevance", merge: true},
		},
		reviews: reviewsModel{
			filters:  reviewsFilters{},
			defaults: reviewsFilters{},
		},
		commits: commitsModel{
			filters:  commitsFilters{mode: commitModeLog},
			defaults: commitsFilters{mode: commitModeLog},
		},
		projects: projectsModel{
			filters:  projectsFilters{},
			defaults: projectsFilters{},
		},
		auth: authModalModel{
			githubCancel: &cancelHolder{},
		},
	}
	return m
}

func (m rootModel) Init() tea.Cmd {
	return loadTUIBootstrapCmd(m.session)
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.overlay != overlayNone {
			return m.updateOverlay(msg)
		}
		return m.updateGlobal(msg)
	case tuiBootstrapLoadedMsg:
		if msg.err != nil {
			m.setToast("Using built-in TUI defaults", "error")
			return m, tea.Batch(clearToastLater(), m.initialLoadCmd(nil), tickDashboardCmd())
		}
		if msg.config != nil {
			m.applyTUIConfig(msg.config)
		}
		return m, tea.Batch(m.initialLoadCmd(msg.config), tickDashboardCmd())
	case dashboardLoadedMsg:
		m.dashboard.loaded = msg.err == nil
		m.dashboard.err = errString(msg.err)
		if msg.err == nil {
			m.dashboard.auth = msg.auth
			m.dashboard.ops = msg.ops
			m.dashboard.builds = msg.builds
			m.dashboard.cache = msg.cache
			m.lastRefresh = time.Now()
		}
	case buildsLoadedMsg:
		m.builds.loaded = msg.err == nil
		m.builds.err = errString(msg.err)
		if msg.err == nil {
			m.builds.rows = msg.rows
			if m.builds.index >= len(m.builds.rows) && len(m.builds.rows) > 0 {
				m.builds.index = len(m.builds.rows) - 1
			}
			m.lastRefresh = time.Now()
			if hasNonTerminalBuilds(m.builds.rows) && !m.tickBuildsActive {
				m.tickBuildsActive = true
				return m, tickBuildsCmd()
			}
		}
	case releasesLoadedMsg:
		m.releases.loaded = msg.err == nil
		m.releases.err = errString(msg.err)
		if msg.err == nil {
			m.releases.rows = msg.rows
			m.releases.artifacts = summarizeReleaseArtifacts(msg.rows)
			if m.releases.index >= len(m.releases.artifacts) && len(m.releases.artifacts) > 0 {
				m.releases.index = len(m.releases.artifacts) - 1
			}
			m.lastRefresh = time.Now()
			if artifact := m.selectedReleaseArtifact(); artifact != nil {
				return m, loadReleaseDetailCmd(m.session, *artifact, m.releases.filters)
			}
		}
	case releaseDetailLoadedMsg:
		if msg.err == nil && msg.key == m.releaseDetailKey() {
			m.releases.detail = msg.detail
		}
	case packagesLoadedMsg:
		m.packages.loaded = msg.err == nil
		m.packages.err = errString(msg.err)
		if msg.err == nil {
			m.packages.inventoryRows = msg.inventoryRows
			m.packages.diffRows = msg.diffRows
			m.packages.diffSources = msg.diffSources
			m.packages.diffHasUpstream = msg.diffHasUpstream
			m.packages.excuseRows = msg.excuseRows
			m.packages.prompt = msg.prompt
			m.packages.index = clampIndex(m.packages.index, m.packages.rowCount())
			m.packages.clearStaleDetail()
			m.lastRefresh = time.Now()
			return m, loadPackageDetailCmd(m.session, m.packages)
		}
	case packageInventoryDetailLoadedMsg:
		if msg.err == nil && msg.key == m.packageDetailKey() {
			m.packages.inventoryDetail = msg.detail
		}
	case packageDiffDetailLoadedMsg:
		if msg.err == nil && msg.key == m.packageDetailKey() {
			m.packages.diffDetail = msg.detail
		}
	case packageExcuseDetailLoadedMsg:
		if msg.err == nil && msg.key == m.packageDetailKey() {
			m.packages.excuseDetail = msg.detail
		}
	case bugsLoadedMsg:
		m.bugs.loaded = msg.err == nil
		m.bugs.err = errString(msg.err)
		if msg.err == nil {
			m.bugs.rows = msg.rows
			m.bugs.searchResults = msg.searchResults
			m.bugs.searchOutcome = msg.searchOutcome
			m.bugs.warnings = msg.warnings
			m.bugs.index = clampIndex(m.bugs.index, len(m.bugs.rows))
			if task := selectedBug(m.bugs.rows, m.bugs.index); task != nil {
				m.lastRefresh = time.Now()
				return m, loadBugDetailCmd(m.session, *task)
			}
			m.bugs.detail = nil
			m.lastRefresh = time.Now()
		}
	case bugDetailLoadedMsg:
		if msg.err == nil && msg.key == m.bugDetailKey() {
			m.bugs.detail = msg.detail
		}
	case reviewsLoadedMsg:
		m.reviews.loaded = msg.err == nil
		m.reviews.err = errString(msg.err)
		if msg.err == nil {
			m.reviews.rows = msg.rows
			m.reviews.warnings = msg.warnings
			m.reviews.detailErr = ""
			m.reviews.index = clampIndex(m.reviews.index, len(m.reviews.rows))
			if mr := selectedReview(m.reviews.rows, m.reviews.index); mr != nil {
				m.lastRefresh = time.Now()
				return m, loadReviewDetailCmd(m.session, *mr)
			}
			m.reviews.detail = nil
			m.lastRefresh = time.Now()
		}
	case reviewDetailLoadedMsg:
		if msg.key == m.reviewDetailKey() {
			m.reviews.detailErr = errString(msg.err)
			if msg.err == nil {
				m.reviews.detail = msg.detail
			} else {
				m.reviews.detail = nil
			}
		}
	case commitsLoadedMsg:
		m.commits.loaded = msg.err == nil
		m.commits.err = errString(msg.err)
		if msg.err == nil {
			m.commits.rows = msg.rows
			m.commits.warnings = msg.warnings
			m.commits.prompt = msg.prompt
			m.commits.index = clampIndex(m.commits.index, len(m.commits.rows))
			m.lastRefresh = time.Now()
		}
	case projectsLoadedMsg:
		m.projects.loaded = msg.err == nil
		m.projects.err = errString(msg.err)
		if msg.err == nil {
			m.projects.config = msg.config
			m.projects.rows = msg.rows
			m.projects.index = clampIndex(m.projects.index, len(m.projects.rows))
			m.lastRefresh = time.Now()
		}
	case opsLoadedMsg:
		m.ops.loaded = msg.err == nil
		m.ops.err = errString(msg.err)
		if msg.err == nil {
			m.ops.rows = msg.rows
			if m.ops.index >= len(m.ops.rows) && len(m.ops.rows) > 0 {
				m.ops.index = len(m.ops.rows) - 1
			}
			m.ops.events = msg.events
		}
	case authStatusLoadedMsg:
		m.auth.loaded = msg.err == nil
		m.auth.err = errString(msg.err)
		if msg.err == nil {
			m.auth.status = msg.status
			if m.dashboard.auth == nil {
				m.dashboard.auth = msg.status
			}
		}
	case authBeginMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			m.auth.err = msg.err.Error()
			return m, clearToastLater()
		}
		m.auth.launchpadBegin = msg.begin
		m.setToast("Launchpad auth started", "info")
		return m, clearToastLater()
	case authFinalizeMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			m.auth.err = msg.err.Error()
			return m, clearToastLater()
		}
		m.auth.launchpadBegin = nil
		status := m.auth.status
		if status == nil {
			status = &dto.AuthStatus{}
		}
		status.Launchpad = msg.result.Launchpad
		m.auth.status = status
		m.dashboard.auth = m.auth.status
		m.setToast("Launchpad login completed", "success")
		return m, tea.Batch(clearToastLater(), loadDashboardCmd(m.session))
	case authLogoutMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
		m.auth.launchpadBegin = nil
		status := m.auth.status
		if status == nil {
			status = &dto.AuthStatus{}
		}
		status.Launchpad = dto.LaunchpadAuthStatus{}
		m.auth.status = status
		m.dashboard.auth = m.auth.status
		m.setToast("Launchpad credentials cleared", "success")
		return m, tea.Batch(clearToastLater(), loadDashboardCmd(m.session))
	case authGitHubBeginMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			m.auth.err = msg.err.Error()
			return m, clearToastLater()
		}
		m.auth.githubBegin = msg.begin
		m.setToast("GitHub auth started", "info")
		cmd := finalizeGitHubAuthCmd(m.session, msg.begin.FlowID, m.auth.githubCancel)
		return m, tea.Batch(clearToastLater(), cmd)
	case authGitHubFinalizeMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			m.auth.err = msg.err.Error()
			return m, clearToastLater()
		}
		m.auth.githubBegin = nil
		status := m.auth.status
		if status == nil {
			status = &dto.AuthStatus{}
		}
		status.GitHub = msg.result.GitHub
		m.auth.status = status
		m.dashboard.auth = m.auth.status
		m.setToast("GitHub login completed", "success")
		return m, tea.Batch(clearToastLater(), loadDashboardCmd(m.session))
	case authGitHubLogoutMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
		m.auth.githubBegin = nil
		m.auth.githubCancel.Cancel()
		status := m.auth.status
		if status == nil {
			status = &dto.AuthStatus{}
		}
		status.GitHub = dto.GitHubAuthStatus{}
		m.auth.status = status
		m.dashboard.auth = m.auth.status
		m.setToast("GitHub credentials cleared", "success")
		return m, tea.Batch(clearToastLater(), loadDashboardCmd(m.session))
	case cacheLoadedMsg:
		m.cache.loaded = msg.err == nil
		m.cache.err = errString(msg.err)
		if msg.err == nil {
			m.cache.status = msg.status
		}
	case sruLoadedMsg:
		m.sru.err = errString(msg.err)
		if msg.err == nil {
			m.sru.snapshot = msg.snapshot
			m.sru.index = clampIndex(m.sru.index, len(msg.snapshot.Rows))
		}
	case projectSyncFinishedMsg:
		if msg.err != nil {
			m.syncModal.err = msg.err.Error()
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
		m.syncModal.err = ""
		m.syncModal.lastAction = fmt.Sprintf("Project sync (%s)", dryRunLabel(msg.req.DryRun))
		m.syncModal.lastSummary = summarizeProjectSyncResult(msg.result)
		m.setToast("Project sync completed", "success")
		return m, clearToastLater()
	case bugSyncFinishedMsg:
		if msg.err != nil {
			m.syncModal.err = msg.err.Error()
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
		m.syncModal.err = ""
		m.syncModal.lastAction = fmt.Sprintf("Bug sync (%s)", dryRunLabel(msg.req.DryRun))
		m.syncModal.lastSummary = summarizeBugSyncResult(msg.result)
		m.setToast("Bug sync completed", "success")
		return m, clearToastLater()
	case teamSyncFinishedMsg:
		if msg.err != nil {
			m.syncModal.err = msg.err.Error()
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
		m.syncModal.err = ""
		m.syncModal.lastAction = fmt.Sprintf("Team sync (%s)", dryRunLabel(msg.req.DryRun))
		m.syncModal.lastSummary = summarizeTeamSyncResult(msg.result)
		m.setToast("Team sync completed", "success")
		return m, clearToastLater()
	case cacheMutationFinishedMsg:
		if msg.err != nil {
			m.cache.err = msg.err.Error()
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
		m.cache.err = ""
		m.cache.lastAction = msg.action
		m.cache.lastSummary = append([]string(nil), msg.summary...)
		m.setToast(msg.action, "success")
		return m, tea.Batch(clearToastLater(), loadCacheCmd(m.session))
	case localServerStatusMsg:
		m.server.loaded = msg.err == nil
		m.server.err = errString(msg.err)
		if msg.err == nil {
			status := msg.status
			m.server.local = &status
		}
	case logsLoadedMsg:
		m.logsModal.loaded = msg.err == nil
		m.logsModal.err = errString(msg.err)
		if msg.err == nil {
			m.logsModal.sessionLines = msg.sessionLines
			m.logsModal.daemonLines = msg.daemonLines
			m.logsModal.daemonNote = msg.daemonNote
		}
	case buildTriggeredMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
		m.overlay = overlayOperations
		m.setToast("Build trigger queued", "success")
		if msg.job != nil {
			m.ops.index = 0
		}
		return m, tea.Batch(clearToastLater(), loadOperationsCmd(m.session, selectedOperationID(m.ops.rows, m.ops.index)))
	case buildRetryMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
		m.setToast("Build retry requested", "success")
		return m, tea.Batch(clearToastLater(), loadBuildsCmd(m.session, m.builds.filters))
	case buildCancelMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
		m.setToast("Build cancel requested", "success")
		return m, tea.Batch(clearToastLater(), loadBuildsCmd(m.session, m.builds.filters))
	case buildCleanupMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
		verb := "Deleted"
		if msg.dryRun {
			verb = "Would delete"
		}
		summary := fmt.Sprintf("%s %d recipes, %d branches", verb, len(msg.deletedRecipes), len(msg.deletedBranches))
		m.setToast(summary, "success")
		return m, tea.Batch(clearToastLater(), loadBuildsCmd(m.session, m.builds.filters))
	case operationCancelledMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
		m.setToast("Operation cancelled", "success")
		return m, tea.Batch(clearToastLater(), loadDashboardCmd(m.session), loadOperationsCmd(m.session, selectedOperationID(m.ops.rows, m.ops.index)))
	case upgradedMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			m.overlay = overlayNone
			m.deferred = deferredAction{}
			return m, clearToastLater()
		}
		m.overlay = overlayNone
		action := m.deferred
		m.deferred = deferredAction{}
		cmd := m.resumeDeferredAction(action)
		m.setToast("Switched to local daemon", "success")
		return m, tea.Batch(clearToastLater(), cmd, loadDashboardCmd(m.session))
	case browserOpenedMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
		} else {
			m.setToast("Opened browser", "info")
		}
		return m, clearToastLater()
	case actionDeniedMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), "error")
			return m, clearToastLater()
		}
	case tickDashboardMsg:
		if m.activeView == viewDashboard && m.overlay == overlayNone {
			return m, tea.Batch(loadDashboardCmd(m.session), tickDashboardCmd())
		}
		return m, tickDashboardCmd()
	case tickOperationsMsg:
		if m.overlay == overlayOperations || hasRunningOperation(m.ops.rows) {
			return m, tea.Batch(loadOperationsCmd(m.session, selectedOperationID(m.ops.rows, m.ops.index)), tickOperationsCmd())
		}
		m.tickOpsActive = false
		return m, nil
	case tickBuildsMsg:
		if m.activeView == viewBuilds && hasNonTerminalBuilds(m.builds.rows) {
			return m, tea.Batch(loadBuildsCmd(m.session, m.builds.filters), tickBuildsCmd())
		}
		m.tickBuildsActive = false
		return m, nil
	case tickLogsMsg:
		if m.overlay == overlayLogs {
			return m, tea.Batch(loadLogsCmd(m.session, m.logs), tickLogsCmd())
		}
		m.tickLogsActive = false
		return m, nil
	case clearToastMsg:
		m.toast = toastState{}
	}
	return m, nil
}

func (m rootModel) updateGlobal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "g":
		if m.pendingG {
			m.pendingG = false
			return m.jumpActiveTop()
		}
		m.pendingG = true
		return m, nil
	case "G":
		m.pendingG = false
		return m.jumpActiveBottom()
	default:
		m.pendingG = false
	}
	switch msg.String() {
	case "esc":
		m.pendingG = false
		m.setContentScroll(0)
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case "m", "?":
		m.overlay = overlayHelp
		m.overlayScroll = 0
		return m, nil
	case "a":
		m.overlay = overlayAuth
		return m, loadAuthStatusCmd(m.session)
	case "o":
		m.overlay = overlayOperations
		cmds := []tea.Cmd{loadOperationsCmd(m.session, selectedOperationID(m.ops.rows, m.ops.index))}
		if !m.tickOpsActive {
			m.tickOpsActive = true
			cmds = append(cmds, tickOperationsCmd())
		}
		return m, tea.Batch(cmds...)
	case "c":
		m.overlay = overlayCache
		m.overlayScroll = 0
		return m, loadCacheCmd(m.session)
	case "u":
		m.overlay = overlaySync
		m.overlayScroll = 0
		return m, nil
	case "l":
		m.overlay = overlayLogs
		m.overlayScroll = 0
		cmds := []tea.Cmd{loadLogsCmd(m.session, m.logs)}
		if !m.tickLogsActive {
			m.tickLogsActive = true
			cmds = append(cmds, tickLogsCmd())
		}
		return m, tea.Batch(cmds...)
	case "s":
		m.overlay = overlayServer
		return m, loadLocalServerStatusCmd(m.session)
	case "1":
		m.activeView = viewDashboard
		return m, nil
	case "2":
		m.activeView = viewBuilds
		return m, nil
	case "3":
		m.activeView = viewReleases
		return m, nil
	case "4":
		m.activeView = viewPackages
		return m, nil
	case "5":
		m.activeView = viewBugs
		return m, nil
	case "6":
		m.activeView = viewReviews
		return m, nil
	case "7":
		m.activeView = viewCommits
		return m, nil
	case "8":
		m.activeView = viewProjects
		return m, nil
	case "9":
		m.activeView = viewSRU
		return m, nil
	case "tab":
		m.activeView = (m.activeView + 1) % 9
		return m, nil
	case "shift+tab":
		m.activeView--
		if m.activeView < 0 {
			m.activeView = viewSRU
		}
		return m, nil
	case "r":
		return m, m.refreshActiveView()
	case "pgdown", "ctrl+d":
		m.setContentScroll(m.contentScroll() + m.scrollStep())
		return m, nil
	case "pgup", "ctrl+u":
		m.setContentScroll(m.contentScroll() - m.scrollStep())
		return m, nil
	case "/":
		switch m.activeView {
		case viewBuilds:
			m.buildFilterForm = newBuildFilterForm(m.builds)
			m.overlay = overlayBuildFilters
			m.overlayScroll = 0
		case viewReleases:
			m.releaseFilterForm = newReleaseFilterForm(m.session, m.releases)
			m.overlay = overlayReleaseFilters
			m.overlayScroll = 0
		case viewPackages:
			m.packageFilterForm = newPackageFilterForm(m.session, m.packages)
			m.overlay = overlayPackageFilters
			m.overlayScroll = 0
		case viewBugs:
			m.bugFilterForm = newBugFilterForm(m.session, m.bugs)
			m.overlay = overlayBugFilters
			m.overlayScroll = 0
		case viewSRU:
			m.sruFilterForm = newSRUFilterForm(m.sru)
			m.overlay = overlaySRUFilters
			m.overlayScroll = 0
		case viewReviews:
			m.reviewFilterForm = newReviewFilterForm(m.session, m.reviews)
			m.overlay = overlayReviewFilters
			m.overlayScroll = 0
		case viewCommits:
			m.commitFilterForm = newCommitFilterForm(m.session, m.commits)
			m.overlay = overlayCommitFilters
			m.overlayScroll = 0
		case viewProjects:
			m.projectFilterForm = newProjectFilterForm(m.projects)
			m.overlay = overlayProjectFilters
			m.overlayScroll = 0
		}
		return m, nil
	case "[":
		switch m.activeView {
		case viewPackages:
			m.packages.filters.mode--
			if m.packages.filters.mode < packageModeInventory {
				m.packages.filters.mode = packageModeExcuses
			}
			m.packages.index = 0
			m.packages.clearDetails()
			return m, loadPackagesCmd(m.session, m.packages.filters)
		case viewCommits:
			m.commits.filters.mode--
			if m.commits.filters.mode < commitModeLog {
				m.commits.filters.mode = commitModeTrack
			}
			m.commits.index = 0
			return m, loadCommitsCmd(m.session, m.commits.filters)
		}
	case "]":
		switch m.activeView {
		case viewPackages:
			m.packages.filters.mode = (m.packages.filters.mode + 1) % 3
			m.packages.index = 0
			m.packages.clearDetails()
			return m, loadPackagesCmd(m.session, m.packages.filters)
		case viewCommits:
			m.commits.filters.mode = (m.commits.filters.mode + 1) % 2
			m.commits.index = 0
			return m, loadCommitsCmd(m.session, m.commits.filters)
		}
	case "up", "k":
		switch m.activeView {
		case viewDashboard:
			if m.dashboard.section > 0 {
				m.dashboard.section--
			}
		case viewBuilds:
			if m.builds.index > 0 {
				m.builds.index--
			}
		case viewReleases:
			if m.releases.index > 0 {
				m.releases.index--
				m.ensureCursorVisible()
				if artifact := m.selectedReleaseArtifact(); artifact != nil {
					return m, loadReleaseDetailCmd(m.session, *artifact, m.releases.filters)
				}
			}
		case viewPackages:
			if m.packages.index > 0 {
				m.packages.index--
				m.ensureCursorVisible()
				return m, loadPackageDetailCmd(m.session, m.packages)
			}
		case viewBugs:
			if m.bugs.index > 0 {
				m.bugs.index--
				m.ensureCursorVisible()
				if task := selectedBug(m.bugs.rows, m.bugs.index); task != nil {
					return m, loadBugDetailCmd(m.session, *task)
				}
			}
		case viewReviews:
			if m.reviews.index > 0 {
				m.reviews.index--
				m.ensureCursorVisible()
				if review := selectedReview(m.reviews.rows, m.reviews.index); review != nil {
					return m, loadReviewDetailCmd(m.session, *review)
				}
			}
		case viewCommits:
			if m.commits.index > 0 {
				m.commits.index--
			}
		case viewProjects:
			if m.projects.index > 0 {
				m.projects.index--
			}
		case viewSRU:
			if m.sru.index > 0 {
				m.sru.index--
			}
		}
		m.ensureCursorVisible()
	case "down", "j":
		switch m.activeView {
		case viewDashboard:
			if m.dashboard.section < 3 {
				m.dashboard.section++
			}
		case viewBuilds:
			if m.builds.index < len(m.builds.rows)-1 {
				m.builds.index++
			}
		case viewReleases:
			if m.releases.index < len(m.releases.artifacts)-1 {
				m.releases.index++
				m.ensureCursorVisible()
				if artifact := m.selectedReleaseArtifact(); artifact != nil {
					return m, loadReleaseDetailCmd(m.session, *artifact, m.releases.filters)
				}
			}
		case viewPackages:
			if m.packages.index < m.packages.rowCount()-1 {
				m.packages.index++
				m.ensureCursorVisible()
				return m, loadPackageDetailCmd(m.session, m.packages)
			}
		case viewBugs:
			if m.bugs.index < len(m.bugs.rows)-1 {
				m.bugs.index++
				m.ensureCursorVisible()
				if task := selectedBug(m.bugs.rows, m.bugs.index); task != nil {
					return m, loadBugDetailCmd(m.session, *task)
				}
			}
		case viewReviews:
			if m.reviews.index < len(m.reviews.rows)-1 {
				m.reviews.index++
				m.ensureCursorVisible()
				if review := selectedReview(m.reviews.rows, m.reviews.index); review != nil {
					return m, loadReviewDetailCmd(m.session, *review)
				}
			}
		case viewCommits:
			if m.commits.index < len(m.commits.rows)-1 {
				m.commits.index++
			}
		case viewProjects:
			if m.projects.index < len(m.projects.rows)-1 {
				m.projects.index++
			}
		case viewSRU:
			if m.sru.snapshot != nil && m.sru.index < len(m.sru.snapshot.Rows)-1 {
				m.sru.index++
			}
		}
		m.ensureCursorVisible()
	case "enter":
		switch m.activeView {
		case viewDashboard:
			switch m.dashboard.section {
			case 0:
				m.overlay = overlayAuth
				m.overlayScroll = 0
				return m, loadAuthStatusCmd(m.session)
			case 1:
				m.overlay = overlayOperations
				m.overlayScroll = 0
				cmds := []tea.Cmd{loadOperationsCmd(m.session, selectedOperationID(m.ops.rows, m.ops.index))}
				if !m.tickOpsActive {
					m.tickOpsActive = true
					cmds = append(cmds, tickOperationsCmd())
				}
				return m, tea.Batch(cmds...)
			case 2:
				m.activeView = viewBuilds
			case 3:
				m.activeView = viewReleases
			}
		case viewReleases:
			if artifact := m.selectedReleaseArtifact(); artifact != nil {
				return m, loadReleaseDetailCmd(m.session, *artifact, m.releases.filters)
			}
		case viewPackages:
			return m, loadPackageDetailCmd(m.session, m.packages)
		case viewBugs:
			if task := selectedBug(m.bugs.rows, m.bugs.index); task != nil {
				return m, loadBugDetailCmd(m.session, *task)
			}
		case viewReviews:
			if review := selectedReview(m.reviews.rows, m.reviews.index); review != nil {
				return m, loadReviewDetailCmd(m.session, *review)
			}
		}
	case "t":
		if m.activeView == viewBuilds {
			m.buildTriggerForm = newBuildTriggerForm(m.session)
			m.overlay = overlayBuildTrigger
			m.overlayScroll = 0
			return m, nil
		}
	case "R":
		if m.activeView == viewSRU {
			return m, refreshSRUCmd(m.session, m.sru.filter)
		}
		if m.activeView == viewBuilds {
			if b := selectedBuild(m.builds.rows, m.builds.index); b != nil && b.CanRetry {
				return m, retryBuildCmd(m.session, *b)
			}
		}
	case "O":
		if m.activeView == viewSRU && m.sru.snapshot != nil && m.sru.index >= 0 && m.sru.index < len(m.sru.snapshot.Rows) {
			return m, openSRUBugCmd(m.session, m.sru.snapshot.Rows[m.sru.index].BugURL)
		}
	case "X":
		if m.activeView == viewBuilds {
			if b := selectedBuild(m.builds.rows, m.builds.index); b != nil && b.CanCancel {
				return m, cancelBuildCmd(m.session, *b)
			}
		}
	case "C":
		if m.activeView == viewBuilds {
			m.buildCleanupForm = newBuildCleanupForm(m.session)
			m.overlay = overlayBuildCleanup
			m.overlayScroll = 0
			return m, nil
		}
	}
	return m, nil
}

func (m rootModel) updateOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayHelp:
		switch msg.String() {
		case "g":
			if m.pendingG {
				m.pendingG = false
				m.overlayScroll = 0
				return m, nil
			}
			m.pendingG = true
			return m, nil
		case "G":
			m.pendingG = false
			m.overlayScroll = viewportEndOffset()
			return m, nil
		default:
			m.pendingG = false
		}
		switch msg.String() {
		case "esc", "q", "ctrl+c", "?":
			m.overlay = overlayNone
			m.overlayScroll = 0
		case "pgdown", "ctrl+d", "down", "j":
			m.overlayScroll += m.scrollStep()
		case "pgup", "ctrl+u", "up", "k":
			m.overlayScroll -= m.scrollStep()
			if m.overlayScroll < 0 {
				m.overlayScroll = 0
			}
		}
		return m, nil
	case overlayAuth:
		m.pendingG = false
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.auth.githubCancel.Cancel()
			m.overlay = overlayNone
			m.overlayScroll = 0
			return m, nil
		case "l":
			if m.session.Target().Kind == runtimeadapter.TargetKindEmbedded {
				m.openUpgradePrompt(deferredAction{kind: deferredLaunchpadAuthLogin})
				return m, nil
			}
			return m, beginLaunchpadAuthCmd(m.session)
		case "x":
			if m.session.Target().Kind == runtimeadapter.TargetKindEmbedded {
				m.openUpgradePrompt(deferredAction{kind: deferredLaunchpadAuthLogout})
				return m, nil
			}
			return m, logoutLaunchpadAuthCmd(m.session)
		case "h":
			if m.session.Target().Kind == runtimeadapter.TargetKindEmbedded {
				m.openUpgradePrompt(deferredAction{kind: deferredGitHubAuthLogin})
				return m, nil
			}
			return m, beginGitHubAuthCmd(m.session)
		case "d":
			if m.session.Target().Kind == runtimeadapter.TargetKindEmbedded {
				m.openUpgradePrompt(deferredAction{kind: deferredGitHubAuthLogout})
				return m, nil
			}
			return m, logoutGitHubAuthCmd(m.session)
		case "o":
			if m.auth.githubBegin != nil {
				return m, openBrowserCmd(m.auth.githubBegin.VerificationURI)
			}
			if m.auth.launchpadBegin != nil {
				return m, openBrowserCmd(m.auth.launchpadBegin.AuthorizeURL)
			}
		}
		return m, nil
	case overlayOperations:
		switch msg.String() {
		case "g":
			if m.pendingG {
				m.pendingG = false
				m.ops.index = 0
				return m, loadOperationsCmd(m.session, selectedOperationID(m.ops.rows, m.ops.index))
			}
			m.pendingG = true
			return m, nil
		case "G":
			m.pendingG = false
			if len(m.ops.rows) == 0 {
				return m, nil
			}
			m.ops.index = len(m.ops.rows) - 1
			return m, loadOperationsCmd(m.session, selectedOperationID(m.ops.rows, m.ops.index))
		default:
			m.pendingG = false
		}
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.overlay = overlayNone
			m.overlayScroll = 0
			return m, nil
		case "up", "k":
			if m.ops.index > 0 {
				m.ops.index--
				return m, loadOperationsCmd(m.session, selectedOperationID(m.ops.rows, m.ops.index))
			}
		case "down", "j":
			if m.ops.index < len(m.ops.rows)-1 {
				m.ops.index++
				return m, loadOperationsCmd(m.session, selectedOperationID(m.ops.rows, m.ops.index))
			}
		case "r":
			return m, loadOperationsCmd(m.session, selectedOperationID(m.ops.rows, m.ops.index))
		case "x":
			job := selectedOperation(m.ops.rows, m.ops.index)
			if job == nil || !job.Cancellable {
				return m, nil
			}
			if m.session.Target().Kind == runtimeadapter.TargetKindEmbedded {
				m.openUpgradePrompt(deferredAction{kind: deferredOperationCancel, operationID: job.ID})
				return m, nil
			}
			return m, cancelOperationCmd(m.session, job.ID)
		}
		return m, nil
	case overlayCache:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.overlay = overlayNone
			m.overlayScroll = 0
			return m, nil
		case "r":
			return m, loadCacheCmd(m.session)
		case "up", "k":
			if m.cache.selected > cacheActionGit {
				m.cache.selected--
			}
		case "down", "j":
			if m.cache.selected < cacheActionSRU {
				m.cache.selected++
			}
		case "s":
			m.cacheSyncForm = newCacheSyncForm(m.session, m.cache.selected)
			m.overlay = overlayCacheSync
			return m, nil
		case "x":
			m.cacheClearForm = newCacheClearForm(m.session, m.cache.selected)
			m.overlay = overlayCacheClear
			return m, nil
		}
		return m, nil
	case overlaySync:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.overlay = overlayNone
			m.overlayScroll = 0
			return m, nil
		case "up", "k":
			if m.syncModal.selected > syncActionProject {
				m.syncModal.selected--
			}
		case "down", "j":
			if m.syncModal.selected < syncActionTeam {
				m.syncModal.selected++
			}
		case "enter":
			switch m.syncModal.selected {
			case syncActionProject:
				m.projectSyncForm = newProjectSyncForm(m.session)
				m.overlay = overlayProjectSync
			case syncActionBug:
				m.bugSyncForm = newBugSyncForm(m.session)
				m.overlay = overlayBugSync
			case syncActionTeam:
				m.teamSyncForm = newTeamSyncForm(m.session)
				m.overlay = overlayTeamSync
			}
		}
		return m, nil
	case overlayLogs:
		switch msg.String() {
		case "g":
			if m.pendingG {
				m.pendingG = false
				m.overlayScroll = 0
				return m, nil
			}
			m.pendingG = true
			return m, nil
		case "G":
			m.pendingG = false
			m.overlayScroll = viewportEndOffset()
			return m, nil
		default:
			m.pendingG = false
		}
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.overlay = overlayNone
			m.overlayScroll = 0
			return m, nil
		case "r":
			return m, loadLogsCmd(m.session, m.logs)
		case "pgdown", "ctrl+d", "down", "j":
			m.overlayScroll += m.scrollStep()
		case "pgup", "ctrl+u", "up", "k":
			m.overlayScroll -= m.scrollStep()
			if m.overlayScroll < 0 {
				m.overlayScroll = 0
			}
		}
		return m, nil
	case overlayServer:
		m.pendingG = false
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.overlay = overlayNone
			m.overlayScroll = 0
			return m, nil
		case "s", "enter":
			if m.session.Target().Kind == runtimeadapter.TargetKindEmbedded {
				m.openUpgradePrompt(deferredAction{kind: deferredSwitchServer})
			}
			return m, nil
		}
		return m, nil
	case overlayPrompt:
		m.pendingG = false
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.overlay = overlayNone
			m.overlayScroll = 0
			m.deferred = deferredAction{}
			return m, nil
		case "enter", "y":
			return m, upgradeSessionCmd(m.session)
		case "n":
			m.overlay = overlayNone
			m.deferred = deferredAction{}
			return m, nil
		}
		return m, nil
	case overlayBuildFilters:
		m.pendingG = false
		return m.updateBuildFilterForm(msg)
	case overlayReleaseFilters:
		m.pendingG = false
		return m.updateReleaseFilterForm(msg)
	case overlayBuildTrigger:
		m.pendingG = false
		return m.updateBuildTriggerForm(msg)
	case overlayPackageFilters:
		m.pendingG = false
		return m.updatePackageFilterForm(msg)
	case overlayBugFilters:
		m.pendingG = false
		return m.updateBugFilterForm(msg)
	case overlaySRUFilters:
		m.pendingG = false
		return m.updateSRUFilterForm(msg)
	case overlayReviewFilters:
		m.pendingG = false
		return m.updateReviewFilterForm(msg)
	case overlayCommitFilters:
		m.pendingG = false
		return m.updateCommitFilterForm(msg)
	case overlayProjectFilters:
		m.pendingG = false
		return m.updateProjectFilterForm(msg)
	case overlayProjectSync:
		m.pendingG = false
		return m.updateProjectSyncForm(msg)
	case overlayBugSync:
		m.pendingG = false
		return m.updateBugSyncForm(msg)
	case overlayTeamSync:
		m.pendingG = false
		return m.updateTeamSyncForm(msg)
	case overlayCacheSync:
		m.pendingG = false
		return m.updateCacheSyncForm(msg)
	case overlayCacheClear:
		m.pendingG = false
		return m.updateCacheClearForm(msg)
	case overlayBuildCleanup:
		m.pendingG = false
		return m.updateBuildCleanupForm(msg)
	}
	return m, nil
}

func (m *rootModel) openUpgradePrompt(action deferredAction) {
	m.deferred = action
	m.prompt = promptModel{
		title:  "Switch to local daemon?",
		body:   "This action needs durable state. Press Enter to switch, or Esc to stay embedded.",
		accept: "Switch",
		reject: "Stay embedded",
	}
	m.overlay = overlayPrompt
	m.overlayScroll = 0
}

func (m rootModel) resumeDeferredAction(action deferredAction) tea.Cmd {
	switch action.kind {
	case deferredLaunchpadAuthLogin:
		return beginLaunchpadAuthCmd(m.session)
	case deferredLaunchpadAuthLogout:
		return logoutLaunchpadAuthCmd(m.session)
	case deferredGitHubAuthLogin:
		return beginGitHubAuthCmd(m.session)
	case deferredGitHubAuthLogout:
		return logoutGitHubAuthCmd(m.session)
	case deferredOperationCancel:
		return cancelOperationCmd(m.session, action.operationID)
	case deferredBuildTrigger:
		return triggerBuildCmd(m.session, action.buildReq)
	case deferredSwitchServer:
		return loadLocalServerStatusCmd(m.session)
	default:
		return nil
	}
}

func (m *rootModel) setToast(message, level string) {
	m.toast = toastState{message: message, level: level}
}

func loadTUIBootstrapCmd(session *runtimeadapter.Session) tea.Cmd {
	return func() tea.Msg {
		if session == nil {
			return tuiBootstrapLoadedMsg{err: errors.New("runtime session not available")}
		}
		// Prefer the locally-loaded config so TUI presets work regardless of
		// which config file the server was started with.
		if session.Config != nil && session.Config.LocalConfig() != nil {
			return tuiBootstrapLoadedMsg{config: frontend.ConfigToDTO(session.Config.LocalConfig())}
		}
		if session.Frontend == nil {
			return tuiBootstrapLoadedMsg{err: errors.New("runtime session does not expose config workflow")}
		}
		cfg, err := session.Frontend.Config().Show(context.Background())
		if err != nil {
			return tuiBootstrapLoadedMsg{err: err}
		}
		return tuiBootstrapLoadedMsg{config: cfg}
	}
}

func loadProjectsFromConfigCmd(cfg *dto.Config, filters projectsFilters) tea.Cmd {
	return func() tea.Msg {
		if cfg == nil {
			return projectsLoadedMsg{err: errors.New("configuration is not loaded")}
		}
		return projectsLoadedMsg{
			config: cfg,
			rows:   summarizeProjects(cfg, filters),
		}
	}
}

func (m rootModel) initialLoadCmd(cfg *dto.Config) tea.Cmd {
	cmds := []tea.Cmd{
		loadDashboardCmd(m.session),
		loadBuildsCmd(m.session, m.builds.filters),
		loadReleasesCmd(m.session, m.releases.filters),
		loadPackagesCmd(m.session, m.packages.filters),
		loadBugsCmd(m.session, m.bugs.filters),
		loadSRUCmd(m.session, m.sru.filter),
		loadReviewsCmd(m.session, m.reviews.filters),
		loadCommitsCmd(m.session, m.commits.filters),
	}
	if cfg != nil {
		cmds = append(cmds, loadProjectsFromConfigCmd(cfg, m.projects.filters))
	} else {
		cmds = append(cmds, loadProjectsCmd(m.session, m.projects.filters))
	}
	return tea.Batch(cmds...)
}

func (m *rootModel) applyTUIConfig(cfg *dto.Config) {
	if m == nil || cfg == nil {
		return
	}
	if view, ok := parseTUIPane(cfg.TUI.DefaultPane); ok {
		m.activeView = view
	}
	m.builds.defaults = applyTUIBuildsDefaults(m.builds.defaults, cfg.TUI.Panes.Builds)
	m.builds.filters = m.builds.defaults
	m.releases.defaults = applyTUIReleasesDefaults(m.releases.defaults, cfg.TUI.Panes.Releases)
	m.releases.filters = m.releases.defaults
	m.packages.defaults = applyTUIPackagesDefaults(m.packages.defaults, cfg.TUI.Panes.Packages)
	m.packages.filters = m.packages.defaults
	m.bugs.defaults = applyTUIBugsDefaults(m.bugs.defaults, cfg.TUI.Panes.Bugs)
	m.bugs.filters = m.bugs.defaults
	m.reviews.defaults = applyTUIReviewsDefaults(m.reviews.defaults, cfg.TUI.Panes.Reviews)
	m.reviews.filters = m.reviews.defaults
	m.commits.defaults = applyTUICommitsDefaults(m.commits.defaults, cfg.TUI.Panes.Commits)
	m.commits.filters = m.commits.defaults
	m.projects.defaults = applyTUIProjectsDefaults(m.projects.defaults, cfg.TUI.Panes.Projects)
	m.projects.filters = m.projects.defaults
	m.projects.config = cfg
}

func parseTUIPane(raw string) (viewID, bool) {
	switch strings.TrimSpace(raw) {
	case "dashboard":
		return viewDashboard, true
	case "builds":
		return viewBuilds, true
	case "releases":
		return viewReleases, true
	case "packages":
		return viewPackages, true
	case "bugs":
		return viewBugs, true
	case "reviews":
		return viewReviews, true
	case "commits":
		return viewCommits, true
	case "projects":
		return viewProjects, true
	case "sru":
		return viewSRU, true
	default:
		return viewDashboard, false
	}
}

func applyTUIBuildsDefaults(base buildsFilters, pane *dto.TUIBuildsPaneConfig) buildsFilters {
	if pane == nil {
		return base
	}
	if pane.Filters.Project != "" {
		base.project = pane.Filters.Project
	}
	if pane.Filters.State != "" {
		base.state = pane.Filters.State
	}
	if pane.Filters.Active != nil {
		base.active = *pane.Filters.Active
	}
	if pane.Filters.Source != "" {
		base.source = pane.Filters.Source
	}
	return base
}

func applyTUIReleasesDefaults(base releasesFilters, pane *dto.TUIReleasesPaneConfig) releasesFilters {
	if pane == nil {
		return base
	}
	if pane.Filters.Project != "" {
		base.project = pane.Filters.Project
	}
	if pane.Filters.ArtifactType != "" {
		base.artifactType = pane.Filters.ArtifactType
	}
	if pane.Filters.Risk != "" {
		base.risk = pane.Filters.Risk
	}
	if pane.Filters.Track != "" {
		base.track = pane.Filters.Track
	}
	if pane.Filters.Branch != "" {
		base.branch = pane.Filters.Branch
	}
	if pane.Filters.TargetProfile != "" {
		base.targetProfile = pane.Filters.TargetProfile
	}
	if pane.Filters.AllTargets != nil {
		base.allTargets = *pane.Filters.AllTargets
	}
	return base
}

func applyTUIPackagesDefaults(base packagesFilters, pane *dto.TUIPackagesPaneConfig) packagesFilters {
	if pane == nil {
		return base
	}
	if mode, err := parsePackageMode(pane.Mode); err == nil && pane.Mode != "" {
		base.mode = mode
	}
	if pane.Filters.Set != "" {
		base.set = pane.Filters.Set
	}
	if pane.Filters.Distro != "" {
		base.distro = pane.Filters.Distro
	}
	if pane.Filters.Release != "" {
		base.release = pane.Filters.Release
	}
	if pane.Filters.Suite != "" {
		base.suite = pane.Filters.Suite
	}
	if pane.Filters.Component != "" {
		base.component = pane.Filters.Component
	}
	if pane.Filters.Backport != "" {
		base.backport = pane.Filters.Backport
	}
	if pane.Filters.Merge != nil {
		base.merge = *pane.Filters.Merge
	}
	if pane.Filters.UpstreamRelease != "" {
		base.upstreamRelease = pane.Filters.UpstreamRelease
	}
	if pane.Filters.BehindUpstream != nil {
		base.behindUpstream = *pane.Filters.BehindUpstream
	}
	if pane.Filters.OnlyIn != "" {
		base.onlyIn = pane.Filters.OnlyIn
	}
	if pane.Filters.Constraints != "" {
		base.constraints = pane.Filters.Constraints
	}
	if pane.Filters.Tracker != "" {
		base.tracker = pane.Filters.Tracker
	}
	if pane.Filters.Name != "" {
		base.name = pane.Filters.Name
	}
	if pane.Filters.Team != "" {
		base.team = pane.Filters.Team
	}
	if pane.Filters.FTBFS != nil {
		base.ftbfs = *pane.Filters.FTBFS
	}
	if pane.Filters.Autopkgtest != nil {
		base.autopkgtest = *pane.Filters.Autopkgtest
	}
	if pane.Filters.BlockedBy != "" {
		base.blockedBy = pane.Filters.BlockedBy
	}
	if pane.Filters.Bugged != nil {
		base.bugged = *pane.Filters.Bugged
	}
	if pane.Filters.MinAge != "" {
		base.minAge = pane.Filters.MinAge
	}
	if pane.Filters.MaxAge != "" {
		base.maxAge = pane.Filters.MaxAge
	}
	if pane.Filters.Limit != "" {
		base.limit = pane.Filters.Limit
	}
	if pane.Filters.Reverse != nil {
		base.reverse = *pane.Filters.Reverse
	}
	return base
}

func applyTUIBugsDefaults(base bugsFilters, pane *dto.TUIBugsPaneConfig) bugsFilters {
	if pane == nil {
		return base
	}
	if pane.Filters.Project != "" {
		base.project = pane.Filters.Project
	}
	if pane.Filters.Status != "" {
		base.status = pane.Filters.Status
	}
	if pane.Filters.Importance != "" {
		base.importance = pane.Filters.Importance
	}
	if pane.Filters.Assignee != "" {
		base.assignee = pane.Filters.Assignee
	}
	if pane.Filters.Tag != "" {
		base.tag = pane.Filters.Tag
	}
	if pane.Filters.Since != "" {
		base.since = pane.Filters.Since
	}
	if pane.Filters.Merge != nil {
		base.merge = *pane.Filters.Merge
	}
	if pane.Filters.Limit != "" {
		base.limit = pane.Filters.Limit
	}
	return base
}

func applyTUIReviewsDefaults(base reviewsFilters, pane *dto.TUIReviewsPaneConfig) reviewsFilters {
	if pane == nil {
		return base
	}
	if pane.Filters.Project != "" {
		base.project = pane.Filters.Project
	}
	if pane.Filters.Forge != "" {
		base.forge = pane.Filters.Forge
	}
	if pane.Filters.State != "" {
		base.state = pane.Filters.State
	}
	if pane.Filters.Author != "" {
		base.author = pane.Filters.Author
	}
	if pane.Filters.Since != "" {
		base.since = pane.Filters.Since
	}
	return base
}

func applyTUICommitsDefaults(base commitsFilters, pane *dto.TUICommitsPaneConfig) commitsFilters {
	if pane == nil {
		return base
	}
	if mode, err := parseCommitMode(pane.Mode); err == nil && pane.Mode != "" {
		base.mode = mode
	}
	if pane.Filters.Project != "" {
		base.project = pane.Filters.Project
	}
	if pane.Filters.Forge != "" {
		base.forge = pane.Filters.Forge
	}
	if pane.Filters.Branch != "" {
		base.branch = pane.Filters.Branch
	}
	if pane.Filters.Author != "" {
		base.author = pane.Filters.Author
	}
	if pane.Filters.IncludeMRs != nil {
		base.includeMRs = *pane.Filters.IncludeMRs
	}
	if pane.Filters.BugID != "" {
		base.bugID = pane.Filters.BugID
	}
	return base
}

func applyTUIProjectsDefaults(base projectsFilters, pane *dto.TUIProjectsPaneConfig) projectsFilters {
	if pane == nil {
		return base
	}
	if pane.Filters.Name != "" {
		base.name = pane.Filters.Name
	}
	if pane.Filters.ArtifactType != "" {
		base.artifactType = pane.Filters.ArtifactType
	}
	if pane.Filters.CodeForge != "" {
		base.codeForge = pane.Filters.CodeForge
	}
	if pane.Filters.BugForge != "" {
		base.bugForge = pane.Filters.BugForge
	}
	if pane.Filters.HasBuild != "" {
		base.hasBuild = pane.Filters.HasBuild
	}
	if pane.Filters.HasRelease != "" {
		base.hasRelease = pane.Filters.HasRelease
	}
	return base
}

func (m rootModel) View() string {
	if m.width == 0 {
		m.width = 120
	}
	if m.height == 0 {
		m.height = 40
	}

	header := m.renderHeader()
	tabs := m.renderTabs()
	content := m.renderContent()
	status := m.renderStatusBar()

	bodyHeight := m.height - 4
	if bodyHeight < 10 {
		bodyHeight = 10
	}
	content = renderViewport(content, bodyHeight, m.contentScroll())
	base := lipgloss.JoinVertical(lipgloss.Left, header, tabs, content, status)
	if m.overlay != overlayNone {
		return m.renderOverlay(base)
	}
	return base
}

func (m rootModel) renderHeader() string {
	target := m.session.Target()
	authText := renderAuthSummaryText(m.dashboard.auth)
	left := m.theme.header.Render("watchtower-tui") + " " + m.theme.badge.Render(string(target.Kind))
	if target.Address != "" {
		left += " " + m.theme.metadata.Render(target.Address)
	}
	right := m.theme.subtle.Render(authText)
	if !m.lastRefresh.IsZero() {
		right += "  " + m.theme.subtle.Render("Refreshed "+m.lastRefresh.Format("15:04:05"))
	}
	if m.toast.message != "" {
		right += "  " + renderToast(m.theme, m.toast)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer(maxInt(1, m.width-lipgloss.Width(left)-lipgloss.Width(right))), right)
}

func (m rootModel) renderTabs() string {
	tabs := []string{
		m.renderTab("1 Dashboard", m.activeView == viewDashboard),
		m.renderTab("2 Builds", m.activeView == viewBuilds),
		m.renderTab("3 Releases", m.activeView == viewReleases),
		m.renderTab("4 Packages", m.activeView == viewPackages),
		m.renderTab("5 Bugs", m.activeView == viewBugs),
		m.renderTab("6 Reviews", m.activeView == viewReviews),
		m.renderTab("7 Commits", m.activeView == viewCommits),
		m.renderTab("8 Projects", m.activeView == viewProjects),
		m.renderTab("9 SRUs", m.activeView == viewSRU),
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m rootModel) renderTab(label string, active bool) string {
	if active {
		return m.theme.tabActive.Render(label)
	}
	return m.theme.tab.Render(label)
}

func (m rootModel) renderContent() string {
	switch m.activeView {
	case viewDashboard:
		return m.renderDashboard()
	case viewBuilds:
		return m.renderBuilds()
	case viewReleases:
		return m.renderReleases()
	case viewPackages:
		return m.renderPackages()
	case viewBugs:
		return m.renderBugs()
	case viewReviews:
		return m.renderReviews()
	case viewCommits:
		return m.renderCommits()
	case viewProjects:
		return m.renderProjects()
	case viewSRU:
		return m.renderSRU()
	default:
		return ""
	}
}

func (m rootModel) renderStatusBar() string {
	target := strings.ToUpper(string(m.session.Target().Kind))
	auth := renderAuthSummaryText(m.dashboard.auth)
	runningOps := countRunningOperations(m.dashboard.ops)
	left := fmt.Sprintf("Mode %s  %s  Ops %d/%d", target, auth, runningOps, len(m.dashboard.ops))
	right := "m Meta  a Auth  o Ops  c Cache  l Logs  s Server  r Refresh  esc Reset  q Quit"
	if m.pendingG {
		right = "g- (pending jump)  " + right
	}
	return m.theme.statusBar.Width(m.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			m.theme.statusLeft.Render(left),
			spacer(maxInt(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)),
			m.theme.statusRight.Render(right),
		),
	)
}

func (m rootModel) renderDashboard() string {
	const gap = 1
	sections := []string{
		m.renderDashboardSection(0, "Runtime / Auth", m.renderDashboardRuntime(), dashboardSectionWidth(m.width, false)),
		m.renderDashboardSection(1, "Active Operations", m.renderDashboardOperations(), dashboardSectionWidth(m.width, false)),
		m.renderDashboardSection(2, "Recent Builds", m.renderDashboardBuilds(), dashboardSectionWidth(m.width, false)),
		m.renderDashboardSection(3, "Release Cache Freshness", m.renderDashboardReleases(), dashboardSectionWidth(m.width, false)),
	}
	if m.width >= 120 {
		colWidth := dashboardSectionWidth(m.width, true)
		sections[0] = m.renderDashboardSection(0, "Runtime / Auth", m.renderDashboardRuntime(), colWidth)
		sections[1] = m.renderDashboardSection(1, "Active Operations", m.renderDashboardOperations(), colWidth)
		sections[2] = m.renderDashboardSection(2, "Recent Builds", m.renderDashboardBuilds(), colWidth)
		sections[3] = m.renderDashboardSection(3, "Release Cache Freshness", m.renderDashboardReleases(), colWidth)
		left := lipgloss.JoinVertical(lipgloss.Left, sections[0], sections[1])
		right := lipgloss.JoinVertical(lipgloss.Left, sections[2], sections[3])
		return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer(gap), right)
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m rootModel) renderDashboardSection(idx int, title, body string, width int) string {
	style := m.theme.panel
	if m.dashboard.section == idx {
		style = style.BorderForeground(lipgloss.Color("#7DD3FC"))
	}
	return renderPanel(style, width, m.theme.panelTitle.Render(title), body)
}

func (m rootModel) renderDashboardRuntime() string {
	target := m.session.Target()
	lines := []string{
		fmt.Sprintf("Target: %s", target.Kind),
		fmt.Sprintf("Address: %s", target.Address),
	}
	lines = append(lines, renderProviderStatusLine("Launchpad", launchpadStatusFromAuth(m.dashboard.auth)))
	lines = append(lines, renderProviderStatusLine("GitHub", githubStatusFromAuth(m.dashboard.auth)))
	return strings.Join(lines, "\n")
}

func (m rootModel) renderDashboardOperations() string {
	if len(m.dashboard.ops) == 0 {
		return m.theme.subtle.Render("No operations yet.")
	}
	lines := make([]string, 0, minInt(5, len(m.dashboard.ops)))
	for _, job := range m.dashboard.ops[:minInt(5, len(m.dashboard.ops))] {
		lines = append(lines, fmt.Sprintf("%s  %s", m.theme.semantic(string(job.State)), job.Kind))
	}
	return strings.Join(lines, "\n")
}

func (m rootModel) renderDashboardBuilds() string {
	if len(m.dashboard.builds) == 0 {
		return m.theme.subtle.Render("No builds loaded.")
	}
	lines := make([]string, 0, minInt(5, len(m.dashboard.builds)))
	for _, build := range m.dashboard.builds[:minInt(5, len(m.dashboard.builds))] {
		lines = append(lines, fmt.Sprintf("%s  %s  %s", build.State.String(), build.Project, build.Title))
	}
	return strings.Join(lines, "\n")
}

func (m rootModel) renderDashboardReleases() string {
	if m.dashboard.cache == nil || len(m.dashboard.cache.Releases.Entries) == 0 {
		return m.theme.subtle.Render("No release cache status.")
	}
	entries := append([]dto.ReleaseCacheStatus(nil), m.dashboard.cache.Releases.Entries...)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].LastUpdated.After(entries[j].LastUpdated)
	})
	lines := make([]string, 0, minInt(5, len(entries)))
	for _, entry := range entries[:minInt(5, len(entries))] {
		lines = append(lines, fmt.Sprintf("%s  %s  %s  %s", entry.Project, entry.ArtifactType.String(), entry.Name, entry.LastUpdated.Format("2006-01-02 15:04")))
	}
	return strings.Join(lines, "\n")
}

func (m rootModel) renderBuilds() string {
	const gap = 1
	listWidth, detailWidth := splitColumns(m.width, gap)
	header := m.theme.panelTitle.Render("Filters") + "\n" +
		fmt.Sprintf("project=%s  state=%s  active=%t  source=%s",
			emptyAsAny(m.builds.filters.project),
			emptyAsAny(m.builds.filters.state),
			m.builds.filters.active,
			emptyAsAny(m.builds.filters.source),
		)
	list := renderBuildRows(m.theme, m.builds.rows, m.builds.index, innerPanelWidth(m.theme.panel, listWidth), m.builds.loaded)
	detail := renderBuildDetail(m.theme, selectedBuild(m.builds.rows, m.builds.index), innerPanelWidth(m.theme.panel, detailWidth))
	if m.width >= 120 {
		left := renderPanel(m.theme.panel, listWidth, "", header+"\n\n"+list)
		right := renderPanel(m.theme.panel, detailWidth, m.theme.panelTitle.Render("Detail"), detail)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer(gap), right)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		renderPanel(m.theme.panel, m.width, "", header),
		renderPanel(m.theme.panel, m.width, m.theme.panelTitle.Render("Builds"), list),
		renderPanel(m.theme.panel, m.width, m.theme.panelTitle.Render("Detail"), detail),
	)
}

func (m rootModel) renderReleases() string {
	const gap = 1
	listWidth, detailWidth := splitColumns(m.width, gap)
	header := m.theme.panelTitle.Render("Filters") + "\n" +
		fmt.Sprintf("project=%s  type=%s  risk=%s  track=%s  branch=%s",
			emptyAsAny(m.releases.filters.project),
			emptyAsAny(m.releases.filters.artifactType),
			emptyAsAny(m.releases.filters.risk),
			emptyAsAny(m.releases.filters.track),
			emptyAsAny(m.releases.filters.branch),
		)
	list := renderReleaseArtifacts(m.theme, m.releases.artifacts, m.releases.index, innerPanelWidth(m.theme.panel, listWidth), m.releases.loaded)
	detail := renderReleaseDetail(m.theme, m.releases.detail, m.selectedReleaseArtifact(), innerPanelWidth(m.theme.panel, detailWidth))
	if m.width >= 120 {
		left := renderPanel(m.theme.panel, listWidth, m.theme.panelTitle.Render("Artifacts"), header+"\n\n"+list)
		right := renderPanel(m.theme.panel, detailWidth, m.theme.panelTitle.Render("Detail"), detail)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer(gap), right)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		renderPanel(m.theme.panel, m.width, "", header),
		renderPanel(m.theme.panel, m.width, m.theme.panelTitle.Render("Releases"), list),
		renderPanel(m.theme.panel, m.width, m.theme.panelTitle.Render("Detail"), detail),
	)
}

func (m rootModel) renderOverlay(base string) string {
	var content string
	fullscreen := m.width < 90 || m.overlay == overlayHelp
	switch m.overlay {
	case overlayHelp:
		content = m.renderHelp()
	case overlayAuth:
		content = m.renderAuthModal()
	case overlayOperations:
		content = m.renderOperationsDrawer(fullscreen)
	case overlayCache:
		content = m.renderCacheModal()
	case overlaySync:
		content = m.renderSyncModal()
	case overlayLogs:
		content = m.renderLogsModal()
	case overlayServer:
		content = m.renderServerModal()
	case overlayPrompt:
		content = m.renderPrompt()
	case overlayBuildFilters:
		content = renderFormModal(m.theme, m.buildFilterForm, m.width, m.height)
	case overlayReleaseFilters:
		content = renderFormModal(m.theme, m.releaseFilterForm, m.width, m.height)
	case overlayBuildTrigger:
		content = renderFormModal(m.theme, m.buildTriggerForm, m.width, m.height)
	case overlayPackageFilters:
		content = renderFormModal(m.theme, m.packageFilterForm, m.width, m.height)
	case overlayBugFilters:
		content = renderFormModal(m.theme, m.bugFilterForm, m.width, m.height)
	case overlaySRUFilters:
		content = renderFormModal(m.theme, m.sruFilterForm, m.width, m.height)
	case overlayReviewFilters:
		content = renderFormModal(m.theme, m.reviewFilterForm, m.width, m.height)
	case overlayCommitFilters:
		content = renderFormModal(m.theme, m.commitFilterForm, m.width, m.height)
	case overlayProjectFilters:
		content = renderFormModal(m.theme, m.projectFilterForm, m.width, m.height)
	case overlayProjectSync:
		content = renderFormModal(m.theme, m.projectSyncForm, m.width, m.height)
	case overlayBugSync:
		content = renderFormModal(m.theme, m.bugSyncForm, m.width, m.height)
	case overlayTeamSync:
		content = renderFormModal(m.theme, m.teamSyncForm, m.width, m.height)
	case overlayCacheSync:
		content = renderFormModal(m.theme, m.cacheSyncForm, m.width, m.height)
	case overlayCacheClear:
		content = renderFormModal(m.theme, m.cacheClearForm, m.width, m.height)
	case overlayBuildCleanup:
		content = renderFormModal(m.theme, m.buildCleanupForm, m.width, m.height)
	}
	if isCenteredFormOverlay(m.overlay) {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}
	if fullscreen {
		return renderViewport(content, m.height-1, m.overlayScroll)
	}
	return lipgloss.JoinVertical(lipgloss.Left, base, renderViewport(content, maxInt(8, m.height/3), m.overlayScroll))
}

func isCenteredFormOverlay(kind overlayKind) bool {
	switch kind {
	case overlayBuildFilters, overlayReleaseFilters, overlayBuildTrigger, overlayBuildCleanup, overlayPackageFilters, overlayBugFilters, overlayReviewFilters, overlayCommitFilters, overlayProjectFilters, overlayProjectSync, overlayBugSync, overlayTeamSync, overlayCacheSync, overlayCacheClear:
		return true
	default:
		return false
	}
}

func (m rootModel) renderHelp() string {
	body := strings.Join([]string{
		"Shortcuts",
		"1..9 switch workflow",
		"Tab / Shift+Tab cycle workflows",
		"[ / ] cycle submodes on Packages and Commits",
		"j/k or arrows move selection",
		"gg jump to the beginning",
		"G jump to the end",
		"PgUp/PgDn or Ctrl+U/Ctrl+D scroll",
		"Enter open detail or action",
		"/ edit filters",
		"m or ? open this meta pane",
		"a auth  o operations  c cache  u sync  l logs  s server",
		"r reload snapshot  R refresh SRUs  O open SRU bug",
		"q quit  esc close overlay",
		"",
		"Forms",
		"Tab / Shift+Tab move between fields",
		"Enter applies filters immediately",
		"Text fields keep autocomplete suggestions",
		"Enum fields use picker lists instead of free-text autocomplete",
		"",
		"Embedded rule",
		"Auth, build trigger, and operation cancel prompt before switching to the local daemon.",
	}, "\n")
	return m.theme.panel.Width(m.width - 2).Height(m.height - 2).Render(m.theme.panelTitle.Render("Meta") + "\n\n" + body)
}

func (m rootModel) renderAuthModal() string {
	lines := []string{m.theme.panelTitle.Render("Auth")}
	if m.auth.err != "" {
		lines = append(lines, m.theme.errorText.Render(m.auth.err))
	}
	lines = append(lines, renderProviderStatusLine("Launchpad", launchpadStatusFromAuth(m.auth.status)))
	lines = append(lines, renderProviderStatusDetail("Launchpad", launchpadStatusFromAuth(m.auth.status))...)
	lines = append(lines, renderProviderStatusLine("GitHub", githubStatusFromAuth(m.auth.status)))
	lines = append(lines, renderProviderStatusDetail("GitHub", githubStatusFromAuth(m.auth.status))...)
	lines = append(lines, renderProviderStatusLine("Snap Store", snapStoreStatusFromAuth(m.auth.status)))
	lines = append(lines, renderProviderStatusDetail("Snap Store", snapStoreStatusFromAuth(m.auth.status))...)
	lines = append(lines, renderProviderStatusLine("Charmhub", charmhubStatusFromAuth(m.auth.status)))
	lines = append(lines, renderProviderStatusDetail("Charmhub", charmhubStatusFromAuth(m.auth.status))...)
	if m.auth.launchpadBegin != nil {
		lines = append(lines, "", "Launchpad authorize URL:", m.auth.launchpadBegin.AuthorizeURL)
	}
	if m.auth.githubBegin != nil {
		lines = append(lines, "", "GitHub verification URI:", m.auth.githubBegin.VerificationURI, "GitHub code: "+m.auth.githubBegin.UserCode)
	}
	lines = append(lines, "", "[l] LP login  [x] LP logout  [h] GH login  [d] GH logout  [o] open browser  [Esc] close")
	return m.theme.panel.Width(maxInt(50, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m rootModel) renderOperationsDrawer(fullscreen bool) string {
	title := m.theme.panelTitle.Render("Operations")
	rows := renderOperationRows(m.theme, m.ops.rows, m.ops.index)
	events := renderOperationEvents(m.theme, m.ops.events)
	box := m.theme.panel.Width(m.width - 2).Render(title + "\n\n" + rows + "\n\n" + m.theme.panelTitle.Render("Events") + "\n" + events + "\n\n[x] cancel  [r] refresh  [Esc] close")
	if fullscreen {
		return lipgloss.NewStyle().Height(m.height - 2).Render(box)
	}
	return box
}

func (m rootModel) renderCacheModal() string {
	lines := []string{m.theme.panelTitle.Render("Cache")}
	if m.cache.err != "" {
		lines = append(lines, m.theme.errorText.Render(m.cache.err))
	}
	if m.cache.status == nil {
		lines = append(lines, m.theme.subtle.Render("No cache status loaded."))
	} else {
		lines = append(lines, "")
		for i, row := range m.cacheRows() {
			line := fitLine(row, maxInt(40, m.width-8))
			if cacheActionTarget(i) == m.cache.selected {
				line = m.theme.selectedRow.Render(line)
			}
			lines = append(lines, line)
		}
	}
	if m.cache.lastAction != "" {
		lines = append(lines, "", m.theme.panelTitle.Render("Last Action"), m.cache.lastAction)
		if len(m.cache.lastSummary) == 0 {
			lines = append(lines, m.theme.subtle.Render("(no details)"))
		} else {
			lines = append(lines, m.cache.lastSummary...)
		}
	}
	lines = append(lines, "", "[Up/Down] select  [s] sync  [x] clear  [r] refresh  [Esc] close")
	return m.theme.panel.Width(maxInt(50, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m rootModel) renderSyncModal() string {
	lines := []string{m.theme.panelTitle.Render("Sync")}
	if m.syncModal.err != "" {
		lines = append(lines, m.theme.errorText.Render(m.syncModal.err))
	}
	lines = append(lines, "")
	for i, row := range m.syncRows() {
		line := fitLine(row, maxInt(40, m.width-8))
		if syncActionTarget(i) == m.syncModal.selected {
			line = m.theme.selectedRow.Render(line)
		}
		lines = append(lines, line)
	}
	if m.syncModal.lastAction != "" {
		lines = append(lines, "", m.theme.panelTitle.Render("Last Action"), m.syncModal.lastAction)
		if len(m.syncModal.lastSummary) == 0 {
			lines = append(lines, m.theme.subtle.Render("(no details)"))
		} else {
			lines = append(lines, m.syncModal.lastSummary...)
		}
	}
	lines = append(lines, "", "[Up/Down] select  [Enter] open  [Esc] close")
	return m.theme.panel.Width(maxInt(50, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m rootModel) renderLogsModal() string {
	lines := []string{m.theme.panelTitle.Render("Logs")}
	if m.logsModal.err != "" {
		lines = append(lines, m.theme.errorText.Render(m.logsModal.err))
	}
	lines = append(lines, "", "Session Logs:")
	if len(m.logsModal.sessionLines) == 0 {
		lines = append(lines, m.theme.subtle.Render("(none)"))
	} else {
		lines = append(lines, m.logsModal.sessionLines...)
	}
	lines = append(lines, "")
	if m.logsModal.daemonNote != "" {
		lines = append(lines, m.theme.subtle.Render(m.logsModal.daemonNote))
	} else {
		lines = append(lines, "Daemon Logs:")
		if len(m.logsModal.daemonLines) == 0 {
			lines = append(lines, m.theme.subtle.Render("(none)"))
		} else {
			lines = append(lines, m.logsModal.daemonLines...)
		}
	}
	lines = append(lines, "", "[r] refresh  [Esc] close")
	return m.theme.panel.Width(maxInt(60, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m rootModel) renderServerModal() string {
	target := m.session.Target()
	lines := []string{
		m.theme.panelTitle.Render("Server / About"),
		fmt.Sprintf("Version: %s", Version),
		fmt.Sprintf("Target: %s", target.Kind),
		fmt.Sprintf("Address: %s", emptyAsDash(target.Address)),
	}
	if m.server.local != nil {
		lines = append(lines,
			fmt.Sprintf("Local daemon running: %t", m.server.local.Running),
			fmt.Sprintf("PID: %d", m.server.local.PID),
			fmt.Sprintf("Log file: %s", emptyAsDash(m.server.local.LogFile)),
		)
	}
	if target.Kind == runtimeadapter.TargetKindEmbedded {
		lines = append(lines, "", "[Enter] switch to local daemon")
	}
	lines = append(lines, "[Esc] close")
	return m.theme.panel.Width(maxInt(50, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m rootModel) renderPrompt() string {
	lines := []string{
		m.theme.panelTitle.Render(m.prompt.title),
		"",
		m.prompt.body,
		"",
		"[Enter] " + m.prompt.accept,
		"[Esc] " + m.prompt.reject,
	}
	return m.theme.panel.Width(maxInt(50, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m rootModel) refreshActiveView() tea.Cmd {
	switch m.activeView {
	case viewDashboard:
		return loadDashboardCmd(m.session)
	case viewBuilds:
		return loadBuildsCmd(m.session, m.builds.filters)
	case viewReleases:
		return tea.Batch(
			loadReleasesCmd(m.session, m.releases.filters),
			loadReleaseDetailCmdIfSelected(m.session, m.selectedReleaseArtifact(), m.releases.filters),
		)
	case viewPackages:
		return tea.Batch(loadPackagesCmd(m.session, m.packages.filters), loadPackageDetailCmd(m.session, m.packages))
	case viewBugs:
		return tea.Batch(loadBugsCmd(m.session, m.bugs.filters), loadBugDetailCmdIfSelected(m.session, selectedBug(m.bugs.rows, m.bugs.index)))
	case viewSRU:
		return loadSRUCmd(m.session, m.sru.filter)
	case viewReviews:
		return tea.Batch(loadReviewsCmd(m.session, m.reviews.filters), loadReviewDetailCmdIfSelected(m.session, selectedReview(m.reviews.rows, m.reviews.index)))
	case viewCommits:
		return loadCommitsCmd(m.session, m.commits.filters)
	case viewProjects:
		return loadProjectsCmd(m.session, m.projects.filters)
	default:
		return nil
	}
}

func (m rootModel) jumpActiveTop() (tea.Model, tea.Cmd) {
	m.setContentScroll(0)
	switch m.activeView {
	case viewDashboard:
		m.dashboard.section = 0
	case viewBuilds:
		m.builds.index = 0
	case viewReleases:
		if len(m.releases.artifacts) == 0 {
			return m, nil
		}
		m.releases.index = 0
		if artifact := m.selectedReleaseArtifact(); artifact != nil {
			return m, loadReleaseDetailCmd(m.session, *artifact, m.releases.filters)
		}
	case viewPackages:
		m.packages.index = 0
		return m, loadPackageDetailCmd(m.session, m.packages)
	case viewBugs:
		m.bugs.index = 0
		return m, loadBugDetailCmdIfSelected(m.session, selectedBug(m.bugs.rows, m.bugs.index))
	case viewSRU:
		m.sru.index = 0
	case viewReviews:
		m.reviews.index = 0
		return m, loadReviewDetailCmdIfSelected(m.session, selectedReview(m.reviews.rows, m.reviews.index))
	case viewCommits:
		m.commits.index = 0
	case viewProjects:
		m.projects.index = 0
	}
	return m, nil
}

func (m rootModel) jumpActiveBottom() (tea.Model, tea.Cmd) {
	m.setContentScroll(viewportEndOffset())
	switch m.activeView {
	case viewDashboard:
		m.dashboard.section = 3
	case viewBuilds:
		if len(m.builds.rows) > 0 {
			m.builds.index = len(m.builds.rows) - 1
		}
	case viewReleases:
		if len(m.releases.artifacts) == 0 {
			return m, nil
		}
		m.releases.index = len(m.releases.artifacts) - 1
		if artifact := m.selectedReleaseArtifact(); artifact != nil {
			return m, loadReleaseDetailCmd(m.session, *artifact, m.releases.filters)
		}
	case viewPackages:
		if m.packages.rowCount() > 0 {
			m.packages.index = m.packages.rowCount() - 1
		}
		return m, loadPackageDetailCmd(m.session, m.packages)
	case viewBugs:
		if len(m.bugs.rows) > 0 {
			m.bugs.index = len(m.bugs.rows) - 1
		}
		return m, loadBugDetailCmdIfSelected(m.session, selectedBug(m.bugs.rows, m.bugs.index))
	case viewSRU:
		if m.sru.snapshot != nil && len(m.sru.snapshot.Rows) > 0 {
			m.sru.index = len(m.sru.snapshot.Rows) - 1
		}
	case viewReviews:
		if len(m.reviews.rows) > 0 {
			m.reviews.index = len(m.reviews.rows) - 1
		}
		return m, loadReviewDetailCmdIfSelected(m.session, selectedReview(m.reviews.rows, m.reviews.index))
	case viewCommits:
		if len(m.commits.rows) > 0 {
			m.commits.index = len(m.commits.rows) - 1
		}
	case viewProjects:
		if len(m.projects.rows) > 0 {
			m.projects.index = len(m.projects.rows) - 1
		}
	}
	return m, nil
}

func (m rootModel) selectedReleaseArtifact() *releaseArtifactSummary {
	if m.releases.index < 0 || m.releases.index >= len(m.releases.artifacts) {
		return nil
	}
	artifact := m.releases.artifacts[m.releases.index]
	return &artifact
}

func (m rootModel) releaseDetailKey() string {
	artifact := m.selectedReleaseArtifact()
	if artifact == nil {
		return ""
	}
	return fmt.Sprintf("%s|%s|%s", artifact.Project, artifact.Name, artifact.ArtifactType.String())
}

func (m rootModel) updateBuildFilterForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.buildFilterForm, func(values []string) tea.Cmd {
		m.builds.filters = buildsFilters{
			project: strings.TrimSpace(values[0]),
			state:   strings.TrimSpace(values[1]),
			active:  strings.TrimSpace(values[2]) != "false",
			source:  defaultString(strings.TrimSpace(values[3]), "remote"),
		}
		m.overlay = overlayNone
		return loadBuildsCmd(m.session, m.builds.filters)
	}, func() {
		m.overlay = overlayNone
	})
	return m, cmd
}

func (m rootModel) updateReleaseFilterForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.releaseFilterForm, func(values []string) tea.Cmd {
		allTargets, err := strconv.ParseBool(strings.TrimSpace(values[6]))
		if err != nil {
			m.releaseFilterForm.errorMsg = "all targets must be true or false"
			return nil
		}
		m.releases.filters = releasesFilters{
			project:       strings.TrimSpace(values[0]),
			artifactType:  strings.TrimSpace(values[1]),
			risk:          strings.TrimSpace(values[2]),
			track:         strings.TrimSpace(values[3]),
			branch:        strings.TrimSpace(values[4]),
			targetProfile: strings.TrimSpace(values[5]),
			allTargets:    allTargets,
		}
		m.overlay = overlayNone
		return loadReleasesCmd(m.session, m.releases.filters)
	}, func() {
		m.overlay = overlayNone
	})
	return m, cmd
}

func (m rootModel) updateBuildTriggerForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.buildTriggerForm, func(values []string) tea.Cmd {
		req, err := buildTriggerRequestFromValues(values)
		if err != nil {
			m.buildTriggerForm.errorMsg = err.Error()
			return nil
		}
		if req.Project == "" {
			m.buildTriggerForm.errorMsg = "project is required"
			return nil
		}
		m.overlay = overlayNone
		if m.session.Target().Kind == runtimeadapter.TargetKindEmbedded {
			m.openUpgradePrompt(deferredAction{kind: deferredBuildTrigger, buildReq: req})
			return nil
		}
		return triggerBuildCmd(m.session, req)
	}, func() {
		m.overlay = overlayNone
	})
	return m, cmd
}

func (m rootModel) updateBuildCleanupForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.buildCleanupForm, func(values []string) tea.Cmd {
		prefix := strings.TrimSpace(values[0])
		owner := strings.TrimSpace(values[1])
		dryRun := strings.TrimSpace(values[2]) != "false"
		m.overlay = overlayNone
		return cleanupBuildCmd(m.session, prefix, owner, dryRun)
	}, func() {
		m.overlay = overlayNone
	})
	return m, cmd
}

func (m rootModel) updateProjectSyncForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.projectSyncForm, func(values []string) tea.Cmd {
		req := frontend.ProjectSyncRequest{
			Projects: splitCSV(values[0]),
			DryRun:   strings.TrimSpace(values[1]) != "false",
		}
		m.overlay = overlaySync
		return syncProjectsCmd(m.session, req)
	}, func() {
		m.overlay = overlaySync
	})
	return m, cmd
}

func (m rootModel) updateBugSyncForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.bugSyncForm, func(values []string) tea.Cmd {
		req := frontend.BugSyncRequest{
			Projects: splitCSV(values[0]),
			BugIDs:   splitCSV(values[1]),
			Since:    strings.TrimSpace(values[2]),
			DryRun:   strings.TrimSpace(values[3]) != "false",
		}
		m.overlay = overlaySync
		return syncBugsCmd(m.session, req)
	}, func() {
		m.overlay = overlaySync
	})
	return m, cmd
}

func (m rootModel) updateTeamSyncForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.teamSyncForm, func(values []string) tea.Cmd {
		req := dto.TeamSyncRequest{
			Projects: splitCSV(values[0]),
			DryRun:   strings.TrimSpace(values[1]) != "false",
		}
		m.overlay = overlaySync
		return syncTeamsCmd(m.session, req)
	}, func() {
		m.overlay = overlaySync
	})
	return m, cmd
}

func (m rootModel) updateCacheSyncForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.cacheSyncForm, func(values []string) tea.Cmd {
		m.overlay = overlayCache
		return syncCacheCmd(m.session, m.cache.selected, values)
	}, func() {
		m.overlay = overlayCache
	})
	return m, cmd
}

func (m rootModel) updateCacheClearForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.cacheClearForm, func(values []string) tea.Cmd {
		m.overlay = overlayCache
		return clearCacheCmd(m.session, m.cache.selected, values)
	}, func() {
		m.overlay = overlayCache
	})
	return m, cmd
}

func buildTriggerRequestFromValues(values []string) (frontend.BuildTriggerRequest, error) {
	req := frontend.BuildTriggerRequest{
		Project:   strings.TrimSpace(values[0]),
		Artifacts: splitCSV(values[1]),
		Source:    defaultString(strings.TrimSpace(values[2]), "remote"),
		LocalPath: strings.TrimSpace(values[3]),
		Async:     true,
	}
	if req.Source != "remote" && req.Source != "local" {
		return frontend.BuildTriggerRequest{}, fmt.Errorf("source must be remote or local")
	}
	if req.Source == "local" && req.LocalPath == "" {
		return frontend.BuildTriggerRequest{}, fmt.Errorf("local path is required for local source")
	}
	return req, nil
}

func loadDashboardCmd(session *runtimeadapter.Session) tea.Cmd {
	return guardSessionAction(session, frontend.ActionDashboardRefresh, func() tea.Msg {
		ctx := context.Background()
		auth, authErr := session.Frontend.Auth().Status(ctx)
		ops, opsErr := session.Frontend.Operations().List(ctx)
		builds, buildsErr := session.Frontend.Builds().List(ctx, frontend.BuildListRequest{All: false, DefaultAll: false, Source: "remote"})
		cacheStatus, cacheErr := session.Frontend.Cache().Status(ctx)
		return dashboardLoadedMsg{
			auth:   auth,
			ops:    ops,
			builds: builds,
			cache:  cacheStatus,
			err:    errorsJoin(authErr, opsErr, buildsErr, cacheErr),
		}
	})
}

func loadBuildsCmd(session *runtimeadapter.Session, filters buildsFilters) tea.Cmd {
	return guardSessionAction(session, frontend.ActionBuildsRefresh, func() tea.Msg {
		rows, err := session.Frontend.Builds().List(context.Background(), frontend.BuildListRequest{
			Projects: firstNonEmptySlice(filters.project),
			State:    filters.state,
			All:      !filters.active,
			Source:   defaultString(filters.source, "remote"),
		})
		return buildsLoadedMsg{rows: rows, err: err}
	})
}

func loadReleasesCmd(session *runtimeadapter.Session, filters releasesFilters) tea.Cmd {
	return guardSessionAction(session, frontend.ActionReleasesRefresh, func() tea.Msg {
		rows, err := session.Frontend.Releases().List(context.Background(), frontend.ReleasesListRequest{
			Projects:      firstNonEmptySlice(filters.project),
			ArtifactType:  filters.artifactType,
			Risks:         firstNonEmptySlice(filters.risk),
			Tracks:        firstNonEmptySlice(filters.track),
			Branches:      firstNonEmptySlice(filters.branch),
			TargetProfile: filters.targetProfile,
			AllTargets:    filters.allTargets,
		})
		return releasesLoadedMsg{rows: rows, err: err}
	})
}

func loadReleaseDetailCmdIfSelected(session *runtimeadapter.Session, artifact *releaseArtifactSummary, filters releasesFilters) tea.Cmd {
	if artifact == nil {
		return nil
	}
	return loadReleaseDetailCmd(session, *artifact, filters)
}

func loadReleaseDetailCmd(session *runtimeadapter.Session, artifact releaseArtifactSummary, filters releasesFilters) tea.Cmd {
	key := fmt.Sprintf("%s|%s|%s", artifact.Project, artifact.Name, artifact.ArtifactType.String())
	return guardSessionAction(session, frontend.ActionReleaseDetailRefresh, func() tea.Msg {
		detail, err := session.Frontend.Releases().Show(context.Background(), frontend.ReleasesShowRequest{
			Name:          artifact.Name,
			ArtifactType:  artifact.ArtifactType.String(),
			TargetProfile: filters.targetProfile,
			AllTargets:    filters.allTargets,
		})
		return releaseDetailLoadedMsg{key: key, detail: detail, err: err}
	})
}

func loadOperationsCmd(session *runtimeadapter.Session, selectedID string) tea.Cmd {
	return guardSessionAction(session, frontend.ActionOperationsRefresh, func() tea.Msg {
		rows, err := session.Frontend.Operations().List(context.Background())
		var events []dto.OperationEvent
		if err == nil && selectedID != "" {
			events, err = session.Frontend.Operations().Events(context.Background(), selectedID)
		}
		return opsLoadedMsg{rows: rows, events: events, err: err}
	})
}

func loadAuthStatusCmd(session *runtimeadapter.Session) tea.Cmd {
	return guardSessionAction(session, frontend.ActionAuthRefresh, func() tea.Msg {
		status, err := session.Frontend.Auth().Status(context.Background())
		return authStatusLoadedMsg{status: status, err: err}
	})
}

func beginLaunchpadAuthCmd(session *runtimeadapter.Session) tea.Cmd {
	return guardSessionAction(session, frontend.ActionAuthLaunchpadBegin, func() tea.Msg {
		begin, err := session.Frontend.Auth().BeginLaunchpad(context.Background())
		return authBeginMsg{begin: begin, err: err}
	})
}

func logoutLaunchpadAuthCmd(session *runtimeadapter.Session) tea.Cmd {
	return guardSessionAction(session, frontend.ActionAuthLaunchpadLogout, func() tea.Msg {
		result, err := session.Frontend.Auth().LogoutLaunchpad(context.Background())
		return authLogoutMsg{result: result, err: err}
	})
}

func beginGitHubAuthCmd(session *runtimeadapter.Session) tea.Cmd {
	return guardSessionAction(session, frontend.ActionAuthGitHubBegin, func() tea.Msg {
		begin, err := session.Frontend.Auth().BeginGitHub(context.Background())
		return authGitHubBeginMsg{begin: begin, err: err}
	})
}

func finalizeGitHubAuthCmd(session *runtimeadapter.Session, flowID string, holder *cancelHolder) tea.Cmd {
	return guardSessionAction(session, frontend.ActionAuthGitHubFinalize, func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		holder.Set(cancel)
		defer cancel()
		result, err := session.Frontend.Auth().FinalizeGitHub(ctx, flowID)
		return authGitHubFinalizeMsg{result: result, err: err}
	})
}

func logoutGitHubAuthCmd(session *runtimeadapter.Session) tea.Cmd {
	return guardSessionAction(session, frontend.ActionAuthGitHubLogout, func() tea.Msg {
		result, err := session.Frontend.Auth().LogoutGitHub(context.Background())
		return authGitHubLogoutMsg{result: result, err: err}
	})
}

func loadCacheCmd(session *runtimeadapter.Session) tea.Cmd {
	return guardSessionAction(session, frontend.ActionCacheRefresh, func() tea.Msg {
		status, err := session.Frontend.Cache().Status(context.Background())
		return cacheLoadedMsg{status: status, err: err}
	})
}

func loadLogsCmd(session *runtimeadapter.Session, logs *logBuffer) tea.Cmd {
	return guardSessionAction(session, frontend.ActionLogsRefresh, func() tea.Msg {
		var sessionLines []string
		if logs != nil {
			sessionLines = logs.Snapshot()
		}
		msg := logsLoadedMsg{sessionLines: sessionLines}
		switch session.Target().Kind {
		case runtimeadapter.TargetKindDaemon:
			lines, err := session.ReadDaemonLogTail(200)
			if err != nil {
				msg.err = err
				msg.daemonNote = "Daemon log unavailable."
				return msg
			}
			msg.daemonLines = lines
		case runtimeadapter.TargetKindRemote:
			msg.daemonNote = "Remote server logs are not available locally."
		default:
			msg.daemonNote = "Daemon logs are only available when connected to the local persistent daemon."
		}
		return msg
	})
}

func loadLocalServerStatusCmd(session *runtimeadapter.Session) tea.Cmd {
	return guardSessionAction(session, frontend.ActionServerStatus, func() tea.Msg {
		status, err := session.LocalServerStatus(context.Background())
		return localServerStatusMsg{status: status, err: err}
	})
}

func syncProjectsCmd(session *runtimeadapter.Session, req frontend.ProjectSyncRequest) tea.Cmd {
	actionID := frontend.ActionProjectSyncApply
	if req.DryRun {
		actionID = frontend.ActionProjectSyncDryRun
	}
	return guardSessionAction(session, actionID, func() tea.Msg {
		result, err := session.Frontend.Projects().Sync(context.Background(), req)
		return projectSyncFinishedMsg{req: req, result: result, err: err}
	})
}

func syncBugsCmd(session *runtimeadapter.Session, req frontend.BugSyncRequest) tea.Cmd {
	actionID := frontend.ActionBugSyncApply
	if req.DryRun {
		actionID = frontend.ActionBugSyncDryRun
	}
	return guardSessionAction(session, actionID, func() tea.Msg {
		result, err := session.Frontend.Bugs().Sync(context.Background(), req)
		return bugSyncFinishedMsg{req: req, result: result, err: err}
	})
}

func syncTeamsCmd(session *runtimeadapter.Session, req dto.TeamSyncRequest) tea.Cmd {
	actionID := frontend.ActionTeamSyncApply
	if req.DryRun {
		actionID = frontend.ActionTeamSyncDryRun
	}
	return guardSessionAction(session, actionID, func() tea.Msg {
		result, err := session.Frontend.Teams().Sync(context.Background(), req)
		return teamSyncFinishedMsg{req: req, result: result, err: err}
	})
}

func syncCacheCmd(session *runtimeadapter.Session, target cacheActionTarget, values []string) tea.Cmd {
	actionID := cacheSyncActionID(target)
	return guardSessionAction(session, actionID, func() tea.Msg {
		ctx := context.Background()
		action := "Cache sync completed"
		var summary []string
		var err error
		switch target {
		case cacheActionGit:
			var result *frontend.CacheSyncGitResponse
			result, err = session.Frontend.Cache().SyncGit(ctx, frontend.CacheSyncGitRequest{Projects: splitCSV(firstValue(values))})
			if err == nil {
				action = "Git cache sync completed"
				summary = append(summary, fmt.Sprintf("Synced: %d", result.Synced))
				summary = append(summary, result.Warnings...)
			}
		case cacheActionPackages:
			err = session.Frontend.Cache().SyncPackagesIndex(ctx, frontend.CacheSyncPackagesIndexRequest{
				Distros:   splitCSV(firstValue(values)),
				Releases:  splitCSV(valueAt(values, 1)),
				Backports: splitCSV(valueAt(values, 2)),
			})
			if err == nil {
				action = "Package index cache sync completed"
				summary = []string{"Status: ok"}
			}
		case cacheActionUpstream:
			var result *frontend.CacheSyncUpstreamResponse
			result, err = session.Frontend.Cache().SyncUpstream(ctx)
			if err == nil {
				action = "Upstream cache sync completed"
				summary = []string{"Status: " + result.Status}
			}
		case cacheActionBugs:
			var result *frontend.CacheSyncBugsResponse
			result, err = session.Frontend.Cache().SyncBugs(ctx, frontend.CacheSyncBugsRequest{Projects: splitCSV(firstValue(values))})
			if err == nil {
				action = "Bug cache sync completed"
				summary = []string{fmt.Sprintf("Synced: %d", result.Synced)}
				if len(result.RebuiltProjects) > 0 {
					summary = append(summary,
						"Rebuilt: "+strings.Join(result.RebuiltProjects, ", "))
				}
			}
		case cacheActionExcuses:
			var result *frontend.CacheSyncExcusesResponse
			result, err = session.Frontend.Cache().SyncExcuses(ctx, frontend.CacheSyncExcusesRequest{Trackers: splitCSV(firstValue(values))})
			if err == nil {
				action = "Excuses cache sync completed"
				summary = []string{"Status: " + result.Status}
			}
		case cacheActionReleases:
			var result *frontend.CacheSyncReleasesResponse
			result, err = session.Frontend.Cache().SyncReleases(ctx)
			if err == nil {
				action = "Release cache sync completed"
				summary = make([]string, 0, 4+len(result.Warnings))
				summary = append(summary,
					"Status: "+result.Status,
					fmt.Sprintf("Discovered: %d", result.Discovered),
					fmt.Sprintf("Synced: %d", result.Synced),
					fmt.Sprintf("Skipped: %d", result.Skipped),
				)
				summary = append(summary, result.Warnings...)
			}
		case cacheActionReviews:
			var result *frontend.CacheSyncReviewsResponse
			result, err = session.Frontend.Cache().SyncReviews(ctx, frontend.CacheSyncReviewsRequest{
				Projects: splitCSV(firstValue(values)),
				Since:    strings.TrimSpace(valueAt(values, 1)),
			})
			if err == nil {
				action = "Review cache sync completed"
				summary = make([]string, 0, 3+len(result.Warnings))
				summary = append(summary,
					fmt.Sprintf("Projects: %d", result.ProjectsSynced),
					fmt.Sprintf("Summaries: %d", result.SummariesSynced),
					fmt.Sprintf("Details: %d", result.DetailsSynced),
				)
				summary = append(summary, result.Warnings...)
			}
		case cacheActionSRU:
			var result *dto.SRUSnapshot
			result, err = session.Frontend.Cache().SyncSRU(ctx)
			if err == nil {
				action = "SRU cache sync completed"
				summary = []string{fmt.Sprintf("Targets: %d", len(result.Rows))}
			}
		}
		return cacheMutationFinishedMsg{action: action, summary: summary, err: err}
	})
}

func clearCacheCmd(session *runtimeadapter.Session, target cacheActionTarget, values []string) tea.Cmd {
	return guardSessionAction(session, frontend.ActionCacheClear, func() tea.Msg {
		req := frontend.CacheClearRequest{Type: cacheActionTypeName(target)}
		switch target {
		case cacheActionGit, cacheActionBugs, cacheActionReviews:
			req.Projects = splitCSV(firstValue(values))
		case cacheActionExcuses:
			req.Trackers = splitCSV(firstValue(values))
		}
		err := session.Frontend.Cache().Clear(context.Background(), req)
		action := fmt.Sprintf("Cleared %s cache", cacheActionDisplayName(target))
		return cacheMutationFinishedMsg{action: action, summary: []string{"Status: ok"}, err: err}
	})
}

func triggerBuildCmd(session *runtimeadapter.Session, req frontend.BuildTriggerRequest) tea.Cmd {
	return guardSessionAction(session, frontend.ActionBuildTrigger, func() tea.Msg {
		result, err := session.Frontend.Builds().Trigger(context.Background(), req)
		if err != nil {
			return buildTriggeredMsg{err: err}
		}
		return buildTriggeredMsg{job: result.Job}
	})
}

func retryBuildCmd(session *runtimeadapter.Session, build dto.Build) tea.Cmd {
	return guardSessionAction(session, frontend.ActionBuildRetry, func() tea.Msg {
		err := session.Frontend.Builds().Retry(context.Background(), build.SelfLink, build.ArtifactType.String())
		return buildRetryMsg{err: err}
	})
}

func cancelBuildCmd(session *runtimeadapter.Session, build dto.Build) tea.Cmd {
	return guardSessionAction(session, frontend.ActionBuildCancel, func() tea.Msg {
		err := session.Frontend.Builds().Cancel(context.Background(), build.SelfLink, build.ArtifactType.String())
		return buildCancelMsg{err: err}
	})
}

func cleanupBuildCmd(session *runtimeadapter.Session, prefix, owner string, dryRun bool) tea.Cmd {
	actionID := frontend.ActionBuildCleanupDryRun
	if !dryRun {
		actionID = frontend.ActionBuildCleanupApply
	}
	return guardSessionAction(session, actionID, func() tea.Msg {
		result, err := session.Frontend.Builds().Cleanup(context.Background(), frontend.BuildCleanupRequest{
			Prefix: prefix,
			Owner:  owner,
			DryRun: dryRun,
		})
		if err != nil {
			return buildCleanupMsg{err: err}
		}
		return buildCleanupMsg{
			deletedRecipes:  result.DeletedRecipes,
			deletedBranches: result.DeletedBranches,
			dryRun:          dryRun,
		}
	})
}

func cancelOperationCmd(session *runtimeadapter.Session, id string) tea.Cmd {
	return guardSessionAction(session, frontend.ActionOperationCancel, func() tea.Msg {
		job, err := session.Frontend.Operations().Cancel(context.Background(), id)
		return operationCancelledMsg{job: job, err: err}
	})
}

func upgradeSessionCmd(session *runtimeadapter.Session) tea.Cmd {
	return guardSessionAction(session, frontend.ActionServerSwitchTarget, func() tea.Msg {
		return upgradedMsg{err: session.UpgradeToPersistent(context.Background())}
	})
}

func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		return browserOpenedMsg{err: openBrowser(url)}
	}
}

func tickDashboardCmd() tea.Cmd {
	return tea.Tick(15*time.Second, func(t time.Time) tea.Msg { return tickDashboardMsg(t) })
}

func tickOperationsCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickOperationsMsg(t) })
}

func tickBuildsCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg { return tickBuildsMsg(t) })
}

func tickLogsCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickLogsMsg(t) })
}

func hasNonTerminalBuilds(rows []dto.Build) bool {
	for _, b := range rows {
		if !b.State.IsTerminal() {
			return true
		}
	}
	return false
}

func clearToastLater() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearToastMsg{} })
}

func newBuildFilterForm(builds buildsModel) formModalModel {
	return newFormModal("Build Filters", []fieldDef{
		{placeholder: "project", value: builds.filters.project, resetValue: builds.defaults.project},
		{placeholder: "state", value: builds.filters.state, resetValue: builds.defaults.state},
		{placeholder: "active", value: fmt.Sprintf("%t", builds.filters.active), resetValue: fmt.Sprintf("%t", builds.defaults.active), kind: fieldKindEnum, suggestions: []string{"false", "true"}},
		{placeholder: "source", value: builds.filters.source, resetValue: builds.defaults.source, kind: fieldKindEnum, suggestions: []string{"remote", "local"}},
	})
}

func newReleaseFilterForm(session *runtimeadapter.Session, releases releasesModel) formModalModel {
	suggestions := releaseFilterSuggestions(session, releases)
	return newFormModal("Release Filters", []fieldDef{
		{placeholder: "project", value: releases.filters.project, resetValue: releases.defaults.project, suggestions: suggestions.projects},
		{placeholder: "artifact type", value: releases.filters.artifactType, resetValue: releases.defaults.artifactType, suggestions: suggestions.artifactTypes, kind: fieldKindEnum},
		{placeholder: "risk", value: releases.filters.risk, resetValue: releases.defaults.risk, suggestions: suggestions.risks, kind: fieldKindEnum},
		{placeholder: "track", value: releases.filters.track, resetValue: releases.defaults.track, suggestions: suggestions.tracks},
		{placeholder: "branch", value: releases.filters.branch, resetValue: releases.defaults.branch, suggestions: suggestions.branches},
		{placeholder: "target profile", value: releases.filters.targetProfile, resetValue: releases.defaults.targetProfile, suggestions: suggestions.targetProfiles, kind: fieldKindEnum},
		{placeholder: "all targets", value: fmt.Sprintf("%t", releases.filters.allTargets), resetValue: fmt.Sprintf("%t", releases.defaults.allTargets), suggestions: []string{"false", "true"}, kind: fieldKindEnum},
	})
}

func newBuildTriggerForm(session *runtimeadapter.Session) formModalModel {
	project := ""
	if cfg := session.Config.LocalConfig(); cfg != nil && len(cfg.Projects) > 0 {
		project = cfg.Projects[0].Name
	}
	return newFormModal("Trigger Build", []fieldDef{
		{placeholder: "project", value: project, resetValue: project},
		{placeholder: "artifacts (comma separated)", value: "", resetValue: ""},
		{placeholder: "source", value: "remote", resetValue: "remote", suggestions: []string{"remote", "local"}, kind: fieldKindEnum},
		{placeholder: "local path", value: ".", resetValue: "."},
	})
}

func newBuildCleanupForm(session *runtimeadapter.Session) formModalModel {
	return newFormModal("Cleanup Builds", []fieldDef{
		{placeholder: "prefix", value: "tmp-build", resetValue: "tmp-build"},
		{placeholder: "owner", value: "", resetValue: ""},
		{placeholder: "dry run", value: "true", resetValue: "true", suggestions: []string{"true", "false"}, kind: fieldKindEnum},
	})
}

func newProjectSyncForm(session *runtimeadapter.Session) formModalModel {
	return newFormModal("Project Sync", []fieldDef{
		{placeholder: "projects", value: "", resetValue: "", suggestions: projectSuggestions(session), kind: fieldKindMultiSelect},
		{placeholder: "dry run", value: "true", resetValue: "true", suggestions: []string{"true", "false"}, kind: fieldKindEnum},
	})
}

func newBugSyncForm(session *runtimeadapter.Session) formModalModel {
	return newFormModal("Bug Sync", []fieldDef{
		{placeholder: "projects", value: "", resetValue: "", suggestions: projectSuggestions(session), kind: fieldKindMultiSelect},
		{placeholder: "bug IDs", value: "", resetValue: ""},
		{placeholder: "since", value: "", resetValue: ""},
		{placeholder: "dry run", value: "true", resetValue: "true", suggestions: []string{"true", "false"}, kind: fieldKindEnum},
	})
}

func newTeamSyncForm(session *runtimeadapter.Session) formModalModel {
	return newFormModal("Team Sync", []fieldDef{
		{placeholder: "projects", value: "", resetValue: "", suggestions: projectSuggestions(session), kind: fieldKindMultiSelect},
		{placeholder: "dry run", value: "true", resetValue: "true", suggestions: []string{"true", "false"}, kind: fieldKindEnum},
	})
}

func newCacheSyncForm(session *runtimeadapter.Session, target cacheActionTarget) formModalModel {
	switch target {
	case cacheActionGit:
		return newFormModal("Sync Git Cache", []fieldDef{
			{placeholder: "projects", value: "", resetValue: "", suggestions: projectSuggestions(session), kind: fieldKindMultiSelect},
		})
	case cacheActionPackages:
		return newFormModal("Sync Package Index Cache", []fieldDef{
			{placeholder: "distros", value: "", resetValue: "", suggestions: distroSuggestions(session), kind: fieldKindMultiSelect},
			{placeholder: "releases", value: "", resetValue: "", suggestions: releaseSuggestions(session), kind: fieldKindMultiSelect},
			{placeholder: "backports", value: "", resetValue: "", suggestions: backportSuggestions(session), kind: fieldKindMultiSelect},
		})
	case cacheActionUpstream:
		return newFormModal("Sync Upstream Cache", nil)
	case cacheActionBugs:
		return newFormModal("Sync Bug Cache", []fieldDef{
			{placeholder: "projects", value: "", resetValue: "", suggestions: projectSuggestions(session), kind: fieldKindMultiSelect},
		})
	case cacheActionExcuses:
		return newFormModal("Sync Excuses Cache", []fieldDef{
			{placeholder: "trackers", value: "", resetValue: "", suggestions: excusesTrackerSuggestions(session), kind: fieldKindMultiSelect},
		})
	case cacheActionReleases:
		return newFormModal("Sync Release Cache", nil)
	case cacheActionReviews:
		return newFormModal("Sync Review Cache", []fieldDef{
			{placeholder: "projects", value: "", resetValue: "", suggestions: projectSuggestions(session), kind: fieldKindMultiSelect},
			{placeholder: "since", value: "", resetValue: ""},
		})
	case cacheActionSRU:
		return newFormModal("Sync SRU Cache", nil)
	default:
		return newFormModal("Sync Cache", nil)
	}
}

func newCacheClearForm(session *runtimeadapter.Session, target cacheActionTarget) formModalModel {
	switch target {
	case cacheActionGit:
		return newFormModal("Clear Git Cache", []fieldDef{
			{placeholder: "projects", value: "", resetValue: "", suggestions: projectSuggestions(session), kind: fieldKindMultiSelect},
		})
	case cacheActionPackages:
		return newFormModal("Clear Package Index Cache", nil)
	case cacheActionUpstream:
		return newFormModal("Clear Upstream Cache", nil)
	case cacheActionBugs:
		return newFormModal("Clear Bug Cache", []fieldDef{
			{placeholder: "projects", value: "", resetValue: "", suggestions: projectSuggestions(session), kind: fieldKindMultiSelect},
		})
	case cacheActionExcuses:
		return newFormModal("Clear Excuses Cache", []fieldDef{
			{placeholder: "trackers", value: "", resetValue: "", suggestions: excusesTrackerSuggestions(session), kind: fieldKindMultiSelect},
		})
	case cacheActionReleases:
		return newFormModal("Clear Release Cache", nil)
	case cacheActionReviews:
		return newFormModal("Clear Review Cache", []fieldDef{
			{placeholder: "projects", value: "", resetValue: "", suggestions: projectSuggestions(session), kind: fieldKindMultiSelect},
		})
	case cacheActionSRU:
		return newFormModal("Clear SRU Cache", nil)
	default:
		return newFormModal("Clear Cache", nil)
	}
}

func releaseFilterSuggestions(session *runtimeadapter.Session, releases releasesModel) releaseFilterOptions {
	projects := []string{}
	targetProfiles := []string{}
	if session != nil && session.Config != nil {
		if cfg := session.Config.LocalConfig(); cfg != nil {
			projects = make([]string, 0, len(cfg.Projects))
			for _, project := range cfg.Projects {
				projects = append(projects, project.Name)
			}
			targetProfiles = make([]string, 0, len(cfg.Releases.TargetProfiles))
			for name := range cfg.Releases.TargetProfiles {
				targetProfiles = append(targetProfiles, name)
			}
		}
	}
	artifactTypes := make([]string, 0, 3+len(releases.rows)+1)
	artifactTypes = append(artifactTypes,
		dto.ArtifactRock.String(),
		dto.ArtifactCharm.String(),
		dto.ArtifactSnap.String(),
	)
	risks := make([]string, 0, len(dto.KnownReleaseRisks()))
	for _, risk := range dto.KnownReleaseRisks() {
		risks = append(risks, string(risk))
	}
	tracks := make([]string, 0, len(releases.rows))
	branches := make([]string, 0, len(releases.rows))
	for _, row := range releases.rows {
		projects = append(projects, row.Project)
		artifactTypes = append(artifactTypes, row.ArtifactType.String())
		risks = append(risks, string(row.Risk))
		tracks = append(tracks, row.Track)
		branches = append(branches, row.Branch)
	}
	projects = append(projects, releases.filters.project)
	artifactTypes = append(artifactTypes, releases.filters.artifactType)
	risks = append(risks, releases.filters.risk)
	tracks = append(tracks, releases.filters.track)
	branches = append(branches, releases.filters.branch)
	targetProfiles = append(targetProfiles, releases.filters.targetProfile)
	return releaseFilterOptions{
		projects:       uniqueSortedStrings(projects...),
		artifactTypes:  uniqueSortedStrings(artifactTypes...),
		risks:          orderedReleaseRiskSuggestions(risks...),
		tracks:         uniqueSortedStrings(tracks...),
		branches:       uniqueSortedStrings(branches...),
		targetProfiles: uniqueSortedStrings(targetProfiles...),
	}
}

type fieldDef struct {
	placeholder string
	value       string
	resetValue  string
	suggestions []string
	kind        fieldKind
}

type fieldKind int

const (
	fieldKindText fieldKind = iota
	fieldKindEnum
	fieldKindMultiSelect
)

func newFormModal(title string, fields []fieldDef) formModalModel {
	models := make([]textinput.Model, 0, len(fields))
	kinds := make([]fieldKind, 0, len(fields))
	options := make([][]string, 0, len(fields))
	optionIndices := make([]int, 0, len(fields))
	resetValues := make([]string, 0, len(fields))
	selected := make([][]string, 0, len(fields))
	selectedSets := make([]map[string]bool, 0, len(fields))
	for i, field := range fields {
		input := textinput.New()
		input.Placeholder = field.placeholder
		input.SetValue(field.value)
		kind := field.kind
		var enumOptions []string
		if kind == fieldKindEnum || kind == fieldKindMultiSelect {
			enumOptions = uniqueSortedStrings(field.suggestions...)
			if kind == fieldKindEnum && field.value != "" && !containsString(enumOptions, field.value) {
				enumOptions = append([]string{field.value}, enumOptions...)
			}
			if len(enumOptions) == 0 {
				kind = fieldKindText
			}
		}
		selectedValues := splitCSV(field.value)
		if kind == fieldKindMultiSelect {
			for _, value := range selectedValues {
				if value != "" && !containsString(enumOptions, value) {
					enumOptions = append(enumOptions, value)
				}
			}
			enumOptions = uniqueSortedStrings(enumOptions...)
			selectedSet := stringSet(selectedValues)
			selectedValues = orderedSelectionFromSet(enumOptions, selectedSet)
			input.ShowSuggestions = false
			input.SetValue(multiSelectDisplay(selectedValues))
			kinds = append(kinds, fieldKindMultiSelect)
			options = append(options, enumOptions)
			selected = append(selected, append([]string(nil), selectedValues...))
			selectedSets = append(selectedSets, selectedSet)
			optionIndices = append(optionIndices, formOptionIndexForMulti(enumOptions, selectedValues))
		} else if kind == fieldKindEnum {
			input.ShowSuggestions = false
			kinds = append(kinds, fieldKindEnum)
			options = append(options, enumOptions)
			selected = append(selected, nil)
			selectedSets = append(selectedSets, nil)
			optionIndices = append(optionIndices, formOptionIndex(enumOptions, field.value))
		} else if len(field.suggestions) > 0 {
			input.ShowSuggestions = true
			input.SetSuggestions(field.suggestions)
			kinds = append(kinds, fieldKindText)
			options = append(options, append([]string(nil), field.suggestions...))
			selected = append(selected, nil)
			selectedSets = append(selectedSets, nil)
			optionIndices = append(optionIndices, -1)
		} else {
			kinds = append(kinds, fieldKindText)
			options = append(options, nil)
			selected = append(selected, nil)
			selectedSets = append(selectedSets, nil)
			optionIndices = append(optionIndices, -1)
		}
		if i == 0 {
			input.Focus()
		}
		models = append(models, input)
		resetValues = append(resetValues, field.resetValue)
	}
	return formModalModel{
		title:         title,
		fields:        models,
		kinds:         kinds,
		options:       options,
		optionIndices: optionIndices,
		resetValues:   resetValues,
		selected:      selected,
		selectedSets:  selectedSets,
		visualAnchor:  -1,
	}
}

func updateFormModal(msg tea.KeyMsg, modal *formModalModel, onSubmit func([]string) tea.Cmd, onCancel func()) tea.Cmd {
	switch msg.String() {
	case "g":
		if modal.activeFieldKind() == fieldKindMultiSelect {
			if modal.pendingG {
				modal.pendingG = false
				modal.moveActiveMultiCursorTo(0)
				return nil
			}
			modal.pendingG = true
			return nil
		}
	default:
		modal.pendingG = false
	}
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		modal.clearVisualMode()
		onCancel()
		return nil
	case "tab":
		if modal.activeFieldKind() == fieldKindText && acceptFormSuggestion(modal) {
			return nil
		}
		modal.clearVisualMode()
		modal.moveActiveField(1)
		return nil
	case "enter":
		return onSubmit(modal.values())
	case "ctrl+r":
		modal.reset()
		return nil
	case "shift+tab":
		modal.clearVisualMode()
		modal.moveActiveField(-1)
		return nil
	case " ":
		if modal.activeFieldKind() == fieldKindEnum {
			modal.applyActiveEnumSelection()
			return nil
		}
		if modal.activeFieldKind() == fieldKindMultiSelect {
			modal.toggleActiveMultiSelection()
			return nil
		}
	case "v":
		if modal.activeFieldKind() == fieldKindMultiSelect {
			modal.toggleVisualMode()
			return nil
		}
	case "G":
		if modal.activeFieldKind() == fieldKindMultiSelect {
			modal.moveActiveMultiCursorTo(len(modal.options[modal.active]) - 1)
			return nil
		}
	case "up", "k":
		if modal.activeFieldKind() == fieldKindEnum {
			modal.stepActiveEnum(-1)
			return nil
		}
		if modal.activeFieldKind() == fieldKindMultiSelect {
			modal.stepActiveMulti(-1)
			return nil
		}
	case "down", "j":
		if modal.activeFieldKind() == fieldKindEnum {
			modal.stepActiveEnum(1)
			return nil
		}
		if modal.activeFieldKind() == fieldKindMultiSelect {
			modal.stepActiveMulti(1)
			return nil
		}
	case "backspace", "delete":
		if modal.activeFieldKind() == fieldKindEnum {
			modal.fields[modal.active].SetValue("")
			return nil
		}
		if modal.activeFieldKind() == fieldKindMultiSelect {
			modal.clearActiveMultiSelection()
			return nil
		}
	}
	if modal.activeFieldKind() == fieldKindEnum || modal.activeFieldKind() == fieldKindMultiSelect {
		return nil
	}
	var cmd tea.Cmd
	modal.fields[modal.active], cmd = modal.fields[modal.active].Update(msg)
	return cmd
}

func (m *formModalModel) values() []string {
	values := make([]string, 0, len(m.fields))
	for i, field := range m.fields {
		if i < len(m.kinds) && m.kinds[i] == fieldKindMultiSelect {
			values = append(values, strings.Join(m.selected[i], ", "))
			continue
		}
		values = append(values, field.Value())
	}
	return values
}

func (m *formModalModel) activeFieldKind() fieldKind {
	if m == nil || m.active < 0 || m.active >= len(m.kinds) {
		return fieldKindText
	}
	return m.kinds[m.active]
}

func (m *formModalModel) moveActiveField(delta int) {
	if m == nil || len(m.fields) == 0 || delta == 0 {
		return
	}
	next := clampIndex(m.active+delta, len(m.fields))
	if next == m.active {
		return
	}
	m.fields[m.active].Blur()
	m.active = next
	m.fields[m.active].Focus()
	m.scroll = maxInt(0, m.fieldScrollAnchor(m.active))
	m.clearVisualMode()
}

func (m *formModalModel) fieldScrollAnchor(idx int) int {
	if idx <= 0 {
		return 0
	}
	anchor := 0
	for i := 0; i < idx; i++ {
		anchor += 2
	}
	return maxInt(0, anchor-2)
}

func (m *formModalModel) stepActiveEnum(delta int) {
	if m == nil || m.activeFieldKind() != fieldKindEnum || delta == 0 {
		return
	}
	opts := m.options[m.active]
	if len(opts) == 0 {
		return
	}
	idx := m.optionIndices[m.active]
	if idx < 0 || idx >= len(opts) {
		idx = formOptionIndex(opts, m.fields[m.active].Value())
	}
	idx += delta
	if idx < 0 {
		idx = len(opts) - 1
	}
	if idx >= len(opts) {
		idx = 0
	}
	m.optionIndices[m.active] = idx
	m.fields[m.active].SetValue(opts[idx])
}

func (m *formModalModel) applyActiveEnumSelection() {
	if m == nil || m.activeFieldKind() != fieldKindEnum {
		return
	}
	opts := m.options[m.active]
	idx := m.optionIndices[m.active]
	if idx >= 0 && idx < len(opts) {
		m.fields[m.active].SetValue(opts[idx])
	}
}

func (m *formModalModel) reset() {
	if m == nil {
		return
	}
	for i := range m.fields {
		if i < len(m.resetValues) {
			m.fields[i].SetValue(m.resetValues[i])
		} else {
			m.fields[i].SetValue("")
		}
		if i < len(m.options) && m.kinds[i] == fieldKindEnum {
			m.optionIndices[i] = formOptionIndex(m.options[i], m.fields[i].Value())
		}
		if i < len(m.options) && m.kinds[i] == fieldKindMultiSelect {
			m.selectedSets[i] = stringSet(splitCSV(m.resetValues[i]))
			m.selected[i] = orderedSelectionFromSet(m.options[i], m.selectedSets[i])
			m.fields[i].SetValue(multiSelectDisplay(m.selected[i]))
			m.optionIndices[i] = formOptionIndexForMulti(m.options[i], m.selected[i])
		}
	}
	m.errorMsg = ""
	m.scroll = maxInt(0, m.fieldScrollAnchor(m.active))
	m.clearVisualMode()
}

func (m *formModalModel) stepActiveMulti(delta int) {
	if m == nil || m.activeFieldKind() != fieldKindMultiSelect || delta == 0 {
		return
	}
	opts := m.options[m.active]
	if len(opts) == 0 {
		return
	}
	idx := m.optionIndices[m.active]
	if idx < 0 || idx >= len(opts) {
		idx = 0
	}
	idx += delta
	if idx < 0 {
		idx = len(opts) - 1
	}
	if idx >= len(opts) {
		idx = 0
	}
	m.optionIndices[m.active] = idx
	m.applyVisualSelection()
}

func (m *formModalModel) moveActiveMultiCursorTo(idx int) {
	if m == nil || m.activeFieldKind() != fieldKindMultiSelect {
		return
	}
	opts := m.options[m.active]
	if len(opts) == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(opts) {
		idx = len(opts) - 1
	}
	m.optionIndices[m.active] = idx
	m.applyVisualSelection()
}

func (m *formModalModel) toggleActiveMultiSelection() {
	if m == nil || m.activeFieldKind() != fieldKindMultiSelect {
		return
	}
	idx := m.optionIndices[m.active]
	opts := m.options[m.active]
	if idx < 0 || idx >= len(opts) {
		return
	}
	m.clearVisualMode()
	value := opts[idx]
	if m.selectedSets[m.active] == nil {
		m.selectedSets[m.active] = make(map[string]bool)
	}
	if m.selectedSets[m.active][value] {
		delete(m.selectedSets[m.active], value)
	} else {
		m.selectedSets[m.active][value] = true
	}
	m.selected[m.active] = orderedSelectionFromSet(opts, m.selectedSets[m.active])
	m.fields[m.active].SetValue(multiSelectDisplay(m.selected[m.active]))
}

func (m *formModalModel) clearActiveMultiSelection() {
	if m == nil || m.activeFieldKind() != fieldKindMultiSelect {
		return
	}
	m.selected[m.active] = nil
	m.selectedSets[m.active] = nil
	m.fields[m.active].SetValue(multiSelectDisplay(nil))
	m.clearVisualMode()
}

func (m *formModalModel) toggleVisualMode() {
	if m == nil || m.activeFieldKind() != fieldKindMultiSelect {
		return
	}
	if m.visualMode {
		m.clearVisualMode()
		return
	}
	m.visualMode = true
	m.visualAnchor = m.optionIndices[m.active]
	if m.visualAnchor < 0 {
		m.visualAnchor = 0
	}
}

func (m *formModalModel) clearVisualMode() {
	if m == nil {
		return
	}
	m.visualMode = false
	m.visualAnchor = -1
}

func (m *formModalModel) applyVisualSelection() {
	if m == nil || m.activeFieldKind() != fieldKindMultiSelect || !m.visualMode {
		return
	}
	opts := m.options[m.active]
	if len(opts) == 0 {
		return
	}
	start := m.visualAnchor
	end := m.optionIndices[m.active]
	if start < 0 || start >= len(opts) {
		return
	}
	if end < 0 || end >= len(opts) {
		return
	}
	if start > end {
		start, end = end, start
	}
	for _, option := range opts[start : end+1] {
		if m.selectedSets[m.active] == nil {
			m.selectedSets[m.active] = make(map[string]bool)
		}
		m.selectedSets[m.active][option] = true
	}
	m.selected[m.active] = orderedSelectionFromSet(opts, m.selectedSets[m.active])
	m.fields[m.active].SetValue(multiSelectDisplay(m.selected[m.active]))
}

func formOptionIndex(options []string, value string) int {
	for i, option := range options {
		if option == value {
			return i
		}
	}
	if len(options) == 0 {
		return -1
	}
	return 0
}

func formOptionIndexForMulti(options []string, values []string) int {
	for _, value := range values {
		if idx := formOptionIndex(options, value); idx >= 0 {
			return idx
		}
	}
	if len(options) == 0 {
		return -1
	}
	return 0
}

func acceptFormSuggestion(modal *formModalModel) bool {
	if modal.active < 0 || modal.active >= len(modal.fields) {
		return false
	}
	field := &modal.fields[modal.active]
	suggestion := strings.TrimSpace(field.CurrentSuggestion())
	if suggestion == "" || suggestion == field.Value() {
		return false
	}
	field.SetValue(suggestion)
	field.CursorEnd()
	return true
}

func renderFormModal(t theme, modal formModalModel, totalWidth, totalHeight int) string {
	modalWidth := minInt(maxInt(60, totalWidth-8), 88)
	if totalWidth <= 64 {
		modalWidth = maxInt(28, totalWidth-4)
	}
	modalHeight := minInt(maxInt(36, totalHeight-4), 48)
	innerWidth := innerPanelWidth(t.panel, modalWidth)
	fieldLines := make([]string, 0, len(modal.fields)*4)
	for i, field := range modal.fields {
		fieldLines = append(fieldLines, field.Placeholder)
		style := t.input
		if i == modal.active {
			style = t.inputFocused
		}
		fieldForRender := field
		contentWidth := innerWidth - style.GetHorizontalFrameSize()
		fieldForRender.Width = maxInt(8, contentWidth-lipgloss.Width(fieldForRender.Prompt))
		if modal.kinds[i] == fieldKindMultiSelect {
			fieldForRender.SetValue(multiSelectDisplay(modal.selected[i]))
		}
		fieldLines = append(fieldLines, style.Width(contentWidth).Render(fieldForRender.View()))
		if modal.kinds[i] == fieldKindEnum && i == modal.active && len(modal.options[i]) > 0 {
			for j, option := range modal.options[i] {
				line := "  " + option
				if j == modal.optionIndices[i] {
					line = t.selectedRow.Render(fitLine(line, innerWidth))
				} else {
					line = t.subtle.Render(fitLine(line, innerWidth))
				}
				fieldLines = append(fieldLines, line)
			}
		}
		if modal.kinds[i] == fieldKindMultiSelect && i == modal.active && len(modal.options[i]) > 0 {
			selectedSet := modal.selectedSets[i]
			for j, option := range modal.options[i] {
				marker := "[ ]"
				if selectedSet[option] {
					marker = "[x]"
				}
				line := fmt.Sprintf("  %s %s", marker, option)
				line = fitLine(line, innerWidth)
				switch {
				case j == modal.optionIndices[i]:
					line = t.selectedRow.Render(line)
				case modal.visualMode && modal.visualAnchor >= 0 && withinVisualRange(modal.visualAnchor, modal.optionIndices[i], j):
					line = t.info.Render(line)
				case selectedSet[option]:
					line = t.success.Render(line)
				default:
					line = t.subtle.Render(line)
				}
				fieldLines = append(fieldLines, line)
			}
		}
	}
	errorLines := []string{""}
	if modal.errorMsg != "" {
		errorLines = wrapTextLines(modal.errorMsg, innerWidth)
	}
	help := "[Tab/Shift+Tab] move  [Enter] apply  [Ctrl+R] reset  [Esc] cancel"
	if modal.activeFieldKind() == fieldKindEnum {
		help = "[Up/Down] pick  [Space] select  [Tab/Shift+Tab] move  [Enter] apply  [Ctrl+R] reset  [Esc] cancel"
	} else if modal.activeFieldKind() == fieldKindMultiSelect {
		help = "[Up/Down] move  [Space] toggle  [v] range  [gg/G] jump-range  [Tab/Shift+Tab] move  [Enter] apply  [Ctrl+R] reset  [Esc] cancel"
	} else if len(modal.fields) > 0 && modal.fields[modal.active].ShowSuggestions {
		help = "[Up/Down] suggestions  [Tab] accept/move  [Enter] apply  [Ctrl+R] reset  [Esc] cancel"
	}
	helpLines := wrapTextLines(help, innerWidth)
	bodyHeight := maxInt(8, modalHeight-(5+len(errorLines)+len(helpLines)))
	fieldBlock := renderViewport(strings.Join(fieldLines, "\n"), bodyHeight, modal.scroll)
	lines := []string{t.panelTitle.Render(modal.title), "", fieldBlock, ""}
	for _, line := range errorLines {
		if line == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, t.errorText.Render(line))
	}
	lines = append(lines, "")
	for _, line := range helpLines {
		lines = append(lines, t.subtle.Render(line))
	}
	return t.panel.Width(innerWidth).MaxWidth(modalWidth).Height(modalHeight).Render(strings.Join(lines, "\n"))
}

func renderBuildRows(t theme, rows []dto.Build, selected int, width int, loaded bool) string {
	if len(rows) == 0 {
		if !loaded {
			return t.subtle.Render("Loading builds...")
		}
		return t.subtle.Render("No builds.")
	}
	lines := make([]string, 0, len(rows))
	for i, row := range rows {
		line := fitLine(fmt.Sprintf("%-10s  %-12s  %s", row.State.String(), row.Project, row.Title), width)
		if i == selected {
			line = t.selectedRow.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderBuildDetail(t theme, build *dto.Build, width int) string {
	if build == nil {
		return t.subtle.Render("No build selected.")
	}
	hints := []string{"[t] trigger"}
	if build.CanRetry {
		hints = append(hints, "[R] retry")
	}
	if build.CanCancel {
		hints = append(hints, "[X] cancel")
	}
	hints = append(hints, "[C] cleanup")
	return fitBlock(strings.Join([]string{
		"Project: " + build.Project,
		"Recipe: " + build.Recipe,
		"Title: " + build.Title,
		"State: " + build.State.String(),
		"Arch: " + build.Arch,
		"Created: " + build.CreatedAt.Format(time.RFC3339),
		"Started: " + emptyTime(build.StartedAt),
		"Built: " + emptyTime(build.BuiltAt),
		"Web: " + build.WebLink,
		"Log: " + emptyAsDash(build.BuildLogURL),
		fmt.Sprintf("Can retry: %t", build.CanRetry),
		fmt.Sprintf("Can cancel: %t", build.CanCancel),
		"",
		strings.Join(hints, "  "),
	}, "\n"), width)
}

func renderReleaseArtifacts(t theme, artifacts []releaseArtifactSummary, selected int, width int, loaded bool) string {
	if len(artifacts) == 0 {
		if !loaded {
			return t.subtle.Render("Loading releases...")
		}
		return t.subtle.Render("No artifacts.")
	}
	const (
		gapWidth       = 2
		typeColWidth   = 5
		targetColWidth = 28
		dateColWidth   = 16
		minNameWidth   = 10
		maxNameWidth   = 28
	)
	lines := make([]string, 0, len(artifacts))
	nameColWidth := width - typeColWidth - targetColWidth - dateColWidth - (gapWidth * 3)
	if nameColWidth < minNameWidth {
		nameColWidth = minNameWidth
	}
	if nameColWidth > maxNameWidth {
		nameColWidth = maxNameWidth
	}
	for i, artifact := range artifacts {
		released := formatListTime(artifact.ReleasedAt)
		latestTarget := emptyAsDash(artifact.LatestVisibleTarget)
		line := padRight(truncateToWidth(artifact.Name, nameColWidth), nameColWidth) +
			spacer(gapWidth) +
			padRight(artifact.ArtifactType.String(), typeColWidth) +
			spacer(gapWidth) +
			padRight(truncateToWidth(latestTarget, targetColWidth), targetColWidth) +
			spacer(gapWidth) +
			padRight(released, dateColWidth)
		if i == selected {
			line = t.selectedRow.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderReleaseDetail(t theme, detail *dto.ReleaseShowResult, selected *releaseArtifactSummary, width int) string {
	if detail == nil || selected == nil {
		return t.subtle.Render("Select an artifact to load its full release matrix.")
	}
	releasedAt := latestReleaseTime(detail)
	lines := []string{
		"Project: " + detail.Project,
		"Name: " + detail.Name,
		"Type: " + detail.ArtifactType.String(),
		"Released: " + emptyTime(releasedAt),
		"",
		"Tracks: " + emptyAsDash(strings.Join(detail.Tracks, ", ")),
		"",
		"Channels:",
	}
	for _, channel := range detail.Channels {
		channelReleased := latestChannelReleaseTime(channel)
		lines = append(lines,
			fmt.Sprintf("- %s", channel.Channel),
			fmt.Sprintf("  track=%s  risk=%s  branch=%s  released=%s", channel.Track, channel.Risk, emptyAsDash(channel.Branch), emptyTime(channelReleased)),
			fmt.Sprintf("  targets: %s", formatReleaseTargets(channel.Targets)),
			fmt.Sprintf("  resources: %s", formatReleaseResources(channel.Resources)),
			"",
		)
	}
	return fitBlock(strings.Join(lines, "\n"), width)
}

func renderOperationRows(t theme, rows []dto.OperationJob, selected int) string {
	if len(rows) == 0 {
		return t.subtle.Render("No operations.")
	}
	lines := make([]string, 0, len(rows))
	for i, row := range rows {
		line := fmt.Sprintf("%-12s  %-12s  %s", row.State, row.Kind, row.Summary)
		if i == selected {
			line = t.selectedRow.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderOperationEvents(t theme, events []dto.OperationEvent) string {
	if len(events) == 0 {
		return t.subtle.Render("No events.")
	}
	lines := make([]string, 0, minInt(8, len(events)))
	for _, event := range events[maxInt(0, len(events)-8):] {
		lines = append(lines, fmt.Sprintf("%s  %s", event.Time.Format("15:04:05"), defaultString(event.Message, event.Type)))
	}
	return strings.Join(lines, "\n")
}

func summarizeReleaseArtifacts(rows []dto.ReleaseListEntry) []releaseArtifactSummary {
	type key struct {
		project string
		name    string
		kind    dto.ArtifactType
	}
	grouped := make(map[key]releaseArtifactSummary, len(rows))
	latestTargetAt := make(map[key]time.Time, len(rows))
	for _, row := range rows {
		k := key{project: row.Project, name: row.Name, kind: row.ArtifactType}
		summary := grouped[k]
		summary.Project = row.Project
		summary.Name = row.Name
		summary.ArtifactType = row.ArtifactType
		summary.ChannelCount++
		summary.ResourceCount += len(row.Resources)
		if row.ReleasedAt.After(summary.ReleasedAt) {
			summary.ReleasedAt = row.ReleasedAt
		}
		summary.VisibleTargetCount += len(row.Targets)
		for _, target := range row.Targets {
			if target.ReleasedAt.After(latestTargetAt[k]) || summary.LatestVisibleTarget == "" {
				latestTargetAt[k] = target.ReleasedAt
				summary.LatestVisibleTarget = frontend.FormatReleaseTargetCompact(target)
			}
		}
		grouped[k] = summary
	}
	artifacts := make([]releaseArtifactSummary, 0, len(grouped))
	for _, summary := range grouped {
		artifacts = append(artifacts, summary)
	}
	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].ReleasedAt.Equal(artifacts[j].ReleasedAt) {
			if artifacts[i].Project == artifacts[j].Project {
				return artifacts[i].Name < artifacts[j].Name
			}
			return artifacts[i].Project < artifacts[j].Project
		}
		return artifacts[i].ReleasedAt.After(artifacts[j].ReleasedAt)
	})
	return artifacts
}

func latestReleaseTime(detail *dto.ReleaseShowResult) time.Time {
	if detail == nil {
		return time.Time{}
	}
	var latest time.Time
	for _, channel := range detail.Channels {
		if ts := latestChannelReleaseTime(channel); ts.After(latest) {
			latest = ts
		}
	}
	return latest
}

func latestChannelReleaseTime(channel dto.ReleaseChannelSnapshot) time.Time {
	var latest time.Time
	for _, target := range channel.Targets {
		if target.ReleasedAt.After(latest) {
			latest = target.ReleasedAt
		}
	}
	return latest
}

func formatReleaseTargets(targets []dto.ReleaseTargetSnapshot) string {
	return frontend.FormatReleaseTargets(targets)
}

func formatReleaseResources(resources []dto.ReleaseResourceSnapshot) string {
	if len(resources) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(resources))
	for _, resource := range resources {
		if resource.Revision > 0 {
			parts = append(parts, fmt.Sprintf("%s:r%d", resource.Name, resource.Revision))
		} else {
			parts = append(parts, resource.Name)
		}
	}
	return strings.Join(parts, ", ")
}

func dashboardSectionWidth(totalWidth int, twoColumns bool) int {
	if !twoColumns {
		return totalWidth
	}
	const gap = 1
	return maxInt(20, (totalWidth-gap)/2)
}

func splitColumns(totalWidth, gap int) (int, int) {
	left := maxInt(20, (totalWidth-gap)/2)
	right := maxInt(20, totalWidth-gap-left)
	return left, right
}

func renderPanel(style lipgloss.Style, totalWidth int, title, body string) string {
	if totalWidth < 8 {
		totalWidth = 8
	}
	innerWidth := innerPanelWidth(style, totalWidth)
	content := fitBlock(body, innerWidth)
	if title != "" {
		content = title + "\n" + content
	}
	return style.Width(innerWidth).MaxWidth(totalWidth).Render(content)
}

func innerPanelWidth(style lipgloss.Style, totalWidth int) int {
	innerWidth := totalWidth - style.GetHorizontalFrameSize()
	if innerWidth < 1 {
		return 1
	}
	return innerWidth
}

func fitBlock(text string, width int) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	fit := make([]string, 0, len(lines))
	for _, line := range lines {
		fit = append(fit, fitLine(line, width))
	}
	return strings.Join(fit, "\n")
}

func fitLine(text string, width int) string {
	if width <= 0 {
		return text
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(text)
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		candidate := string(append(out, r))
		if lipgloss.Width(candidate+"…") > width {
			break
		}
		out = append(out, r)
	}
	return string(out) + "…"
}

func wrapTextLines(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	paragraphs := strings.Split(text, "\n")
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if strings.TrimSpace(paragraph) == "" {
			lines = append(lines, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := words[0]
		if lipgloss.Width(current) > width {
			lines = append(lines, fitLine(current, width))
			current = ""
		}
		for _, word := range words[1:] {
			if current == "" {
				if lipgloss.Width(word) > width {
					lines = append(lines, fitLine(word, width))
					continue
				}
				current = word
				continue
			}
			candidate := current + " " + word
			if lipgloss.Width(candidate) <= width {
				current = candidate
				continue
			}
			lines = append(lines, current)
			if lipgloss.Width(word) > width {
				lines = append(lines, fitLine(word, width))
				current = ""
				continue
			}
			current = word
		}
		if current != "" {
			lines = append(lines, current)
		}
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func truncateToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(text)
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		candidate := string(append(out, r))
		if lipgloss.Width(candidate+"…") > width {
			break
		}
		out = append(out, r)
	}
	return string(out) + "…"
}

func padRight(text string, width int) string {
	padding := width - lipgloss.Width(text)
	if padding <= 0 {
		return text
	}
	return text + spacer(padding)
}

func renderViewport(content string, height, offset int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= height {
		return lipgloss.NewStyle().Height(height).Render(strings.Join(lines, "\n"))
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(lines)-height {
		offset = len(lines) - height
	}
	end := offset + height
	return lipgloss.NewStyle().Height(height).Render(strings.Join(lines[offset:end], "\n"))
}

func viewportEndOffset() int {
	return 1 << 30
}

func (m rootModel) contentScroll() int {
	return m.viewScrolls[m.activeView]
}

func (m *rootModel) setContentScroll(v int) {
	if v < 0 {
		v = 0
	}
	m.viewScrolls[m.activeView] = v
}

func (m rootModel) scrollStep() int {
	if m.height <= 0 {
		return 5
	}
	step := m.height / 4
	if step < 3 {
		return 3
	}
	return step
}

// activeIndex returns the selected row index for the current view.
func (m rootModel) activeIndex() int {
	switch m.activeView {
	case viewDashboard:
		return m.dashboard.section
	case viewBuilds:
		return m.builds.index
	case viewReleases:
		return m.releases.index
	case viewPackages:
		return m.packages.index
	case viewBugs:
		return m.bugs.index
	case viewSRU:
		return m.sru.index
	case viewReviews:
		return m.reviews.index
	case viewCommits:
		return m.commits.index
	case viewProjects:
		return m.projects.index
	default:
		return 0
	}
}

// ensureCursorVisible adjusts the content scroll so the selected list item
// stays within the visible viewport. Each list row is 1 line; overhead lines
// (panel borders, title, filter header) are estimated at ~6 lines.
func (m *rootModel) ensureCursorVisible() {
	bodyHeight := m.height - 4
	if bodyHeight < 10 {
		bodyHeight = 10
	}
	const headerLines = 6
	cursorLine := headerLines + m.activeIndex()
	scroll := m.contentScroll()
	if cursorLine >= scroll+bodyHeight {
		scroll = cursorLine - bodyHeight + 1
	}
	if cursorLine < scroll+headerLines {
		scroll = maxInt(0, cursorLine-headerLines)
	}
	m.setContentScroll(scroll)
}

func projectSuggestions(session *runtimeadapter.Session) []string {
	if session == nil || session.Config == nil {
		return nil
	}
	cfg := session.Config.LocalConfig()
	if cfg == nil {
		return nil
	}
	projects := make([]string, 0, len(cfg.Projects))
	for _, project := range cfg.Projects {
		projects = append(projects, project.Name)
	}
	return uniqueSortedStrings(projects...)
}

func distroSuggestions(session *runtimeadapter.Session) []string {
	if session == nil || session.Config == nil {
		return nil
	}
	cfg := session.Config.LocalConfig()
	if cfg == nil {
		return nil
	}
	distros := make([]string, 0, len(cfg.Packages.Distros))
	for name := range cfg.Packages.Distros {
		distros = append(distros, name)
	}
	return uniqueSortedStrings(distros...)
}

func releaseSuggestions(session *runtimeadapter.Session) []string {
	if session == nil || session.Config == nil {
		return nil
	}
	cfg := session.Config.LocalConfig()
	if cfg == nil {
		return nil
	}
	releases := make([]string, 0)
	for _, distro := range cfg.Packages.Distros {
		for release := range distro.Releases {
			releases = append(releases, release)
		}
	}
	return uniqueSortedStrings(releases...)
}

func backportSuggestions(session *runtimeadapter.Session) []string {
	if session == nil || session.Config == nil {
		return nil
	}
	cfg := session.Config.LocalConfig()
	if cfg == nil {
		return nil
	}
	backports := make([]string, 0)
	for _, distro := range cfg.Packages.Distros {
		for _, release := range distro.Releases {
			for backport := range release.Backports {
				backports = append(backports, backport)
			}
		}
	}
	return uniqueSortedStrings(backports...)
}

func excusesTrackerSuggestions(_ *runtimeadapter.Session) []string {
	return dto.KnownExcusesTrackers()
}

func (m rootModel) syncRows() []string {
	return []string{
		"Project Sync  Preview or apply Launchpad project metadata synchronization",
		"Bug Sync      Preview or apply bug state synchronization from cached commits",
		"Team Sync     Preview or apply team collaborator synchronization to store artifacts",
	}
}

func (m rootModel) cacheRows() []string {
	status := m.cache.status
	if status == nil {
		return []string{
			"git           repos=0",
			"packages      sources=0",
			"upstream      repos=0",
			"bugs          entries=0",
			"excuses       entries=0",
			"releases      entries=0",
			"reviews       entries=0",
			"sru           targets=0",
		}
	}
	refreshRequired := 0
	for _, entry := range status.Bugs.Entries {
		if entry.NeedsRefresh {
			refreshRequired++
		}
	}
	return []string{
		fmt.Sprintf("%-13s repos=%d", "git", len(status.Git.Repos)),
		fmt.Sprintf("%-13s sources=%d", "packages", len(status.Packages.Sources)),
		fmt.Sprintf("%-13s repos=%d", "upstream", len(status.Upstream.Repos)),
		fmt.Sprintf("%-13s entries=%d refresh-required=%d",
			"bugs", len(status.Bugs.Entries), refreshRequired),
		fmt.Sprintf("%-13s entries=%d", "excuses", len(status.Excuses.Entries)),
		fmt.Sprintf("%-13s entries=%d", "releases", len(status.Releases.Entries)),
		fmt.Sprintf("%-13s entries=%d", "reviews", len(status.Reviews.Entries)),
		fmt.Sprintf("%-13s targets=%d", "sru", status.SRU.Status.Targets),
	}
}

func firstValue(values []string) string {
	return valueAt(values, 0)
}

func valueAt(values []string, idx int) string {
	if idx < 0 || idx >= len(values) {
		return ""
	}
	return values[idx]
}

func cacheSyncActionID(target cacheActionTarget) frontend.ActionID {
	switch target {
	case cacheActionGit:
		return frontend.ActionCacheSyncGit
	case cacheActionPackages:
		return frontend.ActionCacheSyncPackages
	case cacheActionUpstream:
		return frontend.ActionCacheSyncUpstream
	case cacheActionBugs:
		return frontend.ActionCacheSyncBugs
	case cacheActionExcuses:
		return frontend.ActionCacheSyncExcuses
	case cacheActionReleases:
		return frontend.ActionCacheSyncReleases
	case cacheActionReviews:
		return frontend.ActionCacheSyncReviews
	case cacheActionSRU:
		return frontend.ActionCacheSyncSRU
	default:
		return frontend.ActionCacheSync
	}
}

func cacheActionDisplayName(target cacheActionTarget) string {
	switch target {
	case cacheActionGit:
		return "git"
	case cacheActionPackages:
		return "package index"
	case cacheActionUpstream:
		return "upstream"
	case cacheActionBugs:
		return "bug"
	case cacheActionExcuses:
		return "excuses"
	case cacheActionReleases:
		return "release"
	case cacheActionReviews:
		return "review"
	case cacheActionSRU:
		return "SRU"
	default:
		return "selected"
	}
}

func cacheActionTypeName(target cacheActionTarget) string {
	switch target {
	case cacheActionGit:
		return "git"
	case cacheActionPackages:
		return "packages-index"
	case cacheActionUpstream:
		return "upstream-repos"
	case cacheActionBugs:
		return "bugs"
	case cacheActionExcuses:
		return "excuses"
	case cacheActionReleases:
		return "releases"
	case cacheActionReviews:
		return "reviews"
	case cacheActionSRU:
		return "sru"
	default:
		return ""
	}
}

func summarizeProjectSyncResult(result *frontend.ProjectSyncResponse) []string {
	if result == nil {
		return nil
	}
	lines := []string{fmt.Sprintf("Actions: %d", len(result.Actions))}
	for _, action := range result.Actions[:minInt(3, len(result.Actions))] {
		lines = append(lines, fmt.Sprintf("%s  %s  %s", action.Project, action.ActionType, emptyAsDash(action.Series)))
	}
	for _, err := range result.Errors {
		lines = append(lines, "warning: "+err)
	}
	return lines
}

func summarizeBugSyncResult(result *frontend.BugSyncResponse) []string {
	if result == nil || result.Result == nil {
		return nil
	}
	lines := []string{
		fmt.Sprintf("Actions: %d", len(result.Result.Actions)),
		fmt.Sprintf("Skipped: %d", result.Result.Skipped),
	}
	for _, action := range result.Result.Actions[:minInt(3, len(result.Result.Actions))] {
		lines = append(lines, fmt.Sprintf("#%s  %s  %s", action.BugID, action.ActionType, emptyAsDash(action.Project)))
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "warning: "+warning)
	}
	return lines
}

func summarizeTeamSyncResult(result *frontend.TeamSyncResponse) []string {
	if result == nil {
		return nil
	}
	lines := []string{fmt.Sprintf("Artifacts: %d", len(result.Artifacts))}
	for _, artifact := range result.Artifacts[:minInt(3, len(result.Artifacts))] {
		if artifact.Unsupported {
			dest := artifact.UnsupportedURL
			if dest == "" {
				dest = "store web UI"
			}
			lines = append(lines, fmt.Sprintf("%s  %s  unsupported: manage at %s", artifact.Project, artifact.ArtifactType, dest))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  %s  invited=%d  extra=%d", artifact.Project, artifact.ArtifactType, len(artifact.Invited), len(artifact.Extra)))
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "warning: "+warning)
	}
	return lines
}

func dryRunLabel(dryRun bool) string {
	if dryRun {
		return "dry-run"
	}
	return "apply"
}

func orderedSelectionFromSet(options []string, selected map[string]bool) []string {
	if len(selected) == 0 {
		return nil
	}
	present := make(map[string]bool, len(selected))
	for value := range selected {
		if value != "" {
			present[value] = true
		}
	}
	out := make([]string, 0, len(selected))
	for _, option := range options {
		if present[option] {
			out = append(out, option)
			delete(present, option)
		}
	}
	extras := make([]string, 0, len(present))
	for value := range present {
		extras = append(extras, value)
	}
	sort.Strings(extras)
	out = append(out, extras...)
	return out
}

func withinVisualRange(anchor, cursor, idx int) bool {
	if anchor > cursor {
		anchor, cursor = cursor, anchor
	}
	return idx >= anchor && idx <= cursor
}

func stringSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func multiSelectDisplay(selected []string) string {
	switch len(selected) {
	case 0:
		return ""
	case 1:
		return selected[0]
	case 2:
		return selected[0] + ", " + selected[1]
	default:
		return fmt.Sprintf("%s, %s (+%d more)", selected[0], selected[1], len(selected)-2)
	}
}

func selectedBuild(rows []dto.Build, idx int) *dto.Build {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	row := rows[idx]
	return &row
}

func selectedOperation(rows []dto.OperationJob, idx int) *dto.OperationJob {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	row := rows[idx]
	return &row
}

func selectedOperationID(rows []dto.OperationJob, idx int) string {
	if job := selectedOperation(rows, idx); job != nil {
		return job.ID
	}
	return ""
}

func countRunningOperations(rows []dto.OperationJob) int {
	count := 0
	for _, row := range rows {
		if row.State == dto.OperationStateQueued || row.State == dto.OperationStateRunning {
			count++
		}
	}
	return count
}

func hasRunningOperation(rows []dto.OperationJob) bool {
	return countRunningOperations(rows) > 0
}

func displayLaunchpadName(status *dto.AuthStatus) string {
	return displayProviderName(launchpadStatusFromAuth(status))
}

func displayGitHubName(status *dto.AuthStatus) string {
	return displayProviderName(githubStatusFromAuth(status))
}

func renderAuthSummaryText(status *dto.AuthStatus) string {
	parts := make([]string, 0, 4)
	if launchpadStatusFromAuth(status).authenticated {
		parts = append(parts, "LP: "+displayLaunchpadName(status))
	} else {
		parts = append(parts, "LP: not authenticated")
	}
	if githubStatusFromAuth(status).authenticated {
		parts = append(parts, "GH: "+displayGitHubName(status))
	} else {
		parts = append(parts, "GH: not authenticated")
	}
	if snapStoreStatusFromAuth(status).authenticated {
		parts = append(parts, "Snap: ok")
	} else {
		parts = append(parts, "Snap: no")
	}
	if charmhubStatusFromAuth(status).authenticated {
		parts = append(parts, "Charm: ok")
	} else {
		parts = append(parts, "Charm: no")
	}
	return strings.Join(parts, "  ")
}

type providerStatusView struct {
	authenticated   bool
	displayName     string
	username        string
	source          string
	credentialsPath string
	err             string
}

func launchpadStatusFromAuth(status *dto.AuthStatus) providerStatusView {
	if status == nil {
		return providerStatusView{}
	}
	return providerStatusView{
		authenticated:   status.Launchpad.Authenticated,
		displayName:     status.Launchpad.DisplayName,
		username:        status.Launchpad.Username,
		source:          status.Launchpad.Source,
		credentialsPath: status.Launchpad.CredentialsPath,
		err:             status.Launchpad.Error,
	}
}

func githubStatusFromAuth(status *dto.AuthStatus) providerStatusView {
	if status == nil {
		return providerStatusView{}
	}
	return providerStatusView{
		authenticated:   status.GitHub.Authenticated,
		displayName:     status.GitHub.DisplayName,
		username:        status.GitHub.Username,
		source:          status.GitHub.Source,
		credentialsPath: status.GitHub.CredentialsPath,
		err:             status.GitHub.Error,
	}
}

func snapStoreStatusFromAuth(status *dto.AuthStatus) providerStatusView {
	if status == nil {
		return providerStatusView{}
	}
	return providerStatusView{
		authenticated:   status.SnapStore.Authenticated,
		source:          status.SnapStore.Source,
		credentialsPath: status.SnapStore.CredentialsPath,
	}
}

func charmhubStatusFromAuth(status *dto.AuthStatus) providerStatusView {
	if status == nil {
		return providerStatusView{}
	}
	return providerStatusView{
		authenticated:   status.Charmhub.Authenticated,
		source:          status.Charmhub.Source,
		credentialsPath: status.Charmhub.CredentialsPath,
	}
}

func displayProviderName(status providerStatusView) string {
	if status.displayName != "" {
		return status.displayName
	}
	if status.username != "" {
		return status.username
	}
	return "guest"
}

func renderProviderStatusLine(provider string, status providerStatusView) string {
	if status.authenticated {
		return fmt.Sprintf("%s: %s", provider, displayProviderName(status))
	}
	if status.err != "" {
		return fmt.Sprintf("%s: invalid credentials", provider)
	}
	return fmt.Sprintf("%s: not authenticated", provider)
}

func renderProviderStatusDetail(provider string, status providerStatusView) []string {
	if !status.authenticated && status.err == "" {
		return nil
	}
	lines := make([]string, 0, 3)
	if status.source != "" {
		lines = append(lines, provider+" source: "+status.source)
	}
	if status.credentialsPath != "" {
		lines = append(lines, provider+" path: "+status.credentialsPath)
	}
	if status.err != "" {
		lines = append(lines, provider+" error: "+status.err)
	}
	return lines
}

func renderToast(t theme, toast toastState) string {
	switch toast.level {
	case "success":
		return t.success.Render(toast.message)
	case "error":
		return t.errorText.Render(toast.message)
	default:
		return t.info.Render(toast.message)
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command("xdg-open", url)
		} else if _, err := exec.LookPath("wslview"); err == nil {
			cmd = exec.Command("wslview", url)
		} else {
			return fmt.Errorf("no browser opener available")
		}
	}
	return cmd.Start()
}

func emptyAsAny(value string) string {
	if strings.TrimSpace(value) == "" {
		return "any"
	}
	return value
}

func emptyAsDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func uniqueSortedStrings(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func orderedReleaseRiskSuggestions(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(dto.KnownReleaseRisks()))
	for _, risk := range dto.KnownReleaseRisks() {
		value := string(risk)
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range uniqueSortedStrings(values...) {
		if _, ok := seen[value]; ok {
			continue
		}
		out = append(out, value)
	}
	return out
}

func emptyTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}

func formatListTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

func firstNonEmptySlice(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []string{value}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func errorsJoin(errs ...error) error {
	var parts []string
	for _, err := range errs {
		if err != nil {
			parts = append(parts, err.Error())
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return errors.New(strings.Join(parts, "; "))
}

func spacer(width int) string {
	if width <= 0 {
		return ""
	}
	return strings.Repeat(" ", width)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
