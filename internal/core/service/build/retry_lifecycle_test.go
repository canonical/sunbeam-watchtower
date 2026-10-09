// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"errors"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// singleRecipeWaitService builds a wait service around one recipe with the
// given builds and tiny timing controls.
func singleRecipeWaitService(t *testing.T, recipe *dto.Recipe, builds []dto.Build, artifacts ...string) (*Service, *mockRecipeBuilder) {
	t.Helper()
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{recipe.Name: recipe},
		builds:  map[string][]dto.Build{recipe.SelfLink: builds},
	}
	return retryServiceWithStrategy(t, builder, &RockStrategy{}, artifacts...), builder
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}
}

// TestWaitForBuilds_StaleFailureNeverReissuesOrFinishes covers contract 5/7:
// repeated identical terminal failures after a successful POST must not issue
// another retry (even when the final budget was consumed) and must not let the
// wait report success while the accepted retry is unacknowledged.
func TestWaitForBuilds_StaleFailureNeverReissuesOrFinishes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		retryCount int
	}{
		{"final-budget", 2},
		{"remaining-budget", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
			svc, builder := singleRecipeWaitService(t, recipe, []dto.Build{
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
				Wait:       true,
				Timeout:    150 * time.Millisecond,
				RetryCount: tc.retryCount,
			})
			requireNoError(t, err)

			if got := len(builder.retried); got != 1 {
				t.Fatalf("retries = %d (%v), want exactly 1; stale snapshots must not reissue retries", got, builder.retried)
			}
			if result.WaitTimeout == nil {
				t.Fatal("WaitTimeout = nil; an accepted retry awaiting transition must not be reported as complete")
			}
			got := result.WaitTimeout.Builds
			if len(got) != 1 || got[0].SelfLink != "/build/1" || got[0].State != "failed" {
				t.Fatalf("timeout builds = %+v, want the stale failed build with its observed state", got)
			}
		})
	}
}

// TestWaitForBuilds_ClearedTimestampDoesNotAcknowledge covers contract 6/7: a
// timestamp becoming zero is not evidence of a new attempt, so a retry whose
// post-POST observation cleared the timestamp stays unacknowledged and is not
// reissued.
func TestWaitForBuilds_ClearedTimestampDoesNotAcknowledge(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0).UTC()
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	svc, builder := singleRecipeWaitService(t, recipe, []dto.Build{
		{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true, BuiltAt: t0},
	})
	// From the first wait read onward, report a cleared (zero) timestamp.
	builder.listHook = func(_ *dto.Recipe, call int) error {
		if call >= 2 {
			updateBuild(builder, "/recipe/keystone", "/build/1", func(b *dto.Build) {
				b.BuiltAt = time.Time{}
			})
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    150 * time.Millisecond,
		RetryCount: 3,
	})
	requireNoError(t, err)

	if got := len(builder.retried); got != 1 {
		t.Fatalf("retries = %d (%v), want exactly 1; a cleared timestamp must not acknowledge a retry", got, builder.retried)
	}
	if result.WaitTimeout == nil || len(result.WaitTimeout.Builds) != 1 {
		t.Fatalf("WaitTimeout = %+v, want the unresolved stale retry", result.WaitTimeout)
	}
}

// TestWaitForBuilds_StaleThenActiveThenNewFailureRetriesAgain covers contract 6:
// a stale failure keeps the retry outstanding, a later active observation
// acknowledges it, and a subsequent failure with an advanced timestamp may use
// the next budget slot.
func TestWaitForBuilds_StaleThenActiveThenNewFailureRetriesAgain(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0).UTC()
	t1 := t0.Add(time.Minute)
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	svc, builder := singleRecipeWaitService(t, recipe, []dto.Build{
		{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true, BuiltAt: t0},
	})
	builder.listHook = func(_ *dto.Recipe, call int) error {
		switch call {
		case 4: // second wait pass: previously failed build is now running
			setState(builder, "/recipe/keystone", "/build/1", dto.BuildPending)
		case 5: // third wait pass: new attempt failed with an advanced timestamp
			updateBuild(builder, "/recipe/keystone", "/build/1", func(b *dto.Build) {
				b.State = dto.BuildFailed
				b.BuiltAt = t1
			})
		case 6: // settle refresh after the second retry: it succeeded
			setState(builder, "/recipe/keystone", "/build/1", dto.BuildSucceeded)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    2 * time.Second,
		RetryCount: 3,
	})
	requireNoError(t, err)

	if got := len(builder.retried); got != 2 {
		t.Fatalf("retries = %d (%v), want 2 (stale, active, new failure)", got, builder.retried)
	}
	for _, link := range builder.retried {
		if link != "/build/1" {
			t.Fatalf("unexpected retried build %q", link)
		}
	}
	if state := buildStateOf(builder, "/recipe/keystone", "/build/1"); state != dto.BuildSucceeded {
		t.Fatalf("final state = %v, want %v", state, dto.BuildSucceeded)
	}
}

