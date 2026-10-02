// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package commit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// mockCommitSource implements CommitSource for testing.
type mockCommitSource struct {
	forgeType forge.ForgeType
	commits   []forge.Commit
	mrCommits []forge.Commit
	err       error
}

func (m *mockCommitSource) ForgeType() forge.ForgeType { return m.forgeType }

func (m *mockCommitSource) ListCommits(_ context.Context, opts forge.ListCommitsOpts) ([]forge.Commit, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []forge.Commit
	for _, c := range m.commits {
		if opts.Author != "" && c.Author != opts.Author {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}

func (m *mockCommitSource) ListMRCommits(_ context.Context) ([]forge.Commit, error) {
	return m.mrCommits, nil
}

func (m *mockCommitSource) ListBranches(_ context.Context) ([]string, error) {
	return []string{"main"}, nil
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

func TestService_List_Aggregation(t *testing.T) {
	now := time.Now()

	svc := NewService(toSources(map[string]testProjectSource{
		"gh-project": {
			Source: &mockCommitSource{
				commits: []forge.Commit{
					{SHA: "aaa", Author: "alice", Date: now.Add(-1 * time.Hour), Message: "fix: thing"},
					{SHA: "bbb", Author: "bob", Date: now.Add(-3 * time.Hour), Message: "feat: stuff"},
				},
			},
			ForgeType: forge.ForgeGitHub,
		},
		"gerrit-project": {
			Source: &mockCommitSource{
				commits: []forge.Commit{
					{SHA: "ccc", Author: "carol", Date: now.Add(-2 * time.Hour), Message: "refactor: code"},
				},
			},
			ForgeType: forge.ForgeGerrit,
		},
	}), nil)

	commits, results, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(commits) != 3 {
		t.Fatalf("len(commits) = %d, want 3", len(commits))
	}

	// Should be sorted by Date descending.
	for i := 1; i < len(commits); i++ {
		if commits[i].Date.After(commits[i-1].Date) {
			t.Errorf("commits[%d].Date (%v) > commits[%d].Date (%v)", i, commits[i].Date, i-1, commits[i-1].Date)
		}
	}

	// Repo field should be set to project name.
	// Branch commits should be annotated as Merged.
	for _, c := range commits {
		if c.Repo == "" {
			t.Errorf("commit %s has empty Repo", c.SHA)
		}
		if c.MergeRequest == nil {
			t.Errorf("commit %s should have MergeRequest annotation", c.SHA)
		} else if c.MergeRequest.State != forge.MergeStateMerged {
			t.Errorf("commit %s MergeRequest state = %v, want Merged", c.SHA, c.MergeRequest.State)
		}
	}

	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error for %s: %v", r.ProjectName, r.Err)
		}
	}
}

func TestService_List_FilterByProject(t *testing.T) {
	svc := NewService(toSources(map[string]testProjectSource{
		"gh-project": {
			Source:    &mockCommitSource{commits: []forge.Commit{{SHA: "aaa", Message: "GH commit"}}},
			ForgeType: forge.ForgeGitHub,
		},
		"gerrit-project": {
			Source:    &mockCommitSource{commits: []forge.Commit{{SHA: "bbb", Message: "Gerrit commit"}}},
			ForgeType: forge.ForgeGerrit,
		},
	}), nil)

	commits, _, _ := svc.List(context.Background(), ListOptions{Projects: []string{"gh-project"}})
	if len(commits) != 1 || commits[0].SHA != "aaa" {
		t.Errorf("expected only GH commit, got %v", commits)
	}
}

func TestService_List_FilterByForge(t *testing.T) {
	svc := NewService(toSources(map[string]testProjectSource{
		"gh-project": {
			Source:    &mockCommitSource{commits: []forge.Commit{{SHA: "aaa"}}},
			ForgeType: forge.ForgeGitHub,
		},
		"gerrit-project": {
			Source:    &mockCommitSource{commits: []forge.Commit{{SHA: "bbb"}}},
			ForgeType: forge.ForgeGerrit,
		},
	}), nil)

	commits, _, _ := svc.List(context.Background(), ListOptions{
		Forges: []forge.ForgeType{forge.ForgeGerrit},
	})
	if len(commits) != 1 || commits[0].SHA != "bbb" {
		t.Errorf("expected only Gerrit commit, got %v", commits)
	}
}

func TestService_List_FilterByAuthor(t *testing.T) {
	svc := NewService(toSources(map[string]testProjectSource{
		"project": {
			Source: &mockCommitSource{
				commits: []forge.Commit{
					{SHA: "aaa", Author: "alice"},
					{SHA: "bbb", Author: "bob"},
				},
			},
			ForgeType: forge.ForgeGitHub,
		},
	}), nil)

	commits, _, _ := svc.List(context.Background(), ListOptions{Author: "alice"})
	if len(commits) != 1 || commits[0].Author != "alice" {
		t.Errorf("expected only alice's commit, got %v", commits)
	}
}

func TestService_List_FilterByBugID(t *testing.T) {
	svc := NewService(toSources(map[string]testProjectSource{
		"project": {
			Source: &mockCommitSource{
				commits: []forge.Commit{
					{SHA: "aaa", Message: "fix: LP: #12345", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}}},
					{SHA: "bbb", Message: "feat: new thing", BugRefs: nil},
					{SHA: "ccc", Message: "fix: LP: #12345 and LP: #99999", BugRefs: []forge.BugRef{{ID: "12345", Type: forge.BugRefCloses}, {ID: "99999", Type: forge.BugRefCloses}}},
				},
			},
			ForgeType: forge.ForgeGitHub,
		},
	}), nil)

	commits, _, _ := svc.List(context.Background(), ListOptions{BugID: "12345"})
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits referencing bug 12345, got %d", len(commits))
	}
	for _, c := range commits {
		if c.SHA != "aaa" && c.SHA != "ccc" {
			t.Errorf("unexpected commit %s", c.SHA)
		}
	}
}

