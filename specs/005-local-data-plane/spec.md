# Spec 005: Local data plane and bounded history

**Status:** Accepted  
**Owner:** Repository owner  
**Created:** 2026-09-09  
**Accepted:** 2026-09-09

## User outcome

An owner can start one observer process, open a polished dashboard on the same machine, and inspect current host facts,
top processes, filesystems, and honest historical CPU, memory, storage, network, and process-count trends. The service
remains read-only, loopback-only, bounded on disk, useful after restart, and explicit when data is missing or stale.

## Slice boundary

This slice delivers the local data plane needed to turn the Spec 004 dashboard into a working product:

- `observer serve` with a periodic native collection loop and graceful shutdown;
- the existing v1 capabilities/current-snapshot API plus a narrowly allowlisted v1 series endpoint;
- embedded SQLite history with deterministic retention, downsampling, corruption isolation, and health signals;
- a generated local bearer secret and a same-origin dashboard unlock flow;
- compiled dashboard assets embedded into the native binary; and
- integration, security, retention, browser, native-OS, and restart tests.

This slice does **not** add Docker, service-manager, GPU, sensor, SMART, native-log, upload, installation, background
service, alerting, or remote action support. Those capabilities remain visibly `UNSUPPORTED` until their own specs.

## Functional requirements

- **R1 Loopback service:** `observer serve` MUST bind `127.0.0.1:9847` by default and MUST reject wildcard,
  non-loopback, hostname-resolved, and malformed bind values. LAN access is not a v1 feature.
- **R2 Periodic collection:** The service MUST collect immediately and every 15 seconds by default, run at most one
  collection at a time, assign monotonic sequence values, retain the last schema-valid snapshot on a partial failure,
  and expose collection/storage failures through observer signals and readiness without terminating the process.
- **R3 Exact current reads:** `/api/v1/capabilities` and `/api/v1/snapshots/current` MUST preserve Spec 002 schemas,
  bounds, media types, query validation, and Problem Details behavior. Health reads MUST reveal only liveness/readiness.
- **R4 Allowlisted series:** `GET /api/v1/metrics/series` MUST accept only `range=1h|6h|24h|7d` and a bounded list
  of code-owned metric identifiers. It MUST return UTC points, explicit gaps as null, actual sample resolution, and
  separate series for CPU utilization, memory utilization, aggregate filesystem utilization, network byte rates, and
  process count when supported. Arbitrary metric names, labels, SQL, expressions, or unbounded ranges are prohibited.
- **R5 Bounded SQLite history:** Local history MUST use SQLite on a local filesystem, WAL mode, transactions, prepared
  statements, a single migration owner, and a schema version. Default retention is seven days or 250 MiB, whichever
  is reached first. Recent 15-second samples MUST be downsampled for longer ranges; eviction MUST be incremental,
  deterministic, observable, and tested without retaining process identities or log bodies.
- **R6 Safe recovery:** A confirmed corrupt or incompatible database MUST be preserved under a collision-safe timestamped quarantine name;
  the observer MAY start a new store but MUST report degraded readiness and a stable reason code. It MUST never silently
  delete or overwrite the failed store. Busy/locked, permission, and transient I/O failures MUST fail safely without
  quarantining an otherwise valid store. Only one process may own a state directory at a time.
- **R7 Local authentication:** On first service start, the observer MUST generate at least 256 bits of cryptographic
  entropy, write the token to a user-owned state file with the strongest supported local permissions, and print its
  location—not the token—during ordinary startup. API credentials are accepted only through `Authorization: Bearer`.
  They MUST NOT appear in URLs, process arguments, environment variables, logs, Problem Details, or persisted metrics.
- **R8 Browser boundary:** The API MUST validate `Host` and browser `Origin`, disable CORS, reject cross-origin and
  unsupported methods, set CSP/anti-framing/content-type/referrer headers, enforce request/response/time ceilings, and
  render source data as text. The dashboard MAY hold a pasted token in memory or session storage only and MUST provide
  a clear lock/forget action.
- **R9 Single embedded UI:** Release builds MUST embed locally built dashboard assets in the Go binary, use no CDN,
  analytics, external fonts, or runtime scripts, and preserve the reusable package boundary for Platform Demo.
- **R10 Truthful dashboard:** The local adapter MUST use the real series endpoint. Unsupported collectors, null points,
  gaps, stale samples, partial snapshots, collection duration, returned/total counts, truncation, retention, and storage
  pressure MUST remain visible and MUST NOT be presented as healthy zero values.
- **R11 Operable process:** The service MUST handle SIGINT/SIGTERM, close the listener and store cleanly, bound all
  queues and goroutines, emit structured observer-owned logs without source payloads or credentials, and expose no
  mutation endpoint.
- **R12 Fast evidence:** Contract, unit, integration, browser, and Windows/macOS/Linux smoke checks MUST run in parallel
  on GitHub-hosted runners and remain below the repository's 15-minute CI target.

## Acceptance evidence

- A clean binary starts on all three native operating systems, serves the embedded dashboard, and produces valid
  current and series responses after authentication.
- Tests cover wrong/missing bearer tokens, hostile Host/Origin values, unsupported methods, malformed and oversized
  queries, Problem Details safety, response bounds, single-flight collection, graceful shutdown, and secret-canary
  non-propagation.
- Store tests cover restart continuity, migrations, UTC ordering, null gaps, downsampling, age/size eviction, WAL
  checkpointing, bounded growth, power-loss/corrupt-file quarantine behavior, and observable drops.
- A packaged browser test at 390, 768, and desktop CSS widths proves unlock/forget, keyboard navigation, real current
  data, real trends, explicit unsupported sections, and no horizontal page overflow.

## Follow-up roadmap

- Spec 006: opt-in read-only Docker inventory and resource readings through a specialized local read model.
  Podman compatibility, OS services, temperatures/sensors/SMART and native log sources are deferred to separately
  reviewed focused collector specifications; log bodies retain explicit local opt-in and redaction requirements.
- Spec 007: signed/checksummed native releases, Linux container, Bash/PowerShell installers, service definitions,
  upgrades, uninstalls, and rollback.
- Spec 008: outbound enrollment/upload into Platform Demo with owner/resource binding and no inbound host exposure.
- A later separately threatened capability may add allowlisted actions; arbitrary shell and raw Docker forwarding remain
  prohibited.