// TestWaitForBuilds_AttemptCompletedBetweenPollsIsRerecognized covers contract
// 6: a retry that runs and fails entirely between polls is recognized through
// advanced attempt timestamps and can consume the next budget slot.
func TestWaitForBuilds_AttemptCompletedBetweenPollsIsRerecognized(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0).UTC()
	t1 := t0.Add(time.Minute)
	t2 := t1.Add(time.Minute)
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	svc, builder := singleRecipeWaitService(t, recipe, []dto.Build{
		{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true, BuiltAt: t0},
	})
	builder.listHook = func(_ *dto.Recipe, call int) error {
		switch call {
		case 3: // refresh after retry 1: a brand-new attempt already failed
			updateBuild(builder, "/recipe/keystone", "/build/1", func(b *dto.Build) { b.BuiltAt = t1 })
		case 5: // refresh after retry 2
			updateBuild(builder, "/recipe/keystone", "/build/1", func(b *dto.Build) { b.BuiltAt = t2 })
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    2 * time.Second,
		RetryCount: 3,
	})
	requireNoError(t, err)

	if got := len(builder.retried); got != 2 {
		t.Fatalf("retries = %d (%v), want 2 through timestamp advancement", got, builder.retried)
	}
}

// TestWaitForBuilds_SuccessWithoutVisibleActiveState covers contract 6: a
// terminal success completes the outstanding retry even without an intervening
// active observation.
func TestWaitForBuilds_SuccessWithoutVisibleActiveState(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	svc, builder := singleRecipeWaitService(t, recipe, []dto.Build{
		{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
	})
	builder.retryHook = func(selfLink string) {
		setState(builder, "/recipe/keystone", selfLink, dto.BuildSucceeded)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    2 * time.Second,
		RetryCount: 3,
	})
	requireNoError(t, err)

	if got := len(builder.retried); got != 1 {
		t.Fatalf("retries = %d (%v), want 1", got, builder.retried)
	}
	if state := buildStateOf(builder, "/recipe/keystone", "/build/1"); state != dto.BuildSucceeded {
		t.Fatalf("final state = %v, want %v", state, dto.BuildSucceeded)
	}
	if result.WaitTimeout != nil {
		t.Fatalf("WaitTimeout = %+v, want nil on terminal success", result.WaitTimeout)
	}
}

// TestWaitForBuilds_RetryPostErrorDoesNotBlockSibling covers contract: a failed
// retry POST exhausts only that build's budget; other eligible builds are still
// retried in the same pass.
func TestWaitForBuilds_RetryPostErrorDoesNotBlockSibling(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	svc, builder := singleRecipeWaitService(t, recipe, []dto.Build{
		{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/err", CanRetry: true},
		{Recipe: "keystone", State: dto.BuildFailed, Arch: "arm64", SelfLink: "/build/ok", CanRetry: true},
	})
	builder.retryErrFor = map[string]error{"/build/err": errors.New("LP retry API error")}
	builder.retryHook = func(selfLink string) {
		if selfLink == "/build/ok" {
			setState(builder, "/recipe/keystone", "/build/ok", dto.BuildSucceeded)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
		Wait:       true,
		Timeout:    2 * time.Second,
		RetryCount: 3,
	})
	requireNoError(t, err)

	want := map[string]int{"/build/err": 1, "/build/ok": 1}
	got := map[string]int{}
	for _, link := range builder.retried {
		got[link]++
	}
	for link, n := range want {
		if got[link] != n {
			t.Fatalf("retries for %s = %d (%v), want %d", link, got[link], builder.retried, n)
		}
	}
	if state := buildStateOf(builder, "/recipe/keystone", "/build/ok"); state != dto.BuildSucceeded {
		t.Fatalf("/build/ok state = %v, want succeeded", state)
	}
}

// TestWaitForBuilds_IncompleteReadDoesNotReportSuccess covers contract 8: a
// listing error or a temporarily empty list must not be treated as a finished
// pass, even when the previous snapshot was terminal.
func TestWaitForBuilds_IncompleteReadDoesNotReportSuccess(t *testing.T) {
	t.Run("listing-error", func(t *testing.T) {
		recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
		svc, builder := singleRecipeWaitService(t, recipe, []dto.Build{
			{Recipe: "keystone", State: dto.BuildSucceeded, Arch: "amd64", SelfLink: "/build/1"},
		})
		builder.listHook = func(_ *dto.Recipe, call int) error {
			if call == 2 {
				return errors.New("transient LP 503")
			}
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
			Wait: true, Timeout: 2 * time.Second, RetryCount: 3,
		})
		requireNoError(t, err)
		if builder.listCalls["/recipe/keystone"] < 3 {
			t.Fatalf("wait completed from an incomplete pass (ListBuilds calls = %d)", builder.listCalls["/recipe/keystone"])
		}
		if len(result.RecipeResults) != 1 || len(result.RecipeResults[0].Builds) != 1 {
			t.Fatalf("final result = %+v, want the resolved build", result.RecipeResults)
		}
	})

	t.Run("empty-list", func(t *testing.T) {
		recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
		succeeded := []dto.Build{{Recipe: "keystone", State: dto.BuildSucceeded, Arch: "amd64", SelfLink: "/build/1"}}
		svc, builder := singleRecipeWaitService(t, recipe, succeeded)
		builder.listHook = func(_ *dto.Recipe, call int) error {
			switch call {
			case 2:
				builder.builds["/recipe/keystone"] = nil
			case 3:
				builder.builds["/recipe/keystone"] = succeeded
			}
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
			Wait: true, Timeout: 2 * time.Second, RetryCount: 3,
		})
		requireNoError(t, err)
		if builder.listCalls["/recipe/keystone"] < 3 {
			t.Fatalf("wait completed from an empty read (ListBuilds calls = %d)", builder.listCalls["/recipe/keystone"])
		}
	})

	t.Run("accepted-retry-unacknowledged-by-error", func(t *testing.T) {
		recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
		svc, builder := singleRecipeWaitService(t, recipe, []dto.Build{
			{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
		})
		builder.listHook = func(_ *dto.Recipe, call int) error {
			switch call {
			case 3: // settle refresh after the accepted retry cannot be read
				return errors.New("transient LP 503")
			case 4:
				setState(builder, "/recipe/keystone", "/build/1", dto.BuildSucceeded)
			}
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
			Wait: true, Timeout: 2 * time.Second, RetryCount: 3,
		})
		requireNoError(t, err)
		if got := len(builder.retried); got != 1 {
			t.Fatalf("retries = %d (%v), want 1", got, builder.retried)
		}
		if builder.listCalls["/recipe/keystone"] < 4 {
			t.Fatalf("wait resolved without re-reading after the failed refresh (calls = %d)", builder.listCalls["/recipe/keystone"])
		}
	})
}

