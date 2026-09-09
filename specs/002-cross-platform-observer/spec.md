# Spec 002: Cross-platform observer

**Status:** Draft  
**Owner:** Repository owner  
**Created:** 2026-09-09

## User outcome

An owner can download and run Home Lab Observer on Windows, macOS, or Linux; view meaningful current and historical machine health locally; inspect processes, services, containers, and explicitly enabled logs; and optionally connect the observer to Platform Demo without installing a language runtime or exposing an inbound internet service.

## Scope

- Native amd64 and arm64 binaries for Windows, macOS, and Linux.
- Headless service operation plus an optional responsive loopback dashboard.
- Normalized host, CPU, memory, filesystem, disk I/O, network, uptime, sensor, process, service, Docker, and observer health observations when supported.
- Bounded local trends and event correlations.
- Log adapters for journald/syslog, Windows Event Log, macOS unified logging, and Docker with metadata-first defaults, source allowlists, redaction, and opt-in message bodies.
- Versioned local JSON API, OpenMetrics endpoint, health/readiness endpoints, and OpenAPI/JSON Schema contracts.
- Optional outbound enrollment and snapshot upload compatible with Platform Demo.
- OS-native background-service installers, a verified portable installation path, upgrades, diagnostics, rollback, and uninstall instructions.
- Optional signed multi-architecture Linux container using a constrained Docker API proxy.

## Explicit non-goals

- Arbitrary command execution, shell access, process termination, service restart, or container restart.
- Full log-management/SIEM replacement or unbounded local retention.
- Kubernetes, Proxmox, SNMP, or multi-node orchestration in this specification.
- Public network listening by default.

## Functional requirements

- **R1 Capabilities:** `/api/v1/capabilities` reports supported, disabled, unavailable, and permission-denied collectors with actionable reasons.
- **R2 Overview:** The local UI provides a responsive health overview with current status, saturation, recent changes, and collection freshness.
- **R3 Trends:** Users can select a bounded time range for resource charts and correlate a point in time with workload and log events.
- **R4 Workloads:** Process, service, and container views support sort, filter, status, resource use, uptime/restart facts, and drill-down without exposing arguments or environment variables by default.
- **R5 Logs:** Users can filter enabled logs by time, source, severity, unit/container, and text; expand structured fields; view context; and see redaction/drop counts. Message storage is disabled until explicitly enabled per source.
- **R6 Storage:** Local retention has configurable age and byte ceilings, deterministic eviction, and visible utilization/drop metrics.
- **R7 API:** The UI and external consumers use the same documented, versioned read-only API. Unsupported fields remain distinguishable from zero.
- **R8 Remote upload:** Enrollment exchanges a one-use secret for a scoped renewable credential, upload uses outbound TLS, queues are bounded, and remote data policy is visible.
- **R9 Packaging:** Clean-machine installers select the correct verified artifact and can install, start, status, diagnose, upgrade, rollback, and uninstall the background service.
- **R10 Container:** Container deployment is documented as Linux-oriented and never claims visibility into the physical Windows/macOS host behind Docker Desktop.
- **R11 Self-observation:** Structured service logs, health/readiness, collection latency/failure counters, queue/storage pressure, version, and build provenance are exposed without leaking secrets.
- **R12 Performance:** Default collection remains lightweight and degrades individual collectors rather than stalling the entire API/UI.

## Data safety defaults

- Excluded: environment variables, raw process command lines, arbitrary file contents, credentials, request authorization, and Docker socket access through the public API.
- Log message bodies: disabled until enabled for an allowlisted source; redacted before persistence/upload.
- Local API: loopback-only with strict host/origin checks; non-loopback requires a separately documented authenticated TLS configuration.
- History and retry queues: bounded by both time and bytes.

## Acceptance matrix

Every release candidate must pass on Windows amd64, macOS amd64/arm64, Linux amd64/arm64, and the Linux container where applicable:

1. Fresh install and first healthy snapshot.
2. Background start, stop, restart, status, upgrade, rollback, and uninstall.
3. Local API contract and loopback enforcement.
4. Responsive dashboard smoke test at small and desktop viewports.
5. Capability reporting for present, missing, disabled, and denied sources.
6. Retention eviction, disk-full protection, redaction, and corrupt-store recovery.
7. Docker unavailable, reachable, denied, and degraded cases.
8. Offline upload queue, credential rotation, revocation, and server rejection.
9. Artifact checksum and GitHub attestation verification.

## Open owner decisions

- Default local trend retention and sampling interval.
- Whether the first prerelease must be Apple-notarized and Windows Authenticode-signed, or may initially rely on checksums and GitHub attestations while certificates are acquired.
- Which log sources may enable message bodies in the guided setup presets.
