# SRU package version view implementation

Reuse live publication collection without bug/changes reads. Resolve all configured UCA series (or the optional selected
series) directly from config; no upstream ordering is necessary.
Calculate currency in the SRU domain service, preserving raw versions and
unknown/empty distinctions. Present the four cells with aligned versions and
state words, coloured using existing CLI styling.

Each configured series has an independent row; comparisons never cross Ubuntu
parent boundaries. The default command has no series flag.

Add the canonical read-only action, shared server/client workflows, API and CLI
with table, JSON and YAML output. Preserve the existing bug-oriented chain and
bulk monitoring behaviour.

Verification boundary: domain regression cases for Debian epochs/revisions,
cloud suffixes, newer staging, unavailable cells, invalid versions and empty
pockets; full HTTP API path through app and Launchpad fixtures with no bug,
changes, package-set or upstream-cache dependency; CLI transport/output and
classification tests; native pre-commit gates; actual CLI live read-only
observation and terminal colour checks. No publishing or eligibility claim.

## Verification observed

The actual read-only CLI inspected Open vSwitch in Yoga: Ubuntu Jammy and UCA
staging were current at 2.17.12-0ubuntu0.22.04.1 (staging adds ~cloud0); proposed
and updates were behind at 2.17.9-0ubuntu0.22.04.1~cloud0. A separate actual-CLI
HTTP fixture passed table/JSON/YAML and PTY checks: green ANSI 121 for current,
yellow ANSI 221 for behind, and no ANSI with --no-color or structured output.

Native lint, architecture, build, module tidiness and file checks passed. The new API fixture's request count was corrected with operator approval:
the simulated 503 is retried four times, yielding 15 HTTP requests rather than
12. Version/state assertions remain unchanged. The operator requested completion
and the local commit after reviewing the proposed correction. No tests are skipped or weakened.

The operator requested all configured series by default. The actual command
without --series returned six live rows: caracal, epoxy, flamingo, gazpacho,
hibiscus and yoga. Currency remains independent per row. The all-series API and
CLI regressions passed, as did native lint, architecture, build and module
checks after this extension. The earlier exact request-count correction is included in the final native
test and coverage verification.

All native hooks passed through the actual commit boundary, including the full
Go test suite and changed-package coverage. No remote push, PR or CI was used.

Cloud rebuild regression cases distinguish cloud0/cloud1/cloud2 and Debian
numeric cloud2/cloud10 ordering between UCA pockets. Only comparison with the
first (Ubuntu parent) cell ignores the trailing UCA cloud suffix.
