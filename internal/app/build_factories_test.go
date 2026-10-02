// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"log/slog"
	"testing"

	lpadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/launchpad"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/build"
	lp "github.com/canonical/sunbeam-watchtower/pkg/launchpad/v1"
)

func TestBuildRecipeBuildersFromConfigSelectsArtifactFactories(t *testing.T) {
	builders, err := buildRecipeBuildersFromConfig(&config.Config{
		Projects: []config.ProjectConfig{
			{Name: "rock", ArtifactType: "rock", Code: config.CodeConfig{Project: "rock"}},
			{Name: "charm", ArtifactType: "charm", Code: config.CodeConfig{Project: "charm"}},
			{Name: "snap", ArtifactType: "snap", Code: config.CodeConfig{Project: "snap"}},
		},
	}, testAppLogger(), &lp.Client{})
	if err != nil {
		t.Fatalf("buildRecipeBuildersFromConfig() error = %v", err)
	}

	if _, ok := builders["rock"].Builder.(*lpadapter.RockBuilder); !ok {
		t.Fatalf("rock builder = %T, want *launchpad.RockBuilder", builders["rock"].Builder)
	}
	if _, ok := builders["rock"].Strategy.(*build.RockStrategy); !ok {
		t.Fatalf("rock strategy = %T, want *build.RockStrategy", builders["rock"].Strategy)
	}
	if _, ok := builders["charm"].Builder.(*lpadapter.CharmBuilder); !ok {
		t.Fatalf("charm builder = %T, want *launchpad.CharmBuilder", builders["charm"].Builder)
	}
	if _, ok := builders["charm"].Strategy.(*build.CharmStrategy); !ok {
		t.Fatalf("charm strategy = %T, want *build.CharmStrategy", builders["charm"].Strategy)
	}
	if _, ok := builders["snap"].Builder.(*lpadapter.SnapBuilder); !ok {
		t.Fatalf("snap builder = %T, want *launchpad.SnapBuilder", builders["snap"].Builder)
	}
	if _, ok := builders["snap"].Strategy.(*build.SnapStrategy); !ok {
		t.Fatalf("snap strategy = %T, want *build.SnapStrategy", builders["snap"].Strategy)
	}
}

func TestBuildRecipeBuildersFromConfigRequiresConfig(t *testing.T) {
	if _, err := buildRecipeBuildersFromConfig(nil, testAppLogger(), &lp.Client{}); err == nil {
		t.Fatal("buildRecipeBuildersFromConfig() error = nil, want missing config error")
	}
}

func TestBuildRecipeBuildersFromConfigReturnsEmptyWithoutAuth(t *testing.T) {
	builders, err := buildRecipeBuildersFromConfig(&config.Config{
		Projects: []config.ProjectConfig{
			{Name: "rock", ArtifactType: "rock", Code: config.CodeConfig{Project: "rock"}},
		},
	}, testAppLogger(), nil)
	if err != nil {
		t.Fatalf("buildRecipeBuildersFromConfig() error = %v", err)
	}
	if len(builders) != 0 {
		t.Fatalf("len(builders) = %d, want 0 without LP auth", len(builders))
	}
}

func testAppLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}
