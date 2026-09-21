// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"

	forge "github.com/gboutry/sunbeam-watchtower/pkg/forge/v1"
)

// BugTracker is the interface for querying and updating bug trackers.
type BugTracker interface {
	Type() forge.ForgeType
	GetBug(ctx context.Context, id string) (*forge.Bug, error)
	ListBugTasks(ctx context.Context, project string, opts forge.ListBugTasksOpts) ([]forge.BugTask, error)
	UpdateBugTaskStatus(ctx context.Context, taskSelfLink, status string) error
	AddBugTask(ctx context.Context, bugID int, seriesSelfLink string) error
	GetProjectSeries(ctx context.Context, projectName string) ([]forge.ProjectSeries, error)
	GetProject(ctx context.Context, projectName string) (*forge.Project, error)
}

// FreshBugReader is implemented by cache-decorated trackers that can bypass
// cached state for mutation planning. Read-only workflows remain cache-first.
type FreshBugReader interface {
	GetBugFresh(ctx context.Context, id string, refreshCache bool) (*forge.Bug, error)
}
