# Sunbeam Watchtower

A unified CLI dashboard for tracking packages, code, reviews, bugs, builds, and Launchpad project metadata across GitHub, Launchpad, and Gerrit forges.

Watchtower is moving toward a server-first runtime model. A persistent Watchtower server is the durable coordination boundary for auth, async operations, and future TUI/MCP clients, while the CLI remains a first-class frontend that can still perform local preparation and, when needed, start an ephemeral embedded server for single-command work. The codebase uses a hexagonal split where `internal/adapter/primary/*` drives `internal/app`, which wires `internal/core/service/*` to `internal/adapter/secondary/*` through `internal/core/port/*`. Public reusable contracts and helpers live under `pkg/`, notably `pkg/client`, `pkg/dto/v1`, `pkg/distro/v1`, `pkg/forge/v1`, and `pkg/launchpad/v1`.

## Installation

```bash
go install github.com/gboutry/sunbeam-watchtower/cmd/watchtower@latest
```

Or build from source:

```bash
git clone https://github.com/gboutry/sunbeam-watchtower.git
cd sunbeam-watchtower
go build -o watchtower ./cmd/watchtower
```

## Quick start

1. Create a configuration file at `~/.config/sunbeam-watchtower/config.yaml`:

```yaml
projects:
  - name: snap-openstack
    code:
      forge: github
      owner: canonical
      project: snap-openstack
    bugs:
      - forge: launchpad
        project: snap-openstack

  - name: sunbeam-charms
    code:
      forge: gerrit
      host: https://review.opendev.org
      project: openstack/sunbeam-charms

  - name: charm-keystone
    code:
      forge: launchpad
      project: charm-keystone-k8s
```

2. Sync the local caches:

```bash
watchtower cache sync
```

3. Start exploring:

```bash
watchtower packages diff released
watchtower commit log
watchtower review list
watchtower bug list
watchtower project sync --dry-run
```

## Architecture

- `cmd/watchtower` is a thin entrypoint into the CLI adapter.
- `internal/adapter/primary/cli` contains Cobra command logic.
- `internal/adapter/primary/api` exposes the application over HTTP/OpenAPI.
- `internal/adapter/primary/frontend` contains frontend-facing shared workflows such as local build preparation and async orchestration helpers.
- `internal/app` is the composition root used by the primary adapters.
- `internal/core/port` contains interfaces only.
- `internal/core/service/*` contains domain logic and use cases.
- `internal/adapter/secondary/*` contains concrete integrations for git, Launchpad, caches, and OpenStack.
- `pkg/*` contains reusable client and DTO packages that can be consumed outside the repository.

## Runtime model

Watchtower currently supports three workflow shapes:

1. `remote-only` workflows, where the server does the work directly
2. `local-only` workflows, where a frontend inspects local state
3. `split` workflows, where a frontend prepares local inputs and the server executes the durable remote part

Examples:

```bash
# Explicit external server
watchtower --server http://127.0.0.1:8472 operation list
WATCHTOWER_SERVER=http://127.0.0.1:8472 watchtower operation list

# Local persistent daemon managed by the CLI
watchtower server start
watchtower server status
watchtower operation list
watchtower server stop

# Split workflow: local checkout stays local, prepared LP refs go to the server
watchtower build trigger demo --source local --local-path .
```

For split workflows, the server never reads raw local paths. Frontends prepare a stable `prepared` build source payload and send that to the server.

The canonical prepared-build payload uses:

- `target_ref`
- `repository_ref`
- `recipes`

### Runtime resolution order

When a command needs the API, the CLI resolves the target in this order:

1. `--server`
2. `WATCHTOWER_SERVER`
3. discovered local daemon on the default Unix socket
4. auto-started local daemon for persistent workflows
5. one-command embedded server for explicitly ephemeral work

Persistent workflows currently include:

- `auth *`
- `operation *`
- `build trigger --async`
- `project sync --async`

The shared runtime policies are:

- CLI durable workflows use `require_persistent`
- CLI ordinary API commands use `prefer_existing_daemon`
- TUI startup uses `prefer_existing_daemon`, reuses the local persistent daemon when one is already running, and otherwise falls back to an embedded session that can still upgrade interactively before durable actions