// TestWaitForBuilds_TimeoutPreservesOutstandingRetry covers contract 10: a
// timeout after an accepted retry must report the retried build using its
// observed state, and must not issue another retry.
func TestWaitForBuilds_TimeoutPreservesOutstandingRetry(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	svc, builder := singleRecipeWaitService(t, recipe, []dto.Build{
		{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
		Wait: true, Timeout: 120 * time.Millisecond, RetryCount: 3,
	})
	requireNoError(t, err)

	if got := len(builder.retried); got != 1 {
		t.Fatalf("retries = %d (%v), want exactly 1", got, builder.retried)
	}
	if result.WaitTimeout == nil || len(result.WaitTimeout.Builds) != 1 {
		t.Fatalf("WaitTimeout = %+v, want the outstanding retry", result.WaitTimeout)
	}
	if b := result.WaitTimeout.Builds[0]; b.SelfLink != "/build/1" || b.State != "failed" {
		t.Fatalf("outstanding build = %+v, want /build/1 failed", b)
	}
}

// TestWaitForBuilds_CancellationDuringRetryProcessingStopsLaterPosts covers
// contract 9: cancellation observed while processing a sibling retry prevents
// any later retry POST in the same pass.
func TestWaitForBuilds_CancellationDuringRetryProcessingStopsLaterPosts(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	builder := &mockRecipeBuilder{
		recipes: map[string]*dto.Recipe{"keystone": recipe},
		builds: map[string][]dto.Build{
			"/recipe/keystone": {
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/A", CanRetry: true},
				{Recipe: "keystone", State: dto.BuildFailed, Arch: "arm64", SelfLink: "/build/B", CanRetry: true},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	builder.retryHook = func(selfLink string) {
		if selfLink == "/build/A" {
			cancel()
		}
	}
	svc := retryServiceWithStrategy(t, builder, &RockStrategy{})

	if _, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
		Wait: true, Timeout: 2 * time.Second, RetryCount: 3,
	}); err != nil {
		t.Fatalf("Trigger() error: %v", err)
	}

	if got := len(builder.retried); got != 1 || builder.retried[0] != "/build/A" {
		t.Fatalf("retries = %v, want only [/build/A] after cancellation", builder.retried)
	}
}

// TestWaitForBuilds_ExpiredDeadlinePreventsRetryPost covers contract 9: an
// already-expired deadline prevents any retry mutation.
func TestWaitForBuilds_ExpiredDeadlinePreventsRetryPost(t *testing.T) {
	recipe := &dto.Recipe{Name: "keystone", SelfLink: "/recipe/keystone"}
	svc, builder := singleRecipeWaitService(t, recipe, []dto.Build{
		{Recipe: "keystone", State: dto.BuildFailed, Arch: "amd64", SelfLink: "/build/1", CanRetry: true},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := svc.Trigger(ctx, "sunbeam", nil, TriggerOpts{
		Wait: true, Timeout: time.Nanosecond, RetryCount: 3,
	})
	requireNoError(t, err)

	if got := len(builder.retried); got != 0 {
		t.Fatalf("retries = %d (%v), want 0 after an expired deadline", got, builder.retried)
	}
}
