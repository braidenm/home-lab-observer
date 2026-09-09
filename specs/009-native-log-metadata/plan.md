# Plan

1. Freeze the selected-field reader, normalized event/batch, atomic cursor/rollup store and summary contracts before
   independent implementation. Put native reading, domain state, storage and HTTP/UI projection behind small interfaces.
2. Native reader slice: fixed Windows WEVTAPI and Linux journalctl adapters, macOS unsupported adapter, bounded fake/native
   tests. Run the log collector immediately and every 60 seconds in its own single-flight lane; it must never block or
   reschedule the 15-second host snapshot lane. Never run an unrestricted host-log query during development.
   Enforce Windows deadlines with a fixed same-executable helper process, preserving query-handle thread affinity and
   cancellation-before-close ordering inside the child. Linux requires journalctl 242+ with private per-attempt cursor
   files. Test actual child timeout/reaping, not only context-aware fakes.
3. Store/domain slice: one bounded session-only current ring, compact minute source/severity rollups, coalesced coverage,
   latest-attempt metadata, atomic checkpoint transaction, coverage queries and bounded retention/migration tests.
   Keep SQLite `user_version=2`; use additive tables plus a dedicated log-metadata version in `store_metadata` so the
   prior preview ignores the new tables. Freeze revision-CAS commits, ambiguous-outcome rereads, metadata-only tail
   reset without counts, reset-pending empty-source behavior and caught-up query-start coverage from
   review-decisions.md. Do not introduce cyclic package dependencies between readers, history and projection.
4. API/UI slice: explicit source startup settings, current/capability projection, closed summary schema, optional source,
   histogram/filter/status presentation and native service smoke. Extend background settings without enabling sources
   in older settings; maintain uninstall/upgrade behavior.
5. Independently review native acquisition, persisted fields, count/coverage semantics and UX. Prove disabled and
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