Stateless queries and one-shot commands can still run against the embedded server when no persistent target is configured.

## Configuration

The config file lives at `~/.config/sunbeam-watchtower/config.yaml` by default. Override with `--config <path>`.

### Top-level sections

| Section     | Description |
|-------------|-------------|
| `launchpad` | Launchpad settings (`default_owner`, `use_keyring`, default `series`, `development_focus`) |
| `github`    | GitHub settings (`client_id`, `use_keyring`) |
| `gerrit`    | Gerrit settings (`hosts` list with `url` entries) |
| `tui`       | TUI startup presets (`default_pane`, per-pane preset filters, and package/commit modes) |
| `projects`  | List of tracked projects |

Example TUI preset:

```yaml
tui:
  default_pane: packages
  panes:
    packages:
      mode: excuses
      filters:
        tracker: ubuntu
        team: ubuntu-openstack
```

The TUI applies these presets once at startup, uses them for the first load of each pane, and treats them as the reset target for filter modals.
| `build`     | Build pipeline settings (`default_prefix`, `timeout_minutes`, `artifacts_dir`) |
| `packages`  | Package source, set, and upstream configuration |
| `otel`      | Server-side OpenTelemetry exporters and metrics listeners |

### OpenTelemetry

Telemetry is a persistent-server feature. It is designed to stay cheap by default:

- `otel.metrics.self` exposes request/runtime/collector-health metrics for the running server
- `otel.metrics.domain` exposes Watchtower domain metrics
- domain metrics default to cached/internal state
- live upstream fan-out is disabled unless you explicitly allow it with `otel.metrics.domain.live_systems`
- the intended production pattern is periodic cache refresh, for example from cron, plus Prometheus scraping the server

Example:

```yaml
otel:
  service_name: watchtower
  service_namespace: sunbeam
  metrics:
    self:
      enabled: true
      listen_addr: 127.0.0.1:9464
      path: /metrics
    domain:
      enabled: true
      listen_addr: 127.0.0.1:9465
      path: /metrics
      live_systems: []
    collectors:
      reviews:
        refresh_interval: 15m
  traces:
    enabled: true
    endpoint: http://127.0.0.1:4318
    protocol: http
  logs:
    enabled: true
    endpoint: http://127.0.0.1:4318
    protocol: http
    mirror_stderr: true
```

Operational model:

```bash
# Persistent server with telemetry
watchtower server start

# Periodic cache refresh keeps domain telemetry cheap
watchtower cache sync
watchtower cache sync releases
watchtower cache sync excuses
```

Label policy:

- use bounded labels only
- do not expect IDs, SHAs, usernames, or raw URLs as metric labels
- release metrics use stable labels such as `project`, `artifact_type`, `artifact`, `track`, `risk`, `branch`, `architecture`, and `resource`

### Project configuration

Each project has:

```yaml
- name: my-project              # required, unique identifier
  artifact_type: rock           # optional: rock, charm, or snap (required if build is set)
  code:
    forge: github               # required: github, gerrit, or launchpad
    owner: my-org               # required for github
    host: https://review.dev.org # required for gerrit
    project: my-project         # required: project name/path
    git_url: https://custom/repo # optional: explicit clone URL override
  bugs:
    - forge: launchpad
      project: my-project
  series:                       # optional: Launchpad series to ensure on the project
    - 2024.1
    - 2024.2
  development_focus: 2024.2     # optional: must be one of the declared series
  build:                        # optional
    owner: my-team
    official_codehosting: false # when true, use LP's default git repo for remote builds
    lp_project: my-lp-project  # LP project for recipe ops (defaults to code.project)
    artifacts:
      - recipe-name
    prepare_command: make prepare
```

When `official_codehosting` is `true`, remote builds resolve the project's default
git repository via the LP API (`GetDefaultRepository`) and create series-based
recipes automatically. The `lp_project` field overrides which LP project is used
for recipe operations (useful when the LP project name differs from `code.project`).

Example project configured for official remote builds:

```yaml
- name: ubuntu-openstack-rocks
  artifact_type: rock
  code:
    forge: github
    owner: canonical
    project: ubuntu-openstack-rocks
  build:
    owner: canonical
    official_codehosting: true
    lp_project: ubuntu-openstack-rocks
```

