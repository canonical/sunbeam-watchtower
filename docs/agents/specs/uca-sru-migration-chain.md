# UCA SRU migration chains

## Scope

Show archive observations and inferred migration prerequisites for a source
package and selected UCA series. Reuse configured Ubuntu releases/backports and
cached OpenStack `series_status.yaml` list positions for release ordering.
Numeric release IDs are optional: older series such as Yoga retain their names
and their position in that list. The scope is
**configured series**, not a statement of Canonical UCA support. Upstream
maintenance status is not displayed or used to filter targets. Yoga remains
in scope when configured even if its upstream support has ended.

The desired fix is selected by Launchpad bug ID and explicit source package.
Package sets scope bulk SRU monitoring only; they do not restrict this query. A current
publication whose `.changes` names that bug supplies explicit association
evidence; versions are not used to associate fixes across releases. The
furthest observed pocket with matching bug evidence satisfies that pocket
prerequisite. Transitions are listed with prerequisites first. This does not establish bug retention in later unassociated
uploads or applicability in releases without matching evidence.

Publications without Launchpad bug references, including security uploads,
remain in the pocket inventory. Missing references, unavailable `.changes`
files and failed association fetches preserve observed source versions with
unknown fix association. They participate in inferred proposed occupancy
using package and pocket versions independently of bug identity.

The existing bug-oriented SRU list, show and cache behavior stays unchanged.
Raw pocket inventory retains all inspected configured targets independently
of bug association. Lower UCA series receive backport labels and inferred fix
steps only when publication evidence identifies a backport of the selected
bug. Absence is not missing work or a dependency.

## Evidence and inference

Pocket observations include source package, archive, series, Ubuntu base,
pocket, source version and publication URL. A failure to fetch a pocket or
resolve its fix evidence is unknown, never an empty pocket or a migration pass.
Version comparisons are confined to one source package and archive target.

The workflow described by the operator supplies the ordering policy:

- A pending proposed publication must reach updates before a later staging
  publication is promoted to proposed in that target.
- A desired fix in staging of a newer configured UCA series must enter proposed
  before the selected series enters proposed.
- When the desired fix is already in proposed or updates of a newer series,
  an unrelated proposed publication does not create that prerequisite.
- Existing configured parent-Ubuntu requirements remain explicit.

Archive state does not prove actual migration blockage or that a publication
is otherwise eligible to migrate. Dependencies must be labeled as inferred
policy, and the view must show the indirect chain and outstanding transitions.
Missing fix evidence does not establish applicability or required backporting.

## Verification boundary

Test observed pocket state separately from inference, including the Nova
Caracal/Epoxy chain with an unrelated occupant, already-released fixes, failed
fetches, absent fix evidence, multiple newer waits and observed Yoga backports.
Exercise the API and actual CLI transport/rendering path. Live read-only checks
confirm archive integration only; they do not establish migration eligibility.

## Telemetry

This view intentionally has no new domain metrics. It reports an on-demand
read-only archive inspection, has no durable operational state, and does not
add a collector or background fan-out. Existing outbound HTTP tracing must
wrap the reused Launchpad client construction.
