# Immediate per-build retries — Implementation Plan

## Goal

Retry each failed, retryable Launchpad build on the first polling pass that detects it, while other recipes and architectures continue building. Remove the batch-wide terminal-state barrier to reduce total waited-build duration.

Status: implemented; validation commands and results recorded in the implementation summary.

## Current behavior and seams

- `internal/core/service/build/service.go`: `Service.waitForBuilds` gathers every recipe's builds, then retries failures only inside `if allTerminal`. This creates a barrier at every retry wave.
- The budget is already independent per build `SelfLink`: `RetryCount` is the maximum number of attempts, including the initial attempt.
- `executeAction` delegates retry ownership to the wait loop when `RetryCount > 1`. Preserve that single-owner model and the legacy one-shot behavior for `RetryCount <= 1`.
- The wait loop is shared by charms, rocks, and snaps. Implement the change once in this service.
- Production polling defaults to 60 seconds; successful retry batches currently receive a two-second settle delay and a read-only snapshot refresh.
- `dto.Build` already exposes `StartedAt` and `BuiltAt`, populated by the Launchpad adapters. Retry calls operate on the same build self-link and return only an error.
- Existing tests cover independent architecture budgets, exhausted budgets, retry-call errors, non-retryable failures, legacy behavior, cancellation, and structured timeouts. They do not cover retries overlapping ongoing builds or stale post-retry failures.

## Behavioral contract

1. With `Wait=true` and `RetryCount > 1`, retry a qualifying failure during the polling pass in which its recipe is successfully read. Do not wait for other recipes to be read or for other builds to become terminal.
2. Eligibility remains `State.IsFailure() && CanRetry && remaining > 0`, with a valid nonempty self-link and no unacknowledged retry for that build. Preserve the existing treatment of cancelled builds through `IsFailure()`.
3. Retry calls remain sequential and context-aware. Concurrency is provided by Launchpad running builds and their retries concurrently; additional retry workers are unnecessary.
4. Decrement the budget exactly once after a successful retry POST. Preserve the current policy that a failed retry POST logs a warning and exhausts only that build's retry budget; do not automatically replay mutation requests.
5. A successful POST establishes an **awaiting-transition** state. Repeated observations of the same terminal failure cannot issue another retry, including after the last available retry has been consumed.
6. A later active observation acknowledges the retry. A terminal success or superseded state also completes the outstanding retry. A later failed/cancelled observation may establish a new completed attempt without an intervening active observation when its nonzero `StartedAt` or `BuiltAt` advances beyond the failed snapshot saved before the POST. A timestamp becoming zero is not evidence of a new completed attempt.
7. If the retry remains indistinguishable from the old failure (including missing timestamps), continue waiting until transition evidence, cancellation, or the original timeout. Do not use a time-based cooldown to re-enable retries: that could spend the budget on stale observations.
8. Finish only when the polling pass is complete, all observed builds are terminal, no accepted retry is awaiting transition, and no new retry was issued. A recipe-listing error or empty build list prevents completion for that pass; keep warnings and retry on later polls rather than reporting success from an incomplete view.
9. Keep the original absolute timeout across all attempts. Check cancellation and expiry before issuing each retry so no new mutation is started after the wait is already over.
10. Return the latest observed build states. Preserve read-only post-retry refresh behavior on cancellation/timeout where possible, without issuing further retries. Never manufacture `Pending` state from an accepted POST. If an accepted retry still has a stale terminal snapshot at timeout, include it in the structured outstanding-build timeout details using its observed state, rather than reporting an empty outstanding set.

“Immediate” means detection by the existing polling loop, not a Launchpad push notification. Keep the 60-second poll default and two-second post-retry settle delay in this change. A short settle/refresh happens after processing the polling pass, so it does not delay retries for sibling recipes within that pass.

## Implementation tasks

### 1. Add deterministic overlap regression tests

Files: `internal/core/service/build/service_test.go` (or a focused retry test file in the same package).

- [x] Add a same-recipe test with one failed architecture and one continuously building architecture. Assert the retry hook runs while the sibling is still building; only then allow the sibling to finish.
- [x] Add a cross-recipe test that establishes the same ordering. Include a later recipe whose listing hook asserts that the earlier recipe's failed build has already been retried.
- [x] Exercise the shared path with real charm and rock strategies where practical; include a snap case to document that the shared scheduler behavior is consistent.
- [x] Use scripted `ListBuilds` responses and synchronous hooks or channels to establish ordering. Avoid sleeps as correctness assertions and avoid concurrently modifying unsynchronized mock maps. Use a bounded context as a deadlock guard.
- [x] Run the focused tests and confirm they expose the existing barrier before changing the scheduler.

