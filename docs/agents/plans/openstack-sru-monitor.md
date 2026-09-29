# OpenStack SRU monitor implementation

- Resolve the union of configured package sets and discover public Launchpad
  bugs on configured Ubuntu package-series and Cloud Archive series targets.
- Build bug/package/target rows, read queue and source publications, and link
  only `.changes` entries whose `Launchpad-Bugs-Fixed` field names that bug.
  Store the snapshot atomically and serve filtered reads from it.
- Expose shared frontend workflows through API, CLI, and TUI; classify list,
  show, browser-open, and cache-sync actions in the action catalog.
- Route snapshot maintenance through `cache sync sru`, `cache status`, and
  `cache clear sru`; keep `sru` commands for list and show.
- Check parsing, version precedence, task/evidence disagreement, parent gates,
  filtering, API contracts, CLI paths, TUI rendering, and a live read-only
  Launchpad example. Keep failed refreshes from replacing the prior snapshot.
- Present attention targets grouped by bug and package in the default CLI
  list; retain all-target, per-target, and detailed-diagnostic views as flags.
- Keep inferred shared-task associations visible in diagnostics without making
  a completed target require attention solely because of that association.
