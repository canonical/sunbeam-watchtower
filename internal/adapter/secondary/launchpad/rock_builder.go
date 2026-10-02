// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package launchpad

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

// RockBuilder implements dto.RecipeBuilder for rock artifacts.
type RockBuilder struct {
	client *lp.Client
}

var _ port.RecipeBuilder = (*RockBuilder)(nil)

func NewRockBuilder(client *lp.Client) *RockBuilder {
	return &RockBuilder{client: client}
}

func (c *RockBuilder) ArtifactType() dto.ArtifactType {
	return dto.ArtifactRock
}

func (c *RockBuilder) GetRecipe(ctx context.Context, owner, project, name string) (*dto.Recipe, error) {
	r, err := c.client.GetRockRecipe(ctx, owner, project, name)
	if err != nil {
		return nil, err
	}
	return rockRecipeToPortRecipe(r), nil
}

func (c *RockBuilder) CreateRecipe(ctx context.Context, opts dto.CreateRecipeOpts) (*dto.Recipe, error) {
	r, err := c.client.CreateRockRecipe(ctx, lp.CreateRockRecipeOpts{
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
	return rockRecipeToPortRecipe(r), nil
}

func (c *RockBuilder) DeleteRecipe(ctx context.Context, selfLink string) error {
	return c.client.DeleteRockRecipe(ctx, selfLink)
}

func (c *RockBuilder) ListRecipesByOwner(ctx context.Context, owner string) ([]*dto.Recipe, error) {
	recipes, err := c.client.FindRockRecipesByOwner(ctx, owner)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.Recipe, len(recipes))
	for i, r := range recipes {
		out[i] = rockRecipeToPortRecipe(r)
	}
	return out, nil
}

// SetProcessors is a no-op for rock recipes — LP rock_recipe has no
// processors field; architectures are passed per-build via requestBuilds.
func (c *RockBuilder) SetProcessors(_ context.Context, _ *dto.Recipe, _ []string) error {
	return nil
}

func (c *RockBuilder) RequestBuilds(ctx context.Context, recipe *dto.Recipe, opts dto.RequestBuildsOpts) (*dto.BuildRequest, error) {
	archiveLink := opts.ArchiveLink
	if archiveLink == "" {
		archiveLink = "/ubuntu/+archive/primary"
	}
	pocket := opts.Pocket
	if pocket == "" {
		pocket = "Updates"
	}
	br, err := c.client.RequestRockRecipeBuilds(ctx, recipe.SelfLink, archiveLink, pocket, opts.Channels, opts.Architectures)
	if err != nil {
		return nil, err
	}
	return buildRequestToPort(br), nil
}

func (c *RockBuilder) ListBuilds(ctx context.Context, recipe *dto.Recipe) ([]dto.Build, error) {
	builds, err := c.client.GetRockRecipeBuilds(ctx, recipe.SelfLink)
	if err != nil {
		return nil, err
	}
	out := make([]dto.Build, len(builds))
	for i, b := range builds {
		out[i] = rockBuildToPortBuild(b, recipe.Name)
	}
	return out, nil
}

func (c *RockBuilder) RetryBuild(ctx context.Context, selfLink string) error {
	return c.client.RetryRockRecipeBuild(ctx, selfLink)
}

func (c *RockBuilder) CancelBuild(ctx context.Context, selfLink string) error {
	return c.client.CancelRockRecipeBuild(ctx, selfLink)
}

func (c *RockBuilder) GetBuildFileURLs(ctx context.Context, selfLink string) ([]string, error) {
	return c.client.GetRockRecipeBuildFileURLs(ctx, selfLink)
}

func rockRecipeToPortRecipe(r lp.RockRecipe) *dto.Recipe {
	return &dto.Recipe{
		Name:         r.Name,
		ArtifactType: dto.ArtifactRock,
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

func rockBuildToPortBuild(b lp.RockRecipeBuild, recipeName string) dto.Build {
	return dto.Build{
		Recipe:       recipeName,
		ArtifactType: dto.ArtifactRock,
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
