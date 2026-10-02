package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

type ReviewsListInput struct {
	Projects []string `query:"project" required:"false" doc:"Filter by project name"`
	Forges   []string `query:"forge" required:"false" doc:"Filter by forge type: github, launchpad, gerrit"`
	State    string   `query:"state" doc:"Filter by state: open, merged, closed, wip, abandoned"`
	Author   string   `query:"author" doc:"Filter by author"`
	Since    string   `query:"since" doc:"Show only MRs updated since (RFC 3339 timestamp)"`
}

type ReviewsListOutput struct {
	Body struct {
		MergeRequests []forge.MergeRequest `json:"merge_requests" doc:"Merge requests matching filters"`
		Warnings      []string             `json:"warnings,omitempty" doc:"Non-fatal warnings"`
	}
}

type ReviewGetInput struct {
	Project string `path:"project" doc:"Project name"`
	ID      string `path:"id" doc:"Merge request ID"`
}

type ReviewGetOutput struct {
	Body *forge.MergeRequest
}

func RegisterReviewsAPI(api huma.API, application *app.App) {
	facade := frontend.NewServerFacade(application)

	huma.Register(api, huma.Operation{
		OperationID: "list-reviews",
		Method:      http.MethodGet,
		Path:        "/api/v1/reviews",
		Summary:     "List merge requests",
		Description: "List merge requests across all configured forges, with optional filters.",
		Tags:        []string{"reviews"},
	}, func(ctx context.Context, input *ReviewsListInput) (*ReviewsListOutput, error) {
		result, err := facade.Reviews().List(ctx, frontend.ReviewListRequest{
			Projects: input.Projects,
			Forges:   input.Forges,
			State:    input.State,
			Author:   input.Author,
			Since:    input.Since,
		})
		if err != nil {
			if isFrontendValidationError(err) {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to list reviews: %v", err))
		}

		out := &ReviewsListOutput{}
		out.Body.MergeRequests = result.MergeRequests
		out.Body.Warnings = result.Warnings
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-review",
		Method:      http.MethodGet,
		Path:        "/api/v1/reviews/{project}/{id}",
		Summary:     "Get a merge request",
		Description: "Retrieve a single merge request by project name and ID.",
		Tags:        []string{"reviews"},
	}, func(ctx context.Context, input *ReviewGetInput) (*ReviewGetOutput, error) {
		mr, err := facade.Reviews().Show(ctx, input.Project, input.ID)
		if err != nil {
			switch {
			case errors.Is(err, port.ErrReviewCacheNotSynced):
				return nil, huma.Error409Conflict("review cache not synced; run `watchtower cache sync reviews` first")
			case errors.Is(err, port.ErrReviewDetailNotCached):
				return nil, huma.Error409Conflict("review detail not cached; run `watchtower cache sync reviews` first")
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("review %s/%s not found", input.Project, input.ID), err)
		}

		out := &ReviewGetOutput{}
		out.Body = mr
		return out, nil
	})
}
