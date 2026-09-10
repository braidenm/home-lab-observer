# Spec 009 review record

## Contract checkpoint — in progress

This review does not approve native readers, persistence, API handlers, UI integration or a new release. Those are
separate acceptance steps in [the evidence ledger](traceability.md).

### Neutral Go contracts

Independent read-only review of `8cdb6c3` reported no remaining blockers for this slice after correcting:

- Reader batches accepting projection-only or volatile storage-overlay reason codes.
- Safe per-source counters overflowing when aggregated across sources.
- UTC timestamps outside the supported RFC3339 year range.

The implementation review also tightened successful/caught-up/reset combinations, partial normalization versus
truncation, the pre-iteration discard-array bound, and checkpoint revision/attempt-time consistency. Regression tests
cover application-only sources, positive counts in gap buckets, null versus invented zero, fractional grid rejection,
deep cloning, encoded private-checkpoint exclusion and aggregate overflow.

Reviewer verification: `go test ./internal/logobs -count=20`, `go vet ./internal/logobs`, and `git diff --check` passed.
Root additionally ran the full Go suite/static analysis and existing contract checks; the final combined PR must rerun
all required CI after the wire checkpoint is integrated. A synthetic credential test triggered repository scanning;
the test now constructs its synthetic prefix at runtime, without weakening repository policy.

### Linux helper process guard

`journalruntime` establishes and verifies process-local zero core limits and disabled dumpability, returning only a
fixed failure code. Its native Linux test runs in an isolated child, proves idempotence and verifies that the parent's
policy is unchanged. Native runtime/delivery CI passed on commit `c659e61`; no journal data was read by this test.
The Linux main dependency graph was also checked to exclude the journal guard/helper and purego loader packages.
Wiring the guard before private input in the actual helper remains required.

### Release and wire contracts

The closed release-v2 profile was independently reviewed: Linux requires its digest-described helper, other targets
retain their exact content set, and v1 rollback validation remains unchanged. Build and installer behavior has not yet
switched to v2.

The summary schema, executable semantic fixtures and latest-quality aggregation still require final cross-slice review.
The draft PR must not be marked ready on the strength of the neutral-contract review alone.
