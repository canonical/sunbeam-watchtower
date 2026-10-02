package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/build"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// --- Trigger builds ---

// BuildsTriggerInput holds the request body for triggering builds.
type BuildsTriggerInput struct {
	Body struct {
		Project    string                   `json:"project" doc:"Project name"`
		Artifacts  []string                 `json:"artifacts,omitempty" required:"false" doc:"Artifact names to build (empty = all configured)"`
		Wait       bool                     `json:"wait,omitempty" required:"false" doc:"Wait for builds to complete"`
		Timeout    string                   `json:"timeout,omitempty" required:"false" doc:"Max wait time as Go duration (e.g. 5h)"`
		Owner      string                   `json:"owner,omitempty" required:"false" doc:"Override backend owner"`
		Prefix     string                   `json:"prefix,omitempty" required:"false" doc:"Temp recipe name prefix"`
		TargetRef  string                   `json:"target_ref,omitempty" required:"false" doc:"Override backend target reference for recipe operations"`
		Prepared   *dto.PreparedBuildSource `json:"prepared,omitempty" required:"false" doc:"Frontend-prepared backend references for split local build workflows"`
		RetryCount int                      `json:"retry_count,omitempty" required:"false" doc:"Max attempts per build (>= 1, default 1). Requires wait=true. Not supported on the async endpoint."`
	}
}

// BuildsTriggerOutput is the response for triggering builds.
type BuildsTriggerOutput struct {
	Body dto.BuildTriggerResult
}

// --- List builds ---

// BuildsListInput holds query parameters for listing builds.
type BuildsListInput struct {
	Projects     []string `query:"project" required:"false" doc:"Filter by project name"`
	All          bool     `query:"all" required:"false" doc:"Show all builds (not just active)"`
	State        string   `query:"state" doc:"Filter by state"`
	Owner        string   `query:"owner" doc:"Override backend owner"`
	TargetRef    string   `query:"target_ref" doc:"Override backend target reference for recipe lookup"`
	RecipeNames  []string `query:"recipe" required:"false" doc:"Explicit recipe names (overrides project config)"`
	RecipePrefix string   `query:"recipe_prefix" doc:"Filter recipes by name prefix"`
}

// BuildsListOutput is the response for listing builds.
type BuildsListOutput struct {
	Body struct {
		Builds []dto.Build `json:"builds" doc:"Builds matching filters"`
	}
}

// --- Download builds ---

// BuildsDownloadInput holds the request body for downloading build artifacts.
type BuildsDownloadInput struct {
	Body struct {
		Project      string   `json:"project" doc:"Project name"`
		Artifacts    []string `json:"artifacts,omitempty" required:"false" doc:"Artifact names to download (empty = all)"`
		RecipePrefix string   `json:"recipe_prefix,omitempty" required:"false" doc:"Filter recipes by name prefix"`
		Owner        string   `json:"owner,omitempty" required:"false" doc:"Override backend owner"`
		TargetRef    string   `json:"target_ref,omitempty" required:"false" doc:"Override backend target reference"`
		ArtifactsDir string   `json:"artifacts_dir,omitempty" required:"false" doc:"Output directory (default from config)"`
		RetryCount   int      `json:"retry_count,omitempty" required:"false" doc:"Max download attempts per artifact file (>= 1, default 1)"`
	}
}

// BuildsDownloadOutput is the response for downloading build artifacts.
type BuildsDownloadOutput struct {
	Body struct {
		Status string `json:"status" example:"ok"`
	}
}

// --- Cleanup builds ---

// BuildsCleanupInput holds the request body for cleaning up temporary recipes.
type BuildsCleanupInput struct {
	Body struct {
		Project string `json:"project,omitempty" doc:"Filter to specific project"`
		Owner   string `json:"owner,omitempty" doc:"LP owner"`
		Prefix  string `json:"prefix,omitempty" doc:"Recipe name prefix to match"`
		DryRun  bool   `json:"dry_run,omitempty" required:"false" doc:"Show what would be deleted"`
	}
}

// BuildsCleanupOutput is the response for cleaning up temporary recipes.
type BuildsCleanupOutput struct {
	Body struct {
		DeletedRecipes  []string `json:"deleted_recipes" doc:"Deleted recipe names"`
		DeletedBranches []string `json:"deleted_branches" doc:"Deleted branch paths"`
	}
}

