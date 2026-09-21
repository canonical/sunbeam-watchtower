// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"

	"github.com/gboutry/sunbeam-watchtower/pkg/client"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/gboutry/sunbeam-watchtower/pkg/forge/v1"
)

// BugListRequest describes one bug-list workflow.
type BugListRequest struct {
	Projects   []string
	Status     []string
	Importance []string
	Assignee   string
	Tags       []string
	Since      string
	Merge      bool
	Limit      int
}

// BugListResponse contains listed bug tasks plus non-fatal warnings.
type BugListResponse struct {
	Tasks    []forge.BugTask
	Warnings []string
}

// BugSearchRequest aliases the public request used by all frontends.
type BugSearchRequest = dto.BugSearchRequest

// BugSearchResponse aliases the public response used by all frontends.
type BugSearchResponse = dto.BugSearchResponse

// BugSyncRequest describes one bug-sync workflow.
type BugSyncRequest struct {
	Projects []string
	BugIDs   []string
	DryRun   bool
	Since    string
}

// BugSyncResponse contains normalized bug-sync results plus non-fatal warnings.
type BugSyncResponse struct {
	Result   *dto.BugSyncResult
	Warnings []string
}

// BugClientWorkflow exposes reusable client-side bug workflows for CLI/TUI/MCP frontends.
type BugClientWorkflow struct {
	client *ClientTransport
}

// NewBugClientWorkflow creates a client-side bug workflow.
func NewBugClientWorkflow(apiClient *ClientTransport) *BugClientWorkflow {
	return &BugClientWorkflow{client: apiClient}
}

// Show returns one bug with its associated tasks.
func (w *BugClientWorkflow) Show(ctx context.Context, id string) (*forge.Bug, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.BugsGet(ctx, id)
}

// List returns bug tasks matching the requested filters.
func (w *BugClientWorkflow) List(ctx context.Context, req BugListRequest) (*BugListResponse, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}

	resolvedSince, err := dto.ResolveSince(req.Since)
	if err != nil {
		return nil, err
	}

	result, err := apiClient.BugsList(ctx, client.BugsListOptions{
		Projects:   req.Projects,
		Status:     req.Status,
		Importance: req.Importance,
		Assignee:   req.Assignee,
		Tags:       req.Tags,
		Since:      resolvedSince,
		Merge:      req.Merge,
		Limit:      req.Limit,
	})
	if err != nil {
		return nil, err
	}

	return &BugListResponse{
		Tasks:    result.Tasks,
		Warnings: result.Warnings,
	}, nil
}

// Search returns ranked, explained bug candidates.
func (w *BugClientWorkflow) Search(ctx context.Context, req BugSearchRequest) (*BugSearchResponse, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	if err := resolveBugSearchTimes(&req); err != nil {
		return nil, err
	}
	return apiClient.BugsSearch(ctx, req)
}

// Sync triggers remote bug correlation/sync work.
func (w *BugClientWorkflow) Sync(ctx context.Context, req BugSyncRequest) (*BugSyncResponse, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}

	resolvedSince, err := dto.ResolveSince(req.Since)
	if err != nil {
		return nil, err
	}
	bugIDs, err := normalizeBugIDs(req.BugIDs)
	if err != nil {
		return nil, err
	}

	result, err := apiClient.BugsSync(ctx, client.BugsSyncOptions{
		Projects: req.Projects,
		BugIDs:   bugIDs,
		DryRun:   req.DryRun,
		Since:    resolvedSince,
	})
	if err != nil {
		return nil, err
	}

	return &BugSyncResponse{
		Result: &dto.BugSyncResult{
			Actions: result.Actions,
			Skipped: result.Skipped,
		},
		Warnings: result.Errors,
	}, nil
}

func resolveBugSearchTimes(req *BugSearchRequest) error {
	for _, item := range []struct {
		value       string
		destination *string
	}{
		{req.CreatedAfter, &req.CreatedAfter},
		{req.CreatedBefore, &req.CreatedBefore},
		{req.ModifiedAfter, &req.ModifiedAfter},
		{req.ModifiedBefore, &req.ModifiedBefore},
	} {
		if item.value == "" {
			continue
		}
		resolved, err := dto.ResolveSince(item.value)
		if err != nil {
			return err
		}
		*item.destination = resolved
	}
	return nil
}

func (w *BugClientWorkflow) resolveClient() (*ClientTransport, error) {
	if w.client == nil {
		return nil, errors.New("bug client workflow requires an API client")
	}
	return w.client, nil
}
