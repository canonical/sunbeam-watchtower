// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import "github.com/gboutry/sunbeam-watchtower/internal/app"

// ServerFacade exposes reusable server-side frontend workflows behind one stable entrypoint.
type ServerFacade struct {
	auth       *AuthWorkflow
	operations *OperationWorkflow
	builds     *BuildServerWorkflow
	releases   *ReleaseServerWorkflow
	projects   *ProjectServerWorkflow
	bugs       *BugServerWorkflow
	sru        *SRUServerWorkflow
	reviews    *ReviewServerWorkflow
	commits    *CommitServerWorkflow
	config     *ConfigServerWorkflow
	teams      *TeamServerWorkflow
}

// NewServerFacade creates a server-side frontend facade.
func NewServerFacade(application *app.App) *ServerFacade {
	async := NewFacade(application)

	return &ServerFacade{
		auth:       NewAuthWorkflow(application),
		operations: NewOperationWorkflow(application),
		builds:     NewBuildServerWorkflow(application, async),
		releases:   NewReleaseServerWorkflow(application),
		projects:   NewProjectServerWorkflow(application, async),
		bugs:       NewBugServerWorkflow(application),
		sru:        NewSRUServerWorkflow(application),
		reviews:    NewReviewServerWorkflow(application),
		commits:    NewCommitServerWorkflow(application),
		config:     NewConfigServerWorkflow(application),
		teams:      NewTeamServerWorkflow(application, async),
	}
}

// Auth returns reusable auth workflows.
func (f *ServerFacade) Auth() *AuthWorkflow { return f.auth }

// Operations returns reusable operation workflows.
func (f *ServerFacade) Operations() *OperationWorkflow { return f.operations }

// Builds returns reusable build workflows.
func (f *ServerFacade) Builds() *BuildServerWorkflow { return f.builds }

// Releases returns reusable release workflows.
func (f *ServerFacade) Releases() *ReleaseServerWorkflow { return f.releases }

// Projects returns reusable project workflows.
func (f *ServerFacade) Projects() *ProjectServerWorkflow { return f.projects }

// Bugs returns reusable bug workflows.
func (f *ServerFacade) Bugs() *BugServerWorkflow { return f.bugs }

// SRU returns the SRU monitoring workflow.
func (f *ServerFacade) SRU() *SRUServerWorkflow { return f.sru }

// Reviews returns reusable review workflows.
func (f *ServerFacade) Reviews() *ReviewServerWorkflow { return f.reviews }

// Commits returns reusable commit workflows.
func (f *ServerFacade) Commits() *CommitServerWorkflow { return f.commits }

// Config returns reusable config workflows.
func (f *ServerFacade) Config() *ConfigServerWorkflow { return f.config }

// Teams returns reusable team workflows.
func (f *ServerFacade) Teams() *TeamServerWorkflow { return f.teams }
