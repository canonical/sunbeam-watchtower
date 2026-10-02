// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package launchpad

import (
	"context"
	"fmt"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

// ProjectManager implements port.ProjectManager using the Launchpad API.
type ProjectManager struct {
	client *lp.Client
}

// NewProjectManager creates a new Launchpad project manager.
func NewProjectManager(client *lp.Client) *ProjectManager {
	return &ProjectManager{client: client}
}

func (m *ProjectManager) GetProject(ctx context.Context, projectName string) (*forge.Project, error) {
	p, err := m.client.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	return &forge.Project{
		Name:                 p.Name,
		SelfLink:             p.SelfLink,
		DevelopmentFocusLink: p.DevelopmentFocusLink,
	}, nil
}

func (m *ProjectManager) GetProjectSeries(ctx context.Context, projectName string) ([]forge.ProjectSeries, error) {
	col, err := m.client.GetProjectSeries(ctx, projectName)
	if err != nil {
		return nil, fmt.Errorf("fetching series for %s: %w", projectName, err)
	}
	result := make([]forge.ProjectSeries, 0, len(col.Entries))
	for _, s := range col.Entries {
		result = append(result, forge.ProjectSeries{
			Name:     s.Name,
			SelfLink: s.SelfLink,
			Active:   s.Active,
		})
	}
	return result, nil
}

func (m *ProjectManager) CreateSeries(ctx context.Context, projectName, seriesName, summary string) (*forge.ProjectSeries, error) {
	s, err := m.client.CreateProjectSeries(ctx, projectName, seriesName, summary)
	if err != nil {
		return nil, err
	}
	return &forge.ProjectSeries{
		Name:     s.Name,
		SelfLink: s.SelfLink,
		Active:   s.Active,
	}, nil
}

func (m *ProjectManager) SetDevelopmentFocus(ctx context.Context, projectName, seriesSelfLink string) error {
	return m.client.SetDevelopmentFocus(ctx, projectName, seriesSelfLink)
}
