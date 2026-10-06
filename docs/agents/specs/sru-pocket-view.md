# Cached UCA pocket package view

`watchtower sru view <uca-series>` shows Ubuntu parent → staging → proposed →
updates for every source package found in the selected series' cached staging
index. Staging determines scope; packages found only in Ubuntu or other UCA
pockets do not add rows. Sort rows by source name and select the highest source
version across matching suite/components for each cell.

Read the package-index cache populated by `cache sync package-index`. This view
must not refresh indexes, query Launchpad, read bugs or changes files, or require
monitoring package sets and upstream ordering. Use configured package-source
expansion to resolve `ubuntu/<series>` and native `ubuntu` groups. Standard UCA
release, proposed and updates suite expansions select the staging/proposed/
updates cells. Parent versions combine release, updates and security suites.
Show source-group cache timestamps explicitly; currency refers to cached data.

Reuse the version view's Debian comparison, cloud-suffix handling and green/
yellow/unknown/empty states. Comparisons remain within each package/series row.
A package absent from a fully evidenced pocket is empty. Existing cache groups
can represent partial syncs and do not retain per-index sync provenance. Require
at least one cached record in every configured suite/component before treating
that selection as fully evidenced. Absence of suite/component coverage, including
an entirely empty index, is unknown rather than proven empty. An unavailable
staging inventory fails with a 409 and cache-sync guidance, rather than returning
an apparently empty package list. Missing components must never become green.

Expose `GET /api/v1/sru/view/{series}` through shared frontend/runtime seams and
canonical `sru.pocket-view`: read-only, local read, embedded-compatible,
MCP-exportable. No new background collector or live fan-out. Existing package
cache collectors already cover source counts and cache age; no additional
telemetry metrics are needed for this transient presentation.
