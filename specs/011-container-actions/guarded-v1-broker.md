# Guarded v1 broker: locked-outcome first release

Status: Accepted for this bounded local implementation and tests only. Runtime source remains under review;
installation and activation are not approved.
Implements [ADR034](../../docs/adr/034-locked-uncertain-outcomes-first-release.md), not the deferred recovery workflow.
The delivered worker is the consumer. This is one runnable broker/executor cut, not separately published primitives.

## Scope and fixed authority

Reuse the delivered worker/HTTP/protocol/profile and existing core, v1 journal and fixed Engine adapter. Adapt the
reviewed broker's protected policy/files, snapshot control, execution checks and bounded lifecycle. No new dependency,
SQLite schema/version, journal-v2, ACK/finalize, admin listener, recovery routes, generic proxy or pressure framework.
The broker has one fixed authenticated worker ingress; only it owns Engine/journal handles. No path/UID/transport
overrides. The fixed no-argument command reuses the identical manager-environment admission; other platforms refuse.

Keep the ADR031 roles, host-namespace witness, protected ancestors/ACL/link/FD checks and approved Engine witness.
Read the protected approval source before opening the guarded Engine; never learn approval from the live socket.
Map each opaque target to its exact approved engine generation/full64 ID, operation, grace and immutable revisions.
START/STOP/RESTART remain independent; no aliases, short IDs, observation credentials or arbitrary command fields.

The first profile supports FIXED mappings only. It has no mapping-edit/replacement/re-enrollment/adoption operation.
Revalidate sources during execution and validate all retained journal mapping references at startup; unsupported
binding/state changes refuse. Preserve the same server/owner/installation/target/native-resource binding across
supported restart/upgrade. Multiple fixed targets are permitted; the first live canary is one disposable target.
As ADR034 states, hostile/out-of-product host-root destruction or coherent rewriting of all retained state is not
prevented by these checks. Product tools must never offer such rewriting as recovery.

### Consumed fixed public-profile barrier

The separately delivered worker-refresh tightening is a prerequisite, not new broker-owned transport code. Before
its one POST, the worker validates the complete existing snapshot head and recomputes its digest using the query's
EXACT whole entry set plus the head's original revision/advertised/expiry metadata. PublicationID is absent from
that digest; a valid synthetic value may satisfy the existing receipt shape without becoming authority. Require
digest equality for every existing head, including expired UNAVAILABLE heads whose entries are hidden. A fresh
publication can renew time/revision only for the unchanged target/alias/operation/parameters/revision/entry set.
Cooldown is not part of that immutable set and may change independently. This intentionally disallows public-policy
updates in the selected fixed first profile; it does not silently redefine the broader server publication API.

Only verified MISSING with all head fields null permits initial publication. Partial/malformed heads, wrong binding
or any changed entry set refuse BEFORE POST; preserve the existing CAS and no-retry rules. A nil head is not proof
that server state never existed or was never destroyed. The barrier does not cover private Engine generation/fullID
changes: protected local source/journal checks and the unsupported-change/trusted-root boundary above remain separate.
It is not protection against a compromised trusted worker. The broker still checks its current protected policy,
receipt digest, native freshness and fresh server permission; a matching historical hash grants none of those.

## Durable uncertainty without another store

Before a complete validated claim is delivered, the broker performs no Engine operation. A committed claim whose
reply is lost remains centrally HELD: the paired claim service selects only REQUESTED work, never reissues a
CLAIMED action, and claimed hard expiry preserves HELD as EXPIRED_UNCONFIRMED. Its unique server+target HELD lane
also refuses a different key/operation for that fixed target. UNKNOWN preserves HELD; neither clock expiry nor
worker/broker restart releases it. This existing central row is the lost-claim barrier, not a new local file/store.
Missing/unavailable/conflicting central state refuses; absence is not proof that a prior claim had no effect.