### 2. Introduce per-build retry lifecycle tracking

Files: `internal/core/service/build/service.go`, retry tests.

- [x] Replace or extend the `remaining` map with private per-self-link state containing the remaining budget, an awaiting-transition flag, and the pre-retry attempt timestamps needed to identify stale failures.
- [x] Separate read-only snapshot collection from retry scheduling. Cancellation and post-retry refresh calls must remain read-only; do not hide mutation side effects inside a reusable `poll` function.
- [x] Feed successful refresh observations into lifecycle tracking as well, so an active state seen only during the settle refresh still acknowledges the retry. Preserve the last successful observation per self-link when a later read fails, for best-effort final snapshots; stale retained observations must not make an incomplete polling pass count as complete.
- [x] Apply observation/transition tracking and eligible retry decisions as each recipe's `ListBuilds` returns. Collect the full snapshot for result rendering and timeout reporting.
- [x] Keep accepted retries outstanding until acknowledgment even if their budget is now zero. Do not let a stale terminal result end the wait immediately after the final retry POST.
- [x] Use timestamp advancement as evidence only when it is nonzero and newer than the saved pre-retry snapshot; retain that baseline across repeated stale observations.
- [x] Guard an empty self-link so unrelated malformed records cannot share a retry budget or generate an invalid retry request.
- [x] Preserve structured logs (`recipe`, `build`, `arch`, attempt index, maximum attempts), retry-call failure policy, single retry ownership, and the overall timeout.
- [x] Preserve the short settle/read-only refresh after any successful retries. Update completion logic to account for incomplete recipe reads and outstanding retries.
- [x] Keep timeout-detail changes local to the wait path: combine active observations with still-unacknowledged retry observations, deduplicate by self-link, and preserve observed states. Do not broaden `BuildState.IsActive()` globally.

### 3. Cover lifecycle and termination edge cases

- [x] Repeated stale failed responses after a successful POST produce exactly one retry and do not prematurely finish, including when `RetryCount=2` consumes the final budget.
- [x] `Failed -> retry -> stale Failed -> Pending/Building -> Failed` permits the next retry within its independent budget.
- [x] A retry which runs and fails entirely between polls is recognized through advanced attempt timestamps and can be retried again.
- [x] Zero/cleared/unchanged timestamps do not acknowledge a new failed attempt; persistent ambiguity reaches the original timeout without extra POSTs.
- [x] `Failed -> retry -> Succeeded` completes without requiring a visible active state.
- [x] Budget exhaustion or a retry POST error on one build does not block eligible retries for another build.
- [x] Listing errors and temporarily empty build lists do not make the wait return success while a recipe or accepted retry is unresolved.
- [x] Cancellation and timeout after a retry preserve the freshest obtainable observed snapshot and outstanding timeout details; neither path emits another retry.
- [x] Cancellation during sibling retry processing and an already-expired deadline prevent later retry POSTs.
- [x] Existing tests whose mocks leave failures unchanged forever after a successful POST must explicitly model a new attempt (active transition or advancing timestamps). Keep their budget assertions rather than preserving the stale-state bug.
- [x] Re-run existing first-success, all-attempts-fail, non-retryable, multi-architecture, legacy `RetryCount <= 1`, and `Wait=false` cases.

### 4. Sync documentation and verify architecture/access impact

- [x] Update comments describing wave-based waits and retry behavior.
- [x] Update `PLAN.md`'s current Build state after implementation: per-build retries overlap active siblings, preserve budgets, and guard against stale Launchpad transitions. Remove the planned near-term entry added with this plan.
- [x] Confirm this remains the existing shared `ActionBuildTrigger` (`build.trigger`): write mutability, no local effect, embedded-compatible runtime, allowed MCP export. Retry timing does not introduce a new action, authorization capability, or durable state claim. Preserve CLI/TUI/API mappings and classification tests; update them only if the implementation actually changes their contracts.
- [x] Record telemetry impact in the implementation summary: retry bookkeeping is request-local and adds no persistent domain snapshot or collector surface. Existing retry logs remain the observability seam; no new live collector or metrics labels are needed.

## Validation

Run targeted checks first:

```bash
go test ./internal/core/service/build -run 'Retry|Timeout' -count=1
go test -race ./internal/core/service/build
go test ./internal/adapter/primary/frontend ./internal/adapter/primary/cli ./internal/adapter/primary/api
```

Before implementation handoff is considered complete, run the repository quality gates:

```bash
go test ./...
golangci-lint run ./...
arch-go --color no
go run ./tools/coverageguard --config .coverage-policy.yaml internal/core/service/build/service.go
pre-commit run --all-files
```

