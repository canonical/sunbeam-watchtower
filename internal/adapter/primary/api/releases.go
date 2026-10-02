// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/canonical/sunbeam-watchtower/internal/app"
	releasesvc "github.com/canonical/sunbeam-watchtower/internal/core/service/release"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ReleasesListInput holds query parameters for listing cached published artifacts.
type ReleasesListInput struct {
	Names        []string `query:"name" required:"false" doc:"Filter by published artifact name"`
	Projects     []string `query:"project" required:"false" doc:"Filter by watchtower project"`
	ArtifactType string   `query:"type" required:"false" doc:"Filter by artifact type (snap|charm)"`
	Tracks       []string `query:"track" required:"false" doc:"Filter by track"`
	Branches     []string `query:"branch" required:"false" doc:"Filter by branch"`
	Risks        []string `query:"risk" required:"false" doc:"Filter by risk (edge|beta|candidate|stable)"`
}

// ReleasesListOutput is the response for listing cached published artifacts.
type ReleasesListOutput struct {
	Body struct {
		Releases []dto.ReleaseListEntry `json:"releases"`
	}
}

// ReleasesShowInput holds parameters for showing one cached artifact release matrix.
type ReleasesShowInput struct {
	Name         string `path:"name" doc:"Published artifact name"`
	ArtifactType string `query:"type" required:"false" doc:"Artifact type to disambiguate duplicate names (snap|charm)"`
	Track        string `query:"track" required:"false" doc:"Optional track filter"`
	Branch       string `query:"branch" required:"false" doc:"Optional branch filter"`
}

// ReleasesShowOutput is the response for showing one cached artifact release matrix.
type ReleasesShowOutput struct {
	Body dto.ReleaseShowResult
}

// RegisterReleasesAPI registers the release-tracking endpoints on the given huma API.
func RegisterReleasesAPI(api huma.API, application *app.App) {
	facade := frontend.NewServerFacade(application)

	huma.Register(api, huma.Operation{
		OperationID: "list-releases",
		Method:      http.MethodGet,
		Path:        "/api/v1/releases",
		Summary:     "List cached published snap and charm releases",
		Tags:        []string{"releases"},
	}, func(ctx context.Context, input *ReleasesListInput) (*ReleasesListOutput, error) {
		query, err := releaseListQueryFromInput(input)
		if err != nil {
			return nil, huma.Error422UnprocessableEntity("invalid release query", err)
		}
		releases, err := facade.Releases().List(ctx, query)
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to list releases: %v", err))
		}
		out := &ReleasesListOutput{}
		out.Body.Releases = releases
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "show-release",
		Method:      http.MethodGet,
		Path:        "/api/v1/releases/{name}",
		Summary:     "Show the cached publication matrix for one snap or charm",
		Tags:        []string{"releases"},
	}, func(ctx context.Context, input *ReleasesShowInput) (*ReleasesShowOutput, error) {
		artifactType, err := parseOptionalArtifactType(input.ArtifactType)
		if err != nil {
			return nil, huma.Error422UnprocessableEntity("invalid artifact type", err)
		}
		result, err := facade.Releases().Show(ctx, input.Name, artifactType, input.Track, input.Branch)
		if err != nil {
			switch {
			case errors.Is(err, releasesvc.ErrNotFound):
				return nil, huma.Error404NotFound(fmt.Sprintf("release %q not found", input.Name))
			case errors.Is(err, releasesvc.ErrAmbiguous):
				return nil, huma.Error409Conflict(releaseAmbiguousMessage(input.Name, err))
			default:
				return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to show release: %v", err))
			}
		}
		out := &ReleasesShowOutput{}
		out.Body = *result
		return out, nil
	})
}

func releaseListQueryFromInput(input *ReleasesListInput) (dto.ReleaseListQuery, error) {
	artifactType, err := parseOptionalArtifactType(input.ArtifactType)
	if err != nil {
		return dto.ReleaseListQuery{}, err
	}
	risks := make([]dto.ReleaseRisk, 0, len(input.Risks))
	for _, risk := range input.Risks {
		parsed, err := dto.ParseReleaseRisk(risk)
		if err != nil {
			return dto.ReleaseListQuery{}, err
		}
		risks = append(risks, parsed)
	}
	return dto.ReleaseListQuery{
		Names:        input.Names,
		Projects:     input.Projects,
		ArtifactType: artifactType,
		Tracks:       input.Tracks,
		Branches:     input.Branches,
		Risks:        risks,
	}, nil
}

func parseOptionalArtifactType(raw string) (*dto.ArtifactType, error) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := dto.ParseArtifactType(raw)
	if err != nil {
		return nil, err
	}
	if parsed == dto.ArtifactRock {
		return nil, fmt.Errorf("release tracking supports only snap and charm artifacts")
	}
	return &parsed, nil
}

func releaseAmbiguousMessage(name string, err error) string {
	var ambiguousErr *releasesvc.AmbiguousReleaseError
	if !errors.As(err, &ambiguousErr) || len(ambiguousErr.ArtifactTypes) == 0 {
		return fmt.Sprintf("release %q matched multiple artifact types; use the type filter", name)
	}
	types := make([]string, 0, len(ambiguousErr.ArtifactTypes))
	for _, artifactType := range ambiguousErr.ArtifactTypes {
		types = append(types, artifactType.String())
	}
	return fmt.Sprintf("release %q matched multiple artifact types (%s); use the type filter", name, strings.Join(types, ", "))
}
