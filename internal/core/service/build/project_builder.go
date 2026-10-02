// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"github.com/canonical/sunbeam-watchtower/internal/core/port"
)

// ProjectBuilder groups a RecipeBuilder with its project-level metadata.
type ProjectBuilder struct {
	Builder             port.RecipeBuilder
	Owner               string
	Project             string // code project name (e.g. github repo name)
	LPProject           string // LP project for recipes (may differ from code project)
	Artifacts           []string
	SkipArtifacts       []string
	Series              []string
	DevFocus            string
	OfficialCodehosting bool
	Strategy            ArtifactStrategy
	PrepareCommand      string            // optional shell command to run before committing
	Channels            map[string]string // snap channels for build tools (e.g. {"charmcraft": "latest/stable"})
}

// RecipeProject returns the LP project to use for recipe operations.
// Falls back to the code Project if LPProject is not set.
func (pb ProjectBuilder) RecipeProject() string {
	if pb.LPProject != "" {
		return pb.LPProject
	}
	return pb.Project
}
