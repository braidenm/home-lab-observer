# Spec 009 acceptance evidence

This is an implementation ledger, not a claim that the feature is released. The reviewed implementation is merged to
`main` at `9a60a28`; the last independently verified published Preview 2 does not collect native log metadata. Preview
3 publication and post-publication verification remain pending and are tracked separately below.

| Requirement | Current evidence | Remaining acceptance work |
| --- | --- | --- |
| L1 Explicit source opt-in | Fixed-alias validation, platform-specific CLI rejection, explicit background persistence, legacy profiles and disabled-no-native-call tests | Preview 3 post-publication verification |
| L2 Selected-field native acquisition | Both native bindings and private helper entrypoints implemented; Linux owned fixture and Windows x64/ARM64 owned fixtures pass selected-field, normalization, bounds and thread/handle tests; macOS remains explicitly unsupported | Preview 3 post-publication verification |
| L3 Independent bounded acquisition | Independent 60-second lane, four-second native limit, real helper timeout/kill/wait and digest identity tests, controlled environment, pre-input crash hardening and lifecycle wiring; both native fixture workflows pass on merged `main` | Preview 3 post-publication verification |
| L4 Sanitized vocabulary and bounded cache | Closed vocabulary, private-checkpoint canaries, owned 200-event session ring, deep clones, safe current/body-omission projection tests and completed independent native/privacy review | Preview 3 post-publication verification |
| L5 Atomic bounded persistence | Additive schema, CAS/rollback/cancellation, safe counters, shared retention, HTTP/collector shutdown ordering; actual packaged synthetic history survives graceful and forced restart | Preview 3 post-publication verification |
| L6 Read-only fixed-grid summary | Authenticated bounded GET/HEAD contract and semantic tests; runtime current and summary share the same cache-only collector | Preview 3 post-publication verification |
| L7 Honest counts, gaps and freshness | Sticky interval reducer, conservative coverage, positive-known/null counts, safe aggregation, backlog/missed-poll and failed-write tests; packaged unavailable source stays explicit alongside retained history; native fixture and independent acceptance reviews pass | Preview 3 post-publication verification |
| L8 Current-memory versus durable history | Current limits 0..200, count/order/cloning and stale-cache tests; actual packaged restart retains history while the new session ring is empty | Preview 3 post-publication verification |
| L9 Responsive independent summary UI | Optional independent data source, range cancellation, explicit legacy/error/disabled/history states, coverage overlays and accessible table; all 45 UI tests, typecheck/build and deterministic embedded-asset check pass; independent synthetic browser review passes at 390/768/1440 pixels | Preview 3 post-publication verification; synthetic browser fixtures do not prove live native acquisition |
| L10 Verified privacy and native delivery | Private protocol/adversarial tests, canaries, independent reviews, v1 compatibility, paired reproducibility, actual v2 Linux/Windows/macOS installer smoke, Windows x64/ARM64 and Linux owned fixtures, and packaged scratch recovery proof all pass on merged `main` | Preview 3 publication and post-publication asset, provenance and anonymous-download verification |

Earlier runtime evidence remains recorded in [the exact hosted CI evidence](runtime-ci-gates.md). Independent review at
`9c65825` found no additional runtime/security/UX blockers and passed focused Go tests/vet. Final merged-head and
release evidence is recorded below. Successful fixture acquisition, packaged degraded/restart behavior, synthetic UI
evidence and published release support remain distinct claims.

## Main and pending release evidence — 2026-09-10

- Merged head [`9a60a28`](https://github.com/braidenm/home-lab-observer/commit/9a60a2845d716d2f252a8852f893a5825f6b7ef6)
  passed the required main workflows: [Foundation](https://github.com/braidenm/home-lab-observer/actions/runs/34527769030),
  [Go runtime](https://github.com/braidenm/home-lab-observer/actions/runs/34527768978),
  [Contracts](https://github.com/braidenm/home-lab-observer/actions/runs/34527768957),
  [Dashboard](https://github.com/braidenm/home-lab-observer/actions/runs/34527768895),
  [Native delivery](https://github.com/braidenm/home-lab-observer/actions/runs/34527768914),
  [Linux native journal fixture](https://github.com/braidenm/home-lab-observer/actions/runs/34527768973), and
  [Windows native event fixture](https://github.com/braidenm/home-lab-observer/actions/runs/34527769112). The Windows
  workflow independently passed its owned synthetic fixture on hosted x64 and ARM64 runners.
- The manually dispatched [paired native reproducibility proof](https://github.com/braidenm/home-lab-observer/actions/runs/34527795041)
  passed against the same full commit, rebuilding and comparing the complete paired output twice.
- The independent merged-UI acceptance reran all 45 UI tests, typechecking, library/demo builds and the deterministic
  embedded build, then reviewed schema-validated synthetic one-hour and seven-day summaries at 390, 768 and 1440
  pixels. It confirmed unclipped grids, keyboard-focusable locally scrolling tables, visible GAP/UNKNOWN coverage with
  positive counts, null-as-unavailable rendering and independent recent-session behavior.
- [Preview 3 publication run 34528402601](https://github.com/braidenm/home-lab-observer/actions/runs/34528402601)
  was still in progress when this checkpoint was written. The publication task remains incomplete until the actual
  immutable release, assets, checksums, provenance, anonymous downloads and supported/degraded runtime behavior are
  independently verified.

## Reviewed contract boundaries

- Private checkpoints are excluded from JSON, including their default base64 representation.
- Cursor/tail probes and a deferred lookahead count toward 513 native visits without becoming captured/discarded rows;
  a continuation reader reserves its probe and sentinel budget before ingesting.
- Individually valid counters must also remain safe after bucket, source and window aggregation.
- A reset is not a successful collection; source status and historical coverage are separate dimensions.
- Missing Linux helper/runtime support must not add a dynamic loader dependency to the main observer executable.

Keep this ledger and [tasks](tasks.md) current as each focused implementation PR adds actual evidence. Tests for a
contract alone must never be used to mark its native, persistence, transport or UI implementation complete.