// RegisterBuildsAPI registers the /api/v1/builds endpoints on the given huma API.
func RegisterBuildsAPI(api huma.API, application *app.App) {
	facade := frontend.NewServerFacade(application)

	huma.Register(api, huma.Operation{
		OperationID: "trigger-builds",
		Method:      http.MethodPost,
		Path:        "/api/v1/builds/trigger",
		Summary:     "Trigger builds for a project",
		Description: "Trigger builds for a project's artifacts, optionally from a local git repo.",
		Tags:        []string{"builds"},
	}, func(ctx context.Context, input *BuildsTriggerInput) (*BuildsTriggerOutput, error) {
		triggerOpts, err := buildTriggerOptionsFromInput(input)
		if err != nil {
			return nil, huma.Error422UnprocessableEntity("invalid timeout duration", err)
		}
		if triggerOpts.RetryCount < 0 {
			return nil, huma.Error422UnprocessableEntity("retry_count must be >= 0")
		}
		if triggerOpts.RetryCount > 1 && !triggerOpts.Wait {
			return nil, huma.Error422UnprocessableEntity("retry_count > 1 requires wait=true")
		}

		result, err := facade.Builds().Trigger(ctx, input.Body.Project, input.Body.Artifacts, triggerOpts)
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("trigger failed: %v", err))
		}

		out := &BuildsTriggerOutput{}
		out.Body = *result
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "trigger-builds-async",
		Method:      http.MethodPost,
		Path:        "/api/v1/builds/trigger/async",
		Summary:     "Trigger builds asynchronously",
		Description: "Queue build triggering as a long-running operation job.",
		Tags:        []string{"builds", "operations"},
	}, func(ctx context.Context, input *BuildsTriggerInput) (*OperationOutput, error) {
		triggerOpts, err := buildTriggerOptionsFromInput(input)
		if err != nil {
			return nil, huma.Error422UnprocessableEntity("invalid timeout duration", err)
		}
		if triggerOpts.RetryCount > 1 {
			return nil, huma.Error422UnprocessableEntity("retry_count is not supported on the async trigger endpoint")
		}

		job, err := facade.Builds().StartTrigger(ctx, input.Body.Project, input.Body.Artifacts, triggerOpts)
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("async trigger failed: %v", err))
		}

		out := &OperationOutput{}
		out.Body = *job
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-builds",
		Method:      http.MethodGet,
		Path:        "/api/v1/builds",
		Summary:     "List builds across projects",
		Description: "List builds across all configured projects, with optional filters.",
		Tags:        []string{"builds"},
	}, func(ctx context.Context, input *BuildsListInput) (*BuildsListOutput, error) {
		builds, err := facade.Builds().List(ctx, build.ListOpts{
			Projects:     input.Projects,
			All:          input.All,
			State:        input.State,
			Owner:        input.Owner,
			TargetRef:    input.TargetRef,
			RecipeNames:  input.RecipeNames,
			RecipePrefix: input.RecipePrefix,
		})
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to list builds: %v", err))
		}

		out := &BuildsListOutput{}
		out.Body.Builds = builds
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "download-builds",
		Method:      http.MethodPost,
		Path:        "/api/v1/builds/download",
		Summary:     "Download build artifacts",
		Description: "Download build artifacts for succeeded builds of the given artifacts.",
		Tags:        []string{"builds"},
	}, func(ctx context.Context, input *BuildsDownloadInput) (*BuildsDownloadOutput, error) {
		if input.Body.RetryCount < 0 {
			return nil, huma.Error422UnprocessableEntity("retry_count must be >= 0")
		}
		artifactsDir := input.Body.ArtifactsDir
		if artifactsDir == "" {
			var err error
			artifactsDir, err = facade.Builds().DefaultArtifactsDir()
			if err != nil {
				return nil, huma.Error500InternalServerError(fmt.Sprintf("download failed: %v", err))
			}
		}

		if err := facade.Builds().Download(ctx, build.DownloadOpts{
			Projects:      []string{input.Body.Project},
			ArtifactNames: input.Body.Artifacts,
			RecipePrefix:  input.Body.RecipePrefix,
			Owner:         input.Body.Owner,
			TargetRef:     input.Body.TargetRef,
			OutputDir:     artifactsDir,
			RetryCount:    input.Body.RetryCount,
		}); err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("download failed: %v", err))
		}

		out := &BuildsDownloadOutput{}
		out.Body.Status = "ok"
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cleanup-builds",
		Method:      http.MethodPost,
		Path:        "/api/v1/builds/cleanup",
		Summary:     "Delete temporary build recipes",
		Description: "Delete temporary build recipes matching a prefix.",
		Tags:        []string{"builds"},
	}, func(ctx context.Context, input *BuildsCleanupInput) (*BuildsCleanupOutput, error) {
		cleanupOpts := build.CleanupOpts{
			Owner:  input.Body.Owner,
			Prefix: input.Body.Prefix,
			DryRun: input.Body.DryRun,
		}
		if input.Body.Project != "" {
			cleanupOpts.Projects = []string{input.Body.Project}
		}

		result, err := facade.Builds().Cleanup(ctx, cleanupOpts)
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("cleanup failed: %v", err))
		}

		out := &BuildsCleanupOutput{}
		out.Body.DeletedRecipes = result.DeletedRecipes
		out.Body.DeletedBranches = result.DeletedBranches
		return out, nil
	})
	// --- Retry a build ---

	huma.Register(api, huma.Operation{
		OperationID: "retry-build",
		Method:      http.MethodPost,
		Path:        "/api/v1/builds/retry",
		Summary:     "Retry a failed build",
		Description: "Retry a failed build by its self-link and artifact type.",
		Tags:        []string{"builds"},
	}, func(ctx context.Context, input *BuildsRetryInput) (*BuildsActionOutput, error) {
		if err := facade.Builds().Retry(ctx, input.Body.BuildSelfLink, input.Body.ArtifactType); err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("retry failed: %v", err))
		}
		out := &BuildsActionOutput{}
		out.Body.Status = "ok"
		return out, nil
	})

	// --- Cancel a build ---

	huma.Register(api, huma.Operation{
		OperationID: "cancel-build",
		Method:      http.MethodPost,
		Path:        "/api/v1/builds/cancel",
		Summary:     "Cancel an active build",
		Description: "Cancel an active build by its self-link and artifact type.",
		Tags:        []string{"builds"},
	}, func(ctx context.Context, input *BuildsCancelInput) (*BuildsActionOutput, error) {
		if err := facade.Builds().Cancel(ctx, input.Body.BuildSelfLink, input.Body.ArtifactType); err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("cancel failed: %v", err))
		}
		out := &BuildsActionOutput{}
		out.Body.Status = "ok"
		return out, nil
	})
}

