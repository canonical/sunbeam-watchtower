// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package port

import (
	"context"

	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

// ProjectManager manages LP project metadata (series, development focus).
type ProjectManager interface {
	// GetProject returns project information.
	GetProject(ctx context.Context, projectName string) (*forge.Project, error)

	// GetProjectSeries returns the series for a project.
	GetProjectSeries(ctx context.Context, projectName string) ([]forge.ProjectSeries, error)

	// CreateSeries creates a new series on a project.
	CreateSeries(ctx context.Context, projectName, seriesName, summary string) (*forge.ProjectSeries, error)

	// SetDevelopmentFocus sets the development focus of a project to the given series.
	SetDevelopmentFocus(ctx context.Context, projectName, seriesSelfLink string) error
}
