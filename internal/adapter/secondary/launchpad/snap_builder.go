// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package launchpad

import (
	"context"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"

	"github.com/canonical/sunbeam-watchtower/internal/core/port"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

// SnapBuilder implements dto.RecipeBuilder for snap artifacts.
// Key differences from rock/charm:
// - GetRecipe ignores the project parameter (snap path is /~owner/+snap/name)
// - RequestBuilds requires archiveLink and pocket from opts
type SnapBuilder struct {
	client         *lp.Client
	defaultArchive string // default Ubuntu archive link
	defaultPocket  string // default pocket (e.g. "Updates")
}

var _ port.RecipeBuilder = (*SnapBuilder)(nil)

func NewSnapBuilder(client *lp.Client, defaultArchive, defaultPocket string) *SnapBuilder {
	return &SnapBuilder{
		client:         client,
		defaultArchive: defaultArchive,
		defaultPocket:  defaultPocket,
	}
}

func (s *SnapBuilder) ArtifactType() dto.ArtifactType {
	return dto.ArtifactSnap
}

func (s *SnapBuilder) GetRecipe(ctx context.Context, owner, _ string, name string) (*dto.Recipe, error) {
	snap, err := s.client.GetSnap(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	return snapToPortRecipe(snap), nil
}

func (s *SnapBuilder) CreateRecipe(ctx context.Context, opts dto.CreateRecipeOpts) (*dto.Recipe, error) {
	snap, err := s.client.CreateSnap(ctx, lp.CreateSnapOpts{
		Name:        opts.Name,
		Owner:       opts.Owner,
		GitRefLink:  opts.GitRefLink,
		Description: opts.Name,
		Processors:  opts.Processors,
	})
	if err != nil {
		return nil, err
	}
	return snapToPortRecipe(snap), nil
}

func (s *SnapBuilder) SetProcessors(ctx context.Context, recipe *dto.Recipe, processors []string) error {
	if recipe == nil || recipe.SelfLink == "" {
		return nil
	}
	if len(processors) == 0 {
		return nil
	}
	return s.client.SetSnapProcessors(ctx, recipe.SelfLink, processors)
}

func (s *SnapBuilder) DeleteRecipe(ctx context.Context, selfLink string) error {
	return s.client.DeleteSnap(ctx, selfLink)
}

func (s *SnapBuilder) ListRecipesByOwner(ctx context.Context, owner string) ([]*dto.Recipe, error) {
	snaps, err := s.client.FindSnapsByOwner(ctx, owner)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.Recipe, len(snaps))
	for i, snap := range snaps {
		out[i] = snapToPortRecipe(snap)
	}
	return out, nil
}

func (s *SnapBuilder) RequestBuilds(ctx context.Context, recipe *dto.Recipe, opts dto.RequestBuildsOpts) (*dto.BuildRequest, error) {
	archiveLink := s.defaultArchive
	if opts.ArchiveLink != "" {
		archiveLink = opts.ArchiveLink
	}
	if archiveLink == "" {
		archiveLink = "/ubuntu/+archive/primary"
	}
	pocket := s.defaultPocket
	if opts.Pocket != "" {
		pocket = opts.Pocket
	}
	if pocket == "" {
		pocket = "Updates"
	}

	br, err := s.client.RequestSnapBuilds(ctx, recipe.SelfLink, archiveLink, pocket, opts.Channels)
	if err != nil {
		return nil, err
	}
	return buildRequestToPort(br), nil
}

func (s *SnapBuilder) ListBuilds(ctx context.Context, recipe *dto.Recipe) ([]dto.Build, error) {
	builds, err := s.client.GetSnapBuilds(ctx, recipe.SelfLink)
	if err != nil {
		return nil, err
	}
	out := make([]dto.Build, len(builds))
	for i, b := range builds {
		out[i] = snapBuildToPortBuild(b, recipe.Name)
	}
	return out, nil
}

func (s *SnapBuilder) RetryBuild(ctx context.Context, selfLink string) error {
	return s.client.RetrySnapBuild(ctx, selfLink)
}

func (s *SnapBuilder) CancelBuild(ctx context.Context, selfLink string) error {
	return s.client.CancelSnapBuild(ctx, selfLink)
}

func (s *SnapBuilder) GetBuildFileURLs(ctx context.Context, selfLink string) ([]string, error) {
	return s.client.GetSnapBuildFileURLs(ctx, selfLink)
}

func snapToPortRecipe(s lp.Snap) *dto.Recipe {
	return &dto.Recipe{
		Name:         s.Name,
		ArtifactType: dto.ArtifactSnap,
		Owner:        extractNameFromLink(s.OwnerLink),
		Project:      "",
		SelfLink:     s.SelfLink,
		WebLink:      s.WebLink,
		GitPath:      s.GitPath,
		BuildPath:    "",
		AutoBuild:    s.AutoBuild,
		CreatedAt:    timeOrZero(s.DateCreated),
	}
}

func snapBuildToPortBuild(b lp.SnapBuild, recipeName string) dto.Build {
	return dto.Build{
		Recipe:       recipeName,
		ArtifactType: dto.ArtifactSnap,
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
