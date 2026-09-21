// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package bugsync

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gboutry/sunbeam-watchtower/internal/core/port"
	forge "github.com/gboutry/sunbeam-watchtower/pkg/forge/v1"
)

// mockCommitSource implements commit.CommitSource for testing.
type mockCommitSource struct {
	forgeType forge.ForgeType
	commits   map[string][]forge.Commit // branch → commits
	revisions map[string][]forge.Commit // tag/revision → commits
	branches  []string
}

func (m *mockCommitSource) ForgeType() forge.ForgeType { return m.forgeType }

func (m *mockCommitSource) ListCommits(_ context.Context, opts forge.ListCommitsOpts) ([]forge.Commit, error) {
	if opts.Revision != "" {
		commits, ok := m.revisions[opts.Revision]
		if !ok {
			return nil, fmt.Errorf("revision %s not found", opts.Revision)
		}
		return commits, nil
	}
	branch := opts.Branch
	if branch == "" {
		branch = "main"
	}
	return m.commits[branch], nil
}

func TestSync_StableRevisionMarksOnlyMatchingSeriesReleased(t *testing.T) {
	const tag = "refs/tags/rev1101^{}"
	source := &mockCommitSource{
		branches: []string{"stable/2024.1"},
		commits: map[string][]forge.Commit{
			"stable/2024.1": {{SHA: "newer", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}}},
		},
		revisions: map[string][]forge.Commit{
			tag: {{SHA: "released", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}}},
		},
	}
	tracker := &mockBugTracker{bugs: map[string]*forge.Bug{
		"12345": {ID: "12345", Tasks: []forge.BugTask{
			{BugID: "12345", TargetName: "snap-openstack", TargetLink: "https://api.launchpad.net/devel/snap-openstack/2024.1", Status: "Fix Committed", SelfLink: "task-2024"},
			{BugID: "12345", TargetName: "snap-openstack", TargetLink: "https://api.launchpad.net/devel/snap-openstack/2025.1", Status: "Fix Committed", SelfLink: "task-2025"},
		}},
	}}
	svc := NewService(
		map[string]port.CommitSource{"openstack": source}, tracker, nil,
		map[string][]string{"openstack": {"snap-openstack"}}, nil,
	).WithReleaseEvidence([]ReleaseBoundary{{
		Project: "openstack", Series: "2024.1", Channel: "2024.1/stable", Revision: 1101, Tag: "rev1101",
	}}).WithProjectPolicy(map[string]bool{"snap-openstack": true}, map[string][]string{"snap-openstack": {"2024.1"}})

	result, err := svc.Sync(context.Background(), SyncOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	var released *SyncAction
	for i := range result.Actions {
		if result.Actions[i].ActionType == ActionStatusUpdate {
			if result.Actions[i].Series == "2025.1" {
				t.Fatalf("2025.1 unexpectedly updated: %+v", result.Actions[i])
			}
			released = &result.Actions[i]
		}
	}
	if released == nil || released.NewStatus != "Fix Released" || released.Revision != 1101 || released.Tag != "rev1101" {
		t.Fatalf("release action = %+v", released)
	}
}

