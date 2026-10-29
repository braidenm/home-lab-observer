# Implementation plan: Native host snapshot preview

1. Extend the safe internal model only with schema-required filesystem type, process state, and list-count metadata.
2. Project raw observations through a dedicated package into closed `observer-current-snapshot/v1` structs.
3. Lock deterministic Go output to an AJV-validated synthetic fixture.
4. Make `collect-once` encode the projection and add native/cross-build CI.

Rollback reverts this slice; it creates no listener, persisted state, or migration obligation.

## Acquisition-budget correction (2026-10-29)

The old provider called gopsutil ProcessesWithContext and read every process before the collector applied its 4096
cap. Use PidsWithContext plus a small injected discovery/detail boundary so both process construction and detail reads
stop at the budget. Reuse the existing failure categories and propagate explicit scan truncation into partial quality.
Tests use synthetic IDs/readers, not a large real host or a second collection implementation. Preserve Windows' unknown
status fallback. No dependency, wire schema, privileges, egress, persistence or runtime service changes are introduced.
The existing native CI jobs prove platform compatibility; local tests do not stand in for all three OS runs.
