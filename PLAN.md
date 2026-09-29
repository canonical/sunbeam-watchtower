# Sunbeam Watchtower — Plan

## Goal

Watchtower is a unified tracking and management tool for Canonical OpenStack (Sunbeam) across development, release, and maintenance. It brings package, bug, build, review, release, and cache state behind one application surface so operators do not need to coordinate across multiple external systems manually.

The long-term design constraints remain:

- keep the core forge-agnostic and push Sunbeam-specific behavior to adapters
- preserve the hexagonal boundary between primary adapters, core ports/services, and secondary adapters
- support server-first operation, with CLI, TUI, and future MCP surfaces reusing the same application/runtime seams

## Architecture Snapshot

### Main packages

- Entrypoints: `cmd/watchtower`, `cmd/watchtower-tui`
- Primary adapters: `internal/adapter/primary/api`, `cli`, `frontend`, `runtime`, `tui`
- Composition root: `internal/app`
- Core interfaces: `internal/core/port`
- Core services: `internal/core/service/*`
- Secondary adapters: `internal/adapter/secondary/*`
- Public client/contracts: `pkg/client`, `pkg/dto/v1`, `pkg/distro/v1`, `pkg/forge/v1`, `pkg/launchpad/v1`

### Enforced boundaries

- `internal/core/port/*` contains interfaces only
- `internal/adapter/*` packages are implementation packages and must not define interfaces
- primary adapters do not import secondary adapters directly
- public `pkg/*` packages do not import `internal/*`
- `internal/app` remains the wiring layer, not a grab-bag frontend API

## Runtime Model

Watchtower is explicitly server-first.

- Persistent server mode is the durable coordination boundary for auth, async operations, and multi-client workflows.
- Embedded mode exists for convenience, but is ephemeral and must not pretend to offer durable state across invocations.
- Split workflows are allowed: local preparation happens client-side, durable execution happens server-side.
- Local filesystem paths stay local. The server receives prepared references, not direct local-path access.

## Current State

Watchtower has a working baseline across the main domains:

- **Architecture** — strict hexagonal layout with boundaries mechanically enforced via `arch-go`
- **HTTP API** — auth, builds, releases, cache, packages, bugs, reviews, commits, config, project sync, team sync
- **Shared frontend/runtime** — CLI, TUI, and API reuse `internal/adapter/primary/frontend` workflows and `internal/adapter/primary/runtime` bootstrap; session target resolution covers embedded, discovered-daemon, and persistent-daemon modes; shared action access catalog classifies every user-invoked operation
- **Auth** — durable interactive flows for Launchpad, GitHub (device flow), Snap Store (httpbakery/Candid), and Charmhub (httpbakery + publisher-token exchange with silent refresh); credentials resolve from env, file, and server
- **Build** — local-path preparation runs in an isolated git worktree, optionally executes `prepare_command`, pushes to a temp branch on the user's LP repo, and triggers LP recipes; supports `--retry N` for waited builds and artifact downloads, automatic retries for transient Launchpad API read failures, `--wait` with structured timeout details, aggregate artifact download failures, cleanup of temp recipes and branches, monorepo layouts, and snap platform auto-detection from `snapcraft.yaml`
- **Releases** — tracking and cache for snaps and charms, with same-name artifacts kept distinct and shared target filtering/rendering across CLI and TUI
- **Bugs** — cache-first sync with incremental overlap, cached Launchpad comments, `since` filtering on task activity, result limiting, group-aware `--merge` output via `bug_groups` config, and bounded explainable search across complete cached bug text; bug sync accepts optional bug IDs, verifies current Launchpad task status live with cache write-through, ensures project/series tasks from branch-unique per-project evidence (excluding ancestry inherited from the development branch), leaves unsupported historical series untouched, explains planned actions, and uses Snap Store revisions plus cached `rev<N>` Git tags to mark matching stable-series tasks `Fix Released`, with conservative shared-project aggregation and warnings for older-before-newer release ordering; search is strictly offline, rejects incompatible cache schemas, omits sensitive records, saturates repeated comment evidence, uses weighted concept/clause coverage, and shares deterministic lexical/fuzzy/regex behavior across API/CLI/TUI
- **Reviews** — cache-first across CLI/API/TUI, with bounded detail hydration during sync and explicit `cache sync reviews` for refresh
- **Packages / Excuses** — provider-owned feed URLs with HEAD-based freshness checks, structured autopkgtest parsing, `--set`/`--blocked-by-set` filtering, CLI OSC 8 hyperlinks; configured Launchpad packagesets use a separate snapshot cache refreshed by explicit `cache sync packagesets`, resolve Ubuntu's current development series during sync, and are read offline by diff and excuses; package diff/show-version uses the selected single backport as its upstream release when none is specified, reads its release-specific constraints, and includes independent deliverables in the default latest-version comparison; behind-upstream comparisons normalize numeric trailing zeros and OpenStack/Debian RC spellings while retaining Debian ordering for other forms, and use each source's highest version when `--merge` is selected; named package queries seek matching bbolt key prefixes rather than scanning full distro indexes; raw Sources files remain cached for full package metadata and `.dsc` lookups; upstream cache sync updates bare branch refs; package cache sync validates explicit distro/release/backport filters and keeps distro releases separate from configured backport sources
- **Git cache** — bare repo caches refresh both remote-tracking refs and local branch refs so `HEAD`-tree consumers such as artifact discovery and release metadata see current manifests; failed fetches are surfaced as sync warnings instead of counted as synced repos
- **Team sync** — LP team → Snap Store / Charmhub collaborator sync with mono-repo fan-out via the canonical `artifactdiscovery` service; per-project discovery failures surface as warnings, not aborts; Snap Store per-snap collaborators intentionally unsupported (dashboard-only); auth expiry reported per-artifact without failing siblings
- **Artifact discovery** — single canonical service at `internal/core/service/artifactdiscovery` for charm, snap, and rock layouts (root and mono-repo)
- **Config** — live reload via fsnotify, SIGHUP, and `POST /api/v1/config/reload` for per-request services (`sync.Once` services require restart); minimal client config via `ConfigResolver` with server token auth middleware; the snap bundles the repository configuration and selects it through `WATCHTOWER_CONFIG` by default
- **TUI** — tabs for Dashboard, Builds, Releases, Packages, Bugs, Reviews, Commits, Projects, plus meta surfaces (auth, operations, cache, logs, server, shortcuts); read-only workflow tabs with centered scrollable modal forms, multi-select with vim-range motions, dense list rows; unified bug list/search with match evidence; build retry/cancel/cleanup; cache and project/bug sync from meta overlays; startup presets via `watchtower.yaml`
- **Telemetry** — cache-first OpenTelemetry confined to `internal/adapter/secondary/otel`

