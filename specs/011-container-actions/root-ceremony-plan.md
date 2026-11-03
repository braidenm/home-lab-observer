# Broker root-ceremony implementation supplement

2026-11-03. **Draft implementation plan; no ceremony code or activation approval.** This consumes the accepted
[root IPC](admin-recovery-ipc.md), [journal v2](action-journal-v2.md) and [fixed profile](process-profile.md).
It changes no wire fields, operations, owner decisions, retention limits or authority. Worker/CLI components and
the nine-file owned-runtime integration are separately reviewed; component tests are not installed-profile proof.

## Existing seams and ownership

- `actionbroker/run_linux.go` owns authenticated ingress and bounded shutdown; admin currently returns REFUSED only.
  `owned_conversation_linux.go` consumes BEGIN before a quiesced clean close. Neither yet performs a root ceremony.
- `owned_workers.go`/`owned_join.go` supply the one-way execution fence, three-handler registry and original75s
  native-polled join. `actionbudget.AdminContext/CheckAdmin` retain the original90s root-operation clock.
- `actionprotocol/local.go` and `recovery.go` already decode closed admin/relay payloads and compare approvals,
  full receipt binding, proposal correlation and exact lane+1 finalization. Decoding is not remote authentication.
- `policy_binding.go:resolveReceipt(..., false)` selects an exact protected current OR retained mapping;
  `brokerFiles.Current` detects source/witness drift. No policy replacement or learned Engine witness is introduced.
- `JournalV2.Lookup`, `InspectEvidence`, `MappingDigestV2` and guarded `CommitRecovery` retain evidence/ACK bytes.
  `GuardedEngine.Inspect` is the only Engine operation used here. There is no Acquire, Start, Stop or Restart.
- Planned broker-only files: `admin_operation_linux.go` (operation ownership/quiescence), `recovery_relay_linux.go`
  (single relay session), `recovery_ceremony_linux.go` (INIT/STATUS/CONTINUE), and focused matching tests. A small
  `actionipc/admin_watch_linux.go` owns disconnect monitoring. Necessary integration edits to run/owned conversation,
  control-slot checks and source checks follow only after their current nine-file freeze is released for editing.
  No worker, admin CLI, profile/dial, launcher, schema, HTTP or producer changes are owned by this supplement.

## One original operation and bounded ownership

1. After the existing kernel/profile peer check, capture the broker's native operation stamp BEFORE HELLO or
   protected-source prework. Construct AdminContext once; recheck CheckAdmin before/after every blocking phase.
   HELLO/command retain5s frames. A reconnect or selected worker's later relay stamp cannot restart broker90s.
2. Strictly decode one command with exact HELLO nonce. The existing single admin slot has no queue. Capture its
   immutable full16 request/digest and selected INIT mapping revision or CONTINUE prepare identifiers; do not
   accept replacement evidence, Engine IDs, approval objects, observation timestamps or ACK bytes from root.
3. Root-operation state is private, owned by that actual authenticated handler. It owns pending relay, cancellation,
   original stamp and completion. No durable QUIESCED flag or permission is created. Completion is an actual closed
   channel, never a cancellation/closing flag. Root-operation replacement cannot inherit an earlier worker reply.
4. All ceremony access to `state.mu` uses TryLock with finite BUSY/RECOVERY_REQUIRED refusal; no Mutex.Lock wait
   hides behind native90/75. Under the lock copy immutable policy/slot identity or make a short state transition;
   recheck original timing/cancellation after acquisition, and release before file reads, SQLite, Engine, IPC,
   cancellation callbacks or waits. New recovery source checks copy
   and compare the same startup policy under this bounded rule, then use protected Current and actual journal
   mapping validation outside the lock. They do not use the execution-only fence as a reason to deny recovery.
5. INIT/CONTINUE capture ONE quiescence stamp BEFORE cancellation, then atomically fence execution and cancel the
   registry's execution contexts BEFORE state locks. Invalidate readiness/nonce permission through a bounded transition, snapshot selected
   non-relay completions, and join under min(original75s, remaining root90s), including native polling after suspend.
   Any refusal preserves the one-way fence. No ACK/Inspect/proposal is allowed until actual required joins succeed.
   Never delete live execution/control reservations or release capacity before actual owning-handler completion; fence/cancel invalidates permission only, retaining cleanup ownership/evidence.
   The snapshot slot remains occupied until its exact joined owner clears it; reserve root relay only after the required joins and exact slot release.
   An unfinished handler keeps Engine/journal open. Greetings survive only to consume BEGIN and cleanly close or
   become an authorized relay; they cannot become execution after the fence. STATUS never newly fences/quiesces.
