# Plan

1. Freeze the selected-field reader, normalized event/batch, atomic cursor/rollup store and summary contracts before
   independent implementation using [the wire checkpoint](contract-checkpoint.md) and
   [the internal ports](internal-contracts.md). Put native reading, domain state, storage and HTTP/UI projection behind
   small interfaces.
2. Native reader slice: fixed Windows WEVTAPI and bundled Linux libsystemd helper, macOS unsupported adapter, bounded fake/native
   tests. Run the log collector immediately and every 60 seconds in its own single-flight acquisition lane; it must
   not run inside or reschedule the 15-second host lane. Brief bounded shared-store contention is expected. Never run
   an unrestricted host-log query during development.
   Enforce Windows deadlines with a fixed same-executable helper process, preserving query-handle thread affinity and
   cancellation-before-close ordering inside the child. Linux uses private pipes, exact native cursor tests and the
   caller-accessible journal view. No visible initial tail means unavailable, not zero. Keep loader imports outside the
   main executable and test missing-helper/runtime fallback. Bound accepted-plus-discarded records to 512, count
   private cursor/tail probes against the same 513-visit ceiling, and reserve one deferred
   sentinel. Test actual child timeout/reaping, not only context-aware fakes.
3. Store/domain slice: one bounded session-only current ring, compact minute source/severity rollups, coalesced coverage,
   latest-attempt metadata, atomic checkpoint transaction, coverage queries and bounded retention/migration tests.
   Keep SQLite `user_version=2`; use additive tables plus a dedicated log-metadata version in `store_metadata` so the
   prior preview ignores the new tables. Freeze revision-CAS commits, ambiguous-outcome rereads, metadata-only tail
   reset without counts, reset-pending empty-source behavior and caught-up query-start coverage from
   internal-contracts.md. Store derives conservative 5-minute/60-second coverage from `PreviousAttemptAt`, retains
   sticky explicit gaps, resolves unknown cells only with evidence, and clamps starts to seven-day retention.
   Verify initial empty success, ordinary success, ten-minute missed poll, failure then success, backlog counts,
   long shutdown and shutdown cancellation/no-commit fixtures. Join log work before the host scheduler closes the
   borrowed store. Do not introduce cyclic package dependencies between readers, history and projection.
4. API/UI slice: explicit source startup settings, current/capability projection, closed summary schema, optional source,
   histogram/filter/status presentation and native service smoke. Extend background settings without enabling sources
   in older settings; maintain uninstall/upgrade behavior.
5. Package the Linux helper under ADR 010's closed manifest-v2 content profile. Embed its build digest in the main
   binary; verify same-commit helper identity, exact archive entries, anonymous install, upgrade/rollback and core
   startup without the helper's dynamic dependencies. Preserve v1 rollback and the existing total archive byte bounds.
6. Independently review native acquisition, persisted fields, count/coverage semantics and UX. Prove disabled and
   degraded cases alongside successful fixtures. All required CI must pass before auto-merge and preview publication.

## Summary shape to finalize at the contract checkpoint

The response is capped at 256 KiB. Its exact top-level vocabulary is `schema_version`, `generated_at`, `range`,
`window_start`, `window_end`, `bucket_interval_seconds`, `expected_bucket_count`, `support_state`, `collection_state`,
`freshness`, `observed_at`, `reason_code`,
`coverage_state`, nullable `counts`, fixed `limits`/`privacy`, and `sources`. Each configured source appears with a fixed
grid and `status`, `coverage_state`, `covered_seconds`, nullable `counts`, and `buckets`, even when unsupported. Status
contains only `support_state`, `collection_state`, `freshness`, `observed_at`, `attempted_at`, `coverage_through` and
`reason_code`. Each bucket contains only `at`, `coverage_state`, `covered_seconds`, `reason_code`, and nullable counts;
bucket counts contain only `captured`, `discarded`, and severity counts named `trace`, `debug`, `info`, `warn`, `error`,
`critical` and `unknown`. `limits` contains only `max_sources`, `max_buckets_per_source` and `max_response_bytes`;
`privacy` contains only `data_classification`, `contains_log_bodies`, `contains_event_codes`,
`contains_identity_fields` and `remote_upload_eligible`. Use 60 one-minute points
for 1h, 72 five-minute points for 6h, 96 fifteen-minute points for 24h and 168 hourly points for 7d, all aligned to UTC
epoch boundaries. Coverage is exactly FULL, PARTIAL, GAP or UNKNOWN and stays independent of captured-event counts.
FULL/PARTIAL buckets with positive coverage always have numeric counts, including proven zero. GAP/UNKNOWN counts can
be positive-known or null, never invented zero; null requires neither known records nor coverage evidence. Attribute
invalid-timestamp discards to attempt time. A latest failure does not erase historical buckets, and `coverage_through`
is only the latest caught-up watermark.

Current event codes are exactly `WIN_<uint32>`, `WIN_<32-lowercase-hex-provider-guid>_<uint32>`,
`SYSTEMD_<32-lowercase-hex-message-id>`, or `SYSTEMD_PRIORITY_<0..7>`, all at most 64 bytes. No native provider name or
arbitrary string passes through. Counters above JavaScript's maximum safe integer reject the complete commit rather
than wrap or round.