// --- Retry/Cancel input/output types ---

// BuildsRetryInput holds the request body for retrying a build.
type BuildsRetryInput struct {
	Body struct {
		BuildSelfLink string `json:"build_self_link" doc:"Self-link of the build to retry" required:"true"`
		ArtifactType  string `json:"artifact_type" doc:"Artifact type: rock, charm, or snap" required:"true"`
	}
}

// BuildsCancelInput holds the request body for cancelling a build.
type BuildsCancelInput struct {
	Body struct {
		BuildSelfLink string `json:"build_self_link" doc:"Self-link of the build to cancel" required:"true"`
		ArtifactType  string `json:"artifact_type" doc:"Artifact type: rock, charm, or snap" required:"true"`
	}
}

// BuildsActionOutput is the response for build retry/cancel operations.
type BuildsActionOutput struct {
	Body struct {
		Status string `json:"status" doc:"Operation result"`
	}
}

func buildTriggerOptionsFromInput(input *BuildsTriggerInput) (build.TriggerOpts, error) {
	var timeout time.Duration
	var err error
	if input.Body.Timeout != "" {
		timeout, err = time.ParseDuration(input.Body.Timeout)
		if err != nil {
			return build.TriggerOpts{}, err
		}
	}

	return build.TriggerOpts{
		Wait:       input.Body.Wait,
		Timeout:    timeout,
		Owner:      input.Body.Owner,
		Prefix:     input.Body.Prefix,
		TargetRef:  input.Body.TargetRef,
		Prepared:   input.Body.Prepared,
		RetryCount: input.Body.RetryCount,
	}, nil
}
