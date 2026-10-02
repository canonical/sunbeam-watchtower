// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

func newLaunchpadTestServer(t *testing.T, mux *http.ServeMux) (*LaunchpadForge, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(mux)
	creds := &lp.Credentials{ConsumerKey: "test", AccessToken: "t", AccessTokenSecret: "s"}
	client := lp.NewClient(creds, nil)
	return NewLaunchpadForge(client), server
}

func TestLaunchpadForge_Type(t *testing.T) {
	creds := &lp.Credentials{}
	forge := NewLaunchpadForge(lp.NewClient(creds, nil))
	if forge.Type() != ForgeLaunchpad {
		t.Errorf("Type() = %v, want ForgeLaunchpad", forge.Type())
	}
}

func TestLaunchpadForge_LpMPToMergeRequest(t *testing.T) {
	// Test the conversion logic directly since the LP client prepends APIBaseURL
	// to relative paths, making httptest-based integration tests impractical.
	mrs := []lp.MergeProposal{
		{
			SelfLink:       "https://api.launchpad.net/devel/~alice/sunbeam/+git/sunbeam/+merge/1",
			WebLink:        "https://code.launchpad.net/~alice/sunbeam/+git/sunbeam/+merge/1",
			QueueStatus:    "Needs review",
			Description:    "Fix the widget\nMore details here",
			CommitMessage:  "Fix widget rendering bug",
			RegistrantLink: "https://api.launchpad.net/devel/~alice",
			SourceGitPath:  "refs/heads/fix-widget",
			TargetGitPath:  "refs/heads/main",
			DateCreated:    &lp.Time{},
		},
		{
			SelfLink:       "https://api.launchpad.net/devel/~bob/sunbeam/+git/sunbeam/+merge/2",
			WebLink:        "https://code.launchpad.net/~bob/sunbeam/+git/sunbeam/+merge/2",
			QueueStatus:    "Approved",
			Description:    "Add feature X",
			RegistrantLink: "https://api.launchpad.net/devel/~bob",
			SourceGitPath:  "refs/heads/feature-x",
			TargetGitPath:  "refs/heads/main",
		},
	}

	mr := lpMPToMergeRequest("sunbeam", &mrs[0])
	if mr.Forge != ForgeLaunchpad {
		t.Errorf("Forge = %v", mr.Forge)
	}
	if mr.Author != "alice" {
		t.Errorf("Author = %q, want alice", mr.Author)
	}
	if mr.Title != "Fix widget rendering bug" {
		t.Errorf("Title = %q, want commit message first line", mr.Title)
	}
	if mr.SourceBranch != "refs/heads/fix-widget" {
		t.Errorf("SourceBranch = %q", mr.SourceBranch)
	}
	if mr.State != MergeStateOpen {
		t.Errorf("State = %v, want Open", mr.State)
	}
	if mr.ReviewState != ReviewStatePending {
		t.Errorf("ReviewState = %v, want Pending", mr.ReviewState)
	}

	mr2 := lpMPToMergeRequest("sunbeam", &mrs[1])
	if mr2.Author != "bob" {
		t.Errorf("Author = %q, want bob", mr2.Author)
	}
	if mr2.ReviewState != ReviewStateApproved {
		t.Errorf("ReviewState = %v, want Approved", mr2.ReviewState)
	}
}

func TestLpMPToMergeRequest_States(t *testing.T) {
	tests := []struct {
		status     string
		wantState  MergeState
		wantReview ReviewState
	}{
		{"Work in progress", MergeStateWIP, ReviewStatePending},
		{"Needs review", MergeStateOpen, ReviewStatePending},
		{"Approved", MergeStateOpen, ReviewStateApproved},
		{"Queued", MergeStateOpen, ReviewStateApproved},
		{"Merged", MergeStateMerged, ReviewStateApproved},
		{"Rejected", MergeStateClosed, ReviewStateRejected},
		{"Superseded", MergeStateClosed, ReviewStatePending},
		{"Code failed to merge", MergeStateOpen, ReviewStatePending},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			mp := &lp.MergeProposal{QueueStatus: tt.status}
			mr := lpMPToMergeRequest("test", mp)
			if mr.State != tt.wantState {
				t.Errorf("State = %v, want %v", mr.State, tt.wantState)
			}
			if mr.ReviewState != tt.wantReview {
				t.Errorf("ReviewState = %v, want %v", mr.ReviewState, tt.wantReview)
			}
		})
	}
}

