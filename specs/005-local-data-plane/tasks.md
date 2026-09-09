# Tasks: Local data plane and bounded history

- [x] Accept the loopback service, bounded-history, authentication, and embedded-dashboard boundary.
- [x] Add the closed series API/schema, fixtures, compatibility notes, and traceability.
- [x] Implement versioned SQLite migrations, low-cardinality samples/rollups, pruning, checkpointing, and quarantine.
- [x] Implement the single-flight scheduler, monotonic sequencing, current cache, and graceful shutdown.
- [x] Implement authenticated handlers, health reads, validation middleware, Problem Details, and response ceilings.
- [x] Build/embed the local dashboard and add token unlock/forget behavior.
- [x] Connect real series data and surface gaps, storage pressure, and unsupported sections truthfully.
- [ ] Add unit, integration, browser, restart, retention, secret-canary, and native-platform tests.
- [x] Update the README, security/privacy docs, CLI help, and operator documentation.
- [ ] Add the required bounded CI check and record acceptance evidence.

## Persistence and scheduler evidence

- `internal/history` pins a pure-Go SQLite driver embedding SQLite 3.53.4, enforces the 3.51.3 WAL safety floor,
  serializes the writer/checkpoint owner, and tests WAL mode, migrations, restart continuity, allowlisting, rollups,
  incremental age/size retention, persisted counters, checkpoints, and non-destructive corruption/version quarantine.
- `internal/scheduler` defaults to 15-second collection, coalesces triggers behind one collection goroutine, persists
  monotonic sequence allocation, caches the latest projected snapshot, extracts only allowlisted aggregate metrics,
  and tests immediate collection, trigger single-flight behavior, restart sequence continuity, and graceful close.

## Integration and review evidence

- Windows actual-binary smoke validates protected capabilities/current/history and Problem responses against all four
  JSON Schemas; two collection sequences produce real numeric trends. `cmd/observer` tests graceful drain and state reuse.
- Playwright against the embedded binary verifies unlock, session reuse/forget, five views, workload keyboard tabs,
  truthful unsupported collectors, real trend responses, and no page overflow at 390/768/1440 CSS pixels.
- The embedded bundle is rebuilt from source and checked for drift, external assets, and source maps.
- Independent cross-slice reviewers inspect storage/scheduler, API/series/auth, and CLI/security/CI before draft PR 8
  is marked ready. Findings and final hosted-platform evidence are recorded in the completed review document.
