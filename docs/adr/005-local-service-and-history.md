# ADR 005: Authenticated local service with bounded metric history

**Status:** Accepted  
**Date:** 2026-09-09

## Context

The native snapshot command and reusable dashboard need a working connection. Historical charts must survive process
restart without requiring another database service. The user wants a portable headless program with an optional local
dashboard and enough machine data to explain resource pressure and failures.

## Decision

`observer serve` starts a periodic collector, SQLite history, and a same-origin local dashboard. The initial listener
accepts only explicit IPv4 loopback addresses. The HTTP boundary validates Host and Origin, requires bearer authentication
for observation reads, and provides public health responses containing only UP or NOT_READY. A random local token lives
in the dedicated owner-controlled state directory. The dashboard unlocks by validating a pasted token and offers a lock
action that clears browser session state.

The history store uses the pure-Go `modernc.org/sqlite` driver to retain the existing CGO-free builds. It verifies a
SQLite version containing the WAL-reset fix. Fixed metric identifiers, prepared statements, bounded queries, short
transactions, retention and checkpoints limit cardinality and disk use. Process identities and log bodies are excluded
from historical series. Current inventories remain in memory.

The dashboard is compiled into locally served HTML/CSS/JavaScript and embedded in the binary. Generated assets are
committed so `go build` works without Node; CI rebuilds them and rejects differences. The reusable React package remains
the integration point for other applications. Platform Demo does not need a browser connection to an owner's localhost.

## Alternatives considered

- An external PostgreSQL/Prometheus stack provides richer shared query facilities but adds installation and operational
  dependencies to the first single-machine release.
- Memory-only history reduces storage complexity but loses the timeline at every restart.
- Cookie authentication adds state-changing session endpoints and CSRF concerns; header bearer authentication fits the
  existing read-only API and local unlock flow.
- Building UI assets only during release avoids generated files in Git but makes a normal checkout unable to reproduce
  the intended single-binary experience without additional build orchestration.

## Consequences

The foreground service is useful without an account or remote connection. Native capabilities remain platform-dependent
and explicitly unavailable when unsupported. Local bearer access is possession-based; owner/delegate permissions belong
to the later remote enrollment specification. First-time users copy a token from a local file; signed service installers
and richer onboarding remain the release specification's responsibility.

## Evidence and rollout

[Spec 005](../../specs/005-local-data-plane/spec.md) owns acceptance: actual native HTTP/schema tests, retained history,
query/authentication abuse tests, physical retention tests, and the packaged dashboard at phone/tablet/desktop widths.
[SQLite WAL documentation](https://sqlite.org/wal.html) supports the local filesystem and checkpoint requirements.
`collect-once` remains available while the foreground service is introduced. No production upload migration is part of
this slice.
