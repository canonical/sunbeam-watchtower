// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
)

func TestNewServerFacadeProvidesWorkflows(t *testing.T) {
	facade := NewServerFacade(app.NewApp(&config.Config{}, discardFrontendLogger()))
	if facade.Auth() == nil {
		t.Fatal("Auth() = nil")
	}
	if facade.Operations() == nil {
		t.Fatal("Operations() = nil")
	}
	if facade.Builds() == nil {
		t.Fatal("Builds() = nil")
	}
	if facade.Projects() == nil {
		t.Fatal("Projects() = nil")
	}
	if facade.Bugs() == nil {
		t.Fatal("Bugs() = nil")
	}
	if facade.Reviews() == nil {
		t.Fatal("Reviews() = nil")
	}
	if facade.Commits() == nil {
		t.Fatal("Commits() = nil")
	}
	if facade.Config() == nil {
		t.Fatal("Config() = nil")
	}
	if facade.Teams() == nil {
		t.Fatal("Teams() = nil")
	}
}
