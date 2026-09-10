# Spec 009 acceptance evidence

This is an implementation ledger, not a claim that the feature is released. Preview 2 does not collect native log
metadata. Contract and helper-hardening tests establish prerequisites; the end-to-end acceptance work below remains
required before enabling or publishing the feature.

| Requirement | Current evidence | Remaining acceptance work |
| --- | --- | --- |
| L1 Explicit source opt-in | `internal/logobs` accepts only the two fixed source aliases | CLI/background settings, platform-specific preset rejection, disabled-no-native-call tests |
| L2 Selected-field native acquisition | Linux and Windows fixed-seam reader kernels with selected-field, normalization, row/byte and thread/close tests; no body type | Actual native bindings and owned fixtures, helper entrypoints, macOS unsupported adapter |
| L3 Independent bounded acquisition | Single-flight 60-second collector with separately bounded Store phases and four-second native lane; crash-policy tests | Real helper timeout/kill/wait, digest identity and controlled environment, production lifecycle wiring |
| L4 Sanitized vocabulary and bounded cache | Closed vocabulary, encoded checkpoint canaries, owned 200-event session ring and deep-clone tests | Safe current projection and body-omission integration tests |
| L5 Atomic bounded persistence | PR 18: isolated additive log schema, CAS/rollback/restart/cancellation tests, safe counters, shared chronological age/size retention and durable eviction frontiers | Production lifecycle/shutdown ordering and packaged restart verification |
| L6 Read-only fixed-grid summary | PR 16: authenticated GET/HEAD handler, exact query/body/response limits, direct Go handler/schema/semantic checks including maximum grid | Wire optional source into production lifecycle |
| L7 Honest counts, gaps and freshness | Sticky interval reducer, whole-second conservative proof, positive-known versus null, safe aggregation, latest-status reduction and failed-write overlays | Cross-component native backlog/missed-poll lifecycle acceptance |
| L8 Current-memory versus durable history | Snapshot count/order bounds and cloned boundary values | Current projection limits 0..200, stale cache retention, restart-with-history/empty-ring tests |
| L9 Responsive independent summary UI | Optional data-source adapter with additive-field projection; independent loading/range cancellation; explicit legacy/error/disabled/history-retained states; coverage-overlaid histogram and keyboard-scrollable complete table; focused 390px DOM regression | 390/768/1440 real-browser visual review against the assembled API |
| L10 Verified privacy and native delivery | Strict bounded private helper codec and adversarial tests; contract canaries and independent reviews; closed release-v2 profile tests preserve v1 boundaries | Actual helper/installer v2 lifecycle, six-target packaged smoke, independent final review and verified release |

## Reviewed contract boundaries

- Private checkpoints are excluded from JSON, including their default base64 representation.
- Cursor/tail probes and a deferred lookahead count toward 513 native visits without becoming captured/discarded rows;
  a continuation reader reserves its probe and sentinel budget before ingesting.
- Individually valid counters must also remain safe after bucket, source and window aggregation.
- A reset is not a successful collection; source status and historical coverage are separate dimensions.
- Missing Linux helper/runtime support must not add a dynamic loader dependency to the main observer executable.

Keep this ledger and [tasks](tasks.md) current as each focused implementation PR adds actual evidence. Tests for a
contract alone must never be used to mark its native, persistence, transport or UI implementation complete.
