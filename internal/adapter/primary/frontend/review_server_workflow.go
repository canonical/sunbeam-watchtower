// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/review"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// ReviewServerWorkflow exposes reusable server-side review workflows for the HTTP API.
type ReviewServerWorkflow struct {
	application *app.App
}

// NewReviewServerWorkflow creates a server-side review workflow.
func NewReviewServerWorkflow(application *app.App) *ReviewServerWorkflow {
	return &ReviewServerWorkflow{application: application}
}

// Show returns one merge request by project and ID.
func (w *ReviewServerWorkflow) Show(ctx context.Context, project, id string) (*forge.MergeRequest, error) {
	clients, err := w.application.BuildReviewProjects()
	if err != nil {
		return nil, err
	}
	return review.NewService(clients, w.application.Logger).Get(ctx, project, id)
}

// List returns merge requests matching the requested filters.
func (w *ReviewServerWorkflow) List(ctx context.Context, req ReviewListRequest) (*ReviewListResponse, error) {
	clients, err := w.application.BuildReviewProjects()
	if err != nil {
		return nil, err
	}

	listOpts := review.ListOptions{
		Projects: req.Projects,
		Author:   req.Author,
		Since:    req.Since,
	}
	if req.State != "" {
		state, err := parseMergeState(req.State)
		if err != nil {
			return nil, err
		}
		listOpts.State = state
	}
	for _, forgeName := range req.Forges {
		forgeType, err := parseForgeType(forgeName)
		if err != nil {
			return nil, err
		}
		listOpts.Forges = append(listOpts.Forges, forgeType)
	}

	mrs, results, err := review.NewService(clients, w.application.Logger).List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	response := &ReviewListResponse{MergeRequests: mrs}
	for _, result := range results {
		if result.Err != nil {
			response.Warnings = append(response.Warnings, result.Err.Error())
		}
	}
	return response, nil
}
