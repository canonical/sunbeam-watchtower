// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	runtimeadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/runtime"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestViewRendersAcrossWidths(t *testing.T) {
	model := newRootModel(nil, true)
	model.dashboard.loaded = true
	model.dashboard.auth = &dto.AuthStatus{}
	model.dashboard.ops = []dto.OperationJob{{ID: "op-1", Kind: dto.OperationKindBuildTrigger, State: dto.OperationStateRunning}}
	model.dashboard.builds = []dto.Build{{Project: "demo", Title: "demo-build", State: dto.BuildBuilding}}
	model.builds.rows = []dto.Build{{Project: "demo", Title: "demo-build", State: dto.BuildBuilding}}
	model.releases.rows = []dto.ReleaseListEntry{{Project: "demo", Name: "demo-artifact", Channel: "latest/edge", ArtifactType: dto.ArtifactSnap, ReleasedAt: time.Now()}}
	model.releases.artifacts = summarizeReleaseArtifacts(model.releases.rows)
	model.releases.detail = &dto.ReleaseShowResult{
		Project:      "demo",
		Name:         "demo-artifact",
		ArtifactType: dto.ArtifactSnap,
		Channels:     []dto.ReleaseChannelSnapshot{{Channel: "latest/edge", Track: "latest", Risk: dto.ReleaseRiskEdge}},
		UpdatedAt:    time.Now(),
	}

	for _, width := range []int{80, 100, 140} {
		model.width = width
		model.height = 40
		view := model.View()
		for _, want := range []string{"watchtower-tui", "Dashboard", "Builds", "Releases", "Packages", "Bugs", "Reviews", "Commits", "Projects"} {
			if !strings.Contains(view, want) {
				t.Fatalf("width %d view missing %q", width, want)
			}
		}
	}
}

func TestRenderReviewRowsKeepsColumnsAlignedWithLongFields(t *testing.T) {
	tm := newTheme()
	rows := []forge.MergeRequest{
		{
			Repo:   "canonical/snap-openstack-super-long-repository-name",
			Forge:  forge.ForgeGitHub,
			State:  forge.MergeStateOpen,
			Author: "very-long-author-name",
			Title:  "First review title",
		},
		{
			Repo:   "short-repo",
			Forge:  forge.ForgeLaunchpad,
			State:  forge.MergeStateMerged,
			Author: "short",
			Title:  "Second review title",
		},
	}

	rendered := renderReviewRows(tm, rows, -1, 90, true)
	lines := strings.Split(rendered, "\n")
	if len(lines) != 2 {
		t.Fatalf("renderReviewRows() produced %d lines, want 2", len(lines))
	}

	firstForge := strings.Index(lines[0], "GitHub")
	secondForge := strings.Index(lines[1], "Launchpad")
	if firstForge < 0 || secondForge < 0 {
		t.Fatalf("missing forge column:\n%s\n%s", lines[0], lines[1])
	}
	if lipgloss.Width(lines[0][:firstForge]) != lipgloss.Width(lines[1][:secondForge]) {
		t.Fatalf("forge column misaligned:\n%s\n%s", lines[0], lines[1])
	}

	if lipgloss.Width(lines[0]) > 90 || lipgloss.Width(lines[1]) > 90 {
		t.Fatalf("renderReviewRows() overflowed width 90:\n%s\n%s", lines[0], lines[1])
	}
}

func TestRenderReviewRowsTruncatesToSingleLineInNarrowPane(t *testing.T) {
	tm := newTheme()
	rows := []forge.MergeRequest{{
		Repo:   "canonical/snap-openstack-super-long-repository-name",
		Forge:  forge.ForgeGitHub,
		State:  forge.MergeStateOpen,
		Author: "very-long-author-name",
		Title:  "This merge request title is also intentionally very long to prove it never wraps into a second line\nwith a second raw line",
	}}

	rendered := renderReviewRows(tm, rows, -1, 40, true)
	if strings.Contains(rendered, "\n") {
		t.Fatalf("renderReviewRows() wrapped to multiple lines:\n%s", rendered)
	}
	if lipgloss.Width(rendered) > 40 {
		t.Fatalf("renderReviewRows() overflowed width 40: %q", rendered)
	}
}

func TestRenderBugRowsTruncatesToSingleLineInNarrowPane(t *testing.T) {
	tm := newTheme()
	rows := []forge.BugTask{{
		Project:    "snap-openstack-super-long-project-name",
		BugID:      "123456789",
		Status:     "Fix Released",
		Importance: "Critical",
		Title:      "This bug title is intentionally very long to prove it stays on one line\nwith a second raw line",
	}}

	rendered := renderBugRows(tm, rows, -1, 40, true)
	if strings.Contains(rendered, "\n") {
		t.Fatalf("renderBugRows() wrapped to multiple lines:\n%s", rendered)
	}
	if lipgloss.Width(rendered) > 40 {
		t.Fatalf("renderBugRows() overflowed width 40: %q", rendered)
	}
}

func TestRenderBugRowsStripsRepeatedBugPrefix(t *testing.T) {
	tm := newTheme()
	rows := []forge.BugTask{{
		Project: "snap-openstack",
		BugID:   "2143746",
		Status:  "Fix Released",
		Title:   "Bug #2143746 in Openstack Snap: \"nova-compute fails to refresh placement inventory\"",
	}}

	rendered := renderBugRows(tm, rows, -1, 120, true)
	if strings.Contains(rendered, "Bug #2143746 in Openstack Snap:") {
		t.Fatalf("renderBugRows() kept repeated bug prefix:\n%s", rendered)
	}
	if strings.Contains(rendered, "\"nova-compute fails to refresh placement inventory\"") {
		t.Fatalf("renderBugRows() kept quoted bug title:\n%s", rendered)
	}
	if !strings.Contains(rendered, "nova-compute fails to refresh placement inventory") {
		t.Fatalf("renderBugRows() lost stripped bug title:\n%s", rendered)
	}
}

func TestRenderCommitRowsTruncatesToSingleLineInNarrowPane(t *testing.T) {
	tm := newTheme()
	rows := []forge.Commit{{
		Repo:    "sunbeam-charms-super-long-repository-name",
		SHA:     "0123456789abcdef",
		Date:    time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		Author:  "very-long-author-name",
		Message: "This commit message is intentionally very long to prove it stays on one line\nwith a second raw line",
	}}

	rendered := renderCommitRows(tm, rows, -1, 40, true)
	if strings.Contains(rendered, "\n") {
		t.Fatalf("renderCommitRows() wrapped to multiple lines:\n%s", rendered)
	}
	if lipgloss.Width(rendered) > 40 {
		t.Fatalf("renderCommitRows() overflowed width 40: %q", rendered)
	}
}

func TestPackageFilterFormIsModeSpecific(t *testing.T) {
	session := newEmbeddedTestSession(t)
	defer session.Close()

	inventory := newPackageFilterForm(session, packagesModel{filters: packagesFilters{mode: packageModeInventory}})
	diff := newPackageFilterForm(session, packagesModel{filters: packagesFilters{mode: packageModeDiff}})
	excuses := newPackageFilterForm(session, packagesModel{filters: packagesFilters{mode: packageModeExcuses}})

	if got := len(inventory.fields); got != 5 {
		t.Fatalf("inventory filter field count = %d, want 5", got)
	}
	if got := len(diff.fields); got != 10 {
		t.Fatalf("diff filter field count = %d, want 10", got)
	}
	if got := len(excuses.fields); got != 14 {
		t.Fatalf("excuses filter field count = %d, want 14", got)
	}
}

