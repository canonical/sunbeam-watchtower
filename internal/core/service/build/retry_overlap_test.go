// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// retryServiceWithStrategy builds a Service for wait-loop tests with the given
// artifact strategy and artifacts, using tiny poll/delay durations so tests
// complete deterministically in milliseconds.
func retryServiceWithStrategy(t *testing.T, builder *mockRecipeBuilder, strategy ArtifactStrategy, artifacts ...string) *Service {
	t.Helper()
	if len(artifacts) == 0 {
		artifacts = []string{"keystone"}
	}
	svc := NewService(
		map[string]ProjectBuilder{
			"sunbeam": {Builder: builder, Owner: "team", Project: "sunbeam", Artifacts: artifacts, Strategy: strategy},
		},
		nil, testLogger(),
	)
	svc.pollInterval = 10 * time.Millisecond
	svc.postRetryDelay = 2 * time.Millisecond
	return svc
}

// buildStateOf returns the current state of one build, or -1 when not found.
func buildStateOf(builder *mockRecipeBuilder, recipeSelfLink, buildSelfLink string) dto.BuildState {
	for _, b := range builder.builds[recipeSelfLink] {
		if b.SelfLink == buildSelfLink {
			return b.State
		}
	}
	return dto.BuildState(-1)
}

// TestWaitForBuilds_RetriesFailedArchWhileSiblingStillBuilding establishes that
// a failed architecture is retried during the same polling pass that observes
// it, without waiting for a continuously building sibling architecture to
// become terminal. The retry hook snapshots the sibling state at the moment the
// retry is issued; only then is the sibling allowed to finish.
func TestWaitForBuilds_RetriesFailedArchWhileSiblingStillBuilding(t *testing.T) {
	for _, tc := range []struct {
		name     string
		strategy ArtifactStrategy
	}{
		{"charm", &CharmStrategy{}},
		{"rock", &RockStrategy{}},
		{"snap", &SnapStrategy{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
			builder := &mockRecipeBuilder{
				recipes: map[string]*dto.Recipe{"keystone": recipe},
				builds: map[string][]dto.Build{
					"/recipe/keystone": {
						{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/fail", CanRetry: true},
						{Recipe: "keystone", State: dto.BuildBuilding, Arch: "arm64", SelfLink: "/build/sibling"},
					},
				},
			}

			siblingStateAtRetry := dto.BuildState(-1)
			builder.retryHook = func(selfLink string) {
				if selfLink != "/build/fail" {
					return
				}
				siblingStateAtRetry = buildStateOf(builder, "/recipe/keystone", "/build/sibling")
				// The retry succeeded and the previously failed arch now runs.
				// Let the sibling finish too so the wait can complete.
				setState(builder, "/recipe/keystone", "/build/fail", dto.BuildSucceeded)
				setState(builder, "/recipe/keystone", "/build/sibling", dto.BuildSucceeded)
			}

			svc := retryServiceWithStrategy(t, builder, tc.strategy)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
				Wait:       true,
				Timeout:    500 * time.Millisecond,
				RetryCount: 3,
			})
			if err != nil {
				t.Fatalf("Trigger() error: %v", err)
			}

			if got := len(builder.retried); got != 1 {
				t.Fatalf("retry calls = %d (%v), want 1 issued while the sibling was building", got, builder.retried)
			}
			if siblingStateAtRetry != dto.BuildBuilding {
				t.Fatalf("retry ran when sibling arch state = %v, want %v (barrier not removed)", siblingStateAtRetry, dto.BuildBuilding)
			}

			rr := result.RecipeResults[0]
			if len(rr.Builds) != 2 {
				t.Fatalf("final builds = %+v, want both archs", rr.Builds)
			}
		})
	}
}

// TestWaitForBuilds_RetriesBeforeLaterRecipeIsListed establishes cross-recipe
// ordering: the first recipe's failed build is retried before the wait loop
// lists the second recipe. The second recipe's listing hook records that it saw
// the first recipe's retry already issued.
func TestWaitForBuilds_RetriesBeforeLaterRecipeIsListed(t *testing.T) {
	recipeA := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	recipeB := &dto.Recipe{Name: "nova", SelfLink: "/recipe/nova"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipeA, "nova": recipeB},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/a", CanRetry: true},
			},
			"/recipe/nova": {
				{Recipe: "nova", State: dto.BuildBuilding, Arch: "amd64", SelfLink: "/build/b"},
			},
		},
	}

	aRetried := false
	bObservedRetryBeforeListing := false
	builder.retryHook = func(selfLink string) {
		if selfLink != "/build/a" {
			return
		}
		aRetried = true
		setState(builder, "/recipe/keystone", "/build/a", dto.BuildSucceeded)
	}
	builder.listHook = func(recipe *dto.Recipe, _ int) error {
		if recipe.SelfLink != "/recipe/nova" || !aRetried {
			return nil
		}
		// The earlier recipe's retry must already have been issued before the
		// wait loop moves on to this later recipe.
		bObservedRetryBeforeListing = true
		setState(builder, "/recipe/nova", "/build/b", dto.BuildSucceeded)
		return nil
	}

	svc := retryServiceWithStrategy(t, builder, &RockStrategy{}, "keystone", "nova")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    500 * time.Millisecond,
		RetryCount: 3,
	}); err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	if !bObservedRetryBeforeListing {
		t.Fatal("later recipe was listed before the earlier recipe's failed build was retried")
	}
	if got := len(builder.retried); got != 1 || builder.retried[0] != "/build/a" {
		t.Fatalf("retry calls = %v, want [/build/a]", builder.retried)
	}
}