The `git_url` field is useful when the clone URL cannot be derived from the forge type and project name (e.g., Launchpad repos with complex paths like `~owner/project/+git/repo`).

Projects can declare the Launchpad series that must exist on the project, plus the desired focus of development:

```yaml
- name: snap-openstack
  code:
    forge: github
    owner: canonical
    project: snap-openstack
  bugs:
    - forge: launchpad
      project: snap-openstack
  series:
    - 2024.1
    - 2024.2
  development_focus: 2024.2
```

`watchtower project sync` ensures those series exist and sets the declared focus of development.

### Release tracking overrides

Published snap and charm tracking is repo-driven:

- snap names come from `snap/snapcraft.yaml` or root `snapcraft.yaml`
- charm names and resources come from `charmcraft.yaml`
- tracked store tracks default to the project's `series`

Use `project.release` only for store-specific overrides:

```yaml
- name: snap-openstack
  artifact_type: snap
  code:
    forge: github
    owner: canonical
    project: snap-openstack
  series:
    - 2024.1
    - 2025.1
  release:
    track_map:
      2025.1: latest
    branches:
      - series: 2024.1
        branch: risc-v
        risks: [edge, beta, candidate, stable]

- name: sunbeam-charms
  artifact_type: charm
  code:
    forge: gerrit
    host: https://review.opendev.org
    project: openstack/sunbeam-charms
  series:
    - 2024.1
  release:
    branches:
      - series: 2024.1
        branch: risc-v
        risks: [edge]
    skip_artifacts:
      - sunbeam-libs
```

Rules:

- `release.track_map` remaps project `series` to store tracks
- `release.tracks` can replace the default series-derived tracks entirely
- `release.branches` declares managed `track/risk/branch` channels for snaps or charms
- `release.skip_artifacts` excludes discovered artifacts that exist in a mono repo but are not meant to be published or tracked
- branch channels are additive; Watchtower does not try to auto-discover arbitrary store branches as managed release lines

### Release target visibility profiles

Release tracking always stores and serves the full target matrix. Target filtering is a frontend-side presentation feature for `watchtower releases` and the TUI.

Use top-level reusable profiles plus optional per-project defaults and overrides:

```yaml
releases:
  default_target_profile: noble-and-newer
  target_profiles:
    noble-and-newer:
      include:
        - base_names: [ubuntu]
          min_base_channel: "24.04"

projects:
  - name: openstack
    artifact_type: charm
    code:
      forge: gerrit
      host: https://review.opendev.org
      project: openstack/sunbeam-charms
    release:
      target_profile: noble-and-newer
      target_profile_overrides:
        exclude:
          - architectures: [s390x]
```

Rules:

- `releases.default_target_profile` sets the default frontend filter when no project-specific or per-invocation profile is chosen
- `releases.target_profiles` defines named include/exclude rules for release targets
- `release.target_profile` selects the default named profile for one project
- `release.target_profile_overrides` merges on top of the selected profile for that project
- `min_base_channel` accepts dotted numeric version channels only, such as `22.04` or `24.04`
- snap targets using `core22`/`core24`/`core26` are normalized against Ubuntu generations for `min_base_channel` matching
- `--all-targets` bypasses all local filtering for that command or TUI session state
- filtering affects rendered output only; server state and API payloads remain unchanged

## Commands

### `watchtower releases`

Inspect cached snap and charm release matrices with target-aware revision output.

When a snap and a charm share the same artifact name, Watchtower tracks and lists both entries separately. Use `--type` with `releases show` when a name matches multiple artifact types.

```bash
# List cached release rows with target-qualified revisions
watchtower releases list openstack

# Filter locally to a named target profile
watchtower releases list openstack --target-profile noble-and-newer

# Show the full matrix for one artifact
watchtower releases show openstack --type snap

# Bypass local filtering and show every tracked target
watchtower releases show openstack --type snap --all-targets
```

### `watchtower packages`

Compare package versions and inspect package metadata across configured distro sources.

