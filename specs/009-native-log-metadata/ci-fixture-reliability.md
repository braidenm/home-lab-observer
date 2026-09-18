# Native history restart fixture reliability

Status: accepted maintenance slice, 2026-09-18.

The retained-history restart fixture uses synthetic SQLite state and verifies
that a new collector process does not erase a previously committed failure.
It is a correctness test, not a five-second performance contract. On a busy
Windows CI runner, concurrent Go builds and SQLite startup can consume the
old five-second test context before the second store reopen, producing a
misleading failure with no underlying store defect.

Use a bounded 30-second overall context for this fixture and report the
actual reopen error and iteration. Preserve the existing checkpoint,
retained-summary and empty-ring assertions; do not retry a failed reopen or
weaken the store's production timeouts. The test should normally finish in
seconds and remain under the repository's 15-minute PR CI target.

Acceptance: the targeted fixture passes repeated local runs; the normal
cross-platform PR checks pass. A future genuine store failure must retain
its error detail in CI.
