# Spec 002: Local API contracts and compatibility fixtures

**Status:** Accepted
**Owner:** Repository owner  
**Created:** 2026-09-09
**Accepted:** 2026-09-09

## User outcome

An owner, local tool, or future dashboard can inspect a machine through a documented read-only contract and can tell
the difference between a healthy zero, an unsupported signal, a disabled collector, a temporary failure, and a
permission denial. Contract consumers can develop before the runtime exists by using synthetic fixtures, while later
Windows, macOS, Linux, container, storage, UI, and upload slices share one stable vocabulary.

## Slice boundary

This specification delivers contracts and executable contract validation only:

- an OpenAPI 3.1 description for local capabilities, the current sanitized snapshot, and detail-free health reads;
- versioned JSON Schema 2020-12 documents for capabilities, the current snapshot, and RFC 9457-style Problem Details;
- valid and intentionally invalid synthetic fixtures;
- bounded query, collection, response, privacy, and log-body rules;
- a compatibility policy and requirements-to-evidence traceability matrix; and
- a GitHub-hosted contract check designed to finish in less than 15 minutes.

This slice does **not** implement a listener, collector, store, uploader, CLI, installer, dashboard, Go package, or
release artifact. Those remain subsequent Spec 002 slices and must consume these contracts rather than redefine them.

## Functional requirements

- **R1 Versioned API:** The canonical local contract MUST be OpenAPI 3.1 under `/api/v1`. It MUST contain only `GET`
  and `HEAD`-compatible read semantics. Detail-free liveness and readiness reads MAY be outside `/api/v1`.
- **R2 Capabilities:** `GET /api/v1/capabilities` MUST report code-owned collectors and distinguish `SUPPORTED`,
  `DISABLED`, `UNAVAILABLE`, `PERMISSION_DENIED`, and `UNSUPPORTED` without fabricating numeric values.
- **R3 Current snapshot:** `GET /api/v1/snapshots/current` MUST return a versioned, timestamped, bounded snapshot with
  overview, filesystems, processes, services, containers, logs, and observer self-health sections.
- **R4 Collection quality:** Every snapshot section MUST report support state, collection state, freshness,
  stable nullable reason code, total and returned counts, and truncation. Missing and zero values MUST remain distinct.
- **R5 Query bounds:** Snapshot projection MUST accept only code-owned section names. Process, container, and log limits
  MUST be bounded to 200, 500, and 200 respectively. Invalid limits MUST return typed Problem Details.
- **R6 Response bounds:** Capabilities responses MUST be at most 128 KiB and current snapshots at most 1 MiB after
  UTF-8 encoding. A producer MUST truncate deterministically within per-section limits or fail with a bounded problem;
  it MUST NOT stream an unbounded response.
- **R7 Privacy profile:** Both contracts MUST disclose their effective privacy classification and upload eligibility.
  Raw process arguments, environment variables, credentials, authorization headers, arbitrary file contents, private
  addresses, and unrestricted attributes are prohibited.
- **R8 Log separation:** Log metadata and message bodies MUST be separate objects. Metadata MAY be included from a
  code-owned source. A message body MUST remain omitted by default; an included body MUST be redacted, local-only,
  bounded to 2 KiB, and explicitly ineligible for upload.
- **R9 Problems:** API failures MUST use `application/problem+json` with HTTP status, stable machine-readable `code`,
  optional bounded field issues, and an opaque request ID. Problems MUST NOT contain stack traces, raw exceptions,
  rejected secrets, authorization values, or source payloads.
- **R10 Compatibility:** Additive optional response fields are compatible and clients MUST ignore fields they do not
  understand. Removing or renaming a field, changing meaning/type/unit, narrowing a bound, changing an enum without an
  unknown-value strategy, or adding a required input requires a new major API/schema version and migration plan.
- **R11 Fixtures:** Each schema MUST have a valid fixture. Invalid fixtures MUST cover an unknown support state,
  inconsistent unsupported data, upload-eligible log bodies, and forbidden problem internals.
- **R12 Machine validation:** One documented command MUST validate OpenAPI structure, all schemas, valid and invalid
  fixtures, read-only paths, security declarations, bounds, fixture size, privacy canaries, and traceability.
- **R13 Cross-platform vocabulary:** Fixtures MUST exercise honest platform/capability differences without claiming
  that this slice ships a Windows, macOS, Linux, or container collector.
- **R14 Legacy boundary:** The existing Platform Demo `home-lab-server-snapshot/v1` payload remains a separate strict
  upload projection. Rich local fields and log bodies MUST NOT be added to it by implication.

## Non-functional requirements

- Contract validation runs on a GitHub-hosted runner with read-only repository permissions and a five-minute job timeout.
- Schema and fixture validation is deterministic and requires no host telemetry, Docker socket, private runner, secret,
  network service, or production identifier.
- Examples use reserved domains, generic platform labels, and synthetic identifiers only.
- Contract files use UTC RFC 3339 timestamps, base units, bounded strings, closed item objects, and code-owned enums.
- The OpenAPI document and schemas are source artifacts. Generated runtime types are deferred until a Go module exists.

## Compatibility policy

`/api/v1` is a stable major contract once a runtime release implements it. Within v1:

1. Producers may add optional response fields after adding fixtures and documentation.
2. Consumers ignore unknown response fields and treat unknown enum values as `UNKNOWN`, never as healthy or offline.
3. Request parameters, required response fields, field meanings, base units, privacy class, and published maxima do not
   change incompatibly.
4. A breaking change is introduced beside v1 as `/api/v2` and `.../v2` schemas, with an explicit support window,
   migration fixtures, observability, and rollback plan.
5. Stored/uploaded schemas version independently from the HTTP surface. The Platform Demo legacy upload projection is
   not the local snapshot schema.

## Acceptance evidence

- `npm test` validates and dereferences the OpenAPI contract.
- Every valid fixture passes its declared schema and every invalid fixture fails it.
- The validator rejects mutation operations, wildcard/non-loopback servers, missing bearer security, missing bounds,
  excessive fixture bytes, secret-shaped valid data, and missing requirement traceability.
- [traceability.md](traceability.md) maps R1-R14 to contract locations, fixtures, and automated assertions.

## Cross-platform roadmap retained

After this accepted slice, delivery remains incremental:

1. normalized domain types and cross-platform capability fakes;
2. native Windows, macOS, and Linux host collectors plus bounded SQLite storage;
3. loopback API runtime, health/OpenMetrics, and embedded responsive dashboard;
4. platform-specific service, container, and allowlisted log adapters;
5. optional outbound enrollment/upload compatibility with Platform Demo; and
6. installers, service lifecycle, container profile, provenance, signing, and clean-machine canaries.

Each slice updates these fixtures and compatibility evidence. A platform remains `UNSUPPORTED` until its actual-host
acceptance suite passes; checksums or attestations do not substitute for Windows Authenticode or Apple notarization.

## Open owner decisions deferred

- Whether the first prerelease must be Apple-notarized and Windows Authenticode-signed.
- Which log sources may offer local message bodies in guided setup.
- When a richer remote history contract is justified beyond the legacy latest-snapshot projection.
