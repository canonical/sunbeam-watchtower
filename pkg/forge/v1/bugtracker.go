// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"time"
)

// Bug is a forge-agnostic bug with its associated tasks.
type Bug struct {
	Forge           ForgeType      `json:"forge" yaml:"forge"`
	ID              string         `json:"id" yaml:"id"`
	Title           string         `json:"title" yaml:"title"`
	Description     string         `json:"description" yaml:"description"`
	Owner           string         `json:"owner" yaml:"owner"`
	Tags            []string       `json:"tags,omitempty" yaml:"tags,omitempty"`
	URL             string         `json:"url" yaml:"url"`
	Tasks           []BugTask      `json:"tasks" yaml:"tasks"`
	Comments        []BugComment   `json:"comments,omitempty" yaml:"comments,omitempty"`
	Links           []BugLink      `json:"links,omitempty" yaml:"links,omitempty"`
	Private         bool           `json:"private,omitempty" yaml:"private,omitempty"`
	SecurityRelated bool           `json:"security_related,omitempty" yaml:"security_related,omitempty"`
	InformationType string         `json:"information_type,omitempty" yaml:"information_type,omitempty"`
	VisibilityKnown bool           `json:"visibility_known" yaml:"visibility_known"`
	Provenance      *BugProvenance `json:"provenance,omitempty" yaml:"provenance,omitempty"`
	CreatedAt       time.Time      `json:"created_at" yaml:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at" yaml:"updated_at"`
}

// Sensitive reports whether a bug requires current tracker authorization
// before cached content may be returned.
func (b Bug) Sensitive() bool {
	return !b.VisibilityKnown || b.Private || b.SecurityRelated
}

// BugLink is a typed link associated with a bug.
type BugLink struct {
	Relation string `json:"relation" yaml:"relation"`
	URL      string `json:"url" yaml:"url"`
}

// BugProvenance describes where the returned bug data came from.
type BugProvenance struct {
	Source     string    `json:"source" yaml:"source"`
	SyncedAt   time.Time `json:"synced_at,omitempty" yaml:"synced_at,omitempty"`
	VerifiedAt time.Time `json:"verified_at,omitempty" yaml:"verified_at,omitempty"`
}

// BugComment is a forge-agnostic comment on a bug.
type BugComment struct {
	Author    string    `json:"author" yaml:"author"`
	Subject   string    `json:"subject,omitempty" yaml:"subject,omitempty"`
	Body      string    `json:"body" yaml:"body"`
	URL       string    `json:"url,omitempty" yaml:"url,omitempty"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

// BugTask is a forge-agnostic bug task.
type BugTask struct {
	Forge           ForgeType `json:"forge"`
	Project         string    `json:"project"`
	BugID           string    `json:"bug_id"`
	Title           string    `json:"title"`
	Status          string    `json:"status"`
	Importance      string    `json:"importance"`
	Assignee        string    `json:"assignee,omitempty"`
	Tags            []string  `json:"tags,omitempty"`
	URL             string    `json:"url"`
	SelfLink        string    `json:"self_link,omitempty"`
	TargetName      string    `json:"target_name,omitempty"`
	TargetLink      string    `json:"target_link,omitempty"`
	Private         bool      `json:"private,omitempty"`
	SecurityRelated bool      `json:"security_related,omitempty"`
	InformationType string    `json:"information_type,omitempty"`
	VisibilityKnown bool      `json:"visibility_known"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Sensitive reports whether a cached task needs current visibility
// verification before it may be displayed.
func (t BugTask) Sensitive() bool {
	return !t.VisibilityKnown || t.Private || t.SecurityRelated
}

// ProjectSeries is a forge-agnostic project series.
type ProjectSeries struct {
	Name     string
	SelfLink string
	Active   bool
}

// Project is a forge-agnostic project.
type Project struct {
	Name                 string
	SelfLink             string
	DevelopmentFocusLink string
}

// ListBugTasksOpts holds options for listing bug tasks.
type ListBugTasksOpts struct {
	Status        []string
	Importance    []string
	Assignee      string
	Tags          []string
	CreatedSince  string // ISO 8601 date string for filtering by creation date
	ModifiedSince string // ISO 8601 date string for filtering by last modification date
}

// BugTracker is the interface for querying and updating bug trackers.
type BugTracker interface {
	// Type returns which forge this bug tracker represents.
	Type() ForgeType

	// GetBug returns a bug by ID with its tasks.
	GetBug(ctx context.Context, id string) (*Bug, error)

	// ListBugTasks returns bug tasks for the given project.
	ListBugTasks(ctx context.Context, project string, opts ListBugTasksOpts) ([]BugTask, error)

	// UpdateBugTaskStatus updates the status of a bug task.
	UpdateBugTaskStatus(ctx context.Context, taskSelfLink, status string) error

	// AddBugTask adds a bug task targeting the given series.
	AddBugTask(ctx context.Context, bugID int, seriesSelfLink string) error

	// GetProjectSeries returns the series for a project.
	GetProjectSeries(ctx context.Context, projectName string) ([]ProjectSeries, error)

	// GetProject returns project information.
	GetProject(ctx context.Context, projectName string) (*Project, error)
}
