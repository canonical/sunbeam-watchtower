# OpenStack SRU monitor

## Purpose

Show the Ubuntu and Ubuntu Cloud Archive maintenance state of source packages
in Watchtower's configured package sets. Include every public bug with an
explicit task for a configured Ubuntu or Cloud Archive series, even when no
matching upload is observed. Keep Launchpad task status, archive publication,
and verification tags as separate signals.

## Data and classification

- One row represents a bug, source package, and affected series task. Ubuntu
  package tasks identify the source package; a Cloud Archive series task on
  the same bug identifies UCA targets. A shared Cloud Archive task on a bug
  with multiple package tasks is associated with each package and flagged as
  inferred. That association warning is diagnostic only; it does not alone
  select an otherwise completed target for attention.
- For Ubuntu, an Unapproved queue upload is review evidence. For UCA, a
  staging PPA publication is review evidence. Proposed and updates
  publications are additional stages. Match an upload to a bug through the
  `Launchpad-Bugs-Fixed` field of its `.changes` file and retain the version,
  stage, and publication URL. Prefer the newest Debian version, then its
  furthest observed stage.
- A series task marked `Fix Released` supplies the displayed release status.
  Disagreement with an observed pocket is visible as a warning. Absence of
  linked evidence is labelled `not observed`; it is not proof of no upload
  and does not alone select a released task for attention.
- Read Ubuntu tags as `verification-<state>-<ubuntu-series>` and UCA tags as
  `verification-<openstack-series>-<state>`. A missing tag is independent of
  archive stage.
- A UCA backport with `sru_parent_required: true` needs the same bug and
  package observed in its configured parent Ubuntu series' updates pocket.
  The flag is false for a parent series that has gone EOL, such as Epoxy's
  Plucky parent.

## Operations and boundaries

`cache sync sru` reads Launchpad and atomically replaces a local snapshot only
after a complete successful run. Bare `cache sync` includes it after packagesets.
`cache status` reports its freshness and target count; `cache clear sru` removes
only its snapshot. `sru list`, `sru show`, the API, and the TUI read that
snapshot. Filters cover package, package set, archive, series, task status,
stage, verification, and attention. Sync never copies packages or changes
Launchpad bugs. Public bugs only are stored in the SRU snapshot.

CLI table output uses the shared terminal-aware styling and aligned table
renderer. `sru list` selects targets needing attention by default, then groups
the selected targets by bug and source package. Each line shows the number of
selected targets, their archive stage and verification progress, and the bug
title. `--all` includes every monitored target before grouping. `--targets`
shows one line per selected target. `--diagnostics` adds detailed warning text
after either table; warnings stay available in `sru show`, which also includes
the full title, versions, and per-pocket evidence. Filters apply to targets
before grouping. JSON and YAML retain the target-row snapshot shape and honor
the same default attention filter and `--all` override.

The snapshot retains per-pocket version and URL evidence so a later,
separately authorized workflow can compare source and destination versions
before proposing a UCA copy. That future workflow must model the
`cloud_get_work` staging-to-proposed and proposed-to-updates transitions,
including bug task, tag, and comment effects, as separate preview and apply
actions. It is outside this monitor's scope.
