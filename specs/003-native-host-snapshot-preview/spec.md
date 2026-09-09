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

## Boundaries

This preview adds no HTTP listener, storage, upload, dashboard, service manager, container adapter, or log collector.
The raw collector may retain safe metrics that the closed v1 schema cannot represent; the projection MUST omit them.

## Acceptance evidence

`go test ./...`, `go vet ./...`, and `npm test` pass. The fixture manifest validates the native preview, projection tests
lock the Go result to that fixture, and runtime CI exercises native test jobs, the Linux race detector, and six no-CGO cross-builds.
