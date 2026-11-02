# Root-only recovery IPC v1

2026-11-02. **Design accepted for local consumed paired-runtime implementation/tests only.** No installed/runtime
proof, release or production activation approval follows; broader feature decisions remain gated.
This narrows the recovery paragraphs in [packaged-worker](packaged-worker.md), including its older unknown-only
description. Four evidence classes below replace that incomplete description; accepted core/journal v1 is unchanged.
Paired authority is Platform Spec075 `container-execution-v1.md`, `container-execution-storage-v1.md` and
`observer-action-journal-v2-binding.md` (storage-vectors-r1 review). Those Proposed codecs are dependencies, not
currently implemented endpoints. No generic admin RPC or alternative local-only authorization is introduced.

## Ingress, limits and closed encoding

Host pathname is fixed `/run/home-lab-observer-actions/admin/recovery.sock`, exposed at that same path only to
the broker/admin CLI profile. Its root-owned directory is0700 and socket0600, no extended/default ACL, links or
worker group access. Root provisions this directory; broker never accepts a caller path. Require Linux initial
user namespace, no user-namespace remapping, and SO_PEERCRED UID0 on every accepted connection. CLI verifies the
protected pathname and exact configured broker peer UID; PID alone is not identity. Reject observation/worker
UIDs even when a supplementary group permits another socket. No abstract/TCP/socket-activation ingress.
Root is trusted; this is not protection against compromised host root or broker. Worker peer authentication on
the DISTINCT worker socket is not remote owner proof and never admits these admin kinds.

One admin connection/operation at a time, no queue. Four-byte unsigned big-endian length precedes each JSON
document, length1..16384; reject length before allocation, depth<=8 including envelope. Strict UTF-8/ASCII closed
values, no BOM, duplicate/unknown/incorrect-case keys, trailing value, escape aliases or implicit omitted nulls.
Canonical keys follow the orders listed here, compact with no whitespace/final LF. Positive revisions are
canonical decimal strings1..9223372036854775807, never JSON numbers. IDs/digests/time spellings and nested HTTP
objects are exactly the paired contract; do not rename nested snake_case members. No tokens/headers/URLs,
arbitrary commands/paths, Engine IDs/generations or raw errors in this codec. No SCM_RIGHTS or other ancillary
data: close all received descriptors before refusal. Fixed error codes only; never echo invalid input.

Envelope order: `version,kind,conversationNonce,ordinal,payload`; version `observer-action-admin/v1`.
Broker first sends HELLO, nonce fresh nonzero32lowerhex, ordinal"1", payload `{}`. Root sends exactly ONE command
at ordinal"2" with that nonce; broker replies at ordinal"3" then closes. No unsolicited/second command or
resumption of a connection. HELLO does not reserve an execution nonce. Framing deadline5s per frame, whole
operation<=90s CLOCK_BOOTTIME with shorter remote/ACK deadlines below; no human approval wait holds a connection.
Timeout/disconnect is not rollback evidence. Retry uses STATUS or the same immutable inputs on a NEW connection.

| Root command | Exact payload (ordered) | Successful response kind/payload |
| --- | --- | --- |
| RECOVERY_INIT | `request,remote_receipt_digest,mapping_revision` | RECOVERY_STATE / State |
| RECOVERY_STATUS | `request,remote_receipt_digest` | RECOVERY_STATE / State |
| RECOVERY_CONTINUE | `request,remote_receipt_digest,recovery_id,prepare_id,approval_revision` | RECOVERY_STATE / State |

`request` is the complete canonical sixteen-field RequestReceipt, not action ID alone. Root cannot submit a
lease, evidence classification, allow decision, ACK bytes, observed state, full container ID or replacement
mapping. Recompute remote receipt digest and compare all fields, installed server/owner/instance and protected
current OR retained mapping selected by exact target/policy/capability/parameters; INIT's mapping revision is an
expected local revision CAS, not a new mapping. Private Key.Fingerprint remains the old distinct preimage.
CONTINUE selects an already remotely approved immutable prepare; it cannot create/approve an owner decision.

State ordered fields: `action_id,remote_receipt_digest,local_state,evidence_kind,mapping_revision,observed_state,
observed_at,ack_digest,remote` (all present). local_state is UNPREPARED/QUIESCED/ACKED/RECOVERY_REQUIRED;
evidence_kind null or the four classes below; mapping_revision null or LongString; observed_state null or
CREATED/RUNNING/EXITED; observed_at null or nine-fraction UTC local time; ack_digest null or64lowerhex;
remote null or exact RecoveryView. ACKED requires a validated durable ACK and its actual digest, not remote
finalization alone. QUIESCED is process-local evidence, invalid after restart/disconnect, never persisted as a
permission. STATUS never newly quiesces/inspects, creates a proposal, ACKs or releases anything; stale inspection
is returned only as the original ACK observation, not described as current. No listing/pagination endpoint.