func TestSync_SharedTaskUsesLeastAdvancedComponent(t *testing.T) {
	sources := map[string]port.CommitSource{
		"openstack": &mockCommitSource{branches: []string{"stable/2024.1"}, commits: map[string][]forge.Commit{
			"stable/2024.1": {{SHA: "snap-fix", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}}},
		}},
		"openstack-hypervisor": &mockCommitSource{branches: []string{"stable/2024.1"}, commits: map[string][]forge.Commit{
			"stable/2024.1": {{SHA: "partial", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefPartial}}}},
		}},
	}
	tracker := &mockBugTracker{bugs: map[string]*forge.Bug{
		"12345": {ID: "12345", Tasks: []forge.BugTask{
			{BugID: "12345", TargetName: "snap-openstack", TargetLink: "https://api.launchpad.net/devel/snap-openstack/2024.1", Status: "New", SelfLink: "common"},
			{BugID: "12345", TargetName: "snap-openstack-hypervisor", TargetLink: "https://api.launchpad.net/devel/snap-openstack-hypervisor/2024.1", Status: "New", SelfLink: "hypervisor"},
		}},
	}}
	svc := NewService(sources, tracker, nil, map[string][]string{
		"openstack":            {"snap-openstack"},
		"openstack-hypervisor": {"snap-openstack", "snap-openstack-hypervisor"},
	}, nil).WithProjectPolicy(map[string]bool{"snap-openstack": true}, nil)

	result, err := svc.Sync(context.Background(), SyncOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	statuses := make(map[string]string)
	for _, action := range result.Actions {
		if action.ActionType == ActionStatusUpdate {
			statuses[action.Project] = action.NewStatus
		}
	}
	if statuses["snap-openstack"] != "In Progress" {
		t.Fatalf("shared status = %q, want In Progress", statuses["snap-openstack"])
	}
	if statuses["snap-openstack-hypervisor"] != "In Progress" {
		t.Fatalf("hypervisor status = %q, want In Progress", statuses["snap-openstack-hypervisor"])
	}
}

func TestSync_ExplicitProjectsEnsureMappedTasks(t *testing.T) {
	sources := map[string]port.CommitSource{
		"openstack":            &mockCommitSource{},
		"openstack-hypervisor": &mockCommitSource{},
		"sunbeam-charms":       &mockCommitSource{},
	}
	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{"12345": {ID: "12345", Tasks: []forge.BugTask{{BugID: "12345", TargetName: "snap-openstack"}}}},
		projects: map[string]*forge.Project{
			"snap-openstack-hypervisor": {Name: "snap-openstack-hypervisor", SelfLink: "lp:hypervisor"},
			"sunbeam-charms":            {Name: "sunbeam-charms", SelfLink: "lp:charms"},
		},
	}
	svc := NewService(sources, tracker, nil, map[string][]string{
		"openstack":            {"snap-openstack"},
		"openstack-hypervisor": {"snap-openstack", "snap-openstack-hypervisor"},
		"sunbeam-charms":       {"snap-openstack", "sunbeam-charms"},
	}, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{
		BugIDs: []string{"12345"}, Projects: []string{"openstack", "openstack-hypervisor", "sunbeam-charms"}, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	added := make(map[string]bool)
	for _, action := range result.Actions {
		if action.ActionType == ActionAddProjectTask {
			added[action.Project] = true
		}
	}
	if !added["snap-openstack-hypervisor"] || !added["sunbeam-charms"] || added["snap-openstack"] {
		t.Fatalf("added tasks = %+v", added)
	}
}

func TestSync_WarnsWhenOlderSeriesReleasedFirst(t *testing.T) {
	const tag = "refs/tags/rev1101^{}"
	source := &mockCommitSource{
		branches: []string{"stable/2024.1", "stable/2025.1"},
		commits: map[string][]forge.Commit{
			"stable/2024.1": {{SHA: "old", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}}},
			"stable/2025.1": {{SHA: "new", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}}},
		},
		revisions: map[string][]forge.Commit{tag: {{SHA: "old", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}}}},
	}
	tracker := &mockBugTracker{bugs: map[string]*forge.Bug{
		"12345": {ID: "12345", Tasks: []forge.BugTask{
			{BugID: "12345", TargetName: "snap-openstack/2024.1", Status: "Fix Committed", SelfLink: "old"},
			{BugID: "12345", TargetName: "snap-openstack/2025.1", Status: "Fix Committed", SelfLink: "new"},
		}},
	}}
	svc := NewService(map[string]port.CommitSource{"openstack": source}, tracker, nil,
		map[string][]string{"openstack": {"snap-openstack"}}, nil).
		WithReleaseEvidence([]ReleaseBoundary{{Project: "openstack", Series: "2024.1", Channel: "2024.1/stable", Revision: 1101, Tag: "rev1101"}}).
		WithProjectPolicy(map[string]bool{"snap-openstack": true}, map[string][]string{"snap-openstack": {"2024.1", "2025.1", "2026.1"}})
	result, err := svc.Sync(context.Background(), SyncOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, warning := range result.Errors {
		if strings.Contains(warning.Error(), "before newer series 2025.1=Fix Committed, 2026.1=missing task") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %+v, want release-order warning", result.Errors)
	}
}