func TestBuildTriggerRequestFromValuesAlwaysAsync(t *testing.T) {
	req, err := buildTriggerRequestFromValues([]string{"demo", "rock,charm", "remote", ""})
	if err != nil {
		t.Fatalf("buildTriggerRequestFromValues() error = %v", err)
	}
	if !req.Async {
		t.Fatal("req.Async = false, want true")
	}
	if got := len(req.Artifacts); got != 2 {
		t.Fatalf("len(req.Artifacts) = %d, want 2", got)
	}
}

func TestNewRootModelDefaultsBugViewToMerge(t *testing.T) {
	model := newRootModel(nil, true)
	if !model.bugs.filters.merge {
		t.Fatal("model.bugs.filters.merge = false, want true")
	}
}

func TestSummarizeProjectsUsesLaunchpadDefaultsForSeriesAndFocus(t *testing.T) {
	rows := summarizeProjects(&dto.Config{
		Launchpad: dto.LaunchpadConfig{
			Series:           []string{"2024.1", "2025.1"},
			DevelopmentFocus: "2025.1",
		},
		Projects: []dto.ProjectConfig{{
			Name: "sunbeam-charms",
			Code: dto.CodeConfig{Forge: "gerrit", Project: "openstack/sunbeam-charms"},
		}},
	}, projectsFilters{})

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if got := rows[0].Series; !reflect.DeepEqual(got, []string{"2024.1", "2025.1"}) {
		t.Fatalf("row series = %v, want launchpad defaults", got)
	}
	if got := rows[0].DevelopmentFocus; got != "2025.1" {
		t.Fatalf("row development focus = %q, want launchpad default", got)
	}
}

func TestApplyTUIConfigSetsStartupPaneAndDefaults(t *testing.T) {
	model := newRootModel(nil, true)
	merge := false
	reverse := true
	cfg := &dto.Config{
		TUI: dto.TUIConfig{
			DefaultPane: "packages",
			Panes: dto.TUIPanesConfig{
				Packages: &dto.TUIPackagesPaneConfig{
					Mode: "excuses",
					Filters: dto.TUIPackagesFiltersConfig{
						Tracker: "ubuntu",
						Team:    "ubuntu-openstack",
						Reverse: &reverse,
					},
				},
				Bugs: &dto.TUIBugsPaneConfig{
					Filters: dto.TUIBugsFiltersConfig{
						Merge: &merge,
					},
				},
			},
		},
	}

	model.applyTUIConfig(cfg)

	if model.activeView != viewPackages {
		t.Fatalf("activeView = %v, want packages", model.activeView)
	}
	if model.packages.filters.mode != packageModeExcuses {
		t.Fatalf("packages mode = %v, want excuses", model.packages.filters.mode)
	}
	if model.packages.filters.tracker != "ubuntu" || model.packages.filters.team != "ubuntu-openstack" {
		t.Fatalf("package preset filters = %+v", model.packages.filters)
	}
	if !model.packages.filters.reverse {
		t.Fatal("packages reverse = false, want true")
	}
	if model.bugs.filters.merge {
		t.Fatal("bugs merge = true, want false from preset")
	}
	if model.projects.config != cfg {
		t.Fatal("projects.config not seeded from bootstrap config")
	}
}

func TestSummarizeReleaseArtifactsDeduplicatesByArtifact(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	artifacts := summarizeReleaseArtifacts([]dto.ReleaseListEntry{
		{
			Project:      "demo",
			Name:         "artifact-a",
			ArtifactType: dto.ArtifactSnap,
			Channel:      "latest/edge",
			ReleasedAt:   now.Add(-time.Hour),
			Targets: []dto.ReleaseTargetSnapshot{{
				Architecture: "amd64",
				Base:         dto.ReleaseBase{Name: "ubuntu", Channel: "22.04"},
				Revision:     40,
				ReleasedAt:   now.Add(-time.Hour),
			}},
		},
		{
			Project:      "demo",
			Name:         "artifact-a",
			ArtifactType: dto.ArtifactSnap,
			Channel:      "latest/stable",
			ReleasedAt:   now,
			Targets: []dto.ReleaseTargetSnapshot{{
				Architecture: "amd64",
				Base:         dto.ReleaseBase{Name: "ubuntu", Channel: "24.04"},
				Revision:     41,
				ReleasedAt:   now,
			}},
		},
		{Project: "demo", Name: "artifact-b", ArtifactType: dto.ArtifactCharm, Channel: "2024.1/stable", ReleasedAt: now.Add(-2 * time.Hour)},
	})
	if got := len(artifacts); got != 2 {
		t.Fatalf("len(artifacts) = %d, want 2", got)
	}
	if artifacts[0].Name != "artifact-a" {
		t.Fatalf("artifacts[0].Name = %q, want artifact-a", artifacts[0].Name)
	}
	if !artifacts[0].ReleasedAt.Equal(now) {
		t.Fatalf("artifacts[0].ReleasedAt = %s, want %s", artifacts[0].ReleasedAt, now)
	}
	if got := artifacts[0].LatestVisibleTarget; got != "amd64@ubuntu/24.04:r41" {
		t.Fatalf("artifacts[0].LatestVisibleTarget = %q, want amd64@ubuntu/24.04:r41", got)
	}
	if got := artifacts[0].VisibleTargetCount; got != 2 {
		t.Fatalf("artifacts[0].VisibleTargetCount = %d, want 2", got)
	}
}

func TestSummarizeReleaseArtifactsKeepsSameNameAcrossTypes(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	artifacts := summarizeReleaseArtifacts([]dto.ReleaseListEntry{
		{Project: "demo", Name: "keystone", ArtifactType: dto.ArtifactSnap, Channel: "latest/stable", ReleasedAt: now},
		{Project: "demo", Name: "keystone", ArtifactType: dto.ArtifactCharm, Channel: "2024.1/stable", ReleasedAt: now.Add(-time.Hour)},
	})
	if got := len(artifacts); got != 2 {
		t.Fatalf("len(artifacts) = %d, want 2", got)
	}
	if artifacts[0].ArtifactType != dto.ArtifactSnap || artifacts[1].ArtifactType != dto.ArtifactCharm {
		t.Fatalf("artifacts = %+v, want separate snap and charm summaries", artifacts)
	}
}

func TestRenderReleaseDetailUsesLatestReleaseTime(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	detail := &dto.ReleaseShowResult{
		Project:      "demo",
		Name:         "artifact-a",
		ArtifactType: dto.ArtifactCharm,
		Tracks:       []string{"latest", "2024.1"},
		UpdatedAt:    now.Add(3 * time.Hour),
		Channels: []dto.ReleaseChannelSnapshot{
			{
				Channel: "latest/stable",
				Track:   "latest",
				Risk:    dto.ReleaseRiskStable,
				Targets: []dto.ReleaseTargetSnapshot{{
					Architecture: "amd64",
					Base:         dto.ReleaseBase{Name: "ubuntu", Channel: "24.04"},
					Revision:     3,
					Version:      "1.2.3",
					ReleasedAt:   now,
				}},
				Resources: []dto.ReleaseResourceSnapshot{
					{Name: "postgresql-image", Revision: 12},
				},
			},
		},
	}
	selected := &releaseArtifactSummary{Name: "artifact-a", ArtifactType: dto.ArtifactCharm}
	rendered := renderReleaseDetail(newTheme(), detail, selected, 120)
	if !strings.Contains(rendered, "Released: 2026-03-08T12:00:00Z") {
		t.Fatalf("detail missing released timestamp:\n%s", rendered)
	}
	if strings.Contains(rendered, "Updated: 2026-03-08T15:00:00Z") {
		t.Fatalf("detail should not use cache updated time:\n%s", rendered)
	}
	if !strings.Contains(rendered, "targets: amd64@ubuntu/24.04:r3/1.2.3") {
		t.Fatalf("detail missing target-aware release formatting:\n%s", rendered)
	}
	if !strings.Contains(rendered, "resources: postgresql-image:r12") {
		t.Fatalf("detail missing resources:\n%s", rendered)
	}
}

