# Proposed implementation plan

**Status:** Proposed; documentation only. Deliver after connected observation, one reviewed slice per PR.

First review [ADR 027](../../docs/adr/027-bounded-action-dispatch-and-uncertainty.md): it reconciles proposed grace,
immutable target/parameter binding and delivery-versus-dispatch recovery with the remote coordinator. It does not
approve the remaining broker isolation/authentication choices or add executable authority to observation roles.

Use the [open-source alignment evidence](../../docs/architecture-research/007-open-source-alignment.md) when freezing
the architecture. The owner delegates production-oriented decisions; retain independent review and executable proof.

## Boundaries

Keep `internal/containerobs` and read-only APIs unchanged. Future modules separate grant policy, durable action ledger,
typed broker transport, fixed Docker adapter and presentation. Constructor-injected interfaces allow a synthetic engine,
clock, authorization service and durable-store failure seam. Module names and exported APIs are not frozen by this plan.

The broker alone owns the authority-bearing local socket/pipe. Its credentials and ledger must be inaccessible to the
web and uploader roles. UI action affordances consume normalized capability/results, not Docker handles or raw responses.
The remote application authenticates actors, but the broker validates installation/owner scope and its owner allowlist
and checks fresh authorization at dispatch. Freeze the standalone-owner equivalent before implementation.

## Delivery order

The first bounded code candidate is the [unused typed coordinator](worker-core.md).
Independent review of that slice precedes code; it does not approve the full executable broker.

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

The next complete dormant slice is the [Linux action journal](action-journal.md), with [ADR 028's limited decision
candidate](../../docs/adr/028-dormant-linux-action-journal.md). Exact spec/ADR and core metadata/byte-compatibility review
precedes code. Reuse the existing SQLite dependency and storage principles, not an extracted framework or changed upload
ledger. Two uncertainty-retaining permits, 1,000 records, 30-day terminal retention and measured/derived 32 MiB total
proof precede a real Engine adapter. Full broker authentication/isolation, owner reconciliation and activation stay gated.

Each implementation PR links requirement IDs to tests and runs focused tests, full Go tests/vet where Go changes,
contract checks for schema changes, and repository policy. Synthetic tests are default; real container lifecycle tests
are isolated hosted jobs with code-owned fixtures. No host workload or production credential is a test fixture.

## Next dormant Engine implementation

The next bounded proposal is the [fixed Linux Engine adapter](engine-adapter.md)/[ADR029](../../docs/adr/029-fixed-linux-action-engine.md).
Exact documents are accepted for unused Linux implementation only; the all-green journal merge prerequisite is complete.
Commit those specs before code. Use stdlib Unix
HTTP, a singleton API1.45 compatibility profile and existing typed methods, not an SDK/framework, caller transport or
observation refactor. Synthetic faults precede separately approved exact disposable native fixtures, one at a time
(at most two sequential graceful/ignore-signal modes with exact cleanup between). No installed
broker, credentials, grants, owner reconciliation, action controls or activation follows from this dormant slice.

## Deferred

Kubernetes Deployment rollout restart has separate UID/RBAC/patch-field/admission and rollout-health semantics; it is
not another branch of the Docker adapter. Pod eviction, host/service commands, bulk actions, schedules and autonomous
repair are also deferred. Read-only permissions never imply any of these future actions.
