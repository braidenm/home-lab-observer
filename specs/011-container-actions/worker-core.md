# Typed action coordinator: first implementation slice

Date: 2026-11-01. Status: independently reviewed for unused synthetic coordinator implementation only.

## Scope

Implement an unused `internal/actioncore` coordinator with constructor-injected authorization,
journal and Engine ports. This is executable sequencing logic with synthetic adapters, not an
installed worker, credential issuer, listener, Docker adapter or production persistence store.
The full Spec 011 and ADR 027 runtime decisions remain Proposed. Existing observation roles,
packages, commands, dependencies and permissions do not change.

The owner requests separate start, stop and restart. Only these operations are representable.
Local policy binds one installation, owner, opaque target, engine generation, full 64-hex container
ID, capability/policy revisions and an independent operation allowlist. Full daemon identities
stay private. Request actor/owner/install/target/revisions and grace must exactly match approved
policy; names, inventory aliases, role names and caller-provided permission booleans are absent.

## Processing contract

1. Validate closed bounded inputs and the locally measured claim timing. Grace is 1..30 seconds
   for stop/restart, zero/absent for start. Claim round trip must be at most 30 seconds. The whole
   local phase ends within 60 seconds of receipt and 90 seconds of request start, additionally
   bounded by the caller context and authenticated request/lease deadline. These timing values
   are process-local observations, never trusted fields from browser JSON.
2. Check fresh authorization before journal lookup. `Authority.Check` is a required trusted port,
   not a cached allow flag; errors fail closed. A later adapter must prove actual authenticated
   current remote authority and local allowlist provenance before an executable profile exists.
3. Acquire the exclusive journal attempt for a canonical fingerprint covering actor, owner,
   installation, target, private engine/container binding, operation, grace, revisions and immutable
   action expiry. Replaceable delivery lease deadlines/IDs and local claim timings are excluded,
   so recovered delivery still looks up the same durable intent instead of becoming a new action.
   Conflicting identity/payload, unresolved target fences and unavailable storage refuse.
   Retained outcomes are returned without another inspect/mutation, but only after fresh authority.
4. Inspect the exact full container ID. Running start is ALREADY_RUNNING; created/exited stop is
   ALREADY_STOPPED. Restart requires running. All other/unknown states refuse. Recheck fresh
   authority after inspection, immediately before intent. Inspection is not a daemon transaction:
   outside controllers can still change state before an authorized operation.
5. Reserve five seconds for durable completion. A mutation call is capped at 45 seconds and must
   accommodate the approved grace plus a five-second call allowance. Refuse if this cannot fit.
   Persist dispatch intent before the call. Do not call the Engine when that persistence fails.
   Recheck context/budget after intent persistence; if the remaining reservation is insufficient,
   make no Engine call and conservatively retain OUTCOME_UNKNOWN for committed/uncertain intent.
6. Call exactly one typed Engine method. Any call error, cancellation, lost response or failure
   to persist the terminal result after intent returns OUTCOME_UNKNOWN, never automatic retry.
   A durable journal must retain that fence after process death, expiry, new keys and cleanup.
   `Close` releases only the in-process handle, never durable unresolved evidence.

Success means a daemon acknowledgement, not application health. The coordinator stores no raw
error, response, output, credential or environment data. Results use finite codes. It creates
no logs or unbounded goroutines and makes no persistence/durability claim for synthetic adapters.
All ports must honor their bounded context. Late acknowledgement/cancellation after intent is
uncertain. A cancellation signal cannot forcibly stop an uncooperative adapter or daemon call;
do not wrap it in a leaking goroutine or claim a hard wall-clock kill guarantee.

## Acceptance and remaining gates

Tests first cover the complete operation/state matrix, independent permissions, every binding,
grace bounds, expiry equality, both authorization checkpoints, depleted budgets, intent/finish
failure, duplicate terminal/unknown outcomes and context cancellation. Assert Engine call counts
and journal ordering. Run focused/full Go tests and vet; Linux CI adds race detection once.

Follow-on small PRs must implement the durable journal (existing SQLite library is a candidate,
not silently approved storage), fixed Docker adapter and disposable-container acceptance,
then separately authenticated outbound delivery, OS/IPC isolation, local policy administration
and authenticated uncertainty recovery. No installer or live control is enabled by this slice.
Existing workloads are never fixtures. Native tests must create their own labelled disposable
container, operate only its full ID, and remove only that verified test-owned resource.

Rollback removes unused code; no persistent runtime state or event/wire contract changes.

## Evidence and alternatives

Rechecked 2026-11-01: [Docker Engine API](https://docs.docker.com/reference/api/engine/),
[stop behavior](https://docs.docker.com/reference/cli/docker/container/stop/),
[daemon authorization](https://docs.docker.com/engine/extend/plugins_authorization/), and
[Go context cancellation](https://go.dev/blog/context). Finite grace and the five-second reserves
are project policy, not Docker guarantees. Daemon access remains broad inside a future broker.
Keeping actions disabled is the safe status quo. A generic Engine proxy or automatic retry is
simpler but violates the selected authority/uncertainty boundary; a typed coordinator preserves
testable sequencing without introducing a new dependency or claiming exactly-once effects.
