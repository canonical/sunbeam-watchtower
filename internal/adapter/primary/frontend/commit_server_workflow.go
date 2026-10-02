// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/commit"
)

// CommitServerWorkflow exposes reusable server-side commit workflows for the HTTP API.
type CommitServerWorkflow struct {
	application *app.App
}

// NewCommitServerWorkflow creates a server-side commit workflow.
func NewCommitServerWorkflow(application *app.App) *CommitServerWorkflow {
	return &CommitServerWorkflow{application: application}
}

// Log returns commits matching the requested filters.
func (w *CommitServerWorkflow) Log(ctx context.Context, req CommitLogRequest) (*CommitListResponse, error) {
	return w.list(ctx, commit.ListOptions{
		Projects:   req.Projects,
		Branch:     req.Branch,
		Author:     req.Author,
		IncludeMRs: req.IncludeMRs,
	}, req.Forges)
}

// Track returns commits referencing the requested bug ID.
func (w *CommitServerWorkflow) Track(ctx context.Context, req CommitTrackRequest) (*CommitListResponse, error) {
	return w.list(ctx, commit.ListOptions{
		Projects:   req.Projects,
		Branch:     req.Branch,
		BugID:      req.BugID,
		IncludeMRs: req.IncludeMRs,
	}, req.Forges)
}

func (w *CommitServerWorkflow) list(ctx context.Context, opts commit.ListOptions, forgeNames []string) (*CommitListResponse, error) {
	sources, err := w.application.BuildCommitSources()
	if err != nil {
		return nil, err
	}
	for _, forgeName := range forgeNames {
		forgeType, err := parseForgeType(forgeName)
		if err != nil {
			return nil, err
		}
		opts.Forges = append(opts.Forges, forgeType)
	}

	commits, results, err := commit.NewService(sources, w.application.Logger).List(ctx, opts)
	if err != nil {
		return nil, err
	}
	response := &CommitListResponse{Commits: commits}
	for _, result := range results {
		if result.Err != nil {
			response.Warnings = append(response.Warnings, result.Err.Error())
		}
	}
	return response, nil
}
