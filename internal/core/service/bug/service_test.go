package bug

import (
	"context"
	"fmt"
	"testing"
	"time"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

type mockBugTracker struct {
	forgeType forge.ForgeType
	tasks     []forge.BugTask
	bug       *forge.Bug
	err       error
	lastOpts  forge.ListBugTasksOpts
}

func (m *mockBugTracker) Type() forge.ForgeType { return m.forgeType }

func (m *mockBugTracker) GetBug(_ context.Context, id string) (*forge.Bug, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.bug != nil {
		return m.bug, nil
	}
	return nil, fmt.Errorf("bug %s not found", id)
}

func (m *mockBugTracker) ListBugTasks(_ context.Context, _ string, opts forge.ListBugTasksOpts) ([]forge.BugTask, error) {
	m.lastOpts = opts
	if m.err != nil {
		return nil, m.err
	}
	return m.tasks, nil
}

func (m *mockBugTracker) UpdateBugTaskStatus(_ context.Context, _, _ string) error { return nil }
func (m *mockBugTracker) AddBugTask(_ context.Context, _ int, _ string) error      { return nil }
func (m *mockBugTracker) GetProjectSeries(_ context.Context, _ string) ([]forge.ProjectSeries, error) {
	return nil, nil
}
func (m *mockBugTracker) GetProject(_ context.Context, _ string) (*forge.Project, error) {
	return nil, nil
}

func TestService_List_Aggregation(t *testing.T) {
	now := time.Now()

	tracker := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		tasks: []forge.BugTask{
			{BugID: "1", Title: "Bug 1", UpdatedAt: now.Add(-1 * time.Hour)},
			{BugID: "2", Title: "Bug 2", UpdatedAt: now.Add(-3 * time.Hour)},
		},
	}

	svc := NewService(
		map[string]ProjectBugTracker{
			"launchpad:snap-openstack": {Tracker: tracker, ProjectID: "snap-openstack"},
		},
		map[string][]ProjectBinding{
			"launchpad:snap-openstack": {
				{ProjectName: "sunbeam"},
				{ProjectName: "microstack"},
			},
		},
		nil)

	tasks, results, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	// 2 bugs × 2 projects = 4 tasks
	if len(tasks) != 4 {
		t.Fatalf("len(tasks) = %d, want 4", len(tasks))
	}

	// Should be sorted by UpdatedAt descending.
	for i := 1; i < len(tasks); i++ {
		if tasks[i].UpdatedAt.After(tasks[i-1].UpdatedAt) {
			t.Errorf("tasks[%d].UpdatedAt (%v) > tasks[%d].UpdatedAt (%v)", i, tasks[i].UpdatedAt, i-1, tasks[i-1].UpdatedAt)
		}
	}

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error for %s: %v", r.ProjectName, r.Err)
		}
	}
}

func TestService_List_FilterByProject(t *testing.T) {
	tracker := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		tasks:     []forge.BugTask{{BugID: "1", Title: "Bug 1"}},
	}

	svc := NewService(
		map[string]ProjectBugTracker{
			"launchpad:snap-openstack": {Tracker: tracker, ProjectID: "snap-openstack"},
		},
		map[string][]ProjectBinding{
			"launchpad:snap-openstack": {
				{ProjectName: "sunbeam"},
				{ProjectName: "microstack"},
			},
		},
		nil)

	tasks, _, _ := svc.List(context.Background(), ListOptions{Projects: []string{"sunbeam"}})
	if len(tasks) != 1 {
		t.Errorf("expected 1 task for sunbeam, got %d", len(tasks))
	}
	if len(tasks) > 0 && tasks[0].Project != "sunbeam" {
		t.Errorf("expected project=sunbeam, got %s", tasks[0].Project)
	}
}

func TestService_List_LimitAppliesAfterSorting(t *testing.T) {
	now := time.Now()
	tracker := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		tasks: []forge.BugTask{
			{BugID: "1", Title: "Oldest", UpdatedAt: now.Add(-3 * time.Hour)},
			{BugID: "2", Title: "Newest", UpdatedAt: now},
			{BugID: "3", Title: "Middle", UpdatedAt: now.Add(-1 * time.Hour)},
		},
	}

	svc := NewService(
		map[string]ProjectBugTracker{
			"launchpad:snap-openstack": {Tracker: tracker, ProjectID: "snap-openstack"},
		},
		map[string][]ProjectBinding{
			"launchpad:snap-openstack": {{ProjectName: "sunbeam"}},
		},
		nil)

	tasks, _, err := svc.List(context.Background(), ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(tasks))
	}
	if tasks[0].BugID != "2" || tasks[1].BugID != "3" {
		t.Fatalf("task order = %s, %s; want newest two bug IDs 2, 3", tasks[0].BugID, tasks[1].BugID)
	}
}

