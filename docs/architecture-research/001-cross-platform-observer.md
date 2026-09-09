# Architecture research: cross-platform observer

**Status:** Complete  
**Date:** 2026-09-09  
**Decision owners:** Repository owner and maintainers

## Decision question

How should a public, independently installable observer collect rich host, workload, container, and log data on Windows, macOS, and Linux; run headless or with a local UI; integrate with Platform Demo; and remain safe and maintainable as a portfolio-quality project?

## Current evidence

The private prototype proved several useful behaviors: separate collector and uploader roles, a strict `home-lab-server-snapshot/v1` latest-state payload, a 1 MiB upload ceiling, bounded inventories, one-use enrollment, and Linux Compose deployment. It also exposed the limits we need to remove:

- publication and provenance are tied to a private infrastructure repository;
- the artifact is Linux/amd64-only and the install flow is Bash/Compose-only;
- Platform Demo is configured around one OCI image rather than a versioned multi-platform release manifest;
- the replace-current snapshot is not a suitable carrier for bounded time series or log observations;
- a container on Docker Desktop observes its Linux VM, not the physical Windows or macOS host;
- access to a Docker-compatible socket is host-equivalent authority even if a client intends to call read endpoints only.

The public replacement therefore keeps v1 remote compatibility during migration but owns a new source history, package identity, native lifecycle, local data model, and explicit security boundary.

## Product patterns studied