func TestSync_DivergentPublishedTargetsStayFixCommitted(t *testing.T) {
	source := &mockCommitSource{
		branches: []string{"stable/2024.1"},
		commits:  map[string][]forge.Commit{"stable/2024.1": {{SHA: "fix", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}}}},
		revisions: map[string][]forge.Commit{
			"refs/tags/rev10^{}": {{SHA: "with-fix", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}}},
			"refs/tags/rev11^{}": {{SHA: "without-fix"}},
		},
	}
	tracker := &mockBugTracker{bugs: map[string]*forge.Bug{"12345": {
		ID: "12345", Tasks: []forge.BugTask{{BugID: "12345", TargetName: "snap-openstack/2024.1", Status: "New", SelfLink: "task"}},
	}}}
	svc := NewService(map[string]port.CommitSource{"openstack": source}, tracker, nil,
		map[string][]string{"openstack": {"snap-openstack"}}, nil).
		WithReleaseEvidence([]ReleaseBoundary{
			{Project: "openstack", Series: "2024.1", Channel: "2024.1/stable", Revision: 10, Tag: "rev10"},
			{Project: "openstack", Series: "2024.1", Channel: "2024.1/stable", Revision: 11, Tag: "rev11"},
		})
	result, err := svc.Sync(context.Background(), SyncOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range result.Actions {
		if action.ActionType == ActionStatusUpdate && action.NewStatus != "Fix Committed" {
			t.Fatalf("status action = %+v, want Fix Committed", action)
		}
	}
	found := false
	for _, warning := range result.Errors {
		found = found || strings.Contains(warning.Error(), "targets disagree")
	}
	if !found {
		t.Fatalf("warnings = %+v, want target disagreement", result.Errors)
	}
}

func TestSync_DoesNotReviveUnsupportedSeries(t *testing.T) {
	for _, withSeriesTask := range []bool{false, true} {
		name := "missing task"
		if withSeriesTask {
			name = "existing task"
		}
		t.Run(name, func(t *testing.T) {
			source := &mockCommitSource{
				branches: []string{"stable/2023.1"},
				commits: map[string][]forge.Commit{
					"stable/2023.1": {{SHA: "old-fix", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}}},
				},
			}
			tasks := []forge.BugTask{{BugID: "12345", TargetName: "snap-openstack", Status: "Fix Released", SelfLink: "project"}}
			if withSeriesTask {
				tasks = append(tasks, forge.BugTask{
					BugID: "12345", TargetName: "snap-openstack/2023.1", Status: "New", SelfLink: "unsupported-series",
				})
			}
			tracker := &mockBugTracker{bugs: map[string]*forge.Bug{"12345": {ID: "12345", Tasks: tasks}}}
			svc := NewService(map[string]port.CommitSource{"openstack": source}, tracker, nil,
				map[string][]string{"openstack": {"snap-openstack"}}, nil).
				WithProjectPolicy(map[string]bool{"snap-openstack": true}, map[string][]string{
					"snap-openstack": {"2024.1", "2025.1", "2026.1"},
				})

			result, err := svc.Sync(context.Background(), SyncOptions{DryRun: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, action := range result.Actions {
				if action.Series == "2023.1" || (action.ActionType == ActionStatusUpdate && action.TaskTitle == "snap-openstack/2023.1") {
					t.Fatalf("unsupported series action = %+v", action)
				}
			}
		})
	}
}

func TestSync_UsesFreshTaskStatusForPlanning(t *testing.T) {
	source := &mockCommitSource{
		branches: []string{"stable/2024.1"},
		commits: map[string][]forge.Commit{
			"stable/2024.1": {{SHA: "fix", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}}},
		},
	}
	stale := &forge.Bug{ID: "12345", Tasks: []forge.BugTask{{
		BugID: "12345", TargetName: "snap-openstack/2024.1", Status: "New", SelfLink: "task",
	}}}
	fresh := &forge.Bug{ID: "12345", Tasks: []forge.BugTask{{
		BugID: "12345", TargetName: "snap-openstack/2024.1", Status: "Fix Committed", SelfLink: "task",
	}}}
	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{"12345": stale}, freshBugs: map[string]*forge.Bug{"12345": fresh},
	}
	svc := NewService(map[string]port.CommitSource{"openstack": source}, tracker, nil,
		map[string][]string{"openstack": {"snap-openstack"}}, nil).
		WithProjectPolicy(nil, map[string][]string{"snap-openstack": {"2024.1"}})

	result, err := svc.Sync(context.Background(), SyncOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range result.Actions {
		if action.ActionType == ActionStatusUpdate {
			t.Fatalf("stale status produced action: %+v", action)
		}
	}
}

