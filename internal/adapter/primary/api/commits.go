package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/canonical/sunbeam-watchtower/internal/app"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

type CommitsListInput struct {
	Projects   []string `query:"project" required:"false" doc:"Filter by project name"`
	Forges     []string `query:"forge" required:"false" doc:"Filter by forge type: github, launchpad, gerrit"`
	Branch     string   `query:"branch" doc:"Branch to list commits from"`
	Author     string   `query:"author" doc:"Filter by author"`
	IncludeMRs bool     `query:"include_mrs" required:"false" doc:"Include commits from merge request refs"`
}

type CommitsListOutput struct {
	Body struct {
		Commits  []forge.Commit `json:"commits" doc:"Commits matching filters"`
		Warnings []string       `json:"warnings,omitempty" doc:"Non-fatal warnings"`
	}
}

type CommitsTrackInput struct {
	BugID      string   `query:"bug_id" required:"true" doc:"Bug ID to track"`
	Projects   []string `query:"project" required:"false" doc:"Filter by project name"`
	Forges     []string `query:"forge" required:"false" doc:"Filter by forge type: github, launchpad, gerrit"`
	Branch     string   `query:"branch" doc:"Branch to list commits from"`
	IncludeMRs bool     `query:"include_mrs" required:"false" doc:"Include commits from merge request refs"`
}

type CommitsTrackOutput struct {
	Body struct {
		Commits  []forge.Commit `json:"commits" doc:"Commits referencing the bug"`
		Warnings []string       `json:"warnings,omitempty" doc:"Non-fatal warnings"`
	}
}

func RegisterCommitsAPI(api huma.API, application *app.App) {
	facade := frontend.NewServerFacade(application)

	huma.Register(api, huma.Operation{
		OperationID: "list-commits",
		Method:      http.MethodGet,
		Path:        "/api/v1/commits",
		Summary:     "List commits",
		Description: "List commits across all configured forges, with optional filters.",
		Tags:        []string{"commits"},
	}, func(ctx context.Context, input *CommitsListInput) (*CommitsListOutput, error) {
		result, err := facade.Commits().Log(ctx, frontend.CommitLogRequest{
			Projects:   input.Projects,
			Forges:     input.Forges,
			Branch:     input.Branch,
			Author:     input.Author,
			IncludeMRs: input.IncludeMRs,
		})
		if err != nil {
			if isFrontendValidationError(err) {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to list commits: %v", err))
		}
		out := &CommitsListOutput{}
		out.Body.Commits = result.Commits
		out.Body.Warnings = result.Warnings
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "track-commits",
		Method:      http.MethodGet,
		Path:        "/api/v1/commits/track",
		Summary:     "Track commits by bug ID",
		Description: "Find commits referencing a specific bug ID across all configured forges.",
		Tags:        []string{"commits"},
	}, func(ctx context.Context, input *CommitsTrackInput) (*CommitsTrackOutput, error) {
		result, err := facade.Commits().Track(ctx, frontend.CommitTrackRequest{
			BugID:      input.BugID,
			Projects:   input.Projects,
			Forges:     input.Forges,
			Branch:     input.Branch,
			IncludeMRs: input.IncludeMRs,
		})
		if err != nil {
			if isFrontendValidationError(err) {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to list commits: %v", err))
		}
		trackOut := &CommitsTrackOutput{}
		trackOut.Body.Commits = result.Commits
		trackOut.Body.Warnings = result.Warnings
		return trackOut, nil
	})
}
