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

Remaining before delivery: share the existing seven-day/250-MiB maintenance budget with log rows (including WAL),
verify bounded eviction under storage pressure, complete independent review and cancellation/concurrency tests,
integrate collector lifecycle ordering, and run all required cross-platform checks.
