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

## Process CPU range correction (2026-10-29)

gopsutil v4.26.9's `CPUPercentWithContext` computes `100 * CPU seconds / process age`; multi-core work can legitimately
exceed 100. The shared host-percent predicate caused the collector to discard these processes. Use a dedicated finite
bounded process predicate, retaining aggregate host validation and the existing public current-v1 projection unchanged.
No new sampling/cache/clock algorithm, dependency, egress or authority is added. Unit tests cover full-range preservation
and invalid-number rejection; one collector-to-projection regression proves legacy saturation remains compatible.
Evidence: [the pinned source](https://github.com/shirou/gopsutil/blob/v4.26.9/process/process.go#L365-L382), verified against
the fetched module on 2026-10-29. This correction does not claim instantaneous CPU sampling.
