# Meaningful machine observability: product research

**Researched:** 2026-09-09  
**Scope:** Read-only, single-machine observer with an optional local UI

## Compared projects and patterns

| Project | Useful pattern | Deliberate boundary here |
| --- | --- | --- |
| [Beszel](https://github.com/henrygd/beszel) | Lightweight agent/hub split, historical host and container data, alerts, user-owned systems | Keep local API/UI useful without a hub; remote ownership comes later |
| [Netdata logs](https://learn.netdata.cloud/docs/logs-management) | Query native OS logs in place; correlate filters/histograms with metrics at the same time | Log sources and bodies remain opt-in, bounded, redacted, and local-first |
| [Cockpit metrics](https://docs.cockpit-project.org/cockpit-guide/main/guide/cockpit-metrics.html) | Treat metrics as time series with declared sampling intervals and reusable presentation grids | Publish a small closed API instead of exposing a generic local control channel |
| [Glances](https://github.com/nicolargo/glances) | Dense cross-platform host/process overview with optional web/API modes | Preserve a calm drill-down hierarchy and explicit support/data-quality states |

The Docker daemon is a privileged boundary, not merely another metrics endpoint. Docker's
[daemon security guidance](https://docs.docker.com/engine/security/) and a 2026 Beszel
[path traversal advisory](https://github.com/henrygd/beszel/security/advisories/GHSA-phwh-4f42-gwf3) reinforce using
fixed, validated reads through a constrained adapter rather than forwarding arbitrary Docker paths or identifiers.

SQLite's [WAL guidance](https://sqlite.org/wal.html) requires a local filesystem, bounded reader lifetimes, and regular
checkpoints to prevent WAL growth. It also documents a WAL-reset race fixed in SQLite 3.51.3 and selected backports.
The observer therefore verifies its embedded SQLite version, uses one write/checkpoint owner, keeps transactions short,
and will not ship a WAL build containing the affected SQLite versions.

## Adopted information architecture

1. **Overview:** present health, freshness, collection gaps, capacity pressure, and important changes first.
2. **Trends:** correlate CPU, memory, filesystems, network rates, process/workload counts, and later events on one clock.
3. **Workloads:** sortable process, OS service, and container inventories with bounded per-item metrics and drill-down.
4. **Logs:** select one explicit native/container source; filter severity/event code/time; show histograms and adjacent
   metrics; keep bodies omitted unless locally enabled and redacted.
5. **Observer & privacy:** show capabilities, permissions, drops, retention, database/WAL size, collection latency,
   version/update state, and exactly what may leave the machine.

## Signal roadmap

| Layer | Signals | Interpretation |
| --- | --- | --- |
| Host | CPU/load, memory/swap, filesystems and I/O, network rates/errors, uptime | saturation, pressure, capacity, connectivity |
| Hardware | temperature/fans, battery, SMART/RAID, supported GPU data | thermal, wear, power, device health |
| Workloads | processes, OS services, Docker/Podman state and resource deltas | regressions, restarts, hot consumers, availability |
| Events/logs | source/severity/event-code rollups; opt-in redacted bodies | correlate state changes and failures without becoming a SIEM |
| Observer | collection latency/failures, queue drops, store/WAL/retention, upload state | determine whether the telemetry itself is trustworthy |

## Product conclusion

A rich dashboard should increase interpretation, not merely cardinality. Each screen therefore pairs values with time,
support, freshness, limits, and source. The first local-data slice stores only code-owned numeric series. Higher-risk
Docker, service, hardware, and log adapters arrive behind explicit capabilities and separate threat-tested specs.