## Current Gaps

- interactive auth exists for Launchpad, GitHub, Snap Store, and Charmhub but has not been extended to other forges such as Gerrit
- `Packages` and `Commits` TUI tabs are read-only; deeper workflow actions remain CLI/API-first
- some tests carry environment-sensitive assumptions and need hardening

## Active Roadmap

### Near term

- finish consolidating artifact discovery onto `internal/core/service/artifactdiscovery`: release bootstrap (`discoverSnapName` / `discoverCharmPublications` in `internal/app/release_helpers.go`) and `build.Strategy.DiscoverRecipes` still carry parallel discoverers; migrate both onto the service (the build worktree walker can stay; only the leaf manifest parser needs to be shared) and delete the legacy helpers
- extend the durable auth-flow model to Gerrit when authenticated Gerrit workflows become necessary

### TUI

- expose cache mutation and richer config inspection as frontend/API contracts mature
- add richer workflow actions (beyond retry/cancel/cleanup) where the server/API model is settled
- keep new selection-heavy workflows on the shared centered-form system so multi-select and vim-range motions are reused, not reimplemented

### API and test contracts

- continue removing host-environment assumptions from tests
- raise tests with feature work to keep changed-package coverage healthy without bypassing the guard
- prefer deterministic completion signals (e.g. `operation.Service.Wait`) over `time.Now().Before(deadline)` polling in tests; reserve polling for cross-process boundaries with a 5x deadline and a `// why:` comment

## Validation Baseline

- `go test ./...`
- `golangci-lint run ./...`
- `arch-go --color no`
- `go run ./tools/coverageguard --config .coverage-policy.yaml $(git diff --cached --name-only -- '*.go')`
- `pre-commit run --all-files`

Architecture boundaries and changed-package coverage are part of the merge contract. Host-specific failures are test-environment hardening, not a reason to weaken guards.

## Roadmap Delivery

Each roadmap chunk ends with:

- a `PLAN.md` sync in the same chunk
- the chunk's validation commands
- one clean commit

Then the next chunk.

## Deferred Testing Note

Broad go-vcr adoption is intentionally deferred. If cassette-backed contract tests are added later, keep them small and endpoint-focused, replay-by-default with explicit rerecording, stored under package-local `testdata/vcr/`, with secrets and unstable metadata normalized before saving.
