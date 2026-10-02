package app

import (
	"log/slog"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/internal/testsupport"
)

func TestBuildRecipeBuildersUsesProjectAndGlobalDefaults(t *testing.T) {
	testsupport.ClearForgeCredentials(t)
	t.Setenv("LP_ACCESS_TOKEN", "token")
	t.Setenv("LP_ACCESS_TOKEN_SECRET", "secret")

	application := NewApp(&config.Config{
		Launchpad: config.LaunchpadConfig{
			Series:           []string{"2024.1", "2024.2"},
			DevelopmentFocus: "2024.2",
		},
		Projects: []config.ProjectConfig{
			{
				Name:         "demo-rock",
				ArtifactType: "rock",
				Code:         config.CodeConfig{Forge: "github", Owner: "canonical", Project: "demo-rock"},
				Build: &config.ProjectBuildConfig{
					Owner:               "team-a",
					Artifacts:           []string{"keystone"},
					LPProject:           "demo-rock-lp",
					OfficialCodehosting: true,
				},
			},
			{
				Name:             "demo-charm",
				ArtifactType:     "charm",
				Code:             config.CodeConfig{Forge: "github", Owner: "canonical", Project: "demo-charm"},
				Series:           []string{"2025.1"},
				DevelopmentFocus: "2025.1",
			},
		},
	}, slog.Default())

	builders, err := application.BuildRecipeBuilders()
	if err != nil {
		t.Fatalf("BuildRecipeBuilders() error = %v", err)
	}

	rock := builders["demo-rock"]
	if rock.Owner != "team-a" || rock.LPProject != "demo-rock-lp" || !rock.OfficialCodehosting {
		t.Fatalf("unexpected rock builder: %+v", rock)
	}
	if len(rock.Series) != 2 || rock.Series[0] != "2024.1" || rock.DevFocus != "2024.2" {
		t.Fatalf("unexpected rock series defaults: %+v", rock)
	}

	charm := builders["demo-charm"]
	if len(charm.Series) != 1 || charm.Series[0] != "2025.1" || charm.DevFocus != "2025.1" {
		t.Fatalf("unexpected charm overrides: %+v", charm)
	}
}