func TestReleaseFilterFormAutocompleteUsesReleaseData(t *testing.T) {
	session := newEmbeddedTestSession(t)
	defer session.Close()
	session.Config.LocalConfig().Releases.TargetProfiles = map[string]config.ReleaseTargetProfileConfig{
		"noble-and-newer": {
			Include: []config.ReleaseTargetMatcherConfig{{BaseNames: []string{"ubuntu"}, MinBaseChannel: "24.04"}},
		},
	}

	form := newReleaseFilterForm(session, releasesModel{
		filters: releasesFilters{project: "demo", targetProfile: "noble-and-newer"},
		rows: []dto.ReleaseListEntry{
			{
				Project:      "demo",
				Name:         "artifact-a",
				ArtifactType: dto.ArtifactCharm,
				Track:        "2024.1",
				Risk:         dto.ReleaseRiskStable,
				Branch:       "risc-v",
			},
		},
	})

	assertSuggestionsContain(t, form.fields[0].AvailableSuggestions(), "demo")
	if form.kinds[1] != fieldKindEnum || form.kinds[2] != fieldKindEnum || form.kinds[5] != fieldKindEnum || form.kinds[6] != fieldKindEnum {
		t.Fatalf("release form enum kinds not configured: %v", form.kinds)
	}
	assertSuggestionsContain(t, form.options[1], "charm")
	assertSuggestionsContain(t, form.options[2], "stable")
	assertSuggestionsContain(t, form.fields[3].AvailableSuggestions(), "2024.1")
	assertSuggestionsContain(t, form.fields[4].AvailableSuggestions(), "risc-v")
	assertSuggestionsContain(t, form.options[5], "noble-and-newer")
	assertSuggestionsContain(t, form.options[6], "true")
	assertSuggestionsContain(t, form.options[6], "false")
}

