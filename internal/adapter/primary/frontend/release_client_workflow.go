// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"context"
	"errors"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/pkg/client"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ReleasesListRequest describes one release-list workflow.
type ReleasesListRequest struct {
	Names         []string
	Projects      []string
	ArtifactType  string
	Tracks        []string
	Branches      []string
	Risks         []string
	TargetProfile string
	AllTargets    bool
}

// ReleasesShowRequest describes one release-show workflow.
type ReleasesShowRequest struct {
	Name          string
	ArtifactType  string
	Track         string
	Branch        string
	TargetProfile string
	AllTargets    bool
}

// ReleaseClientWorkflow exposes reusable client-side release workflows.
type ReleaseClientWorkflow struct {
	client      *ClientTransport
	application *app.App
}

// NewReleaseClientWorkflow creates a client-side release workflow.
func NewReleaseClientWorkflow(apiClient *ClientTransport, application *app.App) *ReleaseClientWorkflow {
	return &ReleaseClientWorkflow{client: apiClient, application: application}
}

// List lists cached published artifact release rows.
func (w *ReleaseClientWorkflow) List(ctx context.Context, req ReleasesListRequest) ([]dto.ReleaseListEntry, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	releases, err := apiClient.ReleasesList(ctx, client.ReleasesListOptions{
		Names:        req.Names,
		Projects:     req.Projects,
		ArtifactType: req.ArtifactType,
		Tracks:       req.Tracks,
		Branches:     req.Branches,
		Risks:        req.Risks,
	})
	if err != nil {
		return nil, err
	}
	return FilterReleaseListEntries(w.config(), releases, req.TargetProfile, req.AllTargets)
}

// Show returns the cached full matrix for one published artifact.
func (w *ReleaseClientWorkflow) Show(ctx context.Context, req ReleasesShowRequest) (*dto.ReleaseShowResult, error) {
	apiClient, err := w.resolveClient()
	if err != nil {
		return nil, err
	}
	result, err := apiClient.ReleasesShow(ctx, req.Name, client.ReleasesShowOptions{
		ArtifactType: req.ArtifactType,
		Track:        req.Track,
		Branch:       req.Branch,
	})
	if err != nil {
		return nil, err
	}
	project := ""
	if result != nil {
		project = result.Project
	}
	return FilterReleaseShowResult(w.config(), project, result, req.TargetProfile, req.AllTargets)
}

func (w *ReleaseClientWorkflow) resolveClient() (*ClientTransport, error) {
	if w.client == nil {
		return nil, errors.New("release client workflow requires an API client")
	}
	return w.client, nil
}

func (w *ReleaseClientWorkflow) config() *config.Config {
	if w == nil || w.application == nil {
		return nil
	}
	return w.application.GetConfig()
}