For dispatch, keep the v1 journal's durable INTENT before the actual write. INTENT without a terminal stays UNKNOWN
with its opaque/physical target fence and global permit across Close/reopen, expiry, new keys and pruning. A broker
startup that sees such uncertainty may conservatively refuse the whole installation. Preserve the worker's existing
process-local execution-disable latch, but do not describe that boolean as a durable global shutdown flag. Central
HELD is target-scoped; an unaffected fixed target does not weaken the affected target's lock.

Normal startup uses only OpenExisting for the exact binding. No implicit Provision, migration, reset, quarantine,
replacement journal, unknown clear or old-action execution exists. Missing/corrupt/foreign/full state fails closed.
Existing v1 pruning/retention/floor semantics are unchanged; this cut adds no pruning scheduler or cleanup workflow.
Manual investigation observes bounded retained state only; it never releases a fence or retries an uncertain action.

## Narrow v1 journal capabilities

1. Add a bounded read-only v1 admission snapshot under the existing exclusive handle/mutex and two-second SQL scope.
   Return validated binding, sampled time and at most1000 records containing key, intent presence, terminal time/code
   and active-handle state. Reuse the existing validated action/event loader; no unbounded query/raw SQL access.
   It cannot create an Attempt/admission, expire/prune an action, release a permit or supply remote permission.
2. Add an invocation-bound execution adapter for one exact key and current full-receipt digest. Its admission,
   INTENT and terminal transitions recheck the ORIGINAL typed execution context before AND after the actual COMMIT.
   The digest is in-memory timing/correlation, not a persisted full-wire identity or permission. No nil/raw-API fallback
   is reachable from the runnable broker. Preserve raw v1 Acquire/Attempt behavior for its existing callers/tests.
3. Keep v1 DDL, metadata/fingerprint bytes, two permits, 1000 records, two events, 30-day retention, physical/opaque
   fences, cooldown, SQL limits and filesystem/lock checks unchanged. A failed/late/ambiguous commit poisons the
   owning handle and retains actual durable evidence; do not fabricate rollback, a terminal result or no effect.

The snapshot is also the exact source for current invocation reporting after its Attempt has actually joined.
Never call Acquire as a read lookup: a missing row could otherwise create an admission.

## One fresh invocation, not resumed execution

Startup/each preclaim reservation checks bounded capacity, retained mappings and uncertainty before advertising
usable capability entries. Snapshot publication remains the existing separate control branch and fresh BEGIN.
Only authenticated bodyless HTTP204 permits NO_CLAIM; EOF/error/timeout remains uncertain. Reserve before HTTP,
bind the one-use original nonce, validate the complete claim/full16 receipt and exact local mapping, and reject
ANY already-seen action ID before accepting it for execution. No new lease resumes an existing local action.

V1 metadata omits remote idempotency/correlation fields. Do not reconstruct the full16 receipt or silently equate
its private fingerprint with full-wire identity. Keep the validated full receipt only in the live invocation;
report only that invocation's exact joined same-action/key evidence. Known terminal reports require an actual
retained allowed terminal; UNKNOWN requires that SAME action's durable INTENT, not another action's target fence.
No prior-action reporting/resumption after broker restart is added. A lost report remains central unconfirmed/HELD
unless the server actually committed the original known result. Local RESULT handoff never proves HTTP acceptance.
No client-side release or mutation retry follows from either a lost reply or observed container state.

Preserve both fresh, full-binding current-authority challenges and protected policy rechecks around core execution.
Keep original native UTC/Go-monotonic/BOOT origins: delivery<30s; the existing execution horizon<=89s; Engine call<=45s;
reporting<119s; control/current-check/frame limits unchanged. Preserve exact equality/continuity/cancellation rules,
including post-capture checks and actual guarded connection Write checks. No new duration/permit callbacks or clock
restart. Check/write/daemon execution remains non-atomic; a late response is uncertainty, not proof of no write.

## Runnable lifecycle and bounded shutdown

Construct real protected sources, v1 journal, native timing and GuardedEngine before the fixed worker listener.
No public constructor accepts a fake authority/Engine/journal; synthetic seams remain private to tests. Preserve
peer-before-frame admission, bounded undecided/execution/control ownership and no application queue. Only snapshot
and execution selections are served; dormant worker recovery support is not exposed through a root/admin ingress.