func TestService_List_PassesSinceToCreatedAndModifiedFilters(t *testing.T) {
	tracker := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		tasks:     []forge.BugTask{{BugID: "1", Title: "Bug 1"}},
	}

	svc := NewService(
		map[string]ProjectBugTracker{
			"launchpad:snap-openstack": {Tracker: tracker, ProjectID: "snap-openstack"},
		},
		map[string][]ProjectBinding{
			"launchpad:snap-openstack": {
				{ProjectName: "sunbeam"},
			},
		},
		nil)

	_, _, err := svc.List(context.Background(), ListOptions{Since: "2026-03-08T00:00:00Z"})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if tracker.lastOpts.CreatedSince != "2026-03-08T00:00:00Z" || tracker.lastOpts.ModifiedSince != "2026-03-08T00:00:00Z" {
		t.Fatalf("tracker opts = %+v, want both created/modified since set", tracker.lastOpts)
	}
}

func TestService_List_Deduplication(t *testing.T) {
	callCount := 0
	tracker := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		tasks:     []forge.BugTask{{BugID: "1", Title: "Bug"}},
	}
	// Wrap to count calls
	countingTracker := &countingBugTracker{inner: tracker, count: &callCount}

	svc := NewService(
		map[string]ProjectBugTracker{
			"launchpad:snap-openstack": {Tracker: countingTracker, ProjectID: "snap-openstack"},
		},
		map[string][]ProjectBinding{
			"launchpad:snap-openstack": {
				{ProjectName: "sunbeam"},
				{ProjectName: "microstack"},
			},
		},
		nil)

	_, _, _ = svc.List(context.Background(), ListOptions{})
	if callCount != 1 {
		t.Errorf("expected 1 API call (deduplication), got %d", callCount)
	}
}

func TestService_List_GracefulDegradation(t *testing.T) {
	goodTracker := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		tasks:     []forge.BugTask{{BugID: "1", Title: "Good Bug"}},
	}
	badTracker := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		err:       fmt.Errorf("connection refused"),
	}

	svc := NewService(
		map[string]ProjectBugTracker{
			"launchpad:good-project": {Tracker: goodTracker, ProjectID: "good-project"},
			"launchpad:bad-project":  {Tracker: badTracker, ProjectID: "bad-project"},
		},
		map[string][]ProjectBinding{
			"launchpad:good-project": {{ProjectName: "good"}},
			"launchpad:bad-project":  {{ProjectName: "bad"}},
		},
		nil)

	tasks, results, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("List() should not return top-level error: %v", err)
	}

	if len(tasks) != 1 {
		t.Errorf("expected 1 task from good project, got %d", len(tasks))
	}

	var hadError bool
	for _, r := range results {
		if r.Err != nil {
			hadError = true
		}
	}
	if !hadError {
		t.Error("expected at least one project result with error")
	}
}

func TestService_List_MergeCollapsesGroupedBugRows(t *testing.T) {
	now := time.Now()
	commonTracker := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		tasks: []forge.BugTask{
			{Forge: forge.ForgeLaunchpad, BugID: "42", Title: "Bug", TargetName: "snap-openstack", UpdatedAt: now.Add(-1 * time.Hour), Status: "Fix Released"},
		},
	}
	projectTracker := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		tasks: []forge.BugTask{
			{Forge: forge.ForgeLaunchpad, BugID: "42", Title: "Bug", TargetName: "sunbeam-charms", UpdatedAt: now, Status: "Fix Committed"},
		},
	}

	svc := NewService(
		map[string]ProjectBugTracker{
			"launchpad:snap-openstack": {Tracker: commonTracker, ProjectID: "snap-openstack"},
			"launchpad:sunbeam-charms": {Tracker: projectTracker, ProjectID: "sunbeam-charms"},
		},
		map[string][]ProjectBinding{
			"launchpad:snap-openstack": {
				{ProjectName: "sunbeam-charms", Group: "sunbeam", CommonProject: "snap-openstack"},
				{ProjectName: "openstack", Group: "sunbeam", CommonProject: "snap-openstack"},
			},
			"launchpad:sunbeam-charms": {
				{ProjectName: "sunbeam-charms", Group: "sunbeam", CommonProject: "snap-openstack"},
			},
		},
		nil,
	)

	tasks, _, err := svc.List(context.Background(), ListOptions{Merge: true})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}
	if tasks[0].Project != "snap-openstack" {
		t.Fatalf("tasks[0].Project = %q, want snap-openstack", tasks[0].Project)
	}
	if tasks[0].TargetName != "snap-openstack" {
		t.Fatalf("tasks[0].TargetName = %q, want snap-openstack representative", tasks[0].TargetName)
	}
}