- [Glances](https://github.com/nicolargo/glances) demonstrates a dense cross-platform overview, plugin-based collectors, processes, sensors, containers, terminal/web/API modes, and multiple exporters. We should copy the discoverability and compact information hierarchy, not its unrestricted network defaults or dependency footprint.
- [Cockpit](https://docs.cockpit-project.org/cockpit-guide/latest/guide/features.html) places indexed journal data in its own view and links logs into service, storage, and network investigations. We should preserve that contextual drill-down.
- [Grafana Explore](https://grafana.com/docs/grafana/latest/visualizations/explore/logs-integration/) combines a log-level histogram, time filtering, structured-field expansion, positive/negative filters, surrounding context, and signal correlations. A smaller form of this interaction fits the local log explorer.
- [Netdata log management](https://learn.netdata.cloud/docs/logs-management) places native log sources and per-second metrics in one time context and keeps OS-native logs available to existing tools. We should prefer indexed observation references and short bounded excerpts over duplicating a full log platform.

## Options

### A. One Go binary with platform adapters

Build one source tree and release native binaries for six OS/architecture pairs. The binary exposes explicit modes, while installed profiles run collection, upload, and local web roles as separately permissioned processes.

Strengths:

- no runtime installation; small, inspectable deployment unit;
- standard cross-compilation and static embedding for the local UI;
- mature standard HTTP/server libraries and race/fuzz/security tooling;
- native OS integration is possible without pretending containers provide host parity;
- one domain, contract, redaction, and update implementation.

Costs:

- platform-specific services and logs still require native adapters and real-host tests;
- native installer signing is operationally different on Windows and macOS;
- a single artifact does not itself create process-level least privilege.

Go supports the latest two major language releases and documents a strong compatibility promise; we will pin an exact supported toolchain per release and update it through Dependabot in grouped intervals. See [Go release policy](https://go.dev/doc/devel/release), [toolchain selection](https://go.dev/doc/toolchain), and [Go 1 compatibility](https://go.dev/doc/go1compat).

### B. Packaged Python runtime

Extend the existing Python collector and use PyInstaller or platform installers.

Strengths: fastest source-level migration and broad system-observation libraries.

Costs: larger and less transparent artifacts, native bundling/signing complexity, slower startup, duplicated packaging behavior, and harder privilege-separated deployment. It preserves prototype convenience but does not simplify the owner experience.

### C. Compose OpenTelemetry/Prometheus/Grafana components

Deploy an OpenTelemetry Collector plus metrics/log backends and a dashboard.

Strengths: powerful ecosystem, standard exporters, sophisticated query/visualization paths.

Costs: too many services and storage engines for a “download and run” desktop/server tool, weak Windows/macOS physical-host parity in container form, and a much larger configuration/security burden. OpenTelemetry export remains a useful optional adapter, not the product core. Its system semantic conventions provide good vocabulary guidance: [OpenTelemetry system metrics](https://opentelemetry.io/docs/specs/semconv/system/).

### D. Embed or iframe the local UI in Platform Demo

Strength: apparent UI reuse.

Costs: hosted-browser access to loopback/private networks creates mixed-origin, permission, DNS-rebinding, and reachability problems; it also collapses the local-owner and remote multi-tenant authentication models. The evolving [Local Network Access specification](https://wicg.github.io/local-network-access/) is additional evidence that browser-to-local integration is an unstable boundary.

## Recommendation

Choose option A, with API-first UI reuse rather than option D.

```text
native OS / container / log adapters
             |
collector process (machine read authority, no Platform credential)
             |
normalize -> redact -> bounded SQLite store
             |                         |
uploader process                  local web process
(credential + outbound TLS)       (sanitized reads + loopback)
             |                         |
Platform Demo APIs                embedded local dashboard
             |
shared dashboard view model through a Platform-specific adapter
```

The modes may share one executable and libraries, but operating-system process boundaries and state-directory ACLs enforce privilege separation. The uploader cannot open Docker/native collector handles. The web process reads only sanitized storage. Future actions use separate credentials and allowlisted typed operations.

## Data and API patterns

- Retain the strict remote snapshot v1 projection during migration.
- Introduce an independent `/api/v1` local contract and publish OpenAPI 3.1 plus JSON Schemas.
- Make support state explicit: `AVAILABLE`, `DEGRADED`, `UNAVAILABLE`, `DISABLED`, `PERMISSION_DENIED`, or `UNKNOWN`.
- Store the current bounded inventories, low-cardinality numeric series, sanitized state transitions, log rollups, and optionally selected redacted messages.
- Default to seven days or 250 MiB, whichever comes first. Sample current resource metrics every 15 seconds, then downsample older points. Retention is configurable and exposes eviction/drop counters.
- Keep raw arguments, environments, arbitrary files, mount details, credentials, and unrestricted log bodies out of storage and APIs.
- Log bodies are local-only, disabled by default, and enabled per allowlisted source. Remote log-body upload requires a future retention/privacy contract.
- Provide `/api/v1/info`, `/status`, `/capabilities`, `/snapshots/latest`, inventory endpoints, bounded `/series`, `/events`, `/logs`, `/privacy`, OpenAPI, liveness/readiness, and optional protected OpenMetrics.
- Apply point-count, cursor, range, string, response-byte, and query-time limits. Use [RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html) without returning stacks or arbitrary exception text.

SQLite WAL supports a local writer with concurrent readers but is not intended for a network filesystem; that fits a single-node observer with an explicit local state directory. See [SQLite WAL](https://www.sqlite.org/wal.html).

## Logging adapters and privacy

- Linux: structured journald when available, explicit syslog/file adapters only when configured.
- Windows: allowlisted Windows Event Log channels; privileged channels report permission denial rather than broadening service authority.
- macOS: allowlisted unified-log predicates; access depends on the installation profile and system permissions.
- Docker: container log metadata and optional bounded messages through a constrained API proxy.

OpenTelemetry Collector recipes confirm these are distinct sources with distinct permissions, cursor persistence, and replay behavior: [cross-platform log collection](https://learn.netdata.cloud/docs/opentelemetry/logs-collection). The observer uses a normalized `LogObservation` stream with source, timestamp, severity, resource reference, stable fingerprint, structured safe fields, redaction count, and optional message. It does not inflate the current snapshot with log arrays.

## Local UI patterns

- Overview: freshness, saturation, capacity, recent changes, and clear partial/degraded state.
- Trends: synchronized CPU/memory/disk/network charts with a shared time cursor.
- Workloads: processes, services, and containers with filter/sort/drill-down and safe fields.
- Logs: severity histogram, source/unit/container filters, text search, structured fields, context, and links back to a selected time window.
- Privacy and sync: exactly what is local, eligible for upload, redacted, dropped, stale, or failing.

The dashboard is an optional same-origin client of the local API, served from embedded assets and bound to explicit loopback addresses. No CDN, analytics, permissive CORS, or LAN listener. Platform Demo uses the same transport-neutral view model against Platform APIs; it never fetches a user's localhost endpoint.

## Distribution and operations

- Native archives: Windows amd64/arm64, macOS amd64/arm64, Linux amd64/arm64.
- Background profiles: Windows Services, macOS LaunchDaemon/LaunchAgent, Linux systemd.
- Optional OCI image: Linux amd64/arm64 at `ghcr.io/braidenm/home-lab-observer`.
- Release assets: manifest, final-byte SHA-256 checksums, SPDX SBOMs, provenance/attestations, installers, license/privacy summary, and changelog.
- Use GitHub-hosted runners for all public pull requests. Pin actions, use job-scoped least privilege, and keep release/signing workflows tag/manual-only.
- Build, natively sign, notarize/staple, archive, hash, attest, verify, then publish an immutable release. [GitHub artifact attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations) support public repositories; [Apple notarization](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution) is external and may exceed the normal 15-minute PR CI target.
- Installers download and verify before execution, use private temporary directories, never place enrollment secrets in URLs/arguments/environment variables, and support an owner-only token file for automation.
- Default to manual versioned updates with current/previous side-by-side rollback. Auto-update is future opt-in because it is privileged remote code installation.

Docker exposes a versioned REST API and recommends SDK negotiation, but the API can perform everything the Docker client can. The collector therefore receives only a constrained read projection, not the raw socket: [Docker Engine API](https://docs.docker.com/reference/api/engine/) and [daemon security](https://docs.docker.com/engine/security/).

## Proof and acceptance strategy

1. Contract fixtures prove remote snapshot v1 parity and additive local API compatibility.
2. Actual-host tests run on Windows, Intel/Apple silicon macOS, and amd64/arm64 Linux; cross-compilation alone is insufficient.
3. Socket inventory proves loopback-only listeners; malicious Host/Origin and mutation methods fail closed.
4. Secret canaries in environment, arguments, labels, mounts, logs, and HTTP headers never appear in stores, APIs, metrics, service logs, uploads, or support bundles.
5. Storage tests cover age/byte eviction, disk-full, corruption, restart, and offline upload pressure.
6. Permission tests prove collector, uploader, web, and future action roles cannot acquire one another's handles or credentials.
7. Clean-machine install/upgrade/rollback/uninstall canaries verify artifact signatures/attestations before Platform Demo switches its approved release.
8. Responsive/accessible UI tests cover keyboard use, 390 px layouts, stale/partial states, filters, tables, and time-to-log correlation.

## Risks and mitigations

- **Metric cardinality and database growth:** code-owned dimensions, current-only inventories, downsampling, age/byte limits.
- **Sensitive log content:** body-off default, allowlists, redaction before persistence, no remote messages in v1.
- **Docker authority:** constrained proxy and separate process/account; never surface its socket through HTTP.
- **False platform parity:** capability states and native acceptance tests.
- **Local UI exposure:** explicit loopback binds, token, Host/Origin validation, no CORS, strict CSP.
- **Broken rollback after migration:** backward-readable state or a verified pre-upgrade backup before switching.
- **Public supply-chain attacks:** GitHub-hosted PR runners, no PR secrets/OIDC, pinned actions, protected releases, checksums, SBOM, attestations.

## Owner decisions

Accepted defaults:

- Go single binary and optional Linux container.
- Headless installed profile, on-demand loopback UI.
- Seven days/250 MiB local retention with 15-second recent samples and downsampling.
- No LAN listener in v1.
- Local-only opt-in log messages; only log rollups/metadata are eligible for first-release remote upload.
- Platform Demo uses a versioned adapter/shared UI model rather than an iframe or browser-to-localhost calls.
- Manual updates first; opt-in automatic updates later.
- Windows/macOS artifacts may be labeled preview and rely on checksums/attestations until signing identities are available; native signing remains a GA gate.

These defaults can be amended before their implementation spec is accepted.
