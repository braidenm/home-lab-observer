# ADR 003: Store bounded observations and privacy-preserving logs

**Status:** Accepted  
**Date:** 2026-09-09

## Context

A latest snapshot cannot explain trends or correlate resource spikes with workload and log events. Unbounded time series, process history, and logs create disk, cardinality, and privacy risk.

## Decision

Use embedded SQLite for single-node local state. Store current bounded inventories, code-owned low-cardinality metric series, sanitized state transitions, log rollups, and optionally redacted messages from explicitly allowlisted sources. Default history is seven days or 250 MiB, whichever is reached first; recent metrics sample every 15 seconds and older points are downsampled.

Log message bodies are disabled by default, local-only when enabled, and redacted before persistence. Only bounded metadata and rollups may enter the first remote telemetry contract. Raw process arguments, environment variables, arbitrary files, credentials, and unrestricted log sources are prohibited.

## Consequences

- The UI can correlate time, health, workloads, and events without becoming a SIEM.
- Retention, redaction, eviction, and drop behavior are visible and testable.
- SQLite state must remain on a local filesystem.
- A richer remote history/log-body service needs a separate privacy, tenancy, retention, and deletion specification.
