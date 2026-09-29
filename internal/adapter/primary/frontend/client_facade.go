// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"github.com/gboutry/sunbeam-watchtower/internal/app"
)

// ClientFacade exposes the reusable client-side frontend workflows behind one stable entrypoint.
type ClientFacade struct {
	auth       *AuthClientWorkflow
	operations *OperationClientWorkflow
	projects   *ProjectClientWorkflow
	builds     *BuildWorkflow
	releases   *ReleaseClientWorkflow
	packages   *PackagesClientWorkflow
	cache      *CacheClientWorkflow
	bugs       *BugClientWorkflow
	sru        *SRUClientWorkflow
	reviews    *ReviewClientWorkflow
	commits    *CommitClientWorkflow
	config     *ConfigClientWorkflow
	teams      *TeamClientWorkflow

	localBuildPreparationErr error
}

// NewClientFacade creates a client-side frontend facade.
func NewClientFacade(apiClient *ClientTransport, application *app.App) *ClientFacade {
	builds := NewBuildWorkflow(apiClient, nil)
	var localBuildPreparationErr error
	if application != nil {
		preparer, err := NewLocalBuildPreparerFromApp(application)
		if err == nil {
			builds = NewBuildWorkflow(apiClient, preparer)
		} else {
			localBuildPreparationErr = err
		}
	}

	return &ClientFacade{
		auth:                     NewAuthClientWorkflow(apiClient),
		operations:               NewOperationClientWorkflow(apiClient),
		projects:                 NewProjectClientWorkflow(apiClient),
		builds:                   builds,
		releases:                 NewReleaseClientWorkflow(apiClient, application),
		packages:                 NewPackagesClientWorkflow(apiClient, application),
		cache:                    NewCacheClientWorkflow(apiClient),
		bugs:                     NewBugClientWorkflow(apiClient),
		sru:                      NewSRUClientWorkflow(apiClient),
		reviews:                  NewReviewClientWorkflow(apiClient),
		commits:                  NewCommitClientWorkflow(apiClient),
		config:                   NewConfigClientWorkflow(apiClient),
		teams:                    NewTeamClientWorkflow(apiClient),
		localBuildPreparationErr: localBuildPreparationErr,
	}
}

// Auth returns reusable auth workflows.
func (f *ClientFacade) Auth() *AuthClientWorkflow { return f.auth }

// Operations returns reusable operation workflows.
func (f *ClientFacade) Operations() *OperationClientWorkflow { return f.operations }

// Projects returns reusable project workflows.
func (f *ClientFacade) Projects() *ProjectClientWorkflow { return f.projects }

// Builds returns reusable build workflows.
func (f *ClientFacade) Builds() *BuildWorkflow { return f.builds }

// Releases returns reusable release workflows.
func (f *ClientFacade) Releases() *ReleaseClientWorkflow { return f.releases }

// Packages returns reusable packages workflows.
func (f *ClientFacade) Packages() *PackagesClientWorkflow { return f.packages }

// Cache returns reusable cache workflows.
func (f *ClientFacade) Cache() *CacheClientWorkflow { return f.cache }

// Bugs returns reusable bug workflows.
func (f *ClientFacade) Bugs() *BugClientWorkflow { return f.bugs }

// SRU returns the SRU monitoring workflow.
func (f *ClientFacade) SRU() *SRUClientWorkflow { return f.sru }

// Reviews returns reusable review workflows.
func (f *ClientFacade) Reviews() *ReviewClientWorkflow { return f.reviews }

// Commits returns reusable commit workflows.
func (f *ClientFacade) Commits() *CommitClientWorkflow { return f.commits }

// Config returns reusable config workflows.
func (f *ClientFacade) Config() *ConfigClientWorkflow { return f.config }

// Teams returns reusable team workflows.
func (f *ClientFacade) Teams() *TeamClientWorkflow { return f.teams }

// LocalBuildPreparationError reports whether local build preparation could be wired from the application.
func (f *ClientFacade) LocalBuildPreparationError() error { return f.localBuildPreparationErr }
