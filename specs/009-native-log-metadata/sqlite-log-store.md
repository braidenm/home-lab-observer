# Additive log-history implementation

Status: transaction and query implementation under review. Shared retention maintenance and application wiring are
required before this slice is enabled or released.

The log Store port borrows the existing SQLite connection and ownership lock. It never opens or closes a second
database and leaves core `user_version=2` unchanged. Its lazy version key is `log_metadata_schema_version=1` in
`store_metadata`. Initialization is transactional and validates the exact code-owned table definitions. An unknown
version, partial schema, cancellation or failed log initialization returns a fixed log-storage error without
quarantining an otherwise valid host database. A cancelled attempt does not poison future initialization attempts.
Summary queries validate existing tables when needed but never create them.

Three bounded data sets are separate from host metrics:

- `log_metadata_state`: at most the two fixed source keys, canonical decimal uint64 revision, bounded private cursor,
  reset state, last attempt/caught-up checkpoint and closed latest status. Checkpoint watermarks survive an unavailable
  status even when that status must expose unknown freshness.
- `log_metadata_minutes`: source/minute keys, captured/discarded counts and the seven severity counters. No event code,
  message, provider, process identity or raw row is persisted.
- `log_metadata_coverage`: sorted disjoint half-open intervals and closed gap reasons. Adjacent identical intervals
  coalesce; commits write only changed intervals instead of rewriting a week of evidence on every poll.

Timestamps use fixed-width UTC text with nine fractional digits, avoiding Unix-nanosecond overflow across accepted
RFC 3339 years. Revision text preserves the full uint64 range and overflow rejects the commit. SQLite checks bound
each counter; transaction-level checks also prevent aggregate overflow across sources. Invalid future record time is
rejected at the Store boundary, and already-expired backlog is not inserted into rollups.

Each commit validates the request/checkpoint transition and atomically changes rollups, coverage, latest status and
revision. A failed check or write rolls everything back. The CAS update distinguishes explicit revision conflict from
an uncertain database result. Only the caller's specified ambiguity procedure may retry an identical batch.

One read transaction projects the exact requested fixed grid, using the same coverage reducer as commit tests.
Known counts do not promote gaps to full coverage; unknown/gap cells without records retain null counts. Latest
failure does not erase historical counts. Every returned object is independently owned and validated before leaving
the Store boundary. Query/read failures never turn into fabricated empty history.

Size eviction also records a monotonic per-source frontier in the two fixed `store_metadata` keys
`log_metadata_evicted_before_system` and `log_metadata_evicted_before_application`. Later commits cannot recreate
coverage before that frontier, even when accepted within-horizon future events were removed before the next poll.
Counts may remain known-positive independently; missing proof is unknown, never a reconstructed full healthy zero.
Each log write rechecks shared database/WAL size after acquiring the connection and refuses further growth while
already over budget. Maintenance can recover space; failed pressure writes do not advance checkpoints.

Independent review identified derived lower-bound underflow for valid early year-1 timestamps. Retention/initial
bounds now clamp to the earliest valid non-zero UTC instant; empty clipped intervals add no evidence. Tests cover
that edge, uint64 revisions, concurrent CAS contenders, cancelled writes and future-skew eviction/reconstruction.

Remaining before delivery: share the existing seven-day/250-MiB maintenance budget with log rows (including WAL),
verify bounded eviction under storage pressure, complete independent review and cancellation/concurrency tests,
integrate collector lifecycle ordering, and run all required cross-platform checks.

## Shared maintenance policy

Maintenance only joins log history when the existing optional schema validates; it never initializes, migrates,
or quarantines that schema. Absent or incompatible log tables leave core-only retention unchanged. The existing
seven-day age and 250-MiB database/WAL/SHM pressure policy applies to the combined store, not a second allowance.
Log age is capped at seven days even when host retention is configured longer; a shorter host retention also applies
to logs. Existing drop counters count deleted rows, not interval trims, although trims consume the work budget.
The at-most-two source state/cursor rows survive retention for checkpoint continuity.

With a compatible schema, each age or pressure pass shares one configured batch budget across samples, host
rollups, log minutes and log coverage. Candidates are compared by actual UTC time (not overflowing UnixNano
conversion of log years 1–9999), oldest first with deterministic ties. Age processing clips coverage crossing the
cutoff; bounded passes converge without a host-table-first starvation rule.

Before removing a log minute, positive coverage preceding that source's minute-end frontier is deleted or trimmed.
Every ancillary interval mutation consumes the same batch budget; if exhausted the count row stays until a later
pass. Missing evidence projects UNKNOWN, never a fabricated FULL zero. Removing a coalesced coverage record can
conservatively lose wider proof while preserving known counts. Private checkpoint and latest-status rows remain
unchanged; their watermark is not itself historical coverage proof. Size pressure is reported until bounded passes,
incremental vacuum and WAL checkpoints reduce the combined allocation below the configured limit.

Two fixed metadata keys, `log_metadata_evicted_before_system` and `log_metadata_evicted_before_application`, persist
monotonic canonical UTC eviction frontiers. A minute deletion atomically advances its source frontier to minute-end
(clamped to the largest representable timestamp in year 9999). Future commits clip new positive coverage to at least
that frontier, including after restart; otherwise a later caught-up read could reconstruct false FULL zero evidence
over an evicted future-skew minute. Counts remain independently meaningful below the frontier. No raw metadata is added.
At most two frontier upserts are fixed transaction bookkeeping, like the existing drop-counter update, outside the
historical-row batch budget. A failed frontier write rolls back all deletion/proof mutations.
