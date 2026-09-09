# Implementation plan: Local data plane and bounded history

## Decision

Build one Go process around explicit collector, projection, history, HTTP, and embedded-asset boundaries. Use the
standard HTTP server and a pure-Go SQLite driver so the six existing native cross-build targets remain CGO-free.
The React package keeps its transport-neutral boundary; only its local adapter gains the accepted series call and
unlock lifecycle.

## Runtime shape

```text
native collectors -> scheduler -> normalized snapshot -> current cache
                                      |                    |
                                      v                    v
                               bounded SQLite       versioned local API
                                      |                    |
                                      +---- series query --+--> embedded dashboard
```

- The scheduler owns sequence allocation, deadlines, one active collection, persistence, and status counters.
- The store accepts only a fixed metric-row model and owns migrations, rollups, pruning, checkpoints, and quarantine.
- HTTP handlers consume interfaces and never call collectors or execute SQL directly.
- Static assets are produced from `web/observer-ui` in a deterministic build step and embedded under an internal
  package. Source maps are not embedded in release binaries.

## API and storage model

The series contract is a closed response shaped for `ObserverDataSource.getTrends`: requested range, actual sample
interval, generated time, and a bounded array of allowlisted series with UTC `{at,value}` points. Missing intervals are
null rather than interpolated. The store records low-cardinality numeric samples keyed only by metric identifier and
time bucket; inventories, process names/PIDs, source text, log bodies, arbitrary labels, and credentials are excluded.

Recent data remains at 15-second resolution. Rollups retain count, minimum, maximum, sum, and last value so the API can
select meaningful aggregates without pretending gaps are observations. Age pruning runs in small batches. Size pressure
prunes oldest raw rows first, checkpoints WAL, then oldest rollups, while incrementing visible eviction/drop counters.

## Security and privacy

Authentication is bearer-header only. The browser unlocks locally; it does not receive a token from query strings,
cookies, HTML, or an unauthenticated endpoint. Host/Origin validation and response security headers are enforced in
middleware shared by all protected reads. Structured runtime logs contain code-owned event names and request IDs but
not authorization headers, query payload echoes, machine-source payloads, or log bodies.

## Rollout and rollback

`collect-once` remains unchanged and provides a rollback-safe diagnostic path. `serve` and its state directory are
additive. Pre-release users can stop the server and remove only the documented observer state directory; corrupt stores
are quarantined rather than destroyed. A later release spec owns service installation and upgrade rollback.
