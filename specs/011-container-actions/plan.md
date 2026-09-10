# Proposed implementation plan

**Status:** Proposed; documentation only. Deliver after connected observation, one reviewed slice per PR.

## Boundaries

Keep `internal/containerobs` and read-only APIs unchanged. Future modules separate grant policy, durable action ledger,
typed broker transport, fixed Docker adapter and presentation. Constructor-injected interfaces allow a synthetic engine,
clock, authorization service and durable-store failure seam. Module names and exported APIs are not frozen by this plan.

The broker alone owns the authority-bearing local socket/pipe. Its credentials and ledger must be inaccessible to the
web and uploader roles. UI action affordances consume normalized capability/results, not Docker handles or raw responses.
The remote application authenticates actors, but the broker validates installation/owner scope and its owner allowlist
and checks fresh authorization at dispatch. Freeze the standalone-owner equivalent before implementation.

## Delivery order

1. Review Spec 011, resolve the three proposed architecture decisions and create a new ADR; do not edit accepted ADR 006.
2. Freeze separate versioned request/result/grant/audit schemas, error vocabulary, state transition table and threat model.
   Include grant-to-full-ID private mapping, signed freshness/replay semantics and crash recovery diagrams.
3. Implement policy and durable intent/dispatch/outcome state machine with only synthetic transports. Prove reservation,
   fsync failure, retention, unknown outcomes, revocation and per-target serialization before any daemon call exists.
4. Implement fixed local Engine adapter and bounded selected-field decoding. Start, stop and restart are distinct typed
   methods, not an arbitrary request function reachable from callers. Native disposable-container tests prove semantics.
5. Implement isolated broker lifecycle and credential/permission boundaries. Decide explicit OS installation support and
   audit recovery; do not broaden existing collector permissions or make containerized observation writable.
6. Integrate authenticated owner/delegate delivery and UI controls with separate review. Verify responsive confirmations,
   duplicate submissions, permission changes and connection loss; status never equates daemon acknowledgement with health.
7. Add operator instructions, release/rollback smoke evidence, independent reviews and explicit activation gate.

## Verification

Each implementation PR links requirement IDs to tests and runs focused tests, full Go tests/vet where Go changes,
contract checks for schema changes, and repository policy. Synthetic tests are default; real container lifecycle tests
are isolated hosted jobs with code-owned fixtures. No host workload or production credential is a test fixture.

## Deferred

Kubernetes Deployment rollout restart has separate UID/RBAC/patch-field/admission and rollout-health semantics; it is
not another branch of the Docker adapter. Pod eviction, host/service commands, bulk actions, schedules and autonomous
repair are also deferred. Read-only permissions never imply any of these future actions.