```bash
# Compare package sets
watchtower packages diff released

# Show all versions of a package
watchtower packages show-version nova

# Show full APT metadata for a specific version
watchtower packages show nova 2:2025.1-0ubuntu1

# Show highest version matching filters
watchtower packages show nova --distro ubuntu --backport gazpacho

# List Ubuntu proposed-migration excuses
watchtower packages excuses list --tracker ubuntu --ftbfs

# Filter Ubuntu excuses by owning team
watchtower packages excuses list --tracker ubuntu --team debcrafters-packages

# Inspect one package's migration blockers
watchtower packages excuses show nova --tracker ubuntu

# List packages in a configured distro release
watchtower packages list --distro ubuntu --release noble
```

### `watchtower commit`

Read commit history from the local git cache.

```bash
# List recent commits across all projects
watchtower commit log

# Filter by project, forge, branch, or author
watchtower commit log --project snap-openstack --branch main --author alice

# Find commits referencing a Launchpad bug
watchtower commit track --bug-id 12345
```

### `watchtower review`

List and inspect cached merge requests across forges. Review browsing is cache-first by default, so run `watchtower cache sync reviews` before relying on `review list/show` in the CLI or TUI.

```bash
# List open merge requests
watchtower cache sync reviews
watchtower review list --state open

# Filter by project or forge
watchtower review list --project snap-openstack --forge github

# List MRs updated in the last week
watchtower review list --since 1w

# Show details for a specific merge request
watchtower review show --project snap-openstack "#42"

# Refresh cached detail for recently updated closed reviews too
watchtower cache sync reviews --since 30d
```

### `watchtower bug`

Track bugs across Launchpad bug trackers.

```bash
# List bug tasks
watchtower bug list

# Filter by status, importance, assignee, or tag
watchtower bug list --status "In Progress" --importance High --assignee alice

# Show a specific bug with its tasks
watchtower bug show 12345

# Refresh the cache, then search complete bug text with explanations
watchtower cache sync bugs
watchtower bug search cinder snapshot lost during service restart

# Require/exclude terms, select fields, and disable fuzzy matching
watchtower bug search snapshot \
  --require cinder --exclude noha \
  --field title --field description --fuzzy=false

# Deterministic regular-expression search and structured agent output
watchtower bug search 'relation-(changed|departed)' --mode regex -o json

# Open a result using the canonical reference returned by search
watchtower bug show launchpad:12345

# Refresh Git/release evidence and preview required bug synchronization
watchtower cache sync git
watchtower cache sync releases
watchtower bug sync --dry-run

# Preview or apply only selected bugs. With explicit projects, the projects
# declare affected scope and missing mapped Launchpad tasks are added.
watchtower bug sync --dry-run 12345 23456
watchtower bug sync 12345 --project openstack --project openstack-hypervisor
```

Bug search is read-only and uses the existing cache; it never refreshes as a
side effect. Results report cache provenance and explain matching fields,
terms, fuzzy correspondences, and bounded excerpts. The Bugs TUI uses the same
workflow: leave its query empty to list tasks, or enter a query to search.

Bug sync derives `In Progress` and `Fix Committed` from per-project commit
references. For snap projects, a series task becomes `Fix Released` only when
the fix is reachable from the `rev<N>` Git tag corresponding to the snap
revision currently published in that series' stable channel. This applies to
all configured and future series rather than a fixed release list. Shared bug
group tasks aggregate all affected components. Sync warns, but does not block,
when an older series is released before a newer configured series. Release
decisions are cache-first, so refresh both Git and release caches beforehand.
Historical series that are not configured are left untouched even when their
Git branches or Launchpad tasks still exist.
Git and release evidence is cache-first, while bug sync verifies current task
statuses live before planning changes so a stale bug cache cannot produce a
redundant status update. Verified statuses are written through to existing
cached task rows during apply; dry-run remains side-effect free and
`watchtower cache sync bugs` remains the bulk refresh path.

### `watchtower build`

Manage Launchpad builds (rocks, charms, snaps). Two build modes are supported:

- **Local** (`--source local`): pushes the local git tree to a temporary LP repo,
  creates temporary recipes, and triggers builds. Ideal for development and testing.
- **Remote** (`--source remote`): uses the project's official LP git repo (code
  mirror) and creates series-based recipes. Used for official builds.