Failure kind REFUSED payload exactly `{code}`: INVALID, BUSY, BINDING_MISMATCH, MAPPING_UNAVAILABLE,
NOT_QUIESCED, INSPECTION_UNAVAILABLE, REMOTE_UNAVAILABLE, OWNER_REQUIRED, FULL, RECOVERY_REQUIRED.
Unsupported version/malformed framing may close without reply. Any failure after potential local commit returns
RECOVERY_REQUIRED; absence of a reply never means absence of ACK. Errors do not clear global permits.

## Broker-owned worker relay (not worker initiation)

An active root command alone permits one broker-generated recovery relay conversation on the existing trusted
worker channel, with fresh nonzero32hex nonce independent of execution/admin nonces. Envelope version remains
`observer-action-local/v1`; exact added kinds RECOVERY_QUERY and RECOVERY_REPLY. Sequential ordinal strings1..5,
one outstanding query; reply must match exact ordinal/nonce/operation/full digest. No worker-originated query,
root admin command, arbitrary endpoint or owner Bearer material. Close/refuse unmatched replies without writes.
Queries carry `operation,action_id,remote_receipt_digest,body`; replies `operation,action_id,remote_receipt_digest,
outcome,body`. Operations and their exact typed body pairs:

| operation | Query body | OK reply body |
| --- | --- | --- |
| RETAINED | null | RetainedStatus |
| PROPOSE | RecoveryProposal | RecoveryView |
| STATUS | null | RecoveryView |
| CHECK | RecoveryCheck | RecoveryDecision |
| FINALIZE | FinalizeRequest | Finalization |

outcome OK/DENIED/UNAVAILABLE/CONFLICT/FULL; non-OK body must be null, never an HTTP error/body/header passthrough.
The worker constructs ONLY the matching fixed-origin/method/path from operation and action ID. All reads bind
returned full request/claim to the original16 receipt; all subsequent approvals bind exact action/server/owner/
instance/digest/evidence/lane/revisions. A fresh check nonce is broker-generated. Trusted worker TLS provenance
is the accepted TCB, not a signed ticket or proof against a compromised worker. Root cannot supply a substitute
Approval/RecoveryDecision. Worker cannot begin reconciliation independently, even with a genuine remote approval.

INIT uses RETAINED then PROPOSE; STATUS uses STATUS only; CONTINUE uses RETAINED, STATUS, CHECK, FINALIZE, with
at most one additional STATUS after ambiguous finalize. No retry of an Engine operation or looped HTTP mutation.
Each relay<=5s BOOTTIME within the original operation budget. A separate later root command performs recovery
lookup/retry. Human owner PREPARE remains the paired authenticated Platform operation, never this IPC.

## Ordering, evidence and partial recovery

INIT first blocks new broker BEGIN/admission, invalidates issued execution nonces, cancels/joins existing execution
within the existing75s stop bound, and holds exclusive journal/policy ownership. Failure to join refuses without
closing live storage or claiming quiescence. Worker may retain an unreceived claim remotely; no nonce can resume
it. No automatic service restart or global host setting is changed. Admin disconnect does not resume claims.
To resume, root explicitly stops and joins both action services, then restarts the unchanged verified profile.
Startup reopens existing journal and protected policy, validates complete binding/integrity and counts persisted
unknown fences/permits/capacity before offering fresh BEGIN. Restart discards every old execution/admin/relay nonce
and cannot resume an old action or interrupted approval check. Partial local ACK/remote HELD remains recoverable
only by a new root ceremony; central HELD capacity still applies to new claims. Unknown fences remain in force,
including the global two-permit limit. Missing/corrupt/ambiguous storage, binding/mapping mismatch or unjoined
execution keeps startup recovery-required; restart is not a force-clear of unresolved safety failures. No new
admin RESUME command or implicit enable-on-success is introduced.

Fetch authoritative retained source. Only OUTCOME_UNKNOWN or EXPIRED_UNCONFIRMED with HELD remote lane permits
proposal; live CLAIMED/REQUESTED waits for normal authoritative expiry, never forces it. Select protected immutable
mapping and bounded exact full-ID Engine Inspect; missing/replaced/unsupported target or mapping refuses. Inspect
is lookup only, never start/stop/restart, causal evidence or proof of no effect. Compute evidence locally:

| evidence_kind | Required local state; immutable consequences |
| --- | --- |
| MISSING_RECORD | Valid existing journal, exact action absent and above anti-replay floor; protected mapping exists. Later ACK transaction inserts RECOVERY_ONLY, never admission/INTENT/result. Absence is uncertainty, not no-effect. |
| ADMITTED_WITHOUT_INTENT | Exact original metadata/full16 digest, no events; prevents any future continuation of that action. |
| INTENT_WITHOUT_TERMINAL | Exact ordinal1 INTENT and no ordinal2; original UNKNOWN/intent remains unchanged; ACK releases only its existing permit. |
| RETAINED_KNOWN_TERMINAL | Exact original ordinal2 retained result; ACK never rewrites it, backdates a remote result or invents another local permit. |

No recovery from missing/corrupt/replaced journal. Proposal ID is deterministic first32lowerhex SHA256 of ASCII
`observer-recovery-proposal/v1\n` + remoteReceiptDigest + `\n` + evidenceKind (no final LF).
Only the public full-receipt digest and closed evidence-kind literal enter this NONAUTHENTICATING correlation;
no private evidence/mapping hash is disclosed indirectly through this ID. Local original metadata/events and
exact mapping are independently validated and compared under exclusive ownership through the ACK transaction.
The ID never replaces those checks or full remote digest/lane/approval comparisons. Repeated INIT must reproduce
the same immutable evidence/mapping, not overwrite an existing remote base. No new pending-recovery table; an interrupted pre-ACK
ceremony re-derives evidence and requires the same remote base. Engine observation may change without changing
mapping/evidence identity; final ACK records its actual final observation. Changed source evidence refuses old approval.

CONTINUE repeats global quiescence/mapping/evidence validation and fresh bounded inspection (unless an ACK already
exists, when its original bytes are authoritative). Fetch exact selected current Approval, then broker CHECK.
Current ACTIVE credential/registration and immutable binding apply throughout; reporting/status/propose need no
renewed human-positive grant. CHECK/final release require current owner source/account/grant and matching approval.
Never catch a poisoned positive owner failure and proceed. Capability removal alone does not block recovery.

Fresh CHECK obeys paired checked_at whole-interval/UTC+BOOT+Go-monotonic tests. Server-only r=expires_at-checked_at
must exceed1s; local ACK deadline B0+min(10s,r-1s), reply within5s. Validate before AND after actual local commit.
No machine-clock equality/offset, suspend inhibitor or promise that check/commit cannot race pause. Late/uncertain
commit latches recovery; inspect retained ACK on next command, never issue a second ACK.

One atomic local v2 transaction compares all unchanged metadata/events/mapping/remote approval fields, stores
the immutable canonical<=2048B ACK in its reserved slot, releases only its local fence/permit, and preserves
original events. Missing-record import consumes one of1000 records and its reserved slot in this SAME commit;
FULL refuses, never evicts unresolved evidence. Two global permits,12MiB DB/32MiB total and30d-after-ACK AND
original-expiry minimum/floor remain. The new remote32-byte digest still requires fresh packing proof.
Only after confirmed local ACK send FINALIZE with actual ACK digest; server CAS releases its separate HELD lane.
No distributed atomicity. Lost finalize reply uses lookup; same finalized identity returns original receipt.
Expired approval after local ACK requires deliberate renewed owner PREPARE bound to that same ACK; never overwrite
ACK with new prepare IDs/inspection. Local private evidence/mapping hashes stay local; only opaque ACK digest
crosses remote wire. At eight remote approvals FULL preserves both histories; no force-clear/reset fallback.

## Frozen design versus remaining implementation proofs

This selects members/order/operations and removes the unspecified admin-ingress gap; it does not claim executable
codec conformance. Add maximum/positive/negative vectors for these admin and relay envelopes to the paired fixture
before code (existing seven worker-kind vectors do NOT cover these additions). Keep16KiB, no truncation. Prove
wrong UID/user namespace/ancillary FD, duplicate/case/depth/length, unsolicited/replayed replies, all binding fields,
four origins, lost claim/reply/ACK, approval renewal/8FULL, quiescence failure, crash/ENOSPC/CAS and old action/new-key
fences through actual worker/broker/Platform consumers. No disconnected helper-only delivery.
Missing current/retained exact protected mapping refuses; this adds no unbounded mapping-history store.
Still gated: exact journal v2 DDL+packing with remote digest, selected bounded protected-mapping storage, actual
initial-namespace/profile/75s join proof and paired root/worker codec vector agreement. No product choice or new
crypto framework is needed; these are finite implementation/readiness evidence obligations, not activation permission.