Shutdown fences admission before cancellation, keeps exact reservations until their real owners finish, closes
owned sockets to unblock I/O and polls the SAME original native75s join budget. Never reset a budget per handler,
use a waiter goroutine or close Engine/journal under an unjoined owner. Unjoined/uncertain shutdown returns a finite
unavailable/recovery-required result and preserves resources/evidence; no restart/resume/repair is attempted.

## Plan, acceptance and delivery gates

- [x] Approve this contract and its exact reuse/adaptation closure; commit the owning spec before runtime code.
- [ ] Integrate the independently reviewed consumed worker head barrier before broker publication. Its tests cover
  READY/expired/EMPTY unchanged heads; changed target/alias/operation/grace/policy/capability revision/addition/removal
  with zero POST; cooldown-only changes; initial nil head; malformed typed heads without panic; and CAS failure with
  no retry. This prerequisite is owned separately; do not edit its protocol/HTTP paths from the broker cut.
- [ ] TDD the v1 snapshot and invocation-bound actual-COMMIT guards, then consume them directly in the fixed broker.
- [ ] Reuse protected policy/files, native timing/guarded Engine, two-check conversation and worker-only lifecycle;
  keep one coherent runnable PR. Stop for review if new production code exceeds approximately3000 lines or needs
  new dependencies/schema/protocol choices. Do not publish an unused helper chain to evade that review boundary.
- [ ] Ordinary Windows compile/unit/vet plus serial ordinary-user Linux race/vet use private files/SQLite/socketpairs,
  fixed synthetic receipts and the existing finite resource/time bounds. No service accounts/real credentials/daemon.
- [ ] Prove guarded context absence, changed key/digest/policy/Engine witness, denied checks and expired/canceled native
  boundaries cause zero Engine writes; prove actual admission/INTENT/terminal commit uncertainty and same-key/new-key
  restart fences without changing v1 bytes/schema. Read snapshots must be side-effect-free and bounded at1000 records.
- [ ] Prove the consumed worker/broker journey and shutdown with owned synthetic sockets and actual v1 SQLite, including
  clean204 versus lost claim, two fresh checks, same-action retained reporting, interrupted control and no under-close.
- [ ] Separately approve/run actual paired server plus disposable Engine acceptance: lost claim/result, restart and
  expiry retain the affected central/local locks; old/new key, each operation and unsupported rebinding cannot bypass.
  An unrelated fixed target must not release/alter the affected lane. No new global database barrier is implied.
- [ ] Independent exact-source review and fresh current-head hosted CI precede publication. Integrate actual main and
  preserve all incoming changes; the dependent ADR034 head is not a claim that its PR is already merged.

Installation/provisioning, effective service isolation, compatible runtime/reader floors, upgrade/rollback and the
exact-package disposable canary remain separate activation gates. No host action, secret handoff, enabled flag or
workload mutation is authorized by this source contract. Existing physical v1 evidence is not a new v2/fault claim.

Local test finding: the existing full server-time bracket already refuses whole claims near30s. The explicit
AcceptClaim own-capture equality check is therefore defense-in-depth/contract alignment, not a demonstrated
reachable authorization hole. A failed initial just-before30s positive fixture exposed this stricter availability
condition; preserve the bracket and an ordinary valid inside1s control rather than widen clocks to manufacture RED.

Local source receipt: the coherent worker-only broker reuses approximately3257 new production lines plus tests;
the explicitly reviewed ceiling exception retains the pinned-inode listener and joined shutdown, not a new scope.
The source includes no admin ingress, journal-v2, recovery workflow, installer or new dependency. Actual ordinary-user
Linux SQLite/Unix tests exercise three operations with two fresh checks, same-action retained reports and bounded
owned shutdown; Engine actuation, role ownership and remote permission in these fixtures are synthetic. This is not
installed isolation, paired-server acceptance or a live-Docker/canary result. Those gates above remain open.
