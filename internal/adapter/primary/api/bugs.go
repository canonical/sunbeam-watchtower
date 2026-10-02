package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/canonical/sunbeam-watchtower/internal/app"
	bugsvc "github.com/canonical/sunbeam-watchtower/internal/core/service/bug"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/bugsearch"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// --- List bugs ---

type BugsListInput struct {
	Projects   []string `query:"project" required:"false" doc:"Filter by project name"`
	Status     []string `query:"status" required:"false" doc:"Filter by status"`
	Importance []string `query:"importance" required:"false" doc:"Filter by importance"`
	Assignee   string   `query:"assignee" doc:"Filter by assignee"`
	Tags       []string `query:"tag" required:"false" doc:"Filter by tag"`
	Since      string   `query:"since" doc:"Return bugs created/modified since this date (ISO 8601)"`
	Merge      bool     `query:"merge" required:"false" doc:"Collapse grouped duplicate bug rows"`
	Limit      int      `query:"limit" required:"false" doc:"Maximum number of bug rows to return"`
}

type BugsListOutput struct {
	Body struct {
		Tasks    []forge.BugTask `json:"tasks" doc:"Bug tasks matching filters"`
		Warnings []string        `json:"warnings,omitempty" doc:"Non-fatal warnings"`
	}
}

type BugGetInput struct {
	ID string `path:"id" doc:"Bug ID (e.g. Launchpad bug number)"`
}

type BugGetOutput struct {
	Body *forge.Bug
}

type BugsSearchInput struct {
	Body dto.BugSearchRequest
}

type BugsSearchOutput struct {
	Body dto.BugSearchResponse
}

type BugSyncInput struct {
	Body struct {
		Projects []string `json:"projects,omitempty" required:"false" doc:"Filter to these project names (empty = all)"`
		BugIDs   []string `json:"bug_ids,omitempty" required:"false" doc:"Filter to these Launchpad bug IDs (empty = all)"`
		DryRun   bool     `json:"dry_run,omitempty" required:"false" doc:"If true, show what would change without updating"`
		Since    string   `json:"since,omitempty" required:"false" doc:"Only consider bugs created/modified since (RFC 3339 timestamp)"`
	}
}

type BugSyncOutput struct {
	Body struct {
		Actions []dto.BugSyncAction `json:"actions" doc:"Actions taken or planned"`
		Skipped int                 `json:"skipped" doc:"Number of tasks skipped"`
		Errors  []string            `json:"errors,omitempty" doc:"Non-fatal error messages"`
	}
}

func RegisterBugsAPI(api huma.API, application *app.App) {
	facade := frontend.NewServerFacade(application)

	huma.Register(api, huma.Operation{
		OperationID: "list-bugs",
		Method:      http.MethodGet,
		Path:        "/api/v1/bugs",
		Summary:     "List bug tasks",
		Description: "List bug tasks across all configured bug trackers, with optional filters.",
	}, func(ctx context.Context, input *BugsListInput) (*BugsListOutput, error) {
		result, err := facade.Bugs().List(ctx, frontend.BugListRequest{
			Projects:   input.Projects,
			Status:     input.Status,
			Importance: input.Importance,
			Assignee:   input.Assignee,
			Tags:       input.Tags,
			Since:      input.Since,
			Merge:      input.Merge,
			Limit:      input.Limit,
		})
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to list bugs: %v", err))
		}

		out := &BugsListOutput{}
		out.Body.Tasks = result.Tasks
		out.Body.Warnings = result.Warnings
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "search-bugs",
		Method:      http.MethodPost,
		Path:        "/api/v1/bugs/search",
		Summary:     "Search bugs",
		Description: "Search complete cached bug text with deterministic ranking and match explanations.",
		Tags:        []string{"bugs"},
	}, func(ctx context.Context, input *BugsSearchInput) (*BugsSearchOutput, error) {
		result, err := facade.Bugs().Search(ctx, input.Body)
		if err != nil {
			switch {
			case errors.Is(err, bugsearch.ErrInvalidQuery):
				return nil, huma.Error422UnprocessableEntity(err.Error())
			case errors.Is(err, bugsearch.ErrCacheEmpty),
				errors.Is(err, bugsearch.ErrCacheIncompatible):
				return nil, huma.NewError(http.StatusConflict, err.Error())
			case errors.Is(err, bugsvc.ErrBugNotFound):
				return nil, huma.Error404NotFound(err.Error())
			default:
				return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to search bugs: %v", err))
			}
		}
		out := &BugsSearchOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-bug",
		Method:      http.MethodGet,
		Path:        "/api/v1/bugs/{id}",
		Summary:     "Get a bug",
		Description: "Retrieve a single bug and its tasks by ID.",
	}, func(ctx context.Context, input *BugGetInput) (*BugGetOutput, error) {
		bug, err := facade.Bugs().Show(ctx, input.ID)
		if err != nil {
			if errors.Is(err, bugsvc.ErrBugNotFound) {
				return nil, huma.Error404NotFound(err.Error())
			}
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to fetch bug %s: %v", input.ID, err))
		}

		out := &BugGetOutput{}
		out.Body = bug
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sync-bugs",
		Method:      http.MethodPost,
		Path:        "/api/v1/bugs/sync",
		Summary:     "Sync bug statuses",
		Description: "Scan cached commits for bug references and update bug task statuses. Also assigns bugs to the appropriate series.",
	}, func(ctx context.Context, input *BugSyncInput) (*BugSyncOutput, error) {
		result, err := facade.Bugs().Sync(ctx, frontend.BugSyncRequest{
			Projects: input.Body.Projects,
			BugIDs:   input.Body.BugIDs,
			DryRun:   input.Body.DryRun,
			Since:    input.Body.Since,
		})
		if err != nil {
			switch {
			case errors.Is(err, frontend.ErrNoBugTrackerConfigured):
				return nil, huma.Error500InternalServerError(err.Error())
			case errors.Is(err, frontend.ErrInvalidBugSyncSince):
				return nil, huma.Error400BadRequest(err.Error())
			case errors.Is(err, frontend.ErrInvalidBugID):
				return nil, huma.Error400BadRequest(err.Error())
			default:
				return nil, huma.Error500InternalServerError(fmt.Sprintf("sync failed: %v", err))
			}
		}

		out := &BugSyncOutput{}
		out.Body.Actions = result.Result.Actions
		out.Body.Skipped = result.Result.Skipped
		out.Body.Errors = result.Warnings
		return out, nil
	})
}
