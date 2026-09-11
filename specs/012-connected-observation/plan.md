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

This slice adds no persistence, API route, event, daemon permission, dependency, metric tags or service configuration.
Stable errors can become bounded uploader health reasons later; do not add success logging in a pure projection.
Rollback removes the unused package/profile with no data migration. Full connectivity remains gated in the spec.