```bash
# Local build (development) — push local tree and trigger
watchtower build trigger --source local ubuntu-openstack-rocks nova-consolidated keystone

# Remote build (official, series-based)
watchtower build trigger --source remote ubuntu-openstack-rocks nova-consolidated keystone

# Trigger and wait for completion
watchtower build trigger my-project --wait --timeout 2h

# Trigger and wait, then download artifacts with one build/artifact retry
watchtower build trigger my-project --wait --download --retry 2

# List builds
watchtower build list my-project

# List local builds by prefix
watchtower build list --source local ubuntu-openstack-rocks --prefix tmp-build-

# Download build artifacts
watchtower build download my-project --artifacts-dir ./output --retry 2

# Download specific artifacts from a local build
watchtower build download --source local ubuntu-openstack-rocks keystone nova-consolidated

# Clean up temporary recipes
watchtower build cleanup my-project --dry-run
```

In remote mode with series configured, each artifact expands into per-series
recipes: `<artifact>` for the dev-focus series (default branch) and
`<artifact>-<series>` for other series (`stable/<series>` branch).

Launchpad API reads are retried automatically for transient service failures
such as HTTP 429, 502, 503, and 504. This is independent of `--retry`, which
controls build retries while waiting and artifact file transfer retries.

### `watchtower project`

Manage Launchpad project metadata derived from configuration.

```bash
# Preview Launchpad project changes
watchtower project sync --dry-run

# Sync a subset of Launchpad projects
watchtower project sync --project snap-openstack --project sunbeam-charms
```

### `watchtower cache`

Manage local caches used by commit tracking, package lookups, upstream repository inspection, bug tracking, release tracking, and review browsing.

```bash
# Sync all cache types
watchtower cache sync

# Sync only git repos
watchtower cache sync git

# Sync only package indexes for selected distros/distro releases
watchtower cache sync packages-index --distro ubuntu --release noble

# Sync only configured package backport sources
watchtower cache sync packages-index --distro ubuntu --backport gazpacho

# Sync only configured upstream repos
watchtower cache sync upstream-repos

# Sync only bug/task caches
watchtower cache sync bugs

# Sync cached review summaries and details
watchtower cache sync reviews

# Sync Ubuntu and Debian excuses caches
watchtower cache sync excuses --tracker ubuntu --tracker debian

# Sync Ubuntu excuses and refresh the companion team mapping used by --team
watchtower cache sync excuses --tracker ubuntu

# Sync a specific project's git cache
watchtower cache sync git --project snap-openstack

# Sync only one project's review cache
watchtower cache sync reviews --project snap-openstack

# Show cached repos and their disk usage
watchtower cache status

# Remove all cached data
watchtower cache clear

# Remove a specific project's git cache
watchtower cache clear git --project snap-openstack

# Remove only Debian excuses cache entries
watchtower cache clear excuses --tracker debian
```

Valid cache types are `git`, `packages-index`, `upstream-repos`, `bugs`, `excuses`, `releases`, and `reviews`.

For `packages-index`, `--release` matches distro release names such as `noble`.
Use `--backport` for configured package backport sources such as UCA or OSBPO
targets.

Review cache data is stored at `$XDG_CACHE_HOME/sunbeam-watchtower/reviews/` (defaults to `~/.cache/sunbeam-watchtower/reviews/`). It stores cached review summaries for all synced items plus full comments/files/diff detail for open reviews and recently updated closed reviews.

Git cache data is stored at `$XDG_CACHE_HOME/sunbeam-watchtower/repos/` (defaults to `~/.cache/sunbeam-watchtower/repos/`). Repos are bare git clones organized by host:

```
~/.cache/sunbeam-watchtower/repos/
├── github.com/canonical/snap-openstack.git
├── review.opendev.org/openstack/sunbeam-charms.git
└── git.launchpad.net/charm-keystone-k8s.git
```

### `watchtower auth`

Manage provider authentication. Launchpad auth is required for Launchpad-backed bugs, Launchpad reviews, and builds. GitHub auth is optional, but it raises GitHub API rate limits for review and commit browsing plus `cache sync reviews`.

```bash
# Check aggregated auth status
watchtower auth status

# Launchpad browser-based OAuth login/logout
watchtower auth launchpad login
watchtower auth launchpad logout

# GitHub device-flow login/logout
watchtower auth github login
watchtower auth github logout
```

