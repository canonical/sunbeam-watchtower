# UCA SRU migration chain implementation

1. Expose cached upstream series metadata through the existing provider port.
2. Resolve configured UCA targets and associated Ubuntu parents using that
   order; reject targets whose order cannot be established.
3. Collect pocket publications independently of bug association and retain
   unknown results on upstream failures.
4. Match desired-fix evidence using the operator-selected identity, expose
   lower observed backports and infer the described policy dependencies.
5. Add a read-only shared frontend action and API/CLI view; keep existing
   SRU monitor behavior unchanged.
6. Test the inference, real adapter integration and CLI path; run native
   quality gates and changed-package coverage, perform a live read-only
   check where possible, sync PLAN.md and commit locally.

## Delivery boundary

The operator selected Launchpad bug identity and configured series scope.
The API and CLI inspect live pockets using the shared frontend seam, with a
read-only, embedded-compatible, MCP-exportable action. The TUI's existing
bug-oriented monitor is unchanged; this first step is the dedicated API/CLI
chain view. No new background collector or dependency is introduced.

Verification includes inference regressions, HTTP publication and `.changes`
fixtures, action classification, API error semantics, and the actual CLI.
Live archive checks verify observation integration only, never migration
eligibility or Canonical support status.

## Verification evidence

The corrected ordering tests retain configured Yoga with a name-only upstream
entry. No support or applicability decision uses upstream lifecycle status.

The actual CLI passed table, JSON and YAML checks against a local API fixture.
The API test exercises the handler, shared frontend workflow, cached release
metadata, Launchpad publication/changes clients and dependency inference.
Regression fixtures cover unrelated and bugless proposed occupants, unknown
fetch/association state, already-released fixes, multiple newer waits, parent
requirements and lower observed backports.

A live read-only CLI inspection of Neutron, Caracal and bug 2167438 returned
12 configured inventory targets (10 relevant chain targets) with known pocket
inventories, including name-only Yoga. The desired fix
was not observed in Caracal, so the result correctly leaves its applicability
and required work unknown. The hypothetical Nova Caracal/Epoxy dependency
scenario is verified through fixtures, not claimed as the current archive
state. Migration eligibility and actual publishing remain outside the boundary.

Native quality gates passed through `pre-commit run --files ...`: lint,
architecture rules, `go build ./...`, `go test ./...`, changed-package coverage
and module tidiness, plus file checks. Lint normalized import grouping on the
first pass and passed after normalization. `GOFLAGS=-buildvcs=false` was set
only for these local checks because Go attempted VCS stamping against an
unrelated home-directory Git directory; no tests or coverage floors changed.

Commit-hook verification exposed inherited Git repository variables in temporary
Git fixtures. Shared fixture commands now remove inherited `GIT_*` variables
before selecting a temporary repository, including config overrides and index
paths. A regression executes a real fixture commit with simulated hook variables
and checks that the invoking repository's config and index stay untouched.
No assertions or coverage floors were relaxed. The failed hook had overwritten
local Git settings with fixture values; those were removed and `core.bare` was
restored to false. The exact prior local identity/signing overrides were not
recorded; identity now inherits the existing global developer configuration.

The fixture-isolation commit passed every native hook through an actual
`git commit`, including lint, architecture rules, full build/test, changed-package
coverage and module tidiness. The feature commit also passed all of those native hooks through an actual
`git commit`. The final CLI fixture rerun passed table, JSON and YAML output.

An operator query for `openvswitch --series yoga --bug-id 2154006` exposed an
incorrect dependency on the bulk SRU monitor's package sets. Explicit chain
queries now bypass that allowlist while preserving configured-series and query
validation. The operator approved correcting the API expectation that had
encoded the restriction. The full API integration regression runs with no
monitoring sets; app regressions cover absent sets and a set omitting the query.

The actual CLI query completed against live Launchpad with 12 inventory targets.
Yoga staging was 2.17.12-0ubuntu0.22.04.1~cloud0; proposed and updates were both
2.17.9-0ubuntu0.22.04.1~cloud0. Their absent Launchpad bug references leave fix
association unknown. This verifies the reported command's observation path,
without establishing migration eligibility.

Native pre-commit checks passed after this correction: lint, architecture, full
build/test, changed-package coverage, module tidiness and file checks.
