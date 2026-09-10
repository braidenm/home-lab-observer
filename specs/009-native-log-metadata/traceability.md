# Spec 009 acceptance evidence

This is an implementation ledger, not a claim that the feature is released. Preview 2 does not collect native log
metadata. Contract and helper-hardening tests establish prerequisites; the end-to-end acceptance work below remains
required before enabling or publishing the feature.

| Requirement | Current evidence | Remaining acceptance work |
| --- | --- | --- |
| L1 Explicit source opt-in | Fixed-alias validation, platform-specific CLI rejection, explicit background persistence, legacy profiles and disabled-no-native-call tests | Final release verification |
| L2 Selected-field native acquisition | Both native bindings and private helper entrypoints implemented; Linux owned fixture, selected-field/normalization/bounds/thread-close tests; macOS unsupported adapter | Successful Windows owned native fixture |
| L3 Independent bounded acquisition | Independent 60-second lane, four-second native limit, real helper timeout/kill/wait and digest identity tests, controlled environment, pre-input crash hardening and lifecycle wiring | Windows owned native fixture; final release verification |
| L4 Sanitized vocabulary and bounded cache | Closed vocabulary, private-checkpoint canaries, owned 200-event session ring, deep clones and safe current/body-omission projection tests | Final native acceptance audit |
| L5 Atomic bounded persistence | Additive schema, CAS/rollback/cancellation, safe counters, shared retention, HTTP/collector shutdown ordering; actual packaged synthetic history survives graceful and forced restart | Final release verification |
| L6 Read-only fixed-grid summary | Authenticated bounded GET/HEAD contract and semantic tests; runtime current and summary share the same cache-only collector | Final release verification |
| L7 Honest counts, gaps and freshness | Sticky interval reducer, conservative coverage, positive-known/null counts, safe aggregation, backlog/missed-poll and failed-write tests; packaged unavailable source stays explicit alongside retained history | Windows owned native fixture; final acceptance audit |
| L8 Current-memory versus durable history | Current limits 0..200, count/order/cloning and stale-cache tests; actual packaged restart retains history while the new session ring is empty | Final release verification |
| L9 Responsive independent summary UI | Optional independent data source, range cancellation, explicit legacy/error/disabled/history states, coverage overlays and accessible table; 390/768/1440 real-browser review with synthetic observations; independent runtime/UX review at `9c65825` | Final release verification; browser fixtures do not prove live native acquisition |
| L10 Verified privacy and native delivery | Private protocol/adversarial tests, canaries, independent reviews, v1 compatibility, paired reproducibility, actual v2 Linux/Windows/macOS installer smoke and packaged scratch recovery proof | Windows owned fixture, six-target publication smoke, final review/merge and verified release |

See [the exact hosted CI evidence](runtime-ci-gates.md) for head `7f5a1e3`. Independent review at `9c65825`
found no additional runtime/security/UX blockers and passed focused Go tests/vet. Successful fixture acquisition,
packaged degraded/restart behavior, synthetic UI evidence and published release support are distinct claims.

## Reviewed contract boundaries

- Private checkpoints are excluded from JSON, including their default base64 representation.
- Cursor/tail probes and a deferred lookahead count toward 513 native visits without becoming captured/discarded rows;
  a continuation reader reserves its probe and sentinel budget before ingesting.
- Individually valid counters must also remain safe after bucket, source and window aggregation.
- A reset is not a successful collection; source status and historical coverage are separate dimensions.
- Missing Linux helper/runtime support must not add a dynamic loader dependency to the main observer executable.

Keep this ledger and [tasks](tasks.md) current as each focused implementation PR adds actual evidence. Tests for a
contract alone must never be used to mark its native, persistence, transport or UI implementation complete.