The build service coverage floor is currently 54%; include every additional changed Go file in the coverageguard invocation. Report environmental failures precisely without weakening the checks.

## Handoff and acceptance

Implement the tasks in order and mark completed checkboxes as the work and its verification finish. Re-read repository instructions and current code before implementation. No commit, push, or PR is authorized by this plan alone.

Acceptance: a failed charm or rock build is retried before an ongoing sibling finishes; subsequent failed attempts have independent budgets; stale Launchpad responses cannot cause duplicate retries or premature completion; timeout/cancellation and existing frontend contracts remain covered; `PLAN.md` reflects the delivered behavior.

## Implementation summary

Delivered in `internal/core/service/build`:

- `waitForBuilds` now retries each eligible failed build during the polling pass that reads its recipe, instead of gating all retries behind an all-terminal barrier. Per-self-link lifecycle state (`buildRetryTracker`: remaining budget, awaiting-transition flag, pre-retry baseline) lives in a request-local `waitBuildObserver`.
- Observation and scheduling are separated: `listRecipe(..., schedule)` issues retries only on a scheduling pass, while `refresh` is a read-only settle/post-cancel snapshot that can still acknowledge a retry. Failed reads retain the last successful observation per self-link for best-effort snapshots; stale retained observations never make an incomplete pass count as complete.
- A successful POST establishes an awaiting state; it is acknowledged by an active observation, a terminal success/superseded state, or a newer nonzero `StartedAt`/`BuiltAt` versus the saved baseline. Stale terminal snapshots cannot reissue retries (even after the last budget slot) or finish the wait early; persistent ambiguity reaches the original timeout.
- Completion requires a complete pass (no listing errors, no empty recipe lists), all observed builds terminal, no outstanding accepted retry, and no retry issued in the pass. Structured timeout details combine active observations with unacknowledged retry observations, deduplicated by self-link, preserving observed states; `BuildState.IsActive()` is unchanged.

Tests added: `retry_overlap_test.go` (same-recipe failed-arch-vs-building-sibling across charm/rock/snap, cross-recipe ordering) and `retry_lifecycle_test.go` (stale snapshot reissue/finish protection, cleared/advancing timestamps, active-gap transitions, success without visible active state, retry-POST error isolation, incomplete reads, timeout/cancellation and expired-deadline guards). Three existing tests that left failures unchanged forever after a POST were updated to model a genuinely new attempt via advanced timestamps while keeping their budget assertions.

Validation run (local Go 1.26.8 toolchain installed under `/tmp/opencode`, no sudo):

- `go test ./internal/core/service/build -run 'Retry|Timeout' -count=1` — pass
- `go test -race ./internal/core/service/build -count=1` — pass (the test mock was made concurrency-safe; without it, pre-existing races on the mock maps fired under `-race`)
- `go test ./internal/adapter/primary/frontend ./internal/adapter/primary/cli ./internal/adapter/primary/api` — pass
- `go test ./...` — pass
- `go vet ./internal/core/service/build/` and `gofmt -l` on changed files — clean
- `golangci-lint run ./internal/core/service/build/...` — 0 issues; repo-wide `golangci-lint run ./...` reports 4 pre-existing issues in untouched files (`build_prepare_test.go`, `git/client.go`, `tui/model.go`, `tui/views_extra.go`)
- `arch-go --color no` — 100% compliance/coverage
- `go run ./tools/coverageguard --config .coverage-policy.yaml internal/core/service/build/service.go` — 76.7% vs 54% floor
- `go build ./...`, `go mod tidy` (no `go.mod`/`go.sum` diff) — pass
- `pre-commit run --all-files` — passed during commit preparation using `uv run --no-project --with pre-commit==4.5.1` with `/tmp/opencode/gotool/go/bin` on PATH and an isolated `PRE_COMMIT_HOME`. All hooks passed, including the full Go test suite, architecture checks, and changed-package coverage. The configured lint hook filters findings with `--new-from-rev HEAD`; it does not enforce full-repository lint. Initial hook runs selected Go 1.27.2 and failed on incompatible export data; selecting Go 1.26.8 resolved that environment failure.

Telemetry impact: retry bookkeeping is request-local (in-memory, per wait invocation) and adds no persistent domain snapshot, collector, or metric-label surface. Existing retry log lines remain the observability seam; no new live collector or metrics changes are needed.

Access impact: retry timing stays within the existing shared `ActionBuildTrigger` (`build.trigger`) classification — write mutability, no local effect, embedded-compatible runtime, MCP export allowed. No new action, authorization capability, or durable-state claim was introduced; CLI/TUI/API mappings and classification tests are unchanged and pass.
