# Contributing to Sunbeam Watchtower

## Prerequisites

- Go 1.24+ (see `go.mod` for exact version)
- Git
- [pre-commit](https://pre-commit.com/) (recommended)
- [arch-go](https://github.com/arch-go/arch-go) (recommended)

## Getting started

```bash
git clone https://github.com/canonical/sunbeam-watchtower.git
cd sunbeam-watchtower
go build ./...
go test ./...
pre-commit install
```

## Project structure

The project follows strict **hexagonal architecture** (ports and adapters), enforced by `arch-go` and `depguard`:

```
cmd/watchtower/                    Thin entrypoint
internal/
├── adapter/
│   ├── primary/                   Driving adapters (input → calls core)
│   │   ├── api/                   HTTP server, handlers, API-specific DTOs
│   │   ├── cli/                   Cobra commands, output rendering
│   │   └── frontend/              Shared frontend workflows (local prep, async helpers)
│   └── secondary/                 Driven adapters (output → implements ports)
│       ├── authflowstore/         Pending auth-flow persistence
│       ├── bugcache/              Bug/task cache (bbolt-backed, decorator pattern)
│       ├── credentials/           Credential persistence helpers
│       ├── distrocache/           Distro package index cache
│       ├── excusescache/          Migration-excuses cache
│       ├── git/                   go-git backed GitClient
│       ├── gitcache/              Local bare-clone cache for commit history
│       ├── launchpad/             LP recipe builders, repo manager, project manager
│       ├── operationstore/        Long-running operation persistence
│       └── openstack/             OpenStack upstream deliverable mapping
├── app/                           Composition root (wires adapters + services)
├── config/                        Config structs, loading, validation
└── core/
    ├── port/                      Interface definitions only (incl. bugcache.go)
    └── service/                   Domain logic and use cases
        ├── bug/                   Bug aggregation across trackers
        ├── bugsync/               Cross-references bugs ↔ commits
        ├── build/                 Build triggering, listing, downloading
        ├── commit/                Commit aggregation across sources
        ├── package/               Package version comparison
        ├── project/               LP project metadata sync
        └── review/                Merge request aggregation
pkg/                               Public reusable packages
├── client/                        Typed HTTP client for the watchtower API
├── distro/v1/                     Distro version parsing and comparison
├── dto/v1/                        Shared data contracts
├── forge/v1/                      Forge implementations (GitHub, Gerrit, Launchpad)
└── launchpad/v1/                  Raw Launchpad REST API client
```

### Dependency rules

- **`internal/core/port`** — interfaces only; depends on nothing except `pkg/*`
- **`internal/core/service/*`** — depends only on `internal/core/port` and `pkg/*`
- **Primary adapters** (`api`, `cli`) — call core services through ports; never import secondary adapters
- **Secondary adapters** — implement port interfaces; never import services or primary adapters
- **`pkg/*`** — never imports `internal/*`

These rules are machine-enforced by `arch-go` (see `arch-go.yml`) and `depguard` (see `.golangci.yml`).

## Runtime model

Watchtower has two runtime modes:

1. `persistent server` mode
   - used by `watchtower serve`
   - also used by the local daemon started through `watchtower server start`
   - owns durable auth-flow and async-operation state
   - is the intended target for multi-client usage such as future TUI and MCP frontends
2. `ephemeral embedded` mode
   - used when the CLI starts a short-lived local server for one command
   - is suitable for stateless queries and other single-command work
   - must not be treated as durable across separate CLI invocations

Workflow design should also distinguish:

1. `remote-only` workflows executed by the server
2. `local-only` workflows executed by a frontend
3. `split` workflows where local preparation happens in a shared frontend layer and the server executes the durable remote part

For split workflows, do not push raw filesystem concepts into the server. The frontend should produce a stable prepared contract and send that over the API.

The canonical prepared-build payload uses:

- `target_ref`
- `repository_ref`
- `recipes`

### CLI runtime resolution

When a command needs an API target, the CLI resolves it in this order:

1. `--server`
2. `WATCHTOWER_SERVER`
3. discovered local daemon on the default Unix socket
4. auto-started local daemon for persistent workflows
5. one-command embedded server

Persistent workflows currently include:

- `auth *`
- `operation *`
- `build trigger --async`
- `project sync --async`

The shared runtime policies are:

- CLI durable workflows use `require_persistent`
- CLI ordinary API commands use `prefer_existing_daemon`
- TUI startup uses `prefer_embedded` and upgrades interactively before durable actions

Contributors should preserve this behavior: stateful workflows must prefer persistent-server semantics, while ephemeral mode remains available for stateless work and local convenience.

## Building

```bash
# Build the binary
go build -o watchtower ./cmd/watchtower

# Build with version info
go build -ldflags "-X github.com/canonical/sunbeam-watchtower/internal/adapter/primary/cli.Version=v1.0.0" -o watchtower ./cmd/watchtower
```

## Running tests

```bash
# All tests
go test ./...

# Specific package
go test ./internal/config/...
go test ./internal/adapter/secondary/gitcache/...
go test ./internal/core/service/commit/...

# With verbose output
go test -v ./internal/core/service/commit/...

# With race detection
go test -race ./...
```

## Linting and architecture checks

```bash
# Full pre-commit suite (recommended)
pre-commit run --all-files

# Individual checks
golangci-lint run ./...
arch-go --color no
go run ./tools/coverageguard --config .coverage-policy.yaml $(git diff --cached --name-only -- '*.go')
```

`.coverage-policy.yaml` carries the per-package coverage floors; raise those thresholds when tests improve, rather than relying on the default floor indefinitely.

The GitHub Actions workflow mirrors this locally-enforced contract: it runs the repo-wide hooks with the coverage hook skipped, then runs the same changed-package coverage hook against the PR diff.

## Debug logging

Watchtower uses Go's `log/slog` for structured logging. All log output goes to stderr.

### Enabling debug logs

```bash
watchtower --verbose <command>
```

This sets the log level to `DEBUG`, which surfaces detailed tracing across every layer:

| Layer | What gets logged |
|-------|-----------------|
| **CLI commands** | Command start with filter parameters, result counts |
| **App wiring** | Forge client configuration, commit source wiring, cache directory resolution |
| **Services** | Per-project query/skip decisions, fetch counts, aggregation totals, warnings on errors |
| **Git cache** | Clone/fetch operations, path resolution, commit read counts |
| **Git client** | Every operation: push, remote management, HEAD resolution |
| **LP repo manager** | Project/repo creation or reuse, git ref polling |
| **LP HTTP client** | Every HTTP request method and URL |

### Log levels

| Level | When used |
|-------|-----------|
| `DEBUG` | Detailed tracing: what's being queried, skipped, resolved, counted |
| `INFO` | Significant operations: cloning a repo, creating LP resources |
| `WARN` | Non-fatal issues: fetch failures, missing auth, per-project errors |
| `ERROR` | Not used directly — errors are returned up the call stack |

### Adding logging to new code

Every constructor that does I/O should accept a `*slog.Logger` and default to a no-op logger when nil:

```go
func NewMyAdapter(logger *slog.Logger) *MyAdapter {
    if logger == nil {
        logger = slog.New(slog.NewTextHandler(io.Discard, nil))
    }
    return &MyAdapter{logger: logger}
}
```

Use structured key-value pairs:

```go
s.logger.Debug("querying project",
    "project", name,
    "forge", forgeType,
)

s.logger.Warn("project query failed",
    "project", name,
    "error", err,
)
```

Guidelines:
- **Debug** at entry and exit of significant operations, with relevant parameters and result counts
- **Info** only for operations that change state (cloning, creating resources)
- **Warn** for non-fatal errors that the user should know about
- Never log sensitive data (credentials, tokens)
- Always use structured key-value pairs, never string formatting in log messages

## Architecture guidelines

### Adding a new forge

1. Implement the `forge.Forge` interface in `pkg/forge/v1/`
2. Add a new `ForgeType` constant
3. Wire it in `internal/app/app.go` (`BuildForgeClients`)
4. Add config validation in `internal/config/config.go`
5. Update `CloneURL()` and `CommitURL()` in `internal/config/giturl.go`

### Adding a new service

1. Define any new port interfaces in `internal/core/port/`
2. Create the service in `internal/core/service/<name>/`
3. Accept port interfaces (not concrete types) in the constructor
4. Accept a `*slog.Logger` with nil-safe default
5. Wire it in `internal/app/app.go`
6. Add API handlers in `internal/adapter/primary/api/<name>.go`
7. Add CLI commands in `internal/adapter/primary/cli/<name>.go`
8. Register the CLI command in `internal/adapter/primary/cli/root.go`

### Adding a new adapter

1. Define or reuse a port interface in `internal/core/port/`
2. Implement the adapter in `internal/adapter/secondary/<name>/`
3. Wire it in `internal/app/app.go`

### Adding a new cache type

The cache directory at `$XDG_CACHE_HOME/sunbeam-watchtower/` is designed for extensibility:

```
sunbeam-watchtower/
├── repos/       ← git repo cache (existing)
├── indices/     ← future: distro package indices
└── <other>/     ← future cache types
```

1. Define a port interface in `internal/core/port/`
2. Implement the adapter in `internal/adapter/secondary/<name>/`
3. Wire it in `internal/app/app.go`

### Build service architecture

The build service (`internal/core/service/build/`) supports local and remote build
workflows on Launchpad. Key types and interfaces:

- **`ArtifactStrategy`** (`strategy.go`) — per-artifact-type strategy interface
  (rock, charm, snap). Includes series-aware helpers:
  - `OfficialRecipeName(artifactName, series, devFocus string) string` — returns
    `artifactName` for the dev-focus series, `artifactName-series` otherwise.
  - `BranchForSeries(series, devFocus, defaultBranch string) string` — returns
    `defaultBranch` for the dev-focus series, `stable/<series>` otherwise.
- **`ProjectBuilder`** (`project_builder.go`) — carries series-aware metadata
  (`LPProject`, `Series`, `DevFocus`, `OfficialCodehosting`) alongside the
  code-project identity. `RecipeProject()` resolves the LP project for recipe
  operations.
- **`port.RepoManager`** (`internal/core/port/build.go`) — abstracts the LP git
  repo lifecycle. `GetDefaultRepo(ctx, projectName)` discovers the project's
  official git repo and default branch. `GetCurrentUser(ctx)` resolves the
  authenticated LP user for local builds.
- **`frontend.LocalBuildPreparer`** (`internal/adapter/primary/frontend/build_prepare.go`) —
  performs split-workflow local preparation shared by CLI and future frontends.
  It resolves local git state, prepares Launchpad repo/ref inputs, and emits a
  `dto.PreparedBuildSource`.
- **`Trigger()`** in the build service consumes `TriggerOpts.Prepared` when a
  frontend has already prepared the Launchpad repo/ref/build-path inputs.
  Otherwise it resolves Launchpad state itself for remote/official workflows.

## Code style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep error messages lowercase, without trailing punctuation
- Prefer returning errors over logging + continuing
- Use table-driven tests
- No external test frameworks — use the standard `testing` package

## Submitting changes

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes with tests
4. Ensure all checks pass: `pre-commit run --all-files`
5. Submit a pull request against `main`
