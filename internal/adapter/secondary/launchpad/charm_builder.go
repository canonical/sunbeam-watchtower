// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package launchpad

import (
	"context"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"

	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

// CharmBuilder adapts LP charm recipe operations to the dto.RecipeBuilder interface.
type CharmBuilder struct {
	client *lp.Client
}

var _ port.RecipeBuilder = (*CharmBuilder)(nil)

// NewCharmBuilder creates a new CharmBuilder backed by the given LP client.
func NewCharmBuilder(client *lp.Client) *CharmBuilder {
	return &CharmBuilder{client: client}
}

func (b *CharmBuilder) ArtifactType() dto.ArtifactType {
	return dto.ArtifactCharm
}

func (b *CharmBuilder) GetRecipe(ctx context.Context, owner, project, name string) (*dto.Recipe, error) {
	r, err := b.client.GetCharmRecipe(ctx, owner, project, name)
	if err != nil {
		return nil, err
	}
	return charmRecipeToPortRecipe(r), nil
}

func (b *CharmBuilder) CreateRecipe(ctx context.Context, opts dto.CreateRecipeOpts) (*dto.Recipe, error) {
	r, err := b.client.CreateCharmRecipe(ctx, lp.CreateCharmRecipeOpts{
		Name:       opts.Name,
		Owner:      opts.Owner,
		Project:    opts.Project,
		GitRefLink: opts.GitRefLink,
		BuildPath:  opts.BuildPath,
		Channels:   opts.Channels,
	})
	if err != nil {
		return nil, err
	}
	return charmRecipeToPortRecipe(r), nil
}

func (b *CharmBuilder) DeleteRecipe(ctx context.Context, recipeSelfLink string) error {
	return b.client.DeleteCharmRecipe(ctx, recipeSelfLink)
}

func (b *CharmBuilder) ListRecipesByOwner(ctx context.Context, owner string) ([]*dto.Recipe, error) {
	recipes, err := b.client.FindCharmRecipesByOwner(ctx, owner)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.Recipe, len(recipes))
	for i, r := range recipes {
		out[i] = charmRecipeToPortRecipe(r)
	}
	return out, nil
}

// SetProcessors is a no-op for charm recipes — LP charm_recipe has no
// processors field; architectures are passed per-build via requestBuilds.
func (b *CharmBuilder) SetProcessors(_ context.Context, _ *dto.Recipe, _ []string) error {
	return nil
}

func (b *CharmBuilder) RequestBuilds(ctx context.Context, recipe *dto.Recipe, opts dto.RequestBuildsOpts) (*dto.BuildRequest, error) {
	// Charm recipes do not take archive/pocket — LP determines the build
	// environment from the charmcraft.yaml bases/platforms declaration.
	// Only channels (for snap tool versions) is passed.
	br, err := b.client.RequestCharmRecipeBuilds(ctx, recipe.SelfLink, "", "", opts.Channels, opts.Architectures)
	if err != nil {
		return nil, err
	}
	return buildRequestToPort(br), nil
}

func (b *CharmBuilder) ListBuilds(ctx context.Context, recipe *dto.Recipe) ([]dto.Build, error) {
	builds, err := b.client.GetCharmRecipeBuilds(ctx, recipe.SelfLink)
	if err != nil {
		return nil, err
	}
	out := make([]dto.Build, len(builds))
	for i, lb := range builds {
		out[i] = charmBuildToPortBuild(lb, recipe.Name)
	}
	return out, nil
}

func (b *CharmBuilder) RetryBuild(ctx context.Context, buildSelfLink string) error {
	return b.client.RetryCharmRecipeBuild(ctx, buildSelfLink)
}

func (b *CharmBuilder) CancelBuild(ctx context.Context, buildSelfLink string) error {
	return b.client.CancelCharmRecipeBuild(ctx, buildSelfLink)
}

func (b *CharmBuilder) GetBuildFileURLs(ctx context.Context, buildSelfLink string) ([]string, error) {
	return b.client.GetCharmRecipeBuildFileURLs(ctx, buildSelfLink)
}

// charmRecipeToPortRecipe converts an LP CharmRecipe to a dto.Recipe.
func charmRecipeToPortRecipe(r lp.CharmRecipe) *dto.Recipe {
	return &dto.Recipe{
		Name:         r.Name,
		ArtifactType: dto.ArtifactCharm,
		Owner:        extractNameFromLink(r.OwnerLink),
		Project:      extractNameFromLink(r.ProjectLink),
		SelfLink:     r.SelfLink,
		WebLink:      r.WebLink,
		GitPath:      r.GitPath,
		BuildPath:    r.BuildPath,
		AutoBuild:    r.AutoBuild,
		CreatedAt:    timeOrZero(r.DateCreated),
	}
}

// charmBuildToPortBuild converts an LP CharmRecipeBuild to a dto.Build.
func charmBuildToPortBuild(b lp.CharmRecipeBuild, recipeName string) dto.Build {
	return dto.Build{
		Recipe:       recipeName,
		ArtifactType: dto.ArtifactCharm,
		Title:        b.Title,
		State:        parseBuildState(b.BuildState),
		Arch:         b.ArchTag,
		BuildLogURL:  b.BuildLogURL,
		WebLink:      b.WebLink,
		SelfLink:     b.SelfLink,
		CanRetry:     b.CanBeRetried,
		CanCancel:    b.CanBeCancelled,
		CreatedAt:    timeOrZero(b.DateCreated),
		StartedAt:    timeOrZero(b.DateStarted),
		BuiltAt:      timeOrZero(b.DateBuilt),
	}
}

// timeOrZero safely dereferences an *lp.Time, returning zero time if nil.
func timeOrZero(t *lp.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.Time
}

// parseBuildState maps LP build state strings to dto.BuildState.
func parseBuildState(lpState string) dto.BuildState {
	switch lpState {
	case "Needs building", "Dependency wait":
		return dto.BuildPending
	case "Building", "Currently building", "Uploading build", "Gathering build output":
		return dto.BuildBuilding
	case "Successfully built":
		return dto.BuildSucceeded
	case "Failed to build", "Failed to upload", "Chroot problem":
		return dto.BuildFailed
	case "Cancelled":
		return dto.BuildCancelled
	case "Cancelling build":
		return dto.BuildCancelling
	case "Superseded":
		return dto.BuildSuperseded
	default:
		return dto.BuildPending
	}
}

// extractNameFromLink extracts the last path segment from an LP link (e.g. "~owner" → "~owner").
func extractNameFromLink(link string) string {
	if link == "" {
		return ""
	}
	for i := len(link) - 1; i >= 0; i-- {
		if link[i] == '/' {
			return link[i+1:]
		}
	}
	return link
}

// buildRequestToPort converts an LP BuildRequest to a dto.BuildRequest.
func buildRequestToPort(br lp.BuildRequest) *dto.BuildRequest {
	return &dto.BuildRequest{
		SelfLink:             br.SelfLink,
		WebLink:              br.WebLink,
		Status:               br.Status,
		ErrorMessage:         br.ErrorMessage,
		BuildsCollectionLink: br.BuildsCollectionLink,
	}
}
