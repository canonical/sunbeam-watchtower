package review

import (
	"context"
	"fmt"
	"testing"
	"time"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// mockForge implements forge.Forge for testing.
type mockForge struct {
	forgeType forge.ForgeType
	mrs       []forge.MergeRequest
	err       error
}

func (m *mockForge) Type() forge.ForgeType { return m.forgeType }

func (m *mockForge) ListMergeRequests(_ context.Context, _ string, opts forge.ListMergeRequestsOpts) ([]forge.MergeRequest, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []forge.MergeRequest
	for _, mr := range m.mrs {
		if opts.Author != "" && mr.Author != opts.Author {
			continue
		}
		result = append(result, mr)
	}
	return result, nil
}

func (m *mockForge) GetMergeRequest(_ context.Context, _ string, id string) (*forge.MergeRequest, error) {
	for _, mr := range m.mrs {
		if mr.ID == id {
			return &mr, nil
		}
	}
	return nil, fmt.Errorf("not found: %s", id)
}

func (m *mockForge) ListCommits(_ context.Context, _ string, _ forge.ListCommitsOpts) ([]forge.Commit, error) {
	return nil, nil
}

func TestService_List_Aggregation(t *testing.T) {
	now := time.Now()

	ghForge := &mockForge{
		forgeType: forge.ForgeGitHub,
		mrs: []forge.MergeRequest{
			{ID: "#1", Title: "GH PR 1", Author: "alice", UpdatedAt: now.Add(-1 * time.Hour)},
			{ID: "#2", Title: "GH PR 2", Author: "bob", UpdatedAt: now.Add(-3 * time.Hour)},
		},
	}
	gerritForge := &mockForge{
		forgeType: forge.ForgeGerrit,
		mrs: []forge.MergeRequest{
			{ID: "100", Title: "Gerrit change", Author: "carol", UpdatedAt: now.Add(-2 * time.Hour)},
		},
	}

	svc := NewService(map[string]ProjectForge{
		"my-gh-project":     {Forge: ghForge, ProjectID: "org/repo"},
		"my-gerrit-project": {Forge: gerritForge, ProjectID: "openstack/nova"},
	}, nil)

	mrs, results, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(mrs) != 3 {
		t.Fatalf("len(mrs) = %d, want 3", len(mrs))
	}

	// Should be sorted by UpdatedAt descending.
	for i := 1; i < len(mrs); i++ {
		if mrs[i].UpdatedAt.After(mrs[i-1].UpdatedAt) {
			t.Errorf("mrs[%d].UpdatedAt (%v) > mrs[%d].UpdatedAt (%v)", i, mrs[i].UpdatedAt, i-1, mrs[i-1].UpdatedAt)
		}
	}

	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error for %s: %v", r.ProjectName, r.Err)
		}
	}
}

func TestService_List_FilterByProject(t *testing.T) {
	ghForge := &mockForge{
		forgeType: forge.ForgeGitHub,
		mrs:       []forge.MergeRequest{{ID: "#1", Title: "PR"}},
	}
	gerritForge := &mockForge{
		forgeType: forge.ForgeGerrit,
		mrs:       []forge.MergeRequest{{ID: "100", Title: "Change"}},
	}

	svc := NewService(map[string]ProjectForge{
		"gh-project":     {Forge: ghForge, ProjectID: "org/repo"},
		"gerrit-project": {Forge: gerritForge, ProjectID: "openstack/nova"},
	}, nil)

	mrs, _, _ := svc.List(context.Background(), ListOptions{Projects: []string{"gh-project"}})
	if len(mrs) != 1 || mrs[0].ID != "#1" {
		t.Errorf("expected only GH PR, got %v", mrs)
	}
}

func TestService_List_FilterByForge(t *testing.T) {
	ghForge := &mockForge{
		forgeType: forge.ForgeGitHub,
		mrs:       []forge.MergeRequest{{ID: "#1", Title: "PR"}},
	}
	gerritForge := &mockForge{
		forgeType: forge.ForgeGerrit,
		mrs:       []forge.MergeRequest{{ID: "100", Title: "Change"}},
	}

	svc := NewService(map[string]ProjectForge{
		"gh-project":     {Forge: ghForge, ProjectID: "org/repo"},
		"gerrit-project": {Forge: gerritForge, ProjectID: "openstack/nova"},
	}, nil)

	mrs, _, _ := svc.List(context.Background(), ListOptions{Forges: []forge.ForgeType{forge.ForgeGerrit}})
	if len(mrs) != 1 || mrs[0].ID != "100" {
		t.Errorf("expected only Gerrit change, got %v", mrs)
	}
}

func TestService_List_FilterByAuthor(t *testing.T) {
	f := &mockForge{
		forgeType: forge.ForgeGitHub,
		mrs: []forge.MergeRequest{
			{ID: "#1", Author: "alice"},
			{ID: "#2", Author: "bob"},
		},
	}

	svc := NewService(map[string]ProjectForge{
		"project": {Forge: f, ProjectID: "org/repo"},
	}, nil)

	mrs, _, _ := svc.List(context.Background(), ListOptions{Author: "alice"})
	if len(mrs) != 1 || mrs[0].Author != "alice" {
		t.Errorf("expected only alice's PR, got %v", mrs)
	}
}

func TestService_List_GracefulDegradation(t *testing.T) {
	goodForge := &mockForge{
		forgeType: forge.ForgeGitHub,
		mrs:       []forge.MergeRequest{{ID: "#1", Title: "Good PR"}},
	}
	badForge := &mockForge{
		forgeType: forge.ForgeGerrit,
		err:       fmt.Errorf("connection refused"),
	}

	svc := NewService(map[string]ProjectForge{
		"good-project": {Forge: goodForge, ProjectID: "org/repo"},
		"bad-project":  {Forge: badForge, ProjectID: "openstack/nova"},
	}, nil)

	mrs, results, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("List() should not return top-level error: %v", err)
	}

	if len(mrs) != 1 {
		t.Errorf("expected 1 MR from good project, got %d", len(mrs))
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

func TestService_Get(t *testing.T) {
	f := &mockForge{
		forgeType: forge.ForgeGitHub,
		mrs: []forge.MergeRequest{
			{ID: "#1", Title: "Fix the thing", Author: "alice"},
			{ID: "#2", Title: "Add feature", Author: "bob"},
		},
	}

	svc := NewService(map[string]ProjectForge{
		"my-project": {Forge: f, ProjectID: "org/repo"},
	}, nil)

	mr, err := svc.Get(context.Background(), "my-project", "#1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if mr.Title != "Fix the thing" {
		t.Errorf("Title = %q, want 'Fix the thing'", mr.Title)
	}
	if mr.Repo != "my-project" {
		t.Errorf("Repo = %q, want 'my-project'", mr.Repo)
	}
}

func TestService_Get_UnknownProject(t *testing.T) {
	svc := NewService(map[string]ProjectForge{}, nil)

	_, err := svc.Get(context.Background(), "nonexistent", "#1")
	if err == nil {
		t.Fatal("expected error for unknown project")
	}
}

func TestService_Get_NotFound(t *testing.T) {
	f := &mockForge{
		forgeType: forge.ForgeGitHub,
		mrs:       []forge.MergeRequest{{ID: "#1", Title: "PR"}},
	}

	svc := NewService(map[string]ProjectForge{
		"my-project": {Forge: f, ProjectID: "org/repo"},
	}, nil)

	_, err := svc.Get(context.Background(), "my-project", "#999")
	if err == nil {
		t.Fatal("expected error for not found MR")
	}
}