func TestVimMotionGGAndGJumpReleases(t *testing.T) {
	model := newRootModel(nil, true)
	model.activeView = viewReleases
	model.releases.artifacts = []releaseArtifactSummary{
		{Name: "artifact-a", ArtifactType: dto.ArtifactCharm},
		{Name: "artifact-b", ArtifactType: dto.ArtifactSnap},
		{Name: "artifact-c", ArtifactType: dto.ArtifactRock},
	}
	model.releases.index = 1

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	model = next.(rootModel)
	if model.releases.index != 1 {
		t.Fatalf("index after first g = %d, want unchanged", model.releases.index)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	model = next.(rootModel)
	if model.releases.index != 0 {
		t.Fatalf("index after gg = %d, want 0", model.releases.index)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	model = next.(rootModel)
	if model.releases.index != 2 {
		t.Fatalf("index after G = %d, want 2", model.releases.index)
	}
}

func TestRenderMetaPaneListsVimShortcuts(t *testing.T) {
	model := newRootModel(nil, true)
	model.width = 120
	model.height = 30
	rendered := model.renderHelp()
	for _, want := range []string{"Meta", "gg jump to the beginning", "G jump to the end", "autocomplete", "l logs"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("meta pane missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderViewsAndOverlays(t *testing.T) {
	session := newEmbeddedTestSession(t)
	defer session.Close()

	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	model := newRootModel(session, true)
	model.width = 120
	model.height = 40
	model.dashboard.auth = &dto.AuthStatus{
		Launchpad: dto.LaunchpadAuthStatus{
			Authenticated:   true,
			DisplayName:     "Tester",
			Source:          "file",
			CredentialsPath: filepath.Join(t.TempDir(), "creds"),
		},
	}
	model.dashboard.ops = []dto.OperationJob{{
		ID:          "op-1",
		Kind:        dto.OperationKindBuildTrigger,
		State:       dto.OperationStateRunning,
		Summary:     "build artifact-a",
		Cancellable: true,
	}}
	model.dashboard.builds = []dto.Build{{
		Project:     "demo",
		Recipe:      "demo-artifact",
		Title:       "demo build",
		State:       dto.BuildBuilding,
		Arch:        "amd64",
		CreatedAt:   now,
		StartedAt:   now,
		WebLink:     "https://example.test/build",
		BuildLogURL: "https://example.test/log",
		CanRetry:    true,
		CanCancel:   true,
	}}
	model.builds.rows = model.dashboard.builds
	model.releases.artifacts = []releaseArtifactSummary{{
		Project:             "demo",
		Name:                "artifact-a",
		ArtifactType:        dto.ArtifactCharm,
		ReleasedAt:          now,
		LatestVisibleTarget: "amd64@ubuntu/24.04:r3",
	}}
	model.releases.detail = &dto.ReleaseShowResult{
		Project:      "demo",
		Name:         "artifact-a",
		ArtifactType: dto.ArtifactCharm,
		Tracks:       []string{"2024.1"},
		Channels: []dto.ReleaseChannelSnapshot{{
			Channel: "2024.1/stable",
			Track:   "2024.1",
			Risk:    dto.ReleaseRiskStable,
			Branch:  "risc-v",
			Targets: []dto.ReleaseTargetSnapshot{{Architecture: "amd64", Revision: 3, Version: "1.2.3", ReleasedAt: now}},
			Resources: []dto.ReleaseResourceSnapshot{{
				Name:     "image",
				Revision: 7,
			}},
		}},
	}
	model.ops.rows = model.dashboard.ops
	model.ops.events = []dto.OperationEvent{{Time: now, Type: "info", Message: "queued"}}
	model.auth.status = model.dashboard.auth
	model.auth.launchpadBegin = &dto.LaunchpadAuthBeginResult{FlowID: "flow-1", AuthorizeURL: "https://example.test/auth"}
	model.cache.status = &frontend.CacheStatusResponse{
		Git: struct {
			Directory string
			Repos     []frontend.CacheEntry
		}{Repos: []frontend.CacheEntry{{Name: "repo"}}},
		Packages: struct {
			Directory string
			Sources   []dto.CacheStatus
			Error     string
		}{Sources: []dto.CacheStatus{{Name: "archive"}}},
		Bugs: struct {
			Directory string
			Entries   []dto.BugCacheStatus
			Error     string
		}{Entries: []dto.BugCacheStatus{{Project: "demo"}}},
		Excuses: struct {
			Directory string
			Entries   []dto.ExcusesCacheStatus
			Error     string
		}{Entries: []dto.ExcusesCacheStatus{{Tracker: "ubuntu", EntryCount: 1}}},
		Releases: struct {
			Directory string
			Entries   []dto.ReleaseCacheStatus
			Error     string
		}{Entries: []dto.ReleaseCacheStatus{{Project: "demo", Name: "artifact-a"}}},
	}
	model.server.local = &runtimeadapter.LocalServerStatus{Running: true, PID: 42, LogFile: filepath.Join(t.TempDir(), "watchtower.log")}
	model.logsModal = logsModalModel{
		sessionLines: []string{"time=... level=INFO msg=\"session\""},
		daemonLines:  []string{"time=... level=INFO msg=\"daemon\""},
	}
	model.prompt = promptModel{title: "Switch?", body: "body", accept: "Yes", reject: "No"}
	model.buildFilterForm = newBuildFilterForm(buildsModel{filters: buildsFilters{project: "demo", state: "building", active: true, source: "remote"}, defaults: buildsFilters{active: true, source: "remote"}})
	model.releaseFilterForm = newReleaseFilterForm(session, releasesModel{
		filters:  releasesFilters{project: "demo"},
		defaults: releasesFilters{},
		rows: []dto.ReleaseListEntry{{
			Project:      "demo",
			Name:         "artifact-a",
			ArtifactType: dto.ArtifactCharm,
			Track:        "2024.1",
			Risk:         dto.ReleaseRiskStable,
		}},
	})
	model.buildTriggerForm = newBuildTriggerForm(session)
	model.packageFilterForm = newPackageFilterForm(session, model.packages)
	model.bugFilterForm = newBugFilterForm(session, model.bugs)
	model.reviewFilterForm = newReviewFilterForm(session, model.reviews)
	model.commitFilterForm = newCommitFilterForm(session, model.commits)
	model.projectFilterForm = newProjectFilterForm(model.projects)

	model.activeView = viewBuilds
	if rendered := model.renderBuilds(); !strings.Contains(rendered, "demo build") {
		t.Fatalf("renderBuilds missing build detail:\n%s", rendered)
	}

	model.activeView = viewReleases
	if rendered := model.renderReleases(); !strings.Contains(rendered, "artifact-a") {
		t.Fatalf("renderReleases missing artifact:\n%s", rendered)
	}
	if rendered := model.renderReleases(); !strings.Contains(rendered, "amd64@ubuntu/24.04:r3") {
		t.Fatalf("renderReleases missing latest visible target:\n%s", rendered)
	}

	model.packages.inventoryRows = []distro.SourcePackage{{
		Package:   "nova",
		Version:   "2:1.0-0ubuntu1",
		Suite:     "noble",
		Component: "main",
	}}
	model.packages.inventoryDetail = &distro.SourcePackageInfo{
		SourcePackage: distro.SourcePackage{
			Package:   "nova",
			Version:   "2:1.0-0ubuntu1",
			Suite:     "noble",
			Component: "main",
		},
		Fields: []distro.FieldEntry{{Key: "Maintainer", Value: "Canonical"}},
	}
	model.activeView = viewPackages
	if rendered := model.renderPackages(); !strings.Contains(rendered, "nova") {
		t.Fatalf("renderPackages missing package detail:\n%s", rendered)
	}

	model.bugs.rows = []forge.BugTask{{Project: "snap-openstack", BugID: "12345", Status: "Fix Released", Title: "nova bug"}}
	model.bugs.detail = &forge.Bug{ID: "12345", Title: "nova bug", Tasks: []forge.BugTask{{Project: "snap-openstack", Status: "Fix Released"}}}
	model.activeView = viewBugs
	if rendered := model.renderBugs(); !strings.Contains(rendered, "12345") {
		t.Fatalf("renderBugs missing bug detail:\n%s", rendered)
	}

	model.reviews.rows = []forge.MergeRequest{{Repo: "snap-openstack", Forge: forge.ForgeGitHub, ID: "#42", Title: "Improve test coverage", Author: "alice", State: forge.MergeStateOpen}}
	model.reviews.detail = &forge.MergeRequest{
		Repo:     "snap-openstack",
		Forge:    forge.ForgeGitHub,
		ID:       "#42",
		Title:    "Improve test coverage",
		Author:   "alice",
		State:    forge.MergeStateOpen,
		Comments: []forge.ReviewComment{{Kind: forge.ReviewCommentGeneral, Author: "bob", Body: "looks good"}},
		Files:    []forge.ReviewFile{{Path: "README.md", Status: "modified", Additions: 1}},
		DiffText: "diff --git a/README.md b/README.md",
	}
	model.activeView = viewReviews
	if rendered := model.renderReviews(); !strings.Contains(rendered, "Improve test coverage") {
		t.Fatalf("renderReviews missing MR detail:\n%s", rendered)
	}
	if rendered := model.renderReviews(); !strings.Contains(rendered, "looks good") || !strings.Contains(rendered, "README.md") {
		t.Fatalf("renderReviews missing cached review detail:\n%s", rendered)
	}

	model.commits.rows = []forge.Commit{{Repo: "snap-openstack", Forge: forge.ForgeGitHub, SHA: "0123456789abcdef", Message: "Fix bug\n\nmore", Author: "alice", Date: now}}
	model.activeView = viewCommits
	if rendered := model.renderCommits(); !strings.Contains(rendered, "0123456789") {
		t.Fatalf("renderCommits missing commit detail:\n%s", rendered)
	}

	model.projects.rows = []projectSummary{{Name: "snap-openstack", CodeForge: "github", CodeProject: "snap-openstack", Series: []string{"2025.1"}}}
	model.activeView = viewProjects
	if rendered := model.renderProjects(); !strings.Contains(rendered, "snap-openstack") {
		t.Fatalf("renderProjects missing project detail:\n%s", rendered)
	}
	projectDetail := renderProjectPane(newTheme(), &projectSummary{
		Name:             "sunbeam-charms",
		CodeForge:        "gerrit",
		CodeProject:      "openstack/sunbeam-charms",
		Series:           []string{"2024.1", "2025.1"},
		DevelopmentFocus: "2025.1",
		Config: dto.ProjectConfig{
			Name: "sunbeam-charms",
			Code: dto.CodeConfig{Forge: "gerrit", Project: "openstack/sunbeam-charms"},
		},
	}, 80)
	if !strings.Contains(projectDetail, "Series: 2024.1, 2025.1") || !strings.Contains(projectDetail, "Development focus: 2025.1") {
		t.Fatalf("renderProjectPane missing effective launchpad defaults:\n%s", projectDetail)
	}

	for _, tc := range []struct {
		name    string
		overlay overlayKind
		want    string
	}{
		{name: "auth", overlay: overlayAuth, want: "Launchpad authorize URL:"},
		{name: "operations", overlay: overlayOperations, want: "Events"},
		{name: "cache", overlay: overlayCache, want: "releases"},
		{name: "sync", overlay: overlaySync, want: "Project Sync"},
		{name: "logs", overlay: overlayLogs, want: "Session Logs:"},
		{name: "server", overlay: overlayServer, want: "PID: 42"},
		{name: "prompt", overlay: overlayPrompt, want: "[Enter] Yes"},
		{name: "build-filters", overlay: overlayBuildFilters, want: "Build Filters"},
		{name: "release-filters", overlay: overlayReleaseFilters, want: "Release Filters"},
		{name: "build-trigger", overlay: overlayBuildTrigger, want: "Trigger Build"},
		{name: "package-filters", overlay: overlayPackageFilters, want: "Package Filters"},
		{name: "bug-filters", overlay: overlayBugFilters, want: "Bug List / Search"},
		{name: "review-filters", overlay: overlayReviewFilters, want: "Review Filters"},
		{name: "commit-filters", overlay: overlayCommitFilters, want: "Commit Filters"},
		{name: "project-filters", overlay: overlayProjectFilters, want: "Project Filters"},
		{name: "project-sync", overlay: overlayProjectSync, want: "Project Sync"},
		{name: "bug-sync", overlay: overlayBugSync, want: "Bug Sync"},
		{name: "cache-sync", overlay: overlayCacheSync, want: "Sync Review Cache"},
		{name: "cache-clear", overlay: overlayCacheClear, want: "Clear Review Cache"},
	} {
		model.overlay = tc.overlay
		switch tc.overlay {
		case overlayProjectSync:
			model.projectSyncForm = newProjectSyncForm(nil)
		case overlayBugSync:
			model.bugSyncForm = newBugSyncForm(nil)
		case overlayCacheSync:
			model.cache.selected = cacheActionReviews
			model.cacheSyncForm = newCacheSyncForm(nil, cacheActionReviews)
		case overlayCacheClear:
			model.cache.selected = cacheActionReviews
			model.cacheClearForm = newCacheClearForm(nil, cacheActionReviews)
		}
		rendered := model.renderOverlay("base")
		if !strings.Contains(rendered, tc.want) {
			t.Fatalf("%s overlay missing %q:\n%s", tc.name, tc.want, rendered)
		}
	}
}

func TestRenderDashboardReleasesIncludesArtifactTypeForSameNameEntries(t *testing.T) {
	model := newRootModel(nil, true)
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	model.dashboard.cache = &frontend.CacheStatusResponse{
		Releases: struct {
			Directory string
			Entries   []dto.ReleaseCacheStatus
			Error     string
		}{
			Entries: []dto.ReleaseCacheStatus{
				{Project: "demo", Name: "keystone", ArtifactType: dto.ArtifactSnap, LastUpdated: now},
				{Project: "demo", Name: "keystone", ArtifactType: dto.ArtifactCharm, LastUpdated: now.Add(-time.Minute)},
			},
		},
	}

	rendered := model.renderDashboardReleases()
	if !strings.Contains(rendered, "snap  keystone") || !strings.Contains(rendered, "charm  keystone") {
		t.Fatalf("renderDashboardReleases() = %q, want type-qualified duplicate names", rendered)
	}
}

func TestUpdateGlobalAndOverlayNavigation(t *testing.T) {
	session := newEmbeddedTestSession(t)
	defer session.Close()

	model := newRootModel(session, true)
	model.width = 120
	model.height = 40
	model.dashboard.ops = []dto.OperationJob{{ID: "op-1", Kind: dto.OperationKindBuildTrigger, State: dto.OperationStateQueued}}
	model.builds.rows = []dto.Build{
		{Project: "demo", Title: "a", State: dto.BuildPending},
		{Project: "demo", Title: "b", State: dto.BuildBuilding},
	}
	model.releases.artifacts = []releaseArtifactSummary{
		{Name: "artifact-a", ArtifactType: dto.ArtifactCharm},
		{Name: "artifact-b", ArtifactType: dto.ArtifactSnap},
	}
	model.ops.rows = []dto.OperationJob{
		{ID: "op-1", Kind: dto.OperationKindBuildTrigger, State: dto.OperationStateRunning, Summary: "one", Cancellable: true},
		{ID: "op-2", Kind: dto.OperationKindBuildTrigger, State: dto.OperationStateSucceeded, Summary: "two"},
	}

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	model = next.(rootModel)
	if model.overlay != overlayHelp {
		t.Fatalf("overlay = %v, want help", model.overlay)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	model = next.(rootModel)
	if model.overlayScroll == 0 {
		t.Fatal("overlayScroll = 0, want end offset")
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(rootModel)
	if model.overlay != overlayNone {
		t.Fatalf("overlay = %v, want none", model.overlay)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	model = next.(rootModel)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model = next.(rootModel)
	if model.builds.index != 1 {
		t.Fatalf("builds.index = %d, want 1", model.builds.index)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = next.(rootModel)
	if model.overlay != overlayBuildFilters {
		t.Fatalf("overlay = %v, want build filters", model.overlay)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(rootModel)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	model = next.(rootModel)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = next.(rootModel)
	if model.overlay != overlayReleaseFilters {
		t.Fatalf("overlay = %v, want release filters", model.overlay)
	}

	model.overlay = overlayNone
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	model = next.(rootModel)
	if model.overlay != overlayLogs {
		t.Fatalf("overlay = %v, want logs", model.overlay)
	}

	model.overlay = overlayNone
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	model = next.(rootModel)
	if model.overlay != overlaySync {
		t.Fatalf("overlay = %v, want sync", model.overlay)
	}

	model.overlay = overlayNone
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	model = next.(rootModel)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = next.(rootModel)
	if model.overlay != overlayPackageFilters {
		t.Fatalf("overlay = %v, want package filters", model.overlay)
	}

	model.overlay = overlayNone
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	model = next.(rootModel)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = next.(rootModel)
	if model.overlay != overlayBugFilters {
		t.Fatalf("overlay = %v, want bug filters", model.overlay)
	}

	model.overlay = overlayNone
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	model = next.(rootModel)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = next.(rootModel)
	if model.overlay != overlayReviewFilters {
		t.Fatalf("overlay = %v, want review filters", model.overlay)
	}

	model.overlay = overlayNone
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("7")})
	model = next.(rootModel)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = next.(rootModel)
	if model.overlay != overlayCommitFilters {
		t.Fatalf("overlay = %v, want commit filters", model.overlay)
	}

	model.overlay = overlayNone
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("8")})
	model = next.(rootModel)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = next.(rootModel)
	if model.overlay != overlayProjectFilters {
		t.Fatalf("overlay = %v, want project filters", model.overlay)
	}

	model.overlay = overlayNone
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	model = next.(rootModel)
	if model.overlay != overlayOperations {
		t.Fatalf("overlay = %v, want operations", model.overlay)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model = next.(rootModel)
	if model.ops.index != 1 {
		t.Fatalf("ops.index = %d, want 1", model.ops.index)
	}

	model.overlay = overlaySync
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(rootModel)
	if model.overlay != overlayProjectSync {
		t.Fatalf("overlay = %v, want project sync form", model.overlay)
	}

	model.overlay = overlayCache
	model.cache.status = &frontend.CacheStatusResponse{}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model = next.(rootModel)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	model = next.(rootModel)
	if model.overlay != overlayCacheSync {
		t.Fatalf("overlay = %v, want cache sync form", model.overlay)
	}
}

func TestRenderLogsModalRemoteNote(t *testing.T) {
	model := newRootModel(nil, true)
	model.width = 120
	model.logsModal = logsModalModel{
		sessionLines: []string{"time=... level=DEBUG msg=\"session\""},
		daemonNote:   "Remote server logs are not available locally.",
	}
	rendered := model.renderLogsModal()
	for _, want := range []string{"Session Logs:", "Remote server logs are not available locally."} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("renderLogsModal missing %q:\n%s", want, rendered)
		}
	}
}

func TestLoadLogsCmdIncludesDaemonTail(t *testing.T) {
	session := newEmbeddedTestSession(t)
	defer session.Close()

	runtimeDir := t.TempDir()
	logFile := filepath.Join(runtimeDir, "watchtower.log")
	if err := os.WriteFile(logFile, []byte("daemon-one\ndaemon-two\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	setSessionTarget(t, session, runtimeadapter.TargetInfo{
		Kind: runtimeadapter.TargetKindDaemon,
		// Display-only address fixture used to verify TargetInfo wiring; the
		// socket is never dialled in this test.
		Address: "unix://" + filepath.Join(runtimeDir, "watchtower.sock"),
		LogFile: logFile,
	})

	logs := newLogBuffer(10)
	_, _ = logs.Write([]byte("session-one\n"))
	msg := loadLogsCmd(session, logs)()
	loaded, ok := msg.(logsLoadedMsg)
	if !ok {
		t.Fatalf("msg = %T, want logsLoadedMsg", msg)
	}
	if loaded.err != nil {
		t.Fatalf("logsLoadedMsg.err = %v", loaded.err)
	}
	if len(loaded.sessionLines) != 1 || loaded.sessionLines[0] != "session-one" {
		t.Fatalf("sessionLines = %v, want session-one", loaded.sessionLines)
	}
	if len(loaded.daemonLines) != 2 || loaded.daemonLines[1] != "daemon-two" {
		t.Fatalf("daemonLines = %v, want daemon tail", loaded.daemonLines)
	}
}

func TestPackagesAndCommitsSubmodeNavigation(t *testing.T) {
	model := newRootModel(nil, true)
	model.activeView = viewPackages

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	model = next.(rootModel)
	if model.packages.filters.mode != packageModeDiff {
		t.Fatalf("package mode after ] = %v, want diff", model.packages.filters.mode)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	model = next.(rootModel)
	if model.packages.filters.mode != packageModeInventory {
		t.Fatalf("package mode after [ = %v, want inventory", model.packages.filters.mode)
	}

	model.activeView = viewCommits
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	model = next.(rootModel)
	if model.commits.filters.mode != commitModeTrack {
		t.Fatalf("commit mode after ] = %v, want track", model.commits.filters.mode)
	}

	model.activeView = viewPackages
	model.width = 120
	model.height = 40
	rendered := model.renderPackages()
	for _, want := range []string{"Inventory", "Diff", "Excuses"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("renderPackages missing mode tab %q:\n%s", want, rendered)
		}
	}
}

func TestPromptRenderingForPackagesDiffAndCommitTrack(t *testing.T) {
	model := newRootModel(nil, true)
	model.width = 120
	model.height = 40

	model.packages.filters.mode = packageModeDiff
	model.packages.prompt = "Select a package set in filters to load diff results."
	if rendered := model.renderPackages(); !strings.Contains(rendered, "Select a package set") {
		t.Fatalf("renderPackages missing diff prompt:\n%s", rendered)
	}

	model.commits.filters.mode = commitModeTrack
	model.commits.prompt = "Enter a bug ID in filters to track matching commits."
	if rendered := model.renderCommits(); !strings.Contains(rendered, "Enter a bug ID") {
		t.Fatalf("renderCommits missing track prompt:\n%s", rendered)
	}
}

func TestFormHelpersAndUtilityRendering(t *testing.T) {
	form := newFormModal("Release Filters", []fieldDef{
		{placeholder: "project", value: "de", suggestions: []string{"demo", "demo2"}},
		{placeholder: "artifact type", value: "snap", suggestions: []string{"snap", "rock", "charm"}, kind: fieldKindEnum},
	})
	if ok := acceptFormSuggestion(&form); !ok {
		t.Fatal("acceptFormSuggestion() = false, want true")
	}
	if got := form.fields[0].Value(); got != "demo" {
		t.Fatalf("form.fields[0].Value() = %q, want demo", got)
	}

	var submitted []string
	cmd := updateFormModal(tea.KeyMsg{Type: tea.KeyTab}, &form, func(values []string) tea.Cmd {
		submitted = append([]string(nil), values...)
		return nil
	}, func() {})
	if cmd != nil {
		t.Fatal("updateFormModal(tab) returned command, want nil")
	}
	if form.active != 1 {
		t.Fatalf("form.active = %d, want 1", form.active)
	}

	_ = updateFormModal(tea.KeyMsg{Type: tea.KeyDown}, &form, func(values []string) tea.Cmd {
		submitted = append([]string(nil), values...)
		return nil
	}, func() {})
	if got := form.fields[1].Value(); got != "charm" {
		t.Fatalf("enum field value after down = %q, want charm", got)
	}

	_ = updateFormModal(tea.KeyMsg{Type: tea.KeyEnter}, &form, func(values []string) tea.Cmd {
		submitted = append([]string(nil), values...)
		return nil
	}, func() {})
	if len(submitted) != 2 {
		t.Fatalf("submitted len = %d, want 2", len(submitted))
	}

	renderedForm := renderFormModal(newTheme(), form, 120, 40)
	for _, want := range []string{"Release Filters", "artifact type", "pick"} {
		if !strings.Contains(renderedForm, want) {
			t.Fatalf("renderFormModal missing %q:\n%s", want, renderedForm)
		}
	}
	narrowRenderedForm := renderFormModal(newTheme(), form, 64, 40)
	for _, want := range []string{"Ctrl+R", "Esc"} {
		if !strings.Contains(narrowRenderedForm, want) {
			t.Fatalf("narrow renderFormModal missing %q:\n%s", want, narrowRenderedForm)
		}
	}

	if got := renderBuildRows(newTheme(), []dto.Build{{Project: "demo", Title: "build-a", State: dto.BuildPending}}, 0, 80, true); !strings.Contains(got, "build-a") {
		t.Fatalf("renderBuildRows missing build title:\n%s", got)
	}
	if got := renderBuildDetail(newTheme(), &dto.Build{Project: "demo", Recipe: "recipe", Title: "build-a", State: dto.BuildPending, CreatedAt: time.Unix(0, 0)}, 80); !strings.Contains(got, "Project: demo") {
		t.Fatalf("renderBuildDetail missing project:\n%s", got)
	}
	if got := renderReleaseArtifacts(newTheme(), []releaseArtifactSummary{{Name: "artifact-a", ArtifactType: dto.ArtifactCharm, ReleasedAt: time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)}}, 0, 40, true); !strings.Contains(got, "artifact-a") {
		t.Fatalf("renderReleaseArtifacts missing artifact:\n%s", got)
	}
	if got := renderOperationRows(newTheme(), []dto.OperationJob{{State: dto.OperationStateQueued, Kind: dto.OperationKindBuildTrigger, Summary: "build"}}, 0); !strings.Contains(got, "build") {
		t.Fatalf("renderOperationRows missing summary:\n%s", got)
	}
	if got := renderOperationEvents(newTheme(), []dto.OperationEvent{{Time: time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC), Message: "queued"}}); !strings.Contains(got, "queued") {
		t.Fatalf("renderOperationEvents missing event:\n%s", got)
	}
	if got := renderViewport("one\ntwo\nthree", 2, 1); !strings.Contains(got, "two") || strings.Contains(got, "one") {
		t.Fatalf("renderViewport unexpected window:\n%s", got)
	}
	if got := truncateToWidth("neutron-baremetal-switch-config-k8s", 12); !strings.HasSuffix(got, "…") {
		t.Fatalf("truncateToWidth() = %q, want ellipsis", got)
	}
	if got := padRight("snap", 6); got != "snap  " {
		t.Fatalf("padRight() = %q, want padded string", got)
	}
	if got := renderToast(newTheme(), toastState{message: "ok", level: "success"}); !strings.Contains(got, "ok") {
		t.Fatalf("renderToast missing message: %q", got)
	}
	if got := displayLaunchpadName(&dto.AuthStatus{Launchpad: dto.LaunchpadAuthStatus{Username: "tester"}}); got != "tester" {
		t.Fatalf("displayLaunchpadName() = %q, want tester", got)
	}
	if got := emptyAsAny(""); got != "any" {
		t.Fatalf("emptyAsAny() = %q, want any", got)
	}
	if got := formatListTime(time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)); got != "2026-03-08 12:00" {
		t.Fatalf("formatListTime() = %q", got)
	}
	if got := firstNonEmptySlice("demo"); len(got) != 1 || got[0] != "demo" {
		t.Fatalf("firstNonEmptySlice() = %v", got)
	}
	if got := errString(context.Canceled); got == "" {
		t.Fatal("errString() = empty, want message")
	}
	if got := errorsJoin(context.Canceled, context.DeadlineExceeded); got == nil {
		t.Fatal("errorsJoin() = nil, want joined error")
	}
}

func TestFormHelpersSupportMultiSelectVisualRange(t *testing.T) {
	form := newFormModal("Project Sync", []fieldDef{
		{placeholder: "projects", suggestions: []string{"alpha", "beta", "gamma"}, kind: fieldKindMultiSelect},
		{placeholder: "dry run", value: "true", suggestions: []string{"true", "false"}, kind: fieldKindEnum},
	})

	_ = updateFormModal(tea.KeyMsg{Type: tea.KeySpace}, &form, func([]string) tea.Cmd { return nil }, func() {})
	if got := form.values()[0]; got != "alpha" {
		t.Fatalf("multi-select after space = %q, want alpha", got)
	}

	_ = updateFormModal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")}, &form, func([]string) tea.Cmd { return nil }, func() {})
	_ = updateFormModal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")}, &form, func([]string) tea.Cmd { return nil }, func() {})
	if got := form.values()[0]; got != "alpha, beta, gamma" {
		t.Fatalf("multi-select after visual G = %q, want all values", got)
	}
	if got := form.fields[0].Value(); got != "alpha, beta (+1 more)" {
		t.Fatalf("display after visual G = %q, want compact summary", got)
	}
	if !form.visualMode {
		t.Fatal("visualMode = false, want true after v/G")
	}

	_ = updateFormModal(tea.KeyMsg{Type: tea.KeyTab}, &form, func([]string) tea.Cmd { return nil }, func() {})
	if form.visualMode {
		t.Fatal("visualMode = true after tab, want cleared")
	}

	form.moveActiveField(-1)
	form.toggleVisualMode()
	_ = updateFormModal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}, &form, func([]string) tea.Cmd { return nil }, func() {})
	_ = updateFormModal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}, &form, func([]string) tea.Cmd { return nil }, func() {})
	if got := form.optionIndices[0]; got != 0 {
		t.Fatalf("cursor after gg = %d, want 0", got)
	}
}