func TestService_List_MergeKeepsSameBugSeparateAcrossGroups(t *testing.T) {
	now := time.Now()
	trackerA := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		tasks:     []forge.BugTask{{Forge: forge.ForgeLaunchpad, BugID: "42", Title: "Bug", TargetName: "snap-openstack", UpdatedAt: now}},
	}
	trackerB := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		tasks:     []forge.BugTask{{Forge: forge.ForgeLaunchpad, BugID: "42", Title: "Bug", TargetName: "other-project", UpdatedAt: now}},
	}

	svc := NewService(
		map[string]ProjectBugTracker{
			"launchpad:snap-openstack": {Tracker: trackerA, ProjectID: "snap-openstack"},
			"launchpad:other-project":  {Tracker: trackerB, ProjectID: "other-project"},
		},
		map[string][]ProjectBinding{
			"launchpad:snap-openstack": {{ProjectName: "sunbeam-charms", Group: "sunbeam", CommonProject: "snap-openstack"}},
			"launchpad:other-project":  {{ProjectName: "other", Group: "other-group", CommonProject: "other-project"}},
		},
		nil,
	)

	tasks, _, err := svc.List(context.Background(), ListOptions{Merge: true})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(tasks))
	}
}

// countingBugTracker wraps a BugTracker and counts ListBugTasks calls.
type countingBugTracker struct {
	inner forge.BugTracker
	count *int
}

func (c *countingBugTracker) Type() forge.ForgeType { return c.inner.Type() }

func (c *countingBugTracker) GetBug(ctx context.Context, id string) (*forge.Bug, error) {
	return c.inner.GetBug(ctx, id)
}

func (c *countingBugTracker) ListBugTasks(ctx context.Context, project string, opts forge.ListBugTasksOpts) ([]forge.BugTask, error) {
	*c.count++
	return c.inner.ListBugTasks(ctx, project, opts)
}

func (c *countingBugTracker) UpdateBugTaskStatus(ctx context.Context, selfLink, status string) error {
	return c.inner.UpdateBugTaskStatus(ctx, selfLink, status)
}
func (c *countingBugTracker) AddBugTask(ctx context.Context, bugID int, seriesSelfLink string) error {
	return c.inner.AddBugTask(ctx, bugID, seriesSelfLink)
}
func (c *countingBugTracker) GetProjectSeries(ctx context.Context, projectName string) ([]forge.ProjectSeries, error) {
	return c.inner.GetProjectSeries(ctx, projectName)
}
func (c *countingBugTracker) GetProject(ctx context.Context, projectName string) (*forge.Project, error) {
	return c.inner.GetProject(ctx, projectName)
}

func TestService_Get(t *testing.T) {
	tracker := &mockBugTracker{
		forgeType: forge.ForgeLaunchpad,
		bug: &forge.Bug{
			ID:    "12345",
			Title: "Test bug",
			Tags:  []string{"sunbeam"},
			Tasks: []forge.BugTask{
				{BugID: "12345", Status: "New", Importance: "High"},
				{BugID: "12345", Status: "Confirmed", Importance: "Medium"},
			},
		},
	}

	svc := NewService(
		map[string]ProjectBugTracker{
			"launchpad:snap-openstack": {Tracker: tracker, ProjectID: "snap-openstack"},
		},
		map[string][]ProjectBinding{
			"launchpad:snap-openstack": {{ProjectName: "sunbeam"}},
		},
		nil)

	bug, err := svc.Get(context.Background(), "12345")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if bug.ID != "12345" {
		t.Errorf("ID = %q, want 12345", bug.ID)
	}
	if len(bug.Tasks) != 2 {
		t.Errorf("len(Tasks) = %d, want 2", len(bug.Tasks))
	}
}

func TestService_Get_NotConfigured(t *testing.T) {
	svc := NewService(
		map[string]ProjectBugTracker{},
		map[string][]ProjectBinding{},
		nil,
	)

	_, err := svc.Get(context.Background(), "12345")
	if err == nil {
		t.Fatal("expected error when no trackers configured")
	}
}