func TestService_List_GracefulDegradation(t *testing.T) {
	svc := NewService(toSources(map[string]testProjectSource{
		"good-project": {
			Source:    &mockCommitSource{commits: []forge.Commit{{SHA: "aaa", Message: "good commit"}}},
			ForgeType: forge.ForgeGitHub,
		},
		"bad-project": {
			Source:    &mockCommitSource{err: fmt.Errorf("connection refused")},
			ForgeType: forge.ForgeGerrit,
		},
	}), nil)

	commits, results, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("List() should not return top-level error: %v", err)
	}

	if len(commits) != 1 {
		t.Errorf("expected 1 commit from good project, got %d", len(commits))
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

func TestService_List_Empty(t *testing.T) {
	svc := NewService(toSources(map[string]testProjectSource{}), nil)

	commits, results, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("expected 0 commits, got %d", len(commits))
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestService_List_BugIDNoMatch(t *testing.T) {
	svc := NewService(toSources(map[string]testProjectSource{
		"project": {
			Source: &mockCommitSource{
				commits: []forge.Commit{
					{SHA: "aaa", Message: "unrelated fix", BugRefs: []forge.BugRef{{ID: "99999", Type: forge.BugRefCloses}}},
				},
			},
			ForgeType: forge.ForgeGitHub,
		},
	}), nil)

	commits, _, _ := svc.List(context.Background(), ListOptions{BugID: "12345"})
	if len(commits) != 0 {
		t.Errorf("expected 0 commits, got %d", len(commits))
	}
}

func TestNewServiceFromForges(t *testing.T) {
	mock := &mockForge{
		forgeType: forge.ForgeGitHub,
		commits:   []forge.Commit{{SHA: "aaa", Message: "test"}},
	}

	svc := NewServiceFromForges(map[string]ProjectForge{
		"project": {Forge: mock, ProjectID: "org/repo"},
	}, nil)

	commits, _, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
}

func TestService_List_IncludeMRs(t *testing.T) {
	now := time.Now()
	svc := NewService(toSources(map[string]testProjectSource{
		"project": {
			Source: &mockCommitSource{
				commits: []forge.Commit{
					{SHA: "aaa", Author: "alice", Date: now.Add(-1 * time.Hour), Message: "fix: thing"},
				},
				mrCommits: []forge.Commit{
					{SHA: "bbb", Author: "bob", Date: now.Add(-30 * time.Minute), Message: "feat: new thing",
						MergeRequest: &forge.CommitMergeRequest{ID: "#42", State: forge.MergeStateOpen, URL: "https://example.com/pr/42"}},
				},
			},
			ForgeType: forge.ForgeGitHub,
		},
	}), nil)

	commits, _, err := svc.List(context.Background(), ListOptions{IncludeMRs: true})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits (1 branch + 1 MR), got %d", len(commits))
	}

	// Verify branch commit is Merged, MR commit has Open annotation.
	for _, c := range commits {
		if c.MergeRequest == nil {
			t.Errorf("commit %s should have MergeRequest annotation", c.SHA)
			continue
		}
		switch c.SHA {
		case "aaa":
			if c.MergeRequest.State != forge.MergeStateMerged {
				t.Errorf("branch commit state = %v, want Merged", c.MergeRequest.State)
			}
		case "bbb":
			if c.MergeRequest.ID != "#42" {
				t.Errorf("MR commit ID = %q, want %q", c.MergeRequest.ID, "#42")
			}
			if c.MergeRequest.State != forge.MergeStateOpen {
				t.Errorf("MR commit state = %v, want Open", c.MergeRequest.State)
			}
		}
	}
}

func TestService_List_IncludeMRs_Dedup(t *testing.T) {
	now := time.Now()
	svc := NewService(toSources(map[string]testProjectSource{
		"project": {
			Source: &mockCommitSource{
				commits: []forge.Commit{
					{SHA: "aaa", Author: "alice", Date: now.Add(-1 * time.Hour), Message: "fix: thing"},
				},
				mrCommits: []forge.Commit{
					// Same SHA as branch commit — should be annotated as Merged, not duplicated.
					{SHA: "aaa", Author: "alice", Date: now.Add(-1 * time.Hour), Message: "fix: thing",
						MergeRequest: &forge.CommitMergeRequest{ID: "#42", State: forge.MergeStateOpen, URL: "https://example.com/pr/42"}},
					// Different SHA — should be included as-is.
					{SHA: "bbb", Author: "bob", Date: now.Add(-30 * time.Minute), Message: "feat: new",
						MergeRequest: &forge.CommitMergeRequest{ID: "#43", State: forge.MergeStateOpen, URL: "https://example.com/pr/43"}},
				},
			},
			ForgeType: forge.ForgeGitHub,
		},
	}), nil)

	commits, _, err := svc.List(context.Background(), ListOptions{IncludeMRs: true})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits (branch annotated + new MR), got %d", len(commits))
	}

	// The branch commit (aaa) should be annotated as Merged with the MR link.
	for _, c := range commits {
		if c.SHA == "aaa" {
			if c.MergeRequest == nil {
				t.Fatal("branch commit 'aaa' should have MergeRequest annotation")
			}
			if c.MergeRequest.State != forge.MergeStateMerged {
				t.Errorf("branch commit 'aaa' MR state = %v, want Merged", c.MergeRequest.State)
			}
			if c.MergeRequest.ID != "#42" {
				t.Errorf("branch commit 'aaa' MR ID = %q, want %q", c.MergeRequest.ID, "#42")
			}
			if c.MergeRequest.URL != "https://example.com/pr/42" {
				t.Errorf("branch commit 'aaa' MR URL = %q, want %q", c.MergeRequest.URL, "https://example.com/pr/42")
			}
		}
	}
}

func TestService_List_IncludeMRs_Disabled(t *testing.T) {
	svc := NewService(toSources(map[string]testProjectSource{
		"project": {
			Source: &mockCommitSource{
				commits: []forge.Commit{
					{SHA: "aaa", Message: "fix: thing"},
				},
				mrCommits: []forge.Commit{
					{SHA: "bbb", Message: "feat: new",
						MergeRequest: &forge.CommitMergeRequest{ID: "#42", State: forge.MergeStateOpen}},
				},
			},
			ForgeType: forge.ForgeGitHub,
		},
	}), nil)

	// Without IncludeMRs, MR commits should not appear.
	commits, _, _ := svc.List(context.Background(), ListOptions{})
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit (MRs disabled), got %d", len(commits))
	}
}

// mockForge implements the Forge-like interface for ProjectForge backward compat testing.
type mockForge struct {
	forgeType forge.ForgeType
	commits   []forge.Commit
	err       error
}

func (m *mockForge) Type() forge.ForgeType { return m.forgeType }

func (m *mockForge) ListCommits(_ context.Context, _ string, opts forge.ListCommitsOpts) ([]forge.Commit, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []forge.Commit
	for _, c := range m.commits {
		if opts.Author != "" && c.Author != opts.Author {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}