func TestCtrlRResetsFormFieldsToDefaults(t *testing.T) {
	form := newFormModal("Bug Filters", []fieldDef{
		{placeholder: "project", value: "demo", resetValue: ""},
		{placeholder: "merge", value: "false", resetValue: "true", suggestions: []string{"false", "true"}, kind: fieldKindEnum},
	})
	form.fields[0].SetValue("openstack")
	form.fields[0].Blur()
	form.active = 1
	form.fields[1].Focus()
	form.fields[1].SetValue("false")
	form.optionIndices[1] = formOptionIndex(form.options[1], "false")
	form.errorMsg = "stale error"

	cancelled := false
	_ = updateFormModal(tea.KeyMsg{Type: tea.KeyCtrlR}, &form, func(values []string) tea.Cmd {
		t.Fatalf("ctrl+r should not submit form: %v", values)
		return nil
	}, func() {
		cancelled = true
	})

	if cancelled {
		t.Fatal("ctrl+r cancelled modal, want reset only")
	}
	if got := form.fields[0].Value(); got != "" {
		t.Fatalf("project after ctrl+r = %q, want empty", got)
	}
	if got := form.fields[1].Value(); got != "true" {
		t.Fatalf("merge after ctrl+r = %q, want true", got)
	}
	if got := form.optionIndices[1]; got != formOptionIndex(form.options[1], "true") {
		t.Fatalf("merge option index after ctrl+r = %d, want reset index", got)
	}
	if form.errorMsg != "" {
		t.Fatalf("errorMsg after ctrl+r = %q, want empty", form.errorMsg)
	}
	if form.active != 1 {
		t.Fatalf("active field after ctrl+r = %d, want 1", form.active)
	}
}

