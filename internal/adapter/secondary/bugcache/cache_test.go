// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package bugcache_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/bugcache"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func newTestCache(t *testing.T) *bugcache.Cache {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bugs")
	c, err := bugcache.NewCache(dir, nil)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestStoreBugsAndGetBug(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	bugs := []*forge.Bug{
		{
			Forge:       forge.ForgeLaunchpad,
			ID:          "100",
			Title:       "Bug 100",
			Description: "First bug",
			Owner:       "alice",
			Tags:        []string{"ods", "ceph"},
			URL:         "https://bugs.lp.net/100",
			CreatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Comments: []forge.BugComment{{
				Author:    "carol",
				Body:      "Confirmed with the attached reproducer.",
				CreatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			}},
		},
		{
			Forge:       forge.ForgeLaunchpad,
			ID:          "200",
			Title:       "Bug 200",
			Description: "Second bug",
			Owner:       "bob",
			URL:         "https://bugs.lp.net/200",
			CreatedAt:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC),
		},
	}

	if err := c.StoreBugs(ctx, bugs); err != nil {
		t.Fatalf("StoreBugs: %v", err)
	}

	// Retrieve first bug and verify fields.
	got, err := c.GetBug(ctx, forge.ForgeLaunchpad, "100")
	if err != nil {
		t.Fatalf("GetBug(100): %v", err)
	}
	if got.ID != "100" || got.Title != "Bug 100" || got.Description != "First bug" || got.Owner != "alice" || got.URL != "https://bugs.lp.net/100" {
		t.Errorf("bug 100 fields mismatch: %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "ods" || got.Tags[1] != "ceph" {
		t.Errorf("bug 100 tags mismatch: %v", got.Tags)
	}
	if got.Forge != forge.ForgeLaunchpad {
		t.Errorf("bug 100 forge: got %v, want Launchpad", got.Forge)
	}
	if len(got.Comments) != 1 || got.Comments[0].Author != "carol" || got.Comments[0].Body != "Confirmed with the attached reproducer." {
		t.Errorf("bug 100 comments mismatch: %+v", got.Comments)
	}

	// Retrieve second bug.
	got2, err := c.GetBug(ctx, forge.ForgeLaunchpad, "200")
	if err != nil {
		t.Fatalf("GetBug(200): %v", err)
	}
	if got2.ID != "200" || got2.Title != "Bug 200" {
		t.Errorf("bug 200 fields mismatch: %+v", got2)
	}

	// Non-existent bug should return error.
	_, err = c.GetBug(ctx, forge.ForgeLaunchpad, "999")
	if err == nil {
		t.Error("GetBug(999) should return error for non-existent bug")
	}
}

func TestSnapshotReturnsCompleteDocumentsAndProvenance(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	syncedAt := time.Date(2026, 7, 23, 9, 30, 0, 0, time.UTC)
	bug := &forge.Bug{
		Forge:           forge.ForgeLaunchpad,
		ID:              "100",
		Title:           "Snapshot race",
		Description:     "Complete cached description",
		VisibilityKnown: true,
	}
	task := forge.BugTask{
		Forge:           forge.ForgeLaunchpad,
		BugID:           "100",
		TargetName:      "cinder",
		VisibilityKnown: true,
	}
	if err := c.ReplaceProject(
		ctx,
		forge.ForgeLaunchpad,
		"cinder",
		[]*forge.Bug{bug, {
			Forge: forge.ForgeLaunchpad, ID: "orphan",
			Title:           "Stale bug without a current project task",
			VisibilityKnown: true,
		}},
		[]forge.BugTask{task},
		syncedAt,
		bugcache.CurrentSchemaVersion,
	); err != nil {
		t.Fatal(err)
	}

	snapshot, err := c.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Documents) != 1 || snapshot.Documents[0].Bug.Description != bug.Description {
		t.Fatalf("documents = %+v", snapshot.Documents)
	}
	if len(snapshot.Documents[0].Tasks) != 1 ||
		snapshot.Documents[0].Tasks[0].TrackerProject != "cinder" {
		t.Fatalf("scoped tasks = %+v", snapshot.Documents[0].Tasks)
	}
	if len(snapshot.Status) != 1 || !snapshot.Status[0].LastSync.Equal(syncedAt) {
		t.Fatalf("status = %+v", snapshot.Status)
	}
	if snapshot.Status[0].NeedsRefresh ||
		snapshot.Status[0].SchemaVersion != bugcache.CurrentSchemaVersion {
		t.Fatalf("cache compatibility = %+v", snapshot.Status[0])
	}
}

