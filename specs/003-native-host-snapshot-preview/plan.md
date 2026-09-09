# Implementation plan: Native host snapshot preview

1. Extend the safe internal model only with schema-required filesystem type, process state, and list-count metadata.
2. Project raw observations through a dedicated package into closed `observer-current-snapshot/v1` structs.
3. Lock deterministic Go output to an AJV-validated synthetic fixture.
4. Make `collect-once` encode the projection and add native/cross-build CI.

Rollback reverts this slice; it creates no listener, persisted state, or migration obligation.
