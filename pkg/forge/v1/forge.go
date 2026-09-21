// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

// Package v1 defines a unified interface for interacting with code forges
// (GitHub, Launchpad, Gerrit) and provides concrete implementations.
package v1

import (
	"context"
	"time"
)

// ForgeType identifies which forge a resource originates from.
type ForgeType int

const (
	ForgeGitHub ForgeType = iota
	ForgeLaunchpad
	ForgeGerrit
)

// String returns a human-readable name for the forge type.
func (f ForgeType) String() string {
	switch f {
	case ForgeGitHub:
		return "GitHub"
	case ForgeLaunchpad:
		return "Launchpad"
	case ForgeGerrit:
		return "Gerrit"
	default:
		return "Unknown"
	}
}

// MergeState represents the lifecycle state of a merge request.
type MergeState int

const (
	MergeStateOpen MergeState = iota
	MergeStateMerged
	MergeStateClosed
	MergeStateAbandoned
	MergeStateWIP
)

func (s MergeState) String() string {
	switch s {
	case MergeStateOpen:
		return "Open"
	case MergeStateMerged:
		return "Merged"
	case MergeStateClosed:
		return "Closed"
	case MergeStateAbandoned:
		return "Abandoned"
	case MergeStateWIP:
		return "WIP"
	default:
		return "Unknown"
	}
}

// ReviewState represents the review status of a merge request.
type ReviewState int

const (
	ReviewStatePending ReviewState = iota
	ReviewStateApproved
	ReviewStateChangesRequested
	ReviewStateRejected
)

func (s ReviewState) String() string {
	switch s {
	case ReviewStatePending:
		return "Pending"
	case ReviewStateApproved:
		return "Approved"
	case ReviewStateChangesRequested:
		return "Changes Requested"
	case ReviewStateRejected:
		return "Rejected"
	default:
		return "Unknown"
	}
}

// CheckState represents the status of a CI check.
type CheckState int

const (
	CheckStatePending CheckState = iota
	CheckStateRunning
	CheckStatePassed
	CheckStateFailed
)

func (s CheckState) String() string {
	switch s {
	case CheckStatePending:
		return "Pending"
	case CheckStateRunning:
		return "Running"
	case CheckStatePassed:
		return "Passed"
	case CheckStateFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// Check represents a CI/CD check result on a merge request.
type Check struct {
	Name  string     `json:"name" yaml:"name"`
	State CheckState `json:"state" yaml:"state"`
	URL   string     `json:"url" yaml:"url"`
}

// ReviewCommentKind classifies one review discussion entry.
type ReviewCommentKind string

const (
	ReviewCommentGeneral ReviewCommentKind = "general"
	ReviewCommentInline  ReviewCommentKind = "inline"
	ReviewCommentSystem  ReviewCommentKind = "system"
)

// ReviewComment normalizes one review discussion entry across forges.
type ReviewComment struct {
	Kind      ReviewCommentKind `json:"kind" yaml:"kind"`
	Author    string            `json:"author,omitempty" yaml:"author,omitempty"`
	Body      string            `json:"body,omitempty" yaml:"body,omitempty"`
	URL       string            `json:"url,omitempty" yaml:"url,omitempty"`
	File      string            `json:"file,omitempty" yaml:"file,omitempty"`
	Line      int               `json:"line,omitempty" yaml:"line,omitempty"`
	CreatedAt time.Time         `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt time.Time         `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

// ReviewFile normalizes one changed file entry across forges.
type ReviewFile struct {
	Path         string `json:"path" yaml:"path"`
	PreviousPath string `json:"previous_path,omitempty" yaml:"previous_path,omitempty"`
	Status       string `json:"status,omitempty" yaml:"status,omitempty"`
	Additions    int    `json:"additions,omitempty" yaml:"additions,omitempty"`
	Deletions    int    `json:"deletions,omitempty" yaml:"deletions,omitempty"`
	Patch        string `json:"patch,omitempty" yaml:"patch,omitempty"`
	Binary       bool   `json:"binary,omitempty" yaml:"binary,omitempty"`
	Truncated    bool   `json:"truncated,omitempty" yaml:"truncated,omitempty"`
}

// MergeRequest unifies GitHub PRs, Launchpad merge proposals, and Gerrit changes.
type MergeRequest struct {
	Forge        ForgeType       `json:"forge" yaml:"forge"`
	Repo         string          `json:"repo" yaml:"repo"`
	ID           string          `json:"id" yaml:"id"`
	Title        string          `json:"title" yaml:"title"`
	Description  string          `json:"description,omitempty" yaml:"description,omitempty"`
	Author       string          `json:"author" yaml:"author"`
	SourceBranch string          `json:"source_branch" yaml:"source_branch"`
	TargetBranch string          `json:"target_branch" yaml:"target_branch"`
	State        MergeState      `json:"state" yaml:"state"`
	ReviewState  ReviewState     `json:"review_state" yaml:"review_state"`
	Checks       []Check         `json:"checks,omitempty" yaml:"checks,omitempty"`
	Comments     []ReviewComment `json:"comments,omitempty" yaml:"comments,omitempty"`
	Files        []ReviewFile    `json:"files,omitempty" yaml:"files,omitempty"`
	DiffText     string          `json:"diff_text,omitempty" yaml:"diff_text,omitempty"`
	URL          string          `json:"url" yaml:"url"`
	CreatedAt    time.Time       `json:"created_at" yaml:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" yaml:"updated_at"`
}

// CommitMergeRequest annotates a commit with its associated merge request info.
// Nil for commits on the main branch.
type CommitMergeRequest struct {
	ID    string     `json:"id" yaml:"id"`
	State MergeState `json:"state" yaml:"state"`
	URL   string     `json:"url" yaml:"url"`
}

// Commit is a forge-agnostic commit.
type Commit struct {
	Forge        ForgeType           `json:"forge" yaml:"forge"`
	Repo         string              `json:"repo" yaml:"repo"`
	SHA          string              `json:"sha" yaml:"sha"`
	Message      string              `json:"message" yaml:"message"`
	Author       string              `json:"author" yaml:"author"`
	Date         time.Time           `json:"date" yaml:"date"`
	URL          string              `json:"url" yaml:"url"`
	BugRefs      []BugRef            `json:"bug_refs,omitempty" yaml:"bug_refs,omitempty"`
	MergeRequest *CommitMergeRequest `json:"merge_request,omitempty" yaml:"merge_request,omitempty"`
}

// ListMergeRequestsOpts holds options for listing merge requests.
type ListMergeRequestsOpts struct {
	State  MergeState
	Author string
}

// ListCommitsOpts holds options for listing commits.
type ListCommitsOpts struct {
	Branch   string
	Revision string // optional branch, tag, or commit to use as the history root
	Since    *time.Time
	Author   string
}

// Forge is the unified interface for interacting with code forges.
// Each forge (GitHub, Launchpad, Gerrit) implements this interface.
type Forge interface {
	// Type returns which forge this client represents.
	Type() ForgeType

	// ListMergeRequests returns merge requests for the given repository.
	ListMergeRequests(ctx context.Context, repo string, opts ListMergeRequestsOpts) ([]MergeRequest, error)

	// GetMergeRequest returns a single merge request by its ID.
	GetMergeRequest(ctx context.Context, repo string, id string) (*MergeRequest, error)

	// ListCommits returns commits for the given repository.
	ListCommits(ctx context.Context, repo string, opts ListCommitsOpts) ([]Commit, error)
}