func TestLegacyProjectRequiresRefresh(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	if err := c.StoreBugs(ctx, []*forge.Bug{{
		Forge: forge.ForgeLaunchpad, ID: "100", Title: "Legacy bug",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := c.StoreBugTasks(ctx, forge.ForgeLaunchpad, "cinder", []forge.BugTask{{
		Forge: forge.ForgeLaunchpad, BugID: "100", TargetName: "cinder",
	}}); err != nil {
		t.Fatal(err)
	}

	compatible, err := c.ProjectCompatible(
		ctx,
		forge.ForgeLaunchpad,
		"cinder",
		bugcache.CurrentSchemaVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	if compatible {
		t.Fatal("legacy project reported compatible without a schema marker")
	}
	status, err := c.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 1 || !status[0].NeedsRefresh || status[0].SchemaVersion != 0 {
		t.Fatalf("status = %+v, want schema version 0 requiring refresh", status)
	}
}

func TestStoreBugTasksAndList(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	tasks := []forge.BugTask{
		{
			Forge:      forge.ForgeLaunchpad,
			Project:    "neutron",
			BugID:      "100",
			Title:      "Task A",
			Status:     "New",
			Importance: "High",
			Assignee:   "alice",
			TargetName: "neutron",
			SelfLink:   "https://api.lp.net/task/1",
			URL:        "https://bugs.lp.net/task/1",
			CreatedAt:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:  time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			Forge:      forge.ForgeLaunchpad,
			Project:    "neutron",
			BugID:      "200",
			Title:      "Task B",
			Status:     "Fix Committed",
			Importance: "Medium",
			TargetName: "neutron",
			SelfLink:   "https://api.lp.net/task/2",
			URL:        "https://bugs.lp.net/task/2",
			CreatedAt:  time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:  time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC),
		},
	}

	if err := c.StoreBugTasks(ctx, forge.ForgeLaunchpad, "neutron", tasks); err != nil {
		t.Fatalf("StoreBugTasks: %v", err)
	}

	got, err := c.ListBugTasks(ctx, forge.ForgeLaunchpad, "neutron", forge.ListBugTasksOpts{})
	if err != nil {
		t.Fatalf("ListBugTasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}

	// Build a map by BugID for deterministic checking.
	byBugID := map[string]forge.BugTask{}
	for _, task := range got {
		byBugID[task.BugID] = task
	}
	if task, ok := byBugID["100"]; !ok || task.Title != "Task A" || task.Status != "New" || task.Importance != "High" {
		t.Errorf("task for bug 100 mismatch: %+v", task)
	}
	if task, ok := byBugID["200"]; !ok || task.Title != "Task B" || task.Status != "Fix Committed" {
		t.Errorf("task for bug 200 mismatch: %+v", task)
	}

	// Listing an empty project should return empty slice.
	empty, err := c.ListBugTasks(ctx, forge.ForgeLaunchpad, "no-such-project", forge.ListBugTasksOpts{})
	if err != nil {
		t.Fatalf("ListBugTasks(empty): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 tasks for unknown project, got %d", len(empty))
	}
}

func TestListBugTasksWithFilter(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	tasks := []forge.BugTask{
		{Forge: forge.ForgeLaunchpad, BugID: "1", TargetName: "nova", Status: "New", Importance: "High"},
		{Forge: forge.ForgeLaunchpad, BugID: "2", TargetName: "nova", Status: "Fix Committed", Importance: "Low"},
		{Forge: forge.ForgeLaunchpad, BugID: "3", TargetName: "nova", Status: "In Progress", Importance: "Medium"},
	}
	if err := c.StoreBugTasks(ctx, forge.ForgeLaunchpad, "nova", tasks); err != nil {
		t.Fatalf("StoreBugTasks: %v", err)
	}

	got, err := c.ListBugTasks(ctx, forge.ForgeLaunchpad, "nova", forge.ListBugTasksOpts{
		Status: []string{"New"},
	})
	if err != nil {
		t.Fatalf("ListBugTasks with filter: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 task with Status=New, got %d", len(got))
	}
	if got[0].BugID != "1" || got[0].Status != "New" {
		t.Errorf("filtered task mismatch: %+v", got[0])
	}
}

func TestListBugTasksWithTagUsesCachedBugMetadata(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	if err := c.StoreBugs(ctx, []*forge.Bug{
		{Forge: forge.ForgeLaunchpad, ID: "1", Tags: []string{"candidate-blocker"}},
		{Forge: forge.ForgeLaunchpad, ID: "2", Tags: []string{"other"}},
	}); err != nil {
		t.Fatalf("StoreBugs: %v", err)
	}
	if err := c.StoreBugTasks(ctx, forge.ForgeLaunchpad, "snap-openstack", []forge.BugTask{
		{Forge: forge.ForgeLaunchpad, BugID: "1", TargetName: "snap-openstack", Status: "Triaged"},
		{Forge: forge.ForgeLaunchpad, BugID: "2", TargetName: "snap-openstack", Status: "Triaged"},
	}); err != nil {
		t.Fatalf("StoreBugTasks: %v", err)
	}

	got, err := c.ListBugTasks(ctx, forge.ForgeLaunchpad, "snap-openstack", forge.ListBugTasksOpts{
		Tags: []string{"candidate-blocker"},
	})
	if err != nil {
		t.Fatalf("ListBugTasks with tag: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].BugID != "1" {
		t.Fatalf("got bug ID %q, want 1", got[0].BugID)
	}
}

func TestListBugTasksWithSinceUsesCreatedOrModifiedUnion(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	tasks := []forge.BugTask{
		{
			Forge:      forge.ForgeLaunchpad,
			BugID:      "1",
			TargetName: "sunbeam-charms",
			Status:     "Fix Released",
			CreatedAt:  time.Date(2024, 7, 25, 5, 23, 26, 0, time.UTC),
			UpdatedAt:  time.Date(2026, 3, 9, 9, 48, 58, 0, time.UTC),
		},
		{
			Forge:      forge.ForgeLaunchpad,
			BugID:      "2",
			TargetName: "sunbeam-charms",
			Status:     "New",
			CreatedAt:  time.Date(2026, 3, 9, 8, 30, 0, 0, time.UTC),
			UpdatedAt:  time.Date(2026, 3, 9, 8, 30, 0, 0, time.UTC),
		},
		{
			Forge:      forge.ForgeLaunchpad,
			BugID:      "3",
			TargetName: "sunbeam-charms",
			Status:     "Triaged",
			CreatedAt:  time.Date(2026, 3, 7, 8, 0, 0, 0, time.UTC),
			UpdatedAt:  time.Date(2026, 3, 7, 8, 0, 0, 0, time.UTC),
		},
	}
	if err := c.StoreBugTasks(ctx, forge.ForgeLaunchpad, "sunbeam-charms", tasks); err != nil {
		t.Fatalf("StoreBugTasks: %v", err)
	}

	got, err := c.ListBugTasks(ctx, forge.ForgeLaunchpad, "sunbeam-charms", forge.ListBugTasksOpts{
		CreatedSince:  "2026-03-08T00:00:00Z",
		ModifiedSince: "2026-03-08T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("ListBugTasks with since: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
}

func TestLastSyncRoundTrip(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	if err := c.SetLastSync(ctx, forge.ForgeLaunchpad, "keystone", now); err != nil {
		t.Fatalf("SetLastSync: %v", err)
	}

	got, err := c.LastSync(ctx, forge.ForgeLaunchpad, "keystone")
	if err != nil {
		t.Fatalf("LastSync: %v", err)
	}
	if !got.Truncate(time.Second).Equal(now) {
		t.Errorf("LastSync mismatch: got %v, want %v", got, now)
	}

	// Non-existent key should return zero time.
	zero, err := c.LastSync(ctx, forge.ForgeLaunchpad, "nonexistent")
	if err != nil {
		t.Fatalf("LastSync(nonexistent): %v", err)
	}
	if !zero.IsZero() {
		t.Errorf("expected zero time for non-existent key, got %v", zero)
	}
}

func TestRemoveProject(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	tasks := []forge.BugTask{
		{Forge: forge.ForgeLaunchpad, BugID: "10", TargetName: "glance", Status: "New"},
	}
	if err := c.StoreBugTasks(ctx, forge.ForgeLaunchpad, "glance", tasks); err != nil {
		t.Fatalf("StoreBugTasks: %v", err)
	}
	if err := c.SetLastSync(ctx, forge.ForgeLaunchpad, "glance", time.Now()); err != nil {
		t.Fatalf("SetLastSync: %v", err)
	}

	if err := c.Remove(ctx, forge.ForgeLaunchpad, "glance"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	got, err := c.ListBugTasks(ctx, forge.ForgeLaunchpad, "glance", forge.ListBugTasksOpts{})
	if err != nil {
		t.Fatalf("ListBugTasks after remove: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 tasks after remove, got %d", len(got))
	}

	ls, err := c.LastSync(ctx, forge.ForgeLaunchpad, "glance")
	if err != nil {
		t.Fatalf("LastSync after remove: %v", err)
	}
	if !ls.IsZero() {
		t.Errorf("expected zero last sync after remove, got %v", ls)
	}
}

func TestRemoveAll(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	tasks1 := []forge.BugTask{{Forge: forge.ForgeLaunchpad, BugID: "1", TargetName: "cinder", Status: "New"}}
	tasks2 := []forge.BugTask{{Forge: forge.ForgeLaunchpad, BugID: "2", TargetName: "swift", Status: "New"}}
	if err := c.StoreBugTasks(ctx, forge.ForgeLaunchpad, "cinder", tasks1); err != nil {
		t.Fatalf("StoreBugTasks(cinder): %v", err)
	}
	if err := c.StoreBugTasks(ctx, forge.ForgeLaunchpad, "swift", tasks2); err != nil {
		t.Fatalf("StoreBugTasks(swift): %v", err)
	}
	if err := c.SetLastSync(ctx, forge.ForgeLaunchpad, "cinder", time.Now()); err != nil {
		t.Fatalf("SetLastSync(cinder): %v", err)
	}
	if err := c.SetLastSync(ctx, forge.ForgeLaunchpad, "swift", time.Now()); err != nil {
		t.Fatalf("SetLastSync(swift): %v", err)
	}

	if err := c.RemoveAll(ctx); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	for _, proj := range []string{"cinder", "swift"} {
		got, err := c.ListBugTasks(ctx, forge.ForgeLaunchpad, proj, forge.ListBugTasksOpts{})
		if err != nil {
			t.Fatalf("ListBugTasks(%s) after RemoveAll: %v", proj, err)
		}
		if len(got) != 0 {
			t.Errorf("expected 0 tasks for %s after RemoveAll, got %d", proj, len(got))
		}
		ls, err := c.LastSync(ctx, forge.ForgeLaunchpad, proj)
		if err != nil {
			t.Fatalf("LastSync(%s) after RemoveAll: %v", proj, err)
		}
		if !ls.IsZero() {
			t.Errorf("expected zero last sync for %s after RemoveAll, got %v", proj, ls)
		}
	}
}

func TestStatus(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	// Store bugs.
	bugs := []*forge.Bug{
		{Forge: forge.ForgeLaunchpad, ID: "10", Title: "B10"},
		{Forge: forge.ForgeLaunchpad, ID: "20", Title: "B20"},
	}
	if err := c.StoreBugs(ctx, bugs); err != nil {
		t.Fatalf("StoreBugs: %v", err)
	}

	// Store tasks for two projects. Bug 10 has a task in each project.
	tasks1 := []forge.BugTask{
		{Forge: forge.ForgeLaunchpad, BugID: "10", TargetName: "heat", Status: "New"},
		{Forge: forge.ForgeLaunchpad, BugID: "20", TargetName: "heat", Status: "New"},
	}
	tasks2 := []forge.BugTask{
		{Forge: forge.ForgeLaunchpad, BugID: "10", TargetName: "horizon", Status: "New"},
	}
	if err := c.StoreBugTasks(ctx, forge.ForgeLaunchpad, "heat", tasks1); err != nil {
		t.Fatalf("StoreBugTasks(heat): %v", err)
	}
	if err := c.StoreBugTasks(ctx, forge.ForgeLaunchpad, "horizon", tasks2); err != nil {
		t.Fatalf("StoreBugTasks(horizon): %v", err)
	}

	syncTime := time.Now().UTC().Truncate(time.Second)
	if err := c.SetLastSync(ctx, forge.ForgeLaunchpad, "heat", syncTime); err != nil {
		t.Fatalf("SetLastSync(heat): %v", err)
	}
	if err := c.SetLastSync(ctx, forge.ForgeLaunchpad, "horizon", syncTime); err != nil {
		t.Fatalf("SetLastSync(horizon): %v", err)
	}

	statuses, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 status entries, got %d", len(statuses))
	}

	byProject := map[string]struct {
		bugCount  int
		taskCount int
		lastSync  time.Time
	}{}
	for _, s := range statuses {
		byProject[s.Project] = struct {
			bugCount  int
			taskCount int
			lastSync  time.Time
		}{s.BugCount, s.TaskCount, s.LastSync}
	}

	if s, ok := byProject["heat"]; !ok || s.taskCount != 2 || s.bugCount != 2 {
		t.Errorf("heat status mismatch: %+v", byProject["heat"])
	}
	if s, ok := byProject["horizon"]; !ok || s.taskCount != 1 || s.bugCount != 1 {
		t.Errorf("horizon status mismatch: %+v", byProject["horizon"])
	}
	for _, s := range statuses {
		if s.LastSync.Truncate(time.Second) != syncTime {
			t.Errorf("project %s last sync mismatch: got %v, want %v", s.Project, s.LastSync, syncTime)
		}
	}
}

func TestCacheDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mybugs")
	c, err := bugcache.NewCache(dir, nil)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	defer c.Close()

	if c.CacheDir() != dir {
		t.Errorf("CacheDir: got %q, want %q", c.CacheDir(), dir)
	}
}
