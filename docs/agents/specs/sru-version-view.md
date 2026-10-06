# SRU package version progression

Provide `sru versions <source-package>` independently of
Launchpad bugs and monitoring package sets. Show every configured UCA series and
its configured Ubuntu parent, left to right: parent, staging, proposed, updates.
`--series <uca-series>` optionally selects one row. All rows use stable series
name order, and currency comparisons stay within each row.
The UCA series selects the configured Ubuntu base and parent; it is not a claim
of Canonical support. EOL does not affect this view.

Each cell retains its full published source version. Parent currency uses the
highest published version in Ubuntu release, updates and security. Each UCA
cell uses the highest published version in the corresponding archive pocket.
Green/current means the highest version across all four cells; yellow/behind
means an older version. Compare full Debian versions including epochs, packaging revisions and
`~cloudN` rebuilds between UCA pockets. Ignore a trailing `~cloudN` only
when comparing UCA with the Ubuntu parent. Do not ignore Ubuntu revision
differences. Thus parent version X and staging X~cloud2 may both be current,
while proposed X~cloud1 is behind staging. This is package version currency, not proof of
identical code, desired bug inclusion or migration eligibility.

A successfully read empty pocket is empty. Failed publication reads and missing
parent configuration remain unknown/unconfigured. When a cell is unavailable,
known older versions may still be behind a known newer version, but no cell may
claim current. Invalid Debian versions are unknown. Keep state words alongside
colour and respect the existing terminal/no-colour behaviour. JSON/YAML return
the same versions and states without ANSI styling.

Use the shared frontend/runtime path and read-only `sru.versions` action, with
embedded runtime allowed and MCP export allowed. The API is
`GET /api/v1/sru/versions/{package}` for all configured rows and
`GET /api/v1/sru/versions/{package}/{series}` for one row. Reuse traced Launchpad clients and
the existing publication collector; this path must not read bugs or changes
files. No new telemetry collector: the result is an ephemeral on-demand view,
not a cached or persistent operational snapshot. Existing cached SRU telemetry
remains unchanged.
