// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"

	"github.com/canonical/sunbeam-watchtower/pkg/client"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// CommitLogRequest describes one commit-log workflow.
type CommitLogRequest struct {
	Projects   []string
	Forges     []string
	Branch     string
	Author     string
	IncludeMRs bool
}

// CommitTrackRequest describes one bug-centric commit-tracking workflow.
type CommitTrackRequest struct {
	BugID      string
	Projects   []string
	Forges     []string
	Branch     string
	IncludeMRs bool
}

// CommitListResponse contains listed commits plus non-fatal warnings.
type CommitListResponse struct {
	Commits  []forge.Commit
	Warnings []string
}

// CommitClientWorkflow exposes reusable client-side commit workflows for CLI/TUI/MCP frontends.
type CommitClientWorkflow struct {
	client *ClientTransport
}

// NewCommitClientWorkflow creates a client-side commit workflow.
func NewCommitClientWorkflow(apiClient *ClientTransport) *CommitClientWorkflow {
	return &CommitClientWorkflow{client: apiClient}
}

// Log returns commits matching the requested filters.
func (w *CommitClientWorkflow) Log(ctx context.Context, req CommitLogRequest) (*CommitListResponse, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}

	result, err := apiClient.CommitsList(ctx, client.CommitsListOptions{
		Projects:   req.Projects,
		Forges:     req.Forges,
		Branch:     req.Branch,
		Author:     req.Author,
		IncludeMRs: req.IncludeMRs,
	})
	if err != nil {
		return nil, err
	}

	return &CommitListResponse{
		Commits:  result.Commits,
		Warnings: result.Warnings,
	}, nil
}

// Track returns commits referencing the requested bug ID.
func (w *CommitClientWorkflow) Track(ctx context.Context, req CommitTrackRequest) (*CommitListResponse, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}

	result, err := apiClient.CommitsTrack(ctx, client.CommitsTrackOptions{
		BugID:      req.BugID,
		Projects:   req.Projects,
		Forges:     req.Forges,
		Branch:     req.Branch,
		IncludeMRs: req.IncludeMRs,
	})
	if err != nil {
		return nil, err
	}

	return &CommitListResponse{
		Commits:  result.Commits,
		Warnings: result.Warnings,
	}, nil
}

func (w *CommitClientWorkflow) resolveClient() (*ClientTransport, error) {
	if w.client == nil {
		return nil, errors.New("commit client workflow requires an API client")
	}
	return w.client, nil
}