func (m *mockCommitSource) ListMRCommits(_ context.Context) ([]forge.Commit, error) {
	return nil, nil
}

func (m *mockCommitSource) ListBranches(_ context.Context) ([]string, error) {
	return m.branches, nil
}

type testProjectSource struct {
	Source    *mockCommitSource
	ForgeType forge.ForgeType
}

func toSources(sources map[string]testProjectSource) map[string]port.CommitSource {
	result := make(map[string]port.CommitSource, len(sources))
	for name, source := range sources {
		if source.Source != nil {
			source.Source.forgeType = source.ForgeType
			result[name] = source.Source
		}
	}
	return result
}

// mockBugTracker implements port.BugTracker for testing.
type mockBugTracker struct {
	bugs           map[string]*forge.Bug
	freshBugs      map[string]*forge.Bug
	updatedTasks   []taskUpdate
	assignments    []assignment
	project        *forge.Project
	projects       map[string]*forge.Project // per-project override
	series         []forge.ProjectSeries
	recentBugTasks []forge.BugTask // returned by ListBugTasks when CreatedSince is set
	updateErr      error
	assignErr      error
}

type taskUpdate struct {
	SelfLink string
	Status   string
}

type assignment struct {
	BugID          int
	SeriesSelfLink string
}

func (m *mockBugTracker) Type() forge.ForgeType { return forge.ForgeLaunchpad }

func (m *mockBugTracker) GetBug(_ context.Context, id string) (*forge.Bug, error) {
	bug, ok := m.bugs[id]
	if !ok {
		return nil, fmt.Errorf("bug %s not found", id)
	}
	return bug, nil
}

func (m *mockBugTracker) GetBugFresh(ctx context.Context, id string, _ bool) (*forge.Bug, error) {
	if bug, ok := m.freshBugs[id]; ok {
		return bug, nil
	}
	return m.GetBug(ctx, id)
}

func (m *mockBugTracker) ListBugTasks(_ context.Context, _ string, opts forge.ListBugTasksOpts) ([]forge.BugTask, error) {
	if opts.CreatedSince != "" && m.recentBugTasks != nil {
		return m.recentBugTasks, nil
	}
	return nil, nil
}

func (m *mockBugTracker) UpdateBugTaskStatus(_ context.Context, selfLink, status string) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updatedTasks = append(m.updatedTasks, taskUpdate{SelfLink: selfLink, Status: status})
	return nil
}

func (m *mockBugTracker) AddBugTask(_ context.Context, bugID int, seriesSelfLink string) error {
	if m.assignErr != nil {
		return m.assignErr
	}
	m.assignments = append(m.assignments, assignment{BugID: bugID, SeriesSelfLink: seriesSelfLink})
	return nil
}

func (m *mockBugTracker) GetProjectSeries(_ context.Context, _ string) ([]forge.ProjectSeries, error) {
	return m.series, nil
}

func (m *mockBugTracker) GetProject(_ context.Context, name string) (*forge.Project, error) {
	if m.projects != nil {
		if p, ok := m.projects[name]; ok {
			return p, nil
		}
	}
	if m.project == nil {
		return nil, fmt.Errorf("project not found")
	}
	return m.project, nil
}

