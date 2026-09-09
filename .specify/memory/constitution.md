# Home Lab Observer Constitution

**Version:** 1.0.0  
**Ratified:** 2026-09-09  
**Last amended:** 2026-09-09

## I. Safe observation before control

The observer is read-only by default. Observation permissions and future action permissions are separate capabilities, credentials, APIs, audit records, and specifications. No generic command execution endpoint is permitted.

## II. Local ownership and explicit egress

The owner can use the product entirely on the observed machine. Remote upload is optional, authenticated, encrypted, tenant-bound, and limited to an explicit data policy. Collection, persistence, display, and upload each apply the same redaction rules.

## III. Portable core, honest capabilities

One product supports Windows, macOS, and Linux through platform adapters. The API reports capabilities and collection failures explicitly; it never fabricates parity or confuse an unavailable signal with a healthy zero value. Native binaries are the primary full-host distribution, with containers as an optional Linux-oriented package.

## IV. Contract-first modularity

Collectors produce normalized, versioned domain observations. Storage, local APIs, the embedded UI, exporters, and remote clients depend on contracts rather than concrete collectors. Breaking contract changes require a new API/schema version and a migration plan.

## V. Privacy and bounded data

Potentially sensitive fields are excluded by default. Raw process arguments, environment variables, secrets, arbitrary file contents, and unrestricted logs are prohibited. Log bodies require explicit source allowlisting and opt-in. Local queues and history have enforceable size and age limits, with visible drop and redaction counters.

## VI. Operable by ordinary owners

Installation, upgrades, rollback, service management, diagnostics, and removal must be documented and scriptable. Failure must be actionable. The service exposes health, readiness, its own structured logs, and metrics without recursively ingesting secrets.

## VII. Verified delivery

Every change begins with a specification and acceptance criteria. Pull requests use parallel, change-aware checks and target completion within 15 minutes. Releases are reproducible, checksummed, attested, scanned, and tested on clean Windows, macOS, and Linux environments. Public pull requests never execute on private self-hosted runners.

## Governance

- Specifications define product behavior; ADRs define durable technical decisions; this constitution overrides both.
- Amendments require a pull request explaining the change, affected specifications, migration impact, and version bump.
- Reviewers verify constitution compliance before merge. Exceptions must be time-bounded and recorded in an ADR with an owner and removal condition.
- Semantic versioning applies to this constitution and to public observer contracts.