func TestCtrlRUsesConfiguredPaneDefaults(t *testing.T) {
	model := newRootModel(nil, true)
	model.bugs.defaults = bugsFilters{project: "snap-openstack", merge: false}
	model.bugs.filters = bugsFilters{project: "ubuntu-openstack-rocks", merge: true}
	model.bugFilterForm = newBugFilterForm(nil, model.bugs)
	model.bugFilterForm.fields[6].SetValue("openstack")
	model.bugFilterForm.fields[12].SetValue("true")

	next, _ := model.updateBugFilterForm(tea.KeyMsg{Type: tea.KeyCtrlR})
	model = next.(rootModel)

	if got := model.bugFilterForm.fields[6].Value(); got != "snap-openstack" {
		t.Fatalf("bug project after ctrl+r = %q, want preset project", got)
	}
	if got := model.bugFilterForm.fields[12].Value(); got != "false" {
		t.Fatalf("bug merge after ctrl+r = %q, want preset false", got)
	}
}

func TestBootstrapFailureFallsBackToBuiltInDefaults(t *testing.T) {
	model := newRootModel(nil, true)

	next, _ := model.Update(tuiBootstrapLoadedMsg{err: errors.New("boom")})
	model = next.(rootModel)

	if model.activeView != viewDashboard {
		t.Fatalf("activeView after bootstrap failure = %v, want dashboard", model.activeView)
	}
	if !model.bugs.filters.merge {
		t.Fatal("bugs merge after bootstrap failure = false, want built-in true")
	}
	if model.toast.message == "" {
		t.Fatal("toast after bootstrap failure = empty, want warning")
	}
}