func TestSync_StatusUpdate_ClosesBug(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"my-project": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "fix: Closes-Bug: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{
			"12345": {
				ID:    "12345",
				Title: "test bug",
				Tasks: []forge.BugTask{
					{
						BugID:      "12345",
						Title:      "Bug #12345 in sunbeam: test bug",
						Status:     "New",
						SelfLink:   "https://api.launchpad.net/devel/sunbeam/+bug/12345",
						URL:        "https://bugs.launchpad.net/sunbeam/+bug/12345",
						TargetName: "sunbeam",
					},
				},
			},
		},
		project: &forge.Project{
			Name:                 "sunbeam",
			SelfLink:             "https://api.launchpad.net/devel/sunbeam",
			DevelopmentFocusLink: "https://api.launchpad.net/devel/sunbeam/trunk",
		},
	}

	svc := NewService(sources, tracker, nil, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Should have updated the task status to Fix Committed.
	statusActions := 0
	for _, a := range result.Actions {
		if a.ActionType == ActionStatusUpdate {
			statusActions++
			if a.OldStatus != "New" {
				t.Errorf("OldStatus = %q, want New", a.OldStatus)
			}
			if a.NewStatus != "Fix Committed" {
				t.Errorf("NewStatus = %q, want Fix Committed", a.NewStatus)
			}
		}
	}
	if statusActions != 1 {
		t.Errorf("expected 1 status update action, got %d", statusActions)
	}

	if len(tracker.updatedTasks) != 1 {
		t.Fatalf("expected 1 task update, got %d", len(tracker.updatedTasks))
	}
	if tracker.updatedTasks[0].Status != "Fix Committed" {
		t.Errorf("updated status = %q, want Fix Committed", tracker.updatedTasks[0].Status)
	}
}

func TestSync_PartialBug_SetsInProgress(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"my-project": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "Partial-Bug: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefPartial}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{
			"12345": {
				ID: "12345",
				Tasks: []forge.BugTask{
					{
						BugID:      "12345",
						Title:      "Bug #12345 in sunbeam: test",
						Status:     "New",
						SelfLink:   "https://api.launchpad.net/devel/sunbeam/+bug/12345",
						TargetName: "sunbeam",
					},
				},
			},
		},
		project: &forge.Project{
			Name:                 "sunbeam",
			SelfLink:             "https://api.launchpad.net/devel/sunbeam",
			DevelopmentFocusLink: "https://api.launchpad.net/devel/sunbeam/trunk",
		},
	}

	svc := NewService(sources, tracker, nil, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	statusActions := 0
	for _, a := range result.Actions {
		if a.ActionType == ActionStatusUpdate {
			statusActions++
			if a.NewStatus != "In Progress" {
				t.Errorf("NewStatus = %q, want In Progress", a.NewStatus)
			}
		}
	}
	if statusActions != 1 {
		t.Errorf("expected 1 status update (In Progress), got %d", statusActions)
	}

	if len(tracker.updatedTasks) != 1 {
		t.Fatalf("expected 1 task update, got %d", len(tracker.updatedTasks))
	}
	if tracker.updatedTasks[0].Status != "In Progress" {
		t.Errorf("updated status = %q, want In Progress", tracker.updatedTasks[0].Status)
	}
}

func TestSync_RelatedBug_Skipped(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"my-project": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "Related-Bug: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefRelated}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{
			"12345": {
				ID: "12345",
				Tasks: []forge.BugTask{
					{BugID: "12345", Status: "New", SelfLink: "link"},
				},
			},
		},
	}

	svc := NewService(sources, tracker, nil, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped (Related-Bug), got %d", result.Skipped)
	}
	if len(tracker.updatedTasks) != 0 {
		t.Errorf("expected no updates for Related-Bug, got %d", len(tracker.updatedTasks))
	}
}

func TestSync_SkipFixReleased(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"my-project": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "Closes-Bug: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{
			"12345": {
				ID: "12345",
				Tasks: []forge.BugTask{
					{
						BugID:    "12345",
						Title:    "Bug #12345 in sunbeam: test",
						Status:   "Fix Released",
						SelfLink: "https://api.launchpad.net/devel/sunbeam/+bug/12345",
					},
				},
			},
		},
	}

	svc := NewService(sources, tracker, nil, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", result.Skipped)
	}
	if len(tracker.updatedTasks) != 0 {
		t.Errorf("expected no updates, got %d", len(tracker.updatedTasks))
	}
}

