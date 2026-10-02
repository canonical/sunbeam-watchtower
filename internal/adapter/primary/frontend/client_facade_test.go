// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/pkg/client"
)

func TestNewClientFacadeProvidesWorkflows(t *testing.T) {
	facade := NewClientFacade(NewClientTransport(client.NewClient("http://example.invalid")), nil)

	if facade.Auth() == nil || facade.Operations() == nil || facade.Projects() == nil {
		t.Fatal("expected auth/operations/projects workflows to be wired")
	}
	if facade.Builds() == nil || facade.Packages() == nil || facade.Cache() == nil {
		t.Fatal("expected builds/packages/cache workflows to be wired")
	}
	if facade.Bugs() == nil || facade.Reviews() == nil || facade.Commits() == nil || facade.Config() == nil {
		t.Fatal("expected bug/review/commit/config workflows to be wired")
	}
	if facade.LocalBuildPreparationError() != nil {
		t.Fatalf("LocalBuildPreparationError() = %v, want nil without app wiring", facade.LocalBuildPreparationError())
	}
}

func TestNewClientFacadeTracksLocalBuildPreparationStatus(t *testing.T) {
	application := app.NewApp(&config.Config{}, discardFrontendLogger())

	facade := NewClientFacade(NewClientTransport(client.NewClient("http://example.invalid")), application)

	if facade.Builds() == nil {
		t.Fatal("Builds() = nil, want workflow")
	}
	if facade.Packages() == nil || facade.Packages().application != application {
		t.Fatal("expected packages workflow to retain application wiring")
	}
	if facade.LocalBuildPreparationError() == nil && facade.Builds().preparer == nil {
		t.Fatal("expected local build preparer when no initialization error is reported")
	}
}