6. A completed ceremony, disconnect or error does not resume execution. Only the separately documented explicit
   stop/join of BOTH services and restart of the unchanged verified profile can reset process-local latches.

## One shared control slot and broker-initiated relay

The root operation reserves the existing single control capacity with a bounded state transition, exclusive with
snapshot control; busy means finite refusal, not a queue or a second control pool. STATUS may coexist with the two
already permitted execution conversations. INIT/CONTINUE require actual quiescence before publishing a pending query.
Every new worker ingress still authenticates and reads stateless BEGIN first. Only a matching live root operation
with a pending fixed query can select RECOVERY_QUERY and classify that handler as relay. Otherwise the existing
execution/control selection applies, or a fenced/no-query greeting closes cleanly. The worker never initiates recovery.

Use one fresh nonzero relay nonce, never an execution/admin nonce. A private one-session rendezvous joins the root
handler to ONE owned worker handler, with one outstanding typed request/reply and no command queue. Reserve/release
by exact operation/session identity. Parent cancellation closes the owned relay connection and invalidates pending
reply/evidence; stale completion cannot satisfy a new root operation. Release session ownership only after its actual
handler completion; an unjoined relay causes RECOVERY_REQUIRED and cannot be silently adopted by a later command.

Each query is checked against the fixed INIT/STATUS/CONTINUE operation ordering below; envelopes match version,
nonce, ordinal, operation, action and full digest. Typed body matching remains necessary after envelope validation.
Each HTTP/relay leg retains its fixed5s limit inside the broker's ORIGINAL90s. Native capture/checks bracket leg I/O;
a narrowly named timing-only admin-relay check may enforce that fixed interval without public duration arguments,
clock resets or an authority token. The independently root-owned worker receive fix must distinguish bounded
processing idle before a next query from the unchanged first-byte+5s transfer limit: local ACK work can legitimately
precede FINALIZE. Do not work around it by increasing every frame timeout or restarting a relay clock.

## Consumed command ordering

| Command | Exact remote relay order | Local operations and response |
| --- | --- | --- |
| INIT | RETAINED1, PROPOSE2 | Fence/join; compare fresh retained full16; select exact mapping and expected mapping revision; bounded full-ID Inspect; InspectEvidence; send deterministic public ProposalID. Return validated QUIESCED state only for the still-live operation. No ACK. |
| STATUS | STATUS1 only | No new fence, Inspect, proposal, ACK, prune or release. Read local retained state through validated exact binding where available, return actual ACK observation only, otherwise no invented current observation/quiescence. Remote state alone never yields local ACKED. |
| CONTINUE | RETAINED1, STATUS2, CHECK3, FINALIZE4; at most STATUS5 after ambiguous finalize | Fence/join; freshly bind source, mapping/evidence and current selected Approval; inspect unless an original durable ACK already exists; fresh CHECK; guarded local ACK or exact retained ACK; finalize with its actual digest. No retry of Engine, local ACK write or remote mutation. |

RETAINED must supply the authenticated exact full16 request equal to root's request, and its recomputed remote
digest; a private metadata subset is insufficient. INIT requires OUTCOME_UNKNOWN or EXPIRED_UNCONFIRMED and HELD;
live CLAIMED/REQUESTED cannot be forced expired and waits for normal authoritative original hard request expiry.
Generic error codes, another action's unknown fence, missing remote source or a timeout never prove dispatched/no-effect.

Mapping selection compares server/owner/instance/target, operation, parameters, policy/capability revision and private
semantic identity, including retained mapping revision and the approved Engine generation. Recheck Current before/after
Inspect and before any ACK. Missing/replaced/unsupported target or changed source refuses; inspection is not causality.
Use existing bounded Inspect with the exact full ID, accept only CREATED/RUNNING/EXITED, sample its actual local UTC
observation once. Private mapping/evidence hashes stay local. No network/Engine I/O occurs inside a journal transaction.

## ACK, finalization and uncertainty

CONTINUE compares the requested recovery/prepare/approval revision to the fresh current Approval and all immutable
request, digest, evidence, lane and owner bindings. Generate a new check nonce. MatchRecoveryDecision must validate
the full original selected Approval, nonce, PERMITTED and credential metadata; transport success is not permission.
Call AcceptRecovery with the SAME original operation stamp and actual CHECK send/receive samples. Its original
90s bound,5s check round trip, server bracket and min(10s, approvalRemaining-1s) ACK deadline cannot restart.