func TestSync_SkipFixCommitted(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"my-project": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "Closes-Bug: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{
			"12345": {
				ID: "12345",
				Tasks: []forge.BugTask{
					{
						BugID:    "12345",
						Title:    "Bug #12345 in sunbeam: test",
						Status:   "Fix Committed",
						SelfLink: "https://api.launchpad.net/devel/sunbeam/+bug/12345",
					},
				},
			},
		},
	}

	svc := NewService(sources, tracker, nil, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", result.Skipped)
	}
}

func TestSync_DryRun(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"my-project": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "Closes-Bug: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{
			"12345": {
				ID: "12345",
				Tasks: []forge.BugTask{
					{
						BugID:      "12345",
						Title:      "Bug #12345 in sunbeam: test",
						Status:     "New",
						SelfLink:   "https://api.launchpad.net/devel/sunbeam/+bug/12345",
						TargetName: "sunbeam",
					},
				},
			},
		},
		project: &forge.Project{
			Name:                 "sunbeam",
			SelfLink:             "https://api.launchpad.net/devel/sunbeam",
			DevelopmentFocusLink: "https://api.launchpad.net/devel/sunbeam/trunk",
		},
	}

	svc := NewService(sources, tracker, nil, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Actions should be planned but not executed.
	if len(result.Actions) == 0 {
		t.Error("expected at least one action in dry-run mode")
	}
	if len(tracker.updatedTasks) != 0 {
		t.Errorf("expected no actual updates in dry-run, got %d", len(tracker.updatedTasks))
	}
	if len(tracker.assignments) != 0 {
		t.Errorf("expected no actual assignments in dry-run, got %d", len(tracker.assignments))
	}
}

