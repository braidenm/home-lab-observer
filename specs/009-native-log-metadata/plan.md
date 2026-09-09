# Plan

1. Freeze the selected-field reader, normalized event/batch, atomic cursor/rollup store and summary contracts before
   independent implementation. Put native reading, domain state, storage and HTTP/UI projection behind small interfaces.
2. Native reader slice: fixed Windows WEVTAPI and Linux journalctl adapters, macOS unsupported adapter, bounded fake/native
   tests. Never run an unrestricted host-log query during development.
   Enforce Windows deadlines with a fixed same-executable helper process, preserving query-handle thread affinity and
   cancellation-before-close ordering inside the child. Linux requires journalctl 242+ with private per-attempt cursor
   files. Test actual child timeout/reaping, not only context-aware fakes.
3. Store/domain slice: sanitized collector cache, atomic rollup/checkpoint transaction, coverage queries and bounded
   retention/migration tests. Freeze revision-CAS commits, ambiguous-outcome rereads, reset-without-counting semantics
   and caught-up query-start coverage from review-decisions.md. Do not introduce cyclic package dependencies between
   readers, history and projection.
4. API/UI slice: explicit source startup settings, current/capability projection, closed summary schema, optional source,
   histogram/filter/status presentation and native service smoke. Extend background settings without enabling sources
   in older settings; maintain uninstall/upgrade behavior.
5. Independently review native acquisition, persisted fields, count/coverage semantics and UX. Prove disabled and
   degraded cases alongside successful fixtures. All required CI must pass before auto-merge and preview publication.

## Summary shape to finalize at the contract checkpoint

The response contains schema version, generation time, range/window/bucket seconds, explicit limits/privacy,
captured/discarded counts and at most two source summaries. Each source has latest status and historical counts/points.
Seven severity columns use fixed enums; missing coverage is nullable. Use 60 one-minute points for 1h, 72 five-minute
points for 6h, 96 fifteen-minute points for 24h and 168 hourly points for 7d. Explain partial buckets and captured-only
counts rather than infer totals from sampling or label deferred lookahead as a loss.

Current event code examples are fixed normalized `WINDOWS_EVENT_<id>`, optional validated provider GUID plus ID, and
`SYSTEMD_<32hex-message-id>` or `SYSTEMD_PRIORITY_<0..7>`. No native provider name or arbitrary string passes through.
