# Spec 009 review record

## Summary dashboard review

Root independently reviewed the known-field decoder, status/grid reduction,
independent resource loading, nullable counts, chart overlays and accessible table.
Review added a visible coverage legend and more readable chart labels. Full seven-day
grids now fit narrow cards without clipping; a single configured source uses the
available width. Browser review also found the existing recent-event search field
inheriting a 240-pixel flex basis after mobile layout switched to a column; its
mobile sizing is corrected and guarded by a real layout assertion.

The reusable UI retains compatibility with clients missing the optional summary
method. Embedded assets are regenerated from the reviewed source. Native source
activation is still pending: browser summary cases use schema- and semantic-checked
synthetic responses and never read the developer's native logs.

Root verification passed: all 45 UI tests, typecheck and builds; deterministic
embedded asset check; contract and direct handler scenarios; real embedded-browser
checks at 390, 768 and 1440 pixels, including one-hour/seven-day grids, uncut final
buckets, compact mobile filters, keyboard table focus and local horizontal scroll.
Root inspected desktop and mobile screenshots. Production native acquisition and
end-to-end history wiring remain separate release gates.

## Windows reader kernel review

Root independently read the complete fixed-seam implementation, specification and
synthetic tests. No blocker remains in this kernel slice. Review covers per-query
and per-record close ownership, same-thread lifetime, strict bookmark proof versus
generic failure, visited-row/byte limits, safe typed normalization and private
checkpoint exclusion. An initial EOF/tail race was caught during design review;
the tail must now prove a timestamp strictly older than the exact initial window
before claiming zero coverage. Tests include the concurrent-arrival boundary.

Native WEVTAPI binding, private bookmark anchor exactness against cleared/reused
record IDs, owned native fixtures and the hidden helper entrypoint are still
required. This review does not claim that Windows event acquisition is enabled.

## Contract checkpoint — reviewed; required CI pending

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

The summary schema and semantic module were independently reviewed, then re-reviewed after closing source-specific
reason branches in `1e63a6e` (author commit `c7dbc41`). The reviewer reported no remaining blockers and ran full
`npm test`. Tests cover nanosecond status ordering, exact whole-second grids, Go zero-time rejection, nullable counts,
cross-source overflow, deterministic latest-quality reduction and the two-source seven-day response-size budget.
Root strengthened that budget fixture with large safe counters, nine-digit timestamp precision and non-null gap reasons.

Native planning also exposed metadata probes missing from the original record budget. `073bd3a` counts those visits
explicitly without raising the 513-record ceiling, and `d7f875a` requires a valid continuation cursor for a normal probe.
Source-summary reasons now match the closed wire matrix while the generic aggregate status remains reusable.

Root combined verification passed: `go test ./...`, `go vet ./...`, `npm run test:contracts`, and repository policy.
The complete checkpoint must pass all required PR checks before merge; this does not authorize release claims for the
unimplemented native, storage or UI slices. The private helper codec is the next separately reviewed contract slice.

## Coverage reducer implementation review

The private helper codec subsequently merged in PR 13 after all 12 required checks passed. This coverage slice adds
only deterministic history-domain functions, not SQLite tables, a new collector, or native acquisition. Independent
review found an initialized-empty checkpoint reset transition that was too permissive; it was corrected and tested
for both reset kinds. Re-review reported no blockers and independently ran coverage tests twenty times and history vet.

Root tests include every ordered historical-reason pair, a seeded interval-cell oracle, exact clipping/coalescing,
fractional gap preservation, initialized-empty/reset/clock rollback rejection, future-attribution boundaries and 11,000
successive minute attempts retaining one coalesced row. Full Go tests, vet and repository policy passed locally.
SQLite transactions, shared age/size maintenance and native integration remain outstanding; this evidence does not
claim those features are delivered.

## Collector/cache implementation review

