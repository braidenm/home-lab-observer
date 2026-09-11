# Plan

Follow [ADR 012](../../docs/adr/012-explicit-remote-host-projection.md) and Platform Demo ADR 044's phased native boundary.
The current receiver is `HomeLabServerSnapshotParser` in `services:observability`; it consumes four legacy sections and
requires numeric memory and uptime fields when overview is available. The native observation model has richer local
data and section quality. Do not reuse the current UI projection as a wire model.

1. Add the strict producer-profile schema and synthetic fixture under `schemas/remote/`.
2. Add tests, then implement a pure bounded encoder with package-private DTOs and explicit assignments only.
3. Validate schema/fixture independently in Node and require Go fixture equality; add mutation negatives to prevent
   unknown properties and numeric edge cases. Existing contracts CI runs the new Node tests.
4. Independent review checks privacy, legacy semantics, bounds and lack of runtime activation; merge with normal CI.

Slice A adds no persistence, API route, event, daemon permission, dependency, metric tags or service configuration.
Stable errors can become bounded uploader health reasons later; do not add success logging in a pure projection.
Rollback removes the unused package/profile with no data migration. Full connectivity remains gated in the spec.

## Slice B: disposable handoff

Follow [the handoff acceptance criteria](handoff.md) and
[ADR 013](../../docs/adr/013-private-latest-snapshot-handoff.md).

1. Validate only canonical source-bound producer bytes; this is not a replacement HTTP receiver parser.
2. Implement one bounded slot behind pinned directory and opened-handle OS privacy checks, with a single writer.
3. Exercise corruption, unsafe existing permissions, links, competing writers, partial writes, sync failures and
   concurrent readers using owned synthetic fixtures. Native macOS tests must prove extended ACL rejection.
4. Independently review security and run all three native CI targets before normal auto-merge.

This adds an unused persistence library, not a collector/uploader worker or remote connection. It contains no
credential and no durable HTTP sequence. The next worker slice must separately satisfy ADR 044 isolation and
credential lifecycle requirements before installation or owner canary activation.

## Proposed next slices

Review [ADR 014](../../docs/adr/014-linux-connected-canary-workers.md), the [worker plan](worker-plan.md) and
[pending worker tasks](worker-tasks.md) before implementing connected workers. These proposed slices do not activate
upload or change the owner-private handoff policy.