func TestSync_MultipleBranches(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"my-project": {
			Source: &mockCommitSource{
				branches: []string{"main", "stable/2024.1"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "Closes-Bug: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}},
					},
					"stable/2024.1": {
						{SHA: "bbb", Message: "Closes-Bug: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{
			"12345": {
				ID: "12345",
				Tasks: []forge.BugTask{
					{
						BugID:      "12345",
						Title:      "Bug #12345 in sunbeam: test",
						Status:     "In Progress",
						SelfLink:   "https://api.launchpad.net/devel/sunbeam/+bug/12345",
						TargetName: "sunbeam",
					},
				},
			},
		},
		project: &forge.Project{
			Name:                 "sunbeam",
			SelfLink:             "https://api.launchpad.net/devel/sunbeam",
			DevelopmentFocusLink: "https://api.launchpad.net/devel/sunbeam/trunk",
		},
		series: []forge.ProjectSeries{
			{Name: "2024.1", SelfLink: "https://api.launchpad.net/devel/sunbeam/2024.1", Active: true},
		},
	}

	svc := NewService(sources, tracker, nil, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Should have status update + series assignments.
	var statusUpdates, assignments int
	for _, a := range result.Actions {
		switch a.ActionType {
		case ActionStatusUpdate:
			statusUpdates++
		case ActionSeriesAssignment:
			assignments++
		}
	}

	if statusUpdates != 1 {
		t.Errorf("expected 1 status update, got %d", statusUpdates)
	}
	// Should assign to both dev focus (main) and 2024.1 (stable/2024.1).
	if assignments != 2 {
		t.Errorf("expected 2 series assignments (dev focus + 2024.1), got %d", assignments)
	}
}

func TestSync_IgnoresIrrelevantBranches(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"my-project": {
			Source: &mockCommitSource{
				branches: []string{"main", "feature/cool-thing"},
				commits: map[string][]forge.Commit{
					"main": {},
					"feature/cool-thing": {
						{SHA: "aaa", Message: "Closes-Bug: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{},
	}

	svc := NewService(sources, tracker, nil, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(result.Actions) != 0 {
		t.Errorf("expected no actions (feature branch should be ignored), got %d", len(result.Actions))
	}
}

func TestSync_NoBugRefs(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"my-project": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "feat: new thing"},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{}

	svc := NewService(sources, tracker, nil, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(result.Actions) != 0 {
		t.Errorf("expected no actions, got %d", len(result.Actions))
	}
}

func TestSync_ProjectFilter(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"project-a": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "Closes-Bug: #11111", BugRefs: []forge.BugRef{{ID: "11111", Type: forge.BugRefCloses}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
		"project-b": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "bbb", Message: "Closes-Bug: #22222", BugRefs: []forge.BugRef{{ID: "22222", Type: forge.BugRefCloses}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{
			"11111": {
				ID: "11111",
				Tasks: []forge.BugTask{
					{BugID: "11111", Title: "Bug #11111 in a: test", Status: "New", SelfLink: "link-a", TargetName: "a"},
				},
			},
		},
		project: &forge.Project{Name: "a", SelfLink: "https://api.launchpad.net/devel/a", DevelopmentFocusLink: "https://api.launchpad.net/devel/a/trunk"},
	}

	svc := NewService(sources, tracker, nil, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{Projects: []string{"project-a"}})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Should only process project-a's bugs.
	for _, a := range result.Actions {
		if a.ActionType == ActionStatusUpdate && a.BugID == "22222" {
			t.Error("should not have processed bug from project-b")
		}
	}
}

func TestSync_AddsMissingProjectTask(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"snap-openstack-hypervisor": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "Closes-Bug: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	// Bug exists on "sunbeam" but not on "snap-openstack-hypervisor".
	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{
			"12345": {
				ID: "12345",
				Tasks: []forge.BugTask{
					{
						BugID:      "12345",
						Title:      "Bug #12345 in sunbeam: test",
						Status:     "New",
						SelfLink:   "https://api.launchpad.net/devel/sunbeam/+bug/12345",
						TargetName: "sunbeam",
					},
				},
			},
		},
		projects: map[string]*forge.Project{
			"sunbeam": {
				Name:                 "sunbeam",
				SelfLink:             "https://api.launchpad.net/devel/sunbeam",
				DevelopmentFocusLink: "https://api.launchpad.net/devel/sunbeam/trunk",
			},
			"snap-openstack-hypervisor": {
				Name:     "snap-openstack-hypervisor",
				SelfLink: "https://api.launchpad.net/devel/snap-openstack-hypervisor",
			},
		},
	}

	// Map the watchtower project to its LP bug project.
	lpProjectMap := map[string][]string{
		"snap-openstack-hypervisor": {"snap-openstack-hypervisor"},
	}

	svc := NewService(sources, tracker, nil, lpProjectMap, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Should have added a project task for snap-openstack-hypervisor.
	addActions := 0
	for _, a := range result.Actions {
		if a.ActionType == ActionAddProjectTask {
			addActions++
			if a.Project != "snap-openstack-hypervisor" {
				t.Errorf("added task on project %q, want snap-openstack-hypervisor", a.Project)
			}
		}
	}
	if addActions != 1 {
		t.Errorf("expected 1 add_project_task action, got %d", addActions)
	}

	// Verify AddBugTask was called with the project self_link.
	found := false
	for _, a := range tracker.assignments {
		if a.BugID == 12345 && a.SeriesSelfLink == "https://api.launchpad.net/devel/snap-openstack-hypervisor" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected AddBugTask call with snap-openstack-hypervisor self_link, assignments: %v", tracker.assignments)
	}
}

func TestSync_DoesNotDowngradeFixCommitted(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"my-project": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "Partial-Bug: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefPartial}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{
			"12345": {
				ID: "12345",
				Tasks: []forge.BugTask{
					{
						BugID:    "12345",
						Title:    "Bug #12345",
						Status:   "Fix Committed",
						SelfLink: "link",
					},
				},
			},
		},
	}

	svc := NewService(sources, tracker, nil, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Partial-Bug should not downgrade Fix Committed to In Progress.
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped (Fix Committed > In Progress), got %d", result.Skipped)
	}
	if len(tracker.updatedTasks) != 0 {
		t.Errorf("expected no updates (no downgrade), got %d", len(tracker.updatedTasks))
	}
}

func TestBranchToSeriesName(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"main", "development"},
		{"master", "development"},
		{"stable/2024.1", "2024.1"},
		{"stable/2025.1", "2025.1"},
		{"feature/cool", ""},
		{"release/v1.0", ""},
	}

	for _, tt := range tests {
		got := branchToSeriesName(tt.branch)
		if got != tt.want {
			t.Errorf("branchToSeriesName(%q) = %q, want %q", tt.branch, got, tt.want)
		}
	}
}

func TestIsRelevantBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   bool
	}{
		{"main", true},
		{"master", true},
		{"stable/2024.1", true},
		{"feature/x", false},
		{"release/v1", false},
	}

	for _, tt := range tests {
		got := isRelevantBranch(tt.branch)
		if got != tt.want {
			t.Errorf("isRelevantBranch(%q) = %v, want %v", tt.branch, got, tt.want)
		}
	}
}

func TestTaskTargetUsesLaunchpadTargetLinkForSeries(t *testing.T) {
	project, series := taskTarget(forge.BugTask{
		TargetName: "snap-openstack",
		TargetLink: "https://api.launchpad.net/devel/snap-openstack/2024.1",
	})
	if project != "snap-openstack" || series != "2024.1" {
		t.Fatalf("taskTarget() = %q, %q", project, series)
	}
}

func TestSync_SinceFiltersViaSearchTasks(t *testing.T) {
	sources := toSources(map[string]testProjectSource{
		"my-project": {
			Source: &mockCommitSource{
				branches: []string{"main"},
				commits: map[string][]forge.Commit{
					"main": {
						{SHA: "aaa", Message: "Closes-Bug: #11111", BugRefs: []forge.BugRef{{ID: "11111", Type: forge.BugRefCloses}}},
						{SHA: "bbb", Message: "Closes-Bug: #22222", BugRefs: []forge.BugRef{{ID: "22222", Type: forge.BugRefCloses}}},
					},
				},
			},
			ForgeType: forge.ForgeLaunchpad,
		},
	})

	tracker := &mockBugTracker{
		bugs: map[string]*forge.Bug{
			"11111": {
				ID: "11111",
				Tasks: []forge.BugTask{
					{BugID: "11111", Title: "Bug #11111 in sunbeam: recent bug", Status: "New", SelfLink: "link-11111", TargetName: "sunbeam"},
				},
			},
			"22222": {
				ID: "22222",
				Tasks: []forge.BugTask{
					{BugID: "22222", Title: "Bug #22222 in sunbeam: old bug", Status: "New", SelfLink: "link-22222", TargetName: "sunbeam"},
				},
			},
		},
		project: &forge.Project{
			Name:                 "sunbeam",
			SelfLink:             "https://api.launchpad.net/devel/sunbeam",
			DevelopmentFocusLink: "https://api.launchpad.net/devel/sunbeam/trunk",
		},
		// Only bug 11111 is returned by searchTasks (created recently).
		recentBugTasks: []forge.BugTask{
			{BugID: "11111"},
		},
	}

	since := time.Now().AddDate(0, 0, -30)
	svc := NewService(sources, tracker, []string{"sunbeam"}, nil, nil)
	result, err := svc.Sync(context.Background(), SyncOptions{Since: &since})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Only bug 11111 should be processed (22222 not in searchTasks results).
	for _, a := range result.Actions {
		if a.BugID == "22222" {
			t.Error("should not have processed bug 22222 (not in recent search results)")
		}
	}

	// Bug 11111 should have been updated.
	if len(tracker.updatedTasks) != 1 {
		t.Fatalf("expected 1 task update, got %d", len(tracker.updatedTasks))
	}
	if tracker.updatedTasks[0].SelfLink != "link-11111" {
		t.Errorf("updated wrong task: %s", tracker.updatedTasks[0].SelfLink)
	}
}