func TestLpMPTitle(t *testing.T) {
	tests := []struct {
		name string
		mp   *lp.MergeProposal
		want string
	}{
		{
			"from commit message",
			&lp.MergeProposal{CommitMessage: "Fix the bug\n\nMore details"},
			"Fix the bug",
		},
		{
			"single line commit message",
			&lp.MergeProposal{CommitMessage: "Single line"},
			"Single line",
		},
		{
			"from description",
			&lp.MergeProposal{Description: "Short description"},
			"Short description",
		},
		{
			"long description truncated",
			&lp.MergeProposal{Description: "This is a very long description that should be truncated at eighty characters because it is way too long for a title"},
			"This is a very long description that should be truncated at eighty characters be...",
		},
		{
			"from source git path",
			&lp.MergeProposal{SourceGitPath: "refs/heads/feature-x"},
			"refs/heads/feature-x",
		},
		{
			"untitled",
			&lp.MergeProposal{},
			"(untitled)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lpMPTitle(tt.mp); got != tt.want {
				t.Errorf("lpMPTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLpExtractName(t *testing.T) {
	tests := []struct {
		link string
		want string
	}{
		{"https://api.launchpad.net/devel/~alice", "alice"},
		{"https://api.launchpad.net/devel/~canonical-bootstack", "canonical-bootstack"},
		{"", ""},
		{"https://api.launchpad.net/devel/something", "something"},
	}

	for _, tt := range tests {
		t.Run(tt.link, func(t *testing.T) {
			if got := lpExtractName(tt.link); got != tt.want {
				t.Errorf("lpExtractName(%q) = %q, want %q", tt.link, got, tt.want)
			}
		})
	}
}

func TestLpMergeStatuses(t *testing.T) {
	tests := []struct {
		state MergeState
		want  int
	}{
		{MergeStateOpen, 3},
		{MergeStateWIP, 1},
		{MergeStateMerged, 1},
		{MergeStateClosed, 2},
		{MergeStateAbandoned, 0},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			got := lpMergeStatuses(tt.state)
			if len(got) != tt.want {
				t.Errorf("lpMergeStatuses(%v) returned %d statuses, want %d", tt.state, len(got), tt.want)
			}
		})
	}
}

func TestLaunchpadForge_GetMergeRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mp/1", func(w http.ResponseWriter, r *http.Request) {
		mp := lp.MergeProposal{
			SelfLink:                  "https://api.launchpad.net/devel/mp/1",
			WebLink:                   "https://code.launchpad.net/mp/1",
			QueueStatus:               "Approved",
			Description:               "A great change",
			RegistrantLink:            "https://api.launchpad.net/devel/~alice",
			SourceGitPath:             "refs/heads/great-change",
			TargetGitPath:             "refs/heads/main",
			AllCommentsCollectionLink: serverURL(r) + "/comments",
			PreviewDiffLink:           serverURL(r) + "/diff",
		}
		json.NewEncoder(w).Encode(mp)
	})
	mux.HandleFunc("/comments", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(lp.Collection[lp.CodeReviewComment]{
			Entries: []lp.CodeReviewComment{{Content: "Looks good", AuthorLink: "https://api.launchpad.net/devel/~bob"}},
		})
	})
	mux.HandleFunc("/diff", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(lp.PreviewDiff{DiffTextLink: serverURL(r) + "/diff.txt"})
	})
	mux.HandleFunc("/diff.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("diff --git a/foo b/foo"))
	})

	forge, server := newLaunchpadTestServer(t, mux)
	defer server.Close()

	mr, err := forge.GetMergeRequest(context.Background(), "sunbeam", server.URL+"/mp/1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if mr.State != MergeStateOpen {
		t.Errorf("State = %v, want Open", mr.State)
	}
	if mr.ReviewState != ReviewStateApproved {
		t.Errorf("ReviewState = %v, want Approved", mr.ReviewState)
	}
	if mr.Author != "alice" {
		t.Errorf("Author = %q", mr.Author)
	}
	if len(mr.Comments) != 1 || mr.Comments[0].Author != "bob" {
		t.Fatalf("Comments = %+v, want one cached comment", mr.Comments)
	}
	if mr.DiffText != "diff --git a/foo b/foo" {
		t.Fatalf("DiffText = %q, want raw diff text", mr.DiffText)
	}
}

func TestLaunchpadForge_ListCommits_NotSupported(t *testing.T) {
	creds := &lp.Credentials{}
	forge := NewLaunchpadForge(lp.NewClient(creds, nil))

	_, err := forge.ListCommits(context.Background(), "sunbeam", ListCommitsOpts{})
	if err == nil {
		t.Error("expected error for unsupported ListCommits")
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
