# Spec 009 acceptance evidence

This is an implementation ledger, not a claim that the feature is released. Preview 2 does not collect native log
metadata. Contract and helper-hardening tests establish prerequisites; the end-to-end acceptance work below remains
required before enabling or publishing the feature.

| Requirement | Current evidence | Remaining acceptance work |
| --- | --- | --- |
| L1 Explicit source opt-in | `internal/logobs` accepts only the two fixed source aliases | CLI/background settings, platform-specific preset rejection, disabled-no-native-call tests |
| L2 Selected-field native acquisition | Accepted ADRs 009/010; no native payload/body type in `logobs` | Linux/Windows adapters, selected-field fakes and native smoke, macOS unsupported adapter |
| L3 Independent bounded acquisition | Batch/checkpoint count and byte bounds; `journalruntime` crash-policy tests, including isolated Linux child | Collector cadence, real helper timeout/kill/wait, source deadlines, exact cursor reset, digest identity and controlled environment |
| L4 Sanitized vocabulary and bounded cache | Closed source/severity/event-code validation; encoded checkpoint JSON canaries; deep-clone tests | Native normalization, 200-event ring, safe current projection and body-omission tests |
| L5 Atomic bounded persistence | Reader/store ports and revision/batch invariants | Additive SQLite schema, CAS/restart/ambiguous-write tests, safe counters, shared retention/WAL budget, shutdown ordering |
| L6 Read-only fixed-grid summary | Fixed-grid domain validation; closed wire contract checkpoint | Executable wire validation, authenticated GET/HEAD handler, request and 256-KiB response limits |
| L7 Honest counts, gaps and freshness | Independent count/coverage validation, positive-known versus null, safe-aggregate regression tests | Store-derived sticky gaps, source-status aggregation, failed-write overlays, backlog and missed-poll integration tests |
| L8 Current-memory versus durable history | Snapshot count/order bounds and cloned boundary values | Current projection limits 0..200, stale cache retention, restart-with-history/empty-ring tests |
| L9 Responsive independent summary UI | Accepted presentation/compatibility contract | Optional data-source adapter, loading/error/disabled states, accessible charts/table and 390/768/1440 visual review |
| L10 Verified privacy and native delivery | Contract canaries and independent review; closed release-v2 profile tests preserve v1 rejection/rollback boundaries | Helper protocol/adversarial tests, installer v2 lifecycle, six-target packaged smoke, independent final review and verified release |

## Reviewed contract boundaries

- Private checkpoints are excluded from JSON, including their default base64 representation.
- A deferred 513th examined row cannot become a captured/discarded row or a caught-up proof.
- Individually valid counters must also remain safe after bucket, source and window aggregation.
- A reset is not a successful collection; source status and historical coverage are separate dimensions.
- Missing Linux helper/runtime support must not add a dynamic loader dependency to the main observer executable.

Keep this ledger and [tasks](tasks.md) current as each focused implementation PR adds actual evidence. Tests for a
contract alone must never be used to mark its native, persistence, transport or UI implementation complete.
