# Cached UCA pocket package view implementation

Resolve selected UCA configuration and reuse package-source suite expansion.
Read each cache group once, verify cached suite/component coverage, choose row
scope from staging, and reuse SRU currency classification. Render a package
matrix with shared cell styling, state words and cache timestamps. Support
JSON/YAML without colours.

Use shared frontend/server/client workflows, the canonical read-only action and
API route. Preserve existing live single-package versions and bug-chain views.

Verification boundary: real APT-index/cache/API fixture exercises staged scope,
parent-only exclusion, highest versions, current/behind/empty states, no index
refresh and unknowns following a partial cache sync. CLI transport tests cover
all formats, warnings, cache timestamps and canonical access classification.
Native hooks and the actual command against existing local cache are required.
No claim is made about current live archive state or migration eligibility.

## Verification evidence

The actual read-only CLI against the existing package-index cache returned 220
Caracal staging packages, with group timestamps for ubuntu and ubuntu/caracal.
Open vSwitch was current in Ubuntu Noble, staging, proposed and updates. This
verifies cached observation, not current live archive state. No sync was run.

An actual CLI HTTP fixture using that response passed table/JSON/YAML, cache-age
output, row count, PTY ANSI styling and --no-color checks. The API fixture uses
real compressed APT indexes and bbolt storage and verifies no additional index
requests while rendering, plus partial-sync unknowns and missing-cache 409s.

All native hooks passed through the actual commit boundary: lint, architecture,
full build/test, changed-package coverage, module tidiness and file checks. The
initial lint warning on dynamic pocket indexing was resolved with explicit suite
mapping. No assertions or coverage floors changed. Nothing was pushed.

The real cache/API fixture also verifies cloud2 staging versus cloud1 proposed
and cloud0 updates with the same Ubuntu parent revision. Full UCA rebuild
versions remain significant, and the cached view still makes no index requests.