func TestEmbeddedAuthLoginPromptsForUpgrade(t *testing.T) {
	session := newEmbeddedTestSession(t)
	defer session.Close()

	model := newRootModel(session, true)
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = next.(rootModel)
	if model.overlay != overlayAuth {
		t.Fatalf("overlay = %v, want auth", model.overlay)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	model = next.(rootModel)
	if model.overlay != overlayPrompt {
		t.Fatalf("overlay = %v, want prompt", model.overlay)
	}
}

func TestCtrlCCancelsOpenModals(t *testing.T) {
	model := newRootModel(nil, true)
	model.overlay = overlayBugFilters
	model.bugFilterForm = newBugFilterForm(nil, bugsModel{})

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = next.(rootModel)
	if model.overlay != overlayNone {
		t.Fatalf("overlay after ctrl+c on filter modal = %v, want none", model.overlay)
	}

	model.overlay = overlayAuth
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = next.(rootModel)
	if model.overlay != overlayNone {
		t.Fatalf("overlay after ctrl+c on auth modal = %v, want none", model.overlay)
	}

	model.overlay = overlaySync
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = next.(rootModel)
	if model.overlay != overlayNone {
		t.Fatalf("overlay after ctrl+c on sync modal = %v, want none", model.overlay)
	}
}

func TestEmbeddedOperationCancelPromptsForUpgrade(t *testing.T) {
	session := newEmbeddedTestSession(t)
	defer session.Close()

	model := newRootModel(session, true)
	model.overlay = overlayOperations
	model.ops.rows = []dto.OperationJob{{ID: "op-1", Kind: dto.OperationKindBuildTrigger, Cancellable: true}}

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = next.(rootModel)
	if model.overlay != overlayPrompt {
		t.Fatalf("overlay = %v, want prompt", model.overlay)
	}
}

func TestEmbeddedBuildTriggerPromptsForUpgrade(t *testing.T) {
	session := newEmbeddedTestSession(t)
	defer session.Close()

	model := newRootModel(session, true)
	model.overlay = overlayBuildTrigger
	model.buildTriggerForm = newBuildTriggerForm(session)
	model.buildTriggerForm.fields[0].SetValue("demo")
	model.buildTriggerForm.fields[2].SetValue("remote")
	model.buildTriggerForm.fields[3].SetValue(".")
	model.buildTriggerForm.active = len(model.buildTriggerForm.fields) - 1

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(rootModel)
	if model.overlay != overlayPrompt {
		t.Fatalf("overlay = %v, want prompt", model.overlay)
	}
}

func TestReadOnlyTriggerBuildIsDenied(t *testing.T) {
	session := newReadOnlyEmbeddedTestSession(t)
	defer session.Close()

	msg := triggerBuildCmd(session, frontend.BuildTriggerRequest{Project: "demo", Async: true})()
	result, ok := msg.(actionDeniedMsg)
	if !ok {
		t.Fatalf("msg = %T, want actionDeniedMsg", msg)
	}
	if result.err == nil {
		t.Fatal("actionDeniedMsg.err = nil, want read-only denial")
	}
}

func TestReadOnlyLogoutIsDenied(t *testing.T) {
	session := newReadOnlyEmbeddedTestSession(t)
	defer session.Close()

	msg := logoutLaunchpadAuthCmd(session)()
	result, ok := msg.(actionDeniedMsg)
	if !ok {
		t.Fatalf("msg = %T, want actionDeniedMsg", msg)
	}
	if result.err == nil {
		t.Fatal("actionDeniedMsg.err = nil, want read-only denial")
	}
}

func TestReadOnlyOperationCancelIsDenied(t *testing.T) {
	session := newReadOnlyEmbeddedTestSession(t)
	defer session.Close()

	msg := cancelOperationCmd(session, "op-1")()
	result, ok := msg.(actionDeniedMsg)
	if !ok {
		t.Fatalf("msg = %T, want actionDeniedMsg", msg)
	}
	if result.err == nil {
		t.Fatal("actionDeniedMsg.err = nil, want read-only denial")
	}
}

func TestReadOnlyApplySyncAndCacheMutationsAreDenied(t *testing.T) {
	session := newReadOnlyEmbeddedTestSession(t)
	defer session.Close()

	for _, tc := range []struct {
		name string
		msg  tea.Msg
	}{
		{name: "project sync apply", msg: syncProjectsCmd(session, frontend.ProjectSyncRequest{DryRun: false})()},
		{name: "bug sync apply", msg: syncBugsCmd(session, frontend.BugSyncRequest{DryRun: false})()},
		{name: "cache sync", msg: syncCacheCmd(session, cacheActionGit, []string{"demo"})()},
		{name: "cache clear", msg: clearCacheCmd(session, cacheActionGit, []string{"demo"})()},
	} {
		result, ok := tc.msg.(actionDeniedMsg)
		if !ok {
			t.Fatalf("%s msg = %T, want actionDeniedMsg", tc.name, tc.msg)
		}
		if result.err == nil {
			t.Fatalf("%s denial err = nil, want read-only denial", tc.name)
		}
	}
}

func TestRenderSyncAndCacheModalsIncludeLastActionSummary(t *testing.T) {
	model := newRootModel(nil, true)
	model.width = 120
	model.syncModal.lastAction = "Project sync (dry-run)"
	model.syncModal.lastSummary = []string{"Actions: 2", "demo  create_series  2025.1"}
	syncRendered := model.renderSyncModal()
	if !strings.Contains(syncRendered, "Project sync (dry-run)") || !strings.Contains(syncRendered, "Actions: 2") {
		t.Fatalf("renderSyncModal missing summary:\n%s", syncRendered)
	}

	model.cache.status = &frontend.CacheStatusResponse{}
	model.cache.lastAction = "Review cache sync completed"
	model.cache.lastSummary = []string{"Projects: 1", "Summaries: 5"}
	cacheRendered := model.renderCacheModal()
	if !strings.Contains(cacheRendered, "Review cache sync completed") || !strings.Contains(cacheRendered, "Summaries: 5") {
		t.Fatalf("renderCacheModal missing summary:\n%s", cacheRendered)
	}
}

func newEmbeddedTestSession(t *testing.T) *runtimeadapter.Session {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	session, err := runtimeadapter.NewSession(context.Background(), runtimeadapter.Options{
		LogWriter:    &bytes.Buffer{},
		TargetPolicy: runtimeadapter.TargetPolicyPreferEmbedded,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	return session
}

func newReadOnlyEmbeddedTestSession(t *testing.T) *runtimeadapter.Session {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	session, err := runtimeadapter.NewSession(context.Background(), runtimeadapter.Options{
		LogWriter:    &bytes.Buffer{},
		TargetPolicy: runtimeadapter.TargetPolicyPreferEmbedded,
		AccessMode:   runtimeadapter.AccessModeReadOnly,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	return session
}

func assertSuggestionsContain(t *testing.T, suggestions []string, want string) {
	t.Helper()
	for _, suggestion := range suggestions {
		if suggestion == want {
			return
		}
	}
	t.Fatalf("suggestions %v do not contain %q", suggestions, want)
}

func TestBugSearchRowsAndEvidence(t *testing.T) {
	results := []dto.BugSearchResult{{
		Reference:      "launchpad:123",
		ID:             "123",
		Title:          "Cinder volume attach fails",
		Projects:       []string{"snap-openstack"},
		Status:         []string{"New"},
		Importance:     []string{"High"},
		Classification: dto.BugSearchDirect,
		Score:          4.25,
		Evidence: []dto.BugMatchEvidence{{
			Field:           "description",
			Excerpt:         "volume attachment failed",
			TruncatedBefore: true,
			Matches: []dto.BugEvidenceMatch{{
				QueryConcept: "volume",
				MatchType:    "stem",
				MatchedText:  "volume",
			}},
		}},
	}}

	rows := bugSearchTasks(results)
	if len(rows) != 1 || rows[0].BugID != "123" || rows[0].Project != "snap-openstack" {
		t.Fatalf("bugSearchTasks() = %#v", rows)
	}
	rendered := renderBugSearchEvidence(newTheme(), results[0], 100)
	for _, want := range []string{"direct", "launchpad:123", "description", "volume attachment failed"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered evidence %q does not contain %q", rendered, want)
		}
	}
}

func setSessionTarget(t *testing.T, session *runtimeadapter.Session, target runtimeadapter.TargetInfo) {
	t.Helper()
	field := reflect.ValueOf(session).Elem().FieldByName("target")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(target))
}
