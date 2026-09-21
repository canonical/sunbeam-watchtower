# Bug sync release evidence

## Goal

Bug synchronization keeps Launchpad project and series tasks aligned with Git
fix evidence and published snap revisions. It supports bugs spanning several
Watchtower projects without allowing one component's completion to hide an
unfinished sibling.

## Project scope and task ownership

A project is affected when a relevant commit contains `Partial-Bug` or
`Closes-Bug`. `Related-Bug` is informational and does not create tasks. For an
explicit invocation, positional bug IDs combined with `--project` declare the
affected projects even before commits exist.

Every affected Watchtower project expands through its configured `bugs`
mappings. Missing Launchpad project tasks and branch-derived series tasks are
created from the deduplicated union of those mappings.

Dedicated project tasks use only evidence from source projects mapped to that
target. A `bug_groups.common_project` task aggregates all affected components
mapped to it and is capped at the least-advanced component state.

## Status evidence

- `Partial-Bug` means `In Progress`.
- `Closes-Bug` in a relevant branch means `Fix Committed`.
- `Closes-Bug` reachable from every snap revision currently published in the
  matching base `<track>/stable` channel means `Fix Released` for that series.

A stable branch's inherited ancestry is not series evidence. Stable-series
correlation considers only commits reachable from the stable branch that are
not reachable from that project's `main` (or `master`) branch. This captures
series-specific fixes and cherry-picked backports without assigning every bug
that happened to be fixed before the branch was created. If no development
branch can be read, stable correlation is skipped with a warning rather than
falling back to unsafe full-history scanning.

The configured Launchpad development-focus series follows active development
branch evidence as well as any branch-unique fixes for its matching stable
branch. It is not treated as an unrelated stable-only task, and an existing
development-focus task is not assigned a second time under a synthetic name.

Stable release provenance is the Snap Store revision mapped to the Git tag
`rev<revision>`. Annotated tags are peeled to their commit. Snap version text is
diagnostic only and is not authoritative.

Release tracks map to Launchpad series by `release.track_map`, or by identical
name when no mapping exists. Risks below stable, branch channels, and
non-version tracks do not establish `Fix Released`.

Configured series are an allow-list. Historical Git branches and existing
Launchpad series tasks outside that list are left untouched and are never
re-created or advanced by synchronization.

Missing release cache data, tags, or commits are warnings. They must fall back
to `Fix Committed` and must never guess that a fix was released.

## Ordering warnings

Before applying an older `YYYY.N` `Fix Released` transition, bug sync checks
all newer configured series for the same Launchpad project. A newer task that
is missing or will not be `Fix Released` after the plan produces a warning but
does not block the authoritative older-series update.

## CLI

`watchtower bug sync [bug-id...]` applies the plan. `--dry-run` lists the same
plan without mutation. Empty bug IDs mean all discovered bugs. Actions include
human-readable reasons and structured channel, revision, tag, and commit
evidence where applicable.

Commit and release discovery is cache-first, but current Launchpad bug/task
status is fetched live before planning. A mutating workflow must not propose
updates from a stale bug-cache snapshot. Ordinary bug list, show, and search
operations remain cache-first. During an apply, successfully fetched live
metadata and existing task rows are written through to the bug cache so later
reads see the verified state. Dry-run remains side-effect free, and full cache
synchronization remains responsible for discovering tasks that are not already
cached.
