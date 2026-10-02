// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"
	"fmt"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/build"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// BuildServerWorkflow exposes reusable server-side build workflows for the HTTP API.
type BuildServerWorkflow struct {
	application *app.App
	async       *Facade
}

// NewBuildServerWorkflow creates a server-side build workflow.
func NewBuildServerWorkflow(application *app.App, async *Facade) *BuildServerWorkflow {
	return &BuildServerWorkflow{
		application: application,
		async:       async,
	}
}

// Trigger runs one synchronous build trigger.
func (w *BuildServerWorkflow) Trigger(ctx context.Context, project string, artifacts []string, opts build.TriggerOpts) (*dto.BuildTriggerResult, error) {
	service, err := w.application.BuildService()
	if err != nil {
		return nil, err
	}
	return service.Trigger(ctx, project, artifacts, opts)
}

// StartTrigger queues one asynchronous build trigger.
func (w *BuildServerWorkflow) StartTrigger(ctx context.Context, project string, artifacts []string, opts build.TriggerOpts) (*dto.OperationJob, error) {
	return w.async.StartBuildTrigger(ctx, BuildTriggerOptions{
		Project:   project,
		Artifacts: artifacts,
		Trigger:   opts,
	})
}

// List returns build records matching the requested filters.
func (w *BuildServerWorkflow) List(ctx context.Context, opts build.ListOpts) ([]dto.Build, error) {
	service, err := w.application.BuildService()
	if err != nil {
		return nil, err
	}
	builds, _, err := service.List(ctx, opts)
	return builds, err
}

// Download downloads build artifacts for the requested filters.
func (w *BuildServerWorkflow) Download(ctx context.Context, opts build.DownloadOpts) error {
	service, err := w.application.BuildService()
	if err != nil {
		return err
	}
	return service.Download(ctx, opts)
}

// Cleanup removes temporary build recipes and branches.
func (w *BuildServerWorkflow) Cleanup(ctx context.Context, opts build.CleanupOpts) (*build.CleanupResult, error) {
	service, err := w.application.BuildService()
	if err != nil {
		return nil, err
	}
	return service.Cleanup(ctx, opts)
}

// Retry retries a failed build by finding the matching builder for the artifact type.
func (w *BuildServerWorkflow) Retry(ctx context.Context, buildSelfLink, artifactType string) error {
	builders, err := w.application.BuildRecipeBuilders()
	if err != nil {
		return err
	}
	for _, pb := range builders {
		if pb.Builder.ArtifactType().String() == artifactType {
			return pb.Builder.RetryBuild(ctx, buildSelfLink)
		}
	}
	return fmt.Errorf("no builder for artifact type %q", artifactType)
}

// Cancel cancels an active build by finding the matching builder for the artifact type.
func (w *BuildServerWorkflow) Cancel(ctx context.Context, buildSelfLink, artifactType string) error {
	builders, err := w.application.BuildRecipeBuilders()
	if err != nil {
		return err
	}
	for _, pb := range builders {
		if pb.Builder.ArtifactType().String() == artifactType {
			return pb.Builder.CancelBuild(ctx, buildSelfLink)
		}
	}
	return fmt.Errorf("no builder for artifact type %q", artifactType)
}

// DefaultArtifactsDir returns the configured build artifact output directory.
func (w *BuildServerWorkflow) DefaultArtifactsDir() (string, error) {
	if w.application == nil || w.application.GetConfig() == nil {
		return "", errors.New("no configuration loaded")
	}
	return w.application.GetConfig().Build.ArtifactsDir, nil
}