GitHub auth uses an OAuth app client ID resolved in this order:

- `WATCHTOWER_GITHUB_CLIENT_ID`
- `github.client_id` in config

If either `GH_TOKEN` or `GITHUB_TOKEN` is set, Watchtower uses that GitHub credential source first and reports it as an environment-backed auth source in `auth status`. In that mode, `watchtower auth github login` and `watchtower auth github logout` are blocked to avoid creating shadowed file-backed state.

`github.use_keyring` is reserved for future support and is not implemented yet. If enabled, GitHub auth mutation flows fail with a clear error instead of silently falling back to file storage.

### `watchtower config`

```bash
# Display the current configuration
watchtower config show
```

### `watchtower serve`

Run the HTTP API server directly instead of using the embedded per-command server.

```bash
# Serve on TCP
watchtower serve --listen 127.0.0.1:8472

# Serve on a Unix domain socket
watchtower serve --listen unix:///tmp/watchtower.sock
```

When served over TCP:

- OpenAPI spec: `http://127.0.0.1:8472/openapi.json`
- Interactive docs: `http://127.0.0.1:8472/docs`

## Output formats

All list/show commands support `--output` (`-o`):

| Format  | Flag            | Description |
|---------|-----------------|-------------|
| Table   | `-o table`      | Human-readable aligned columns (default) |
| JSON    | `-o json`       | Machine-readable JSON |
| YAML    | `-o yaml`       | Machine-readable YAML |

## Global flags

| Flag        | Description |
|-------------|-------------|
| `--config`  | Path to config file (default: `~/.config/sunbeam-watchtower/config.yaml`) |
| `--verbose` | Enable debug logging (CLI/API write to stderr; TUI shows logs in its log pane) |
| `-o, --output` | Output format: `table`, `json`, `yaml` |
| `--no-color` | Disable colored output |

## Debug logging

Enable verbose logging to see exactly what watchtower is doing:

```bash
watchtower --verbose commit log
```

For CLI and API runs, this writes structured debug logs to stderr, including:
- Which forge clients are being configured
- Which projects are being queried or skipped by filters
- How many commits/reviews/bugs were fetched per project
- Git cache operations (clone, fetch, path resolution)
- Cache directory resolution

For `watchtower-tui`, `--verbose` enables the same debug logs but keeps them inside the TUI. Press `l` to open the log pane, which shows the current session logs and, when connected to the local persistent daemon, a tail of the daemon log file.

Example output:

```
time=... level=DEBUG msg="building commit sources" project_count=3
time=... level=DEBUG msg="resolved cache directory" path=/home/user/.cache/sunbeam-watchtower
time=... level=DEBUG msg="initializing git cache" cacheDir=/home/user/.cache/sunbeam-watchtower/repos
time=... level=DEBUG msg="configured commit source" project=snap-openstack cloneURL=https://github.com/canonical/snap-openstack.git
time=... level=DEBUG msg="listing commits" project_count=3 branch=main author=""
time=... level=DEBUG msg="querying project" project=snap-openstack forge=GitHub
time=... level=DEBUG msg="listing commits from cache" cloneURL=https://github.com/canonical/snap-openstack.git branch=main
time=... level=DEBUG msg="commits read from cache" cloneURL=https://github.com/canonical/snap-openstack.git count=47
time=... level=DEBUG msg="project commits fetched" project=snap-openstack commit_count=47
time=... level=DEBUG msg="commits aggregated" total_count=142
time=... level=DEBUG msg="commit log complete" total_commits=142
```

## Developer tooling

This repository ships with pre-commit hooks, `arch-go` architecture checks, and a curated `golangci-lint` v2 configuration.

```bash
pre-commit install
pre-commit run --all-files
```

The hooks run whitespace/YAML checks plus `arch-go`, `golangci-lint`, `go build`, `go test`, changed-package coverage via `go run ./tools/coverageguard --config .coverage-policy.yaml`, and `go mod tidy`.

GitHub Actions runs the same contract in CI: repo-wide quality checks plus changed-package coverage against the pull request diff.

## License

Apache-2.0
