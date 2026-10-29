# Spec 003: Native host snapshot preview

**Status:** Accepted
**Owner:** Repository owner
**Created:** 2026-09-09
**Accepted:** 2026-09-09

## User outcome

An owner can run one native command on Windows, macOS, or Linux and receive a bounded, privacy-safe JSON snapshot that
conforms exactly to `observer-current-snapshot/v1` without starting a listener or writing machine history.

## Requirements

- **R1 Contract projection:** `observer collect-once` MUST emit the existing current-snapshot v1 shape, not the raw collector model.
- **R2 Honest capabilities:** Unimplemented service, container, log, and observer sections MUST be `UNSUPPORTED`, `NOT_RUN`, and empty.
- **R3 Safe native data:** Overview, filesystem, and process fields MUST remain bounded and MUST exclude paths, user identity, arguments, environment variables, and raw errors.
- **R4 Quality:** Unavailable and permission-denied data MUST not be represented as healthy zeroes; total, returned, and truncated list counts MUST be explicit.
- **R5 Portability:** The runtime MUST test on GitHub-hosted Windows, macOS, and Linux and cross-build amd64 and arm64 binaries for all three systems with pinned actions in less than 15 minutes. Linux CI MUST run the race detector.
- **R6 Lockstep:** A deterministic Go projection fixture MUST be validated by both Go tests and the canonical AJV contract suite.
- **R7 CLI semantics:** Help MUST exit 0. Invalid usage MUST exit 2. The command MUST emit a schema-valid snapshot and exit 1 when all implemented visible sections fail; `OK` and `PARTIAL` snapshots exit 0.
- **R8 Acquisition budget:** Enumerate process IDs through the maintained OS adapter, but attempt details for at most
  4096 IDs per collection, including failed attempts. Check cancellation before discovery and between detail reads.
  Bound result allocation to that limit and preserve discovered/scanned/error counts. A scan-budget cutoff is partial
  coverage, not a complete machine-wide top-N list or a collection error. OS ID enumeration itself is not claimed
  constant-memory; the cap applies before process construction and metadata reads.
- **R9 Process CPU basis:** The pinned process adapter reports lifetime CPU time / process age relative to one core,
  not current whole-host utilization. Preserve finite 0..409600 values internally (up to 4096 logical cores) instead
  of dropping valid multi-core processes above 100. Keep aggregate host CPU at 0..100. The existing public current-v1
  projection retains its 0..100 saturating display contract; it is not a raw full-range or recent-sample measure.
  Rich workload contracts must declare the measurement basis and expose full-range data separately.

## Boundaries

This preview adds no HTTP listener, storage, upload, dashboard, service manager, container adapter, or log collector.
The raw collector may retain safe metrics that the closed v1 schema cannot represent; the projection MUST omit them.

## Acceptance evidence

`go test ./...`, `go vet ./...`, and `npm test` pass. The fixture manifest validates the native preview, projection tests
lock the Go result to that fixture, and every native runtime CI job validates a real `collect-once` snapshot through AJV.
Tests also cover shared 16-filesystem/200-process projection caps, true truncation totals, access-denied process scans,
projection-boundary name sanitization, projected collection quality, CLI help, and failed-snapshot exit behavior.
