// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"

	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// ReviewListRequest describes one review-list workflow.
type ReviewListRequest struct {
	Projects []string
	Forges   []string
	State    string
	Author   string
	Since    string
}

// ReviewListResponse contains listed merge requests plus non-fatal warnings.
type ReviewListResponse struct {
	MergeRequests []forge.MergeRequest
	Warnings      []string
}

// ReviewClientWorkflow exposes reusable client-side review workflows for CLI/TUI/MCP frontends.
type ReviewClientWorkflow struct {
	client *ClientTransport
}

// NewReviewClientWorkflow creates a client-side review workflow.
func NewReviewClientWorkflow(apiClient *ClientTransport) *ReviewClientWorkflow {
	return &ReviewClientWorkflow{client: apiClient}
}

// Show returns one merge request by project and ID.
func (w *ReviewClientWorkflow) Show(ctx context.Context, project, id string) (*forge.MergeRequest, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	return apiClient.ReviewsGet(ctx, project, id)
}

// List returns merge requests matching the requested filters.
func (w *ReviewClientWorkflow) List(ctx context.Context, req ReviewListRequest) (*ReviewListResponse, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}

	resolvedSince, err := dto.ResolveSince(req.Since)
	if err != nil {
		return nil, err
	}

	result, err := apiClient.ReviewsList(ctx, client.ReviewsListOptions{
		Projects: req.Projects,
		Forges:   req.Forges,
		State:    req.State,
		Author:   req.Author,
		Since:    resolvedSince,
	})
	if err != nil {
		return nil, err
	}

	return &ReviewListResponse{
		MergeRequests: result.MergeRequests,
		Warnings:      result.Warnings,
	}, nil
}

func (w *ReviewClientWorkflow) resolveClient() (*ClientTransport, error) {
	if w.client == nil {
		return nil, errors.New("review client workflow requires an API client")
	}
	return w.client, nil
}