Root and an independent reviewer examined the fixed-cadence collector, immutable commit retry, request correlation,
latest-status overlays and session-only cache. Review found and corrected: an older volatile write-failure overlay
overwriting newer durable status; storage time consuming the shared native deadline; treating definite revision
conflicts as ambiguous confirmation; and retaining discarded cache records in an oversized backing array. Reset and
summary-grid correlation were tightened as well. Root re-read the corrected implementation and ran the regressions.

The collector now preloads checkpoints before the native lane and commits only after readers finish. Every Store
phase has a fixed two-second context; native readers share a separate four-second context. Stop cancels and joins
without closing the borrowed Store. Definite conflicts cannot append rejected events; uncertain commits retain only
the documented one-retry/revision-resolution flow. The 200-event ring owns bounded retained storage.

Root replaced a wall-clock sleep in the load/lane test with direct phase-order/deadline assertions; thirty repeated
domain test runs and vet passed. Native process kill/reap remains the responsibility of the fixed reader adapters,
not a capability claimed by cooperative test fakes. Application lifecycle wiring remains pending.

## Summary HTTP implementation review

The optional authenticated GET/HEAD summary handler was independently reviewed by root, including strict query
parsing, the shared latest-status reducer, closed DTO projection and unavailable/error responses. Its source is a
read-only summary port; HTTP never starts native acquisition. The response clock is captured after querying, with at
most one fixed-grid rollover retry so concurrent latest status is retained without silently shifting the requested
window. Missing optional wiring returns a fixed unavailable Problem, not invented empty data.

Root requested direct producer/schema verification and stronger large-grid fixtures. The resulting Go-handler
response is validated by the closed JSON schema and shared semantic validator, including two sources, 168 buckets,
non-null partial reasons, large safe counters and nanosecond timestamps. Root independently ran all 12 emitted
handler scenarios, focused API/domain tests, vet and repository policy successfully. Native wiring, current-event
projection and UI remain pending; this endpoint slice does not claim those are enabled.
## SQLite transaction and shared-maintenance review

The optional log Store implementation received independent review of CAS atomicity, rollback, schema isolation,
timestamp representation, privacy, safe counters and fixed-grid projection. Review found an early year-1 subtraction
edge; derived floors now clamp without wrapping. Full history tests passed twenty repetitions and vet before the
maintenance integration. No message body, event code or arbitrary native field is stored in minute history.

Shared maintenance was separately implemented and reviewed by root. All historical row deletions/coverage trims share
the configured work budget across host and log data, with actual chronological ordering and no host-first starvation.
Review exposed a future-skew eviction case that could later reconstruct a false full zero. Two bounded durable
eviction frontiers now prevent that reconstruction, and injected failures prove count/proof/frontier writes roll back
together. The reviewer independently inspected the root frontier/pressure fixes and reported no blocker.

Combined tests cover a one-row maintenance budget, 5,000 host plus 5,000 log records under pressure, actual combined
allocation reclamation, fixed-width dates through year 9999, malformed/incompatible optional schema fallback,
checkpoint preservation, concurrent CAS contenders and cancellation. Root full Go tests, vet, repository policy and
diff checks pass. This is storage-library evidence; production/native wiring and release proof remain separate gates.

## Fixed helper process and identity review

Independent transport review found resolver errors escaping verbatim and missing
working-directory regression evidence. Both were corrected: only canonical fixed
resolver codes may escape, and real synthetic children prove the pinned directory.
Re-review approved bounded pipes, overflow termination, timeout/reaping, the
non-queuing occupied slot, hardening-before-input and cleanup-before-output.
The cleanup callback now reports errors; those reject the response as well.

Root independently reviewed all platform identity code and tests: fixed adjacent
Linux digest, same-executable Windows identity, bounded files, link/privilege and
owner/ACL checks, minimal environment and read-only refusal. A second reviewer
approved the correlated Reader bridge and explicit unsupported/no-launch states.
Full combined Go tests, vet and repository policy pass locally. Windows native
node/ACL tests execute locally; a positive whole-path test explicitly refuses this
development host's permissive ancestors, and must pass rather than skip in CI.
Linux native filesystem and child tests remain required CI evidence. This slice
does not enable sources, install a helper or claim packaged native support.