For a new ACK, build the exact nineteen-field template from validated local evidence/mapping, final observation
and remote Approval/decision. Leave acknowledgedAt empty: only CommitRecovery samples it. Revalidate source and
root-operation ownership immediately before the transaction; the journal revalidates unchanged local record/events,
slot, digests and mapping binding, refuses live attempts, and checks the required recovery kind before/after actual
COMMIT. Cancellation or failed post-commit validation suppresses success/FINALIZE; it cannot assert rollback.

For an existing ACK, decode and validate its actual immutable bytes and digest; never rebuild it with a new time,
inspection or prepare. The original matching approval may finalize the same ACK; renewed approval must reference
that same committed ACK digest. A changed evidence/mapping or mismatched prepare identity refuses, not overwrites.
MatchFinalization checks the selected current Approval, exact ACK digest and held lane+1, including overflow refusal.
Local ACKED never means central HELD released. An ambiguous FINALIZE permits only the one fixed STATUS lookup;
missing/late/denied lookup returns RECOVERY_REQUIRED while preserving the actual local ACK.

Poisoned/ambiguous journal handles cannot be used to manufacture retained success. Return RECOVERY_REQUIRED;
discovery of committed state requires a separately validated reopen through the existing stopped/joined lifecycle,
not an in-flight retry, unguarded fallback or automatic resume. No new reopen/repair mechanism is introduced here.
Ordinary retention policy remains unchanged; a full missing-record import returns FULL without stealing a slot.

## Admin disconnect ownership

After the command, start one joined, bounded read monitor on the actual admin connection. EOF, cancellation, any
unexpected second bytes or ancillary data cancels this root operation and its pending relay/evidence. Close only
successfully parsed received SCM_RIGHTS; never infer descriptor numbers from malformed bytes. The watcher never
reads another command or grants authority. During long waits native90 is still polled, not only a paused Go timer.

Before a final response, deliberately stop/unblock and JOIN the monitor and its deadline/cancellation callback;
reset only its owned read deadline before writing. An intentional wakeup timeout is not peer EOF, but real EOF or
unexpected bytes observed during that stop still cancels. The controlled stop itself does not cancel a healthy
operation or poison its final write. Recheck cancellation/native budget after sampling and before replying. There
is no detached watcher or waiter goroutine. If cancellation races possible ACK commit, return recovery-required
if possible and require retained-state lookup; no response is never proof that nothing committed.

## Test and delivery cuts

Implement consumed local checkpoints, not separate unused-helper PRs:

1. Actual Unix admin peer/HELLO/one-command/original90, bounded state contention and disconnect monitor, wired to
   runtime ingress; no successful ceremony until its typed remote/evidence dependencies are consumed.
2. Actual BEGIN selection and single relay session shared with snapshot capacity; STATUS-only and INIT/CONTINUE
   quiescence, root cancellation during pending/active I/O, stale nonce/ordinal/reply refusal and no reservation reuse.
3. Real SQLite four-origin INIT/CONTINUE/retained-ACK/finalization journeys with synthetic Unix worker/Engine Inspect;
   exact fresh source/current approval, changed mapping/evidence and both partial-commit outcomes. Typed test doubles
   are private only; no production fake permits, optional guards, raw Bind, arbitrary paths or HTTP/Engine callbacks.

Required cases include held state/registry locks (fence first, finite refusal), all six shutdown completion signals,
unjoined resources remaining open, EOF during quiescence/remote CHECK/actual ACK, controlled watcher stop without
false cancellation, extra bytes/FD cleanup, original90 and75 equality/suspend, local ACK before FINALIZE and lost
reply/renewal, old known terminal not relabeled UNKNOWN, missing/corrupt/foreign local evidence, FULL and no replay.
Where COMMIT is claimed, assert actual stored state after reopen and meaningful injected transaction boundary,
not only a mocked successful return. Schema max-packing/crash/ENOSPC and effective installed roles/NSFS/75s process
termination remain separate acceptance work, not falsely completed by these ordinary fixtures.

Freeze actual source/test hashes and finite native race/vet receipts for root plus independent review. Paired real
Platform consumer acceptance, capacity/fault acceptance and installed-profile/package proof still precede release;
no publisher, production flags, host service/credentials or server workloads change under this plan.
