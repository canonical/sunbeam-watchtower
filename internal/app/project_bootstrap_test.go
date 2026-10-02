package app

import (
	"log/slog"
	"testing"

	"github.com/canonical/sunbeam-watchtower/internal/config"
)

func TestBuildProjectSyncConfigsUsesProjectOverrides(t *testing.T) {
	application := NewApp(&config.Config{
		Launchpad: config.LaunchpadConfig{
			Series:           []string{"2024.1", "2024.2"},
			DevelopmentFocus: "2024.2",
		},
		Projects: []config.ProjectConfig{
			{
				Name: "demo-one",
				Bugs: []config.BugTrackerConfig{{Forge: "launchpad", Project: "lp-one"}},
			},
			{
				Name:             "demo-two",
				Series:           []string{"2025.1"},
				DevelopmentFocus: "2025.1",
				Bugs:             []config.BugTrackerConfig{{Forge: "launchpad", Project: "lp-two"}},
			},
		},
	}, slog.Default())

	configs, err := application.BuildProjectSyncConfigs()
	if err != nil {
		t.Fatalf("BuildProjectSyncConfigs() error = %v", err)
	}

	if got := configs["lp-one"]; len(got.Series) != 2 || got.DevelopmentFocus != "2024.2" {
		t.Fatalf("unexpected lp-one config: %+v", got)
	}
	if got := configs["lp-two"]; len(got.Series) != 1 || got.Series[0] != "2025.1" || got.DevelopmentFocus != "2025.1" {
		t.Fatalf("unexpected lp-two config: %+v", got)
	}
}
