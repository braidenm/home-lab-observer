# Packaged action journal v2: immutable recovery acknowledgement

2026-11-02. **Design accepted for local consumed paired-runtime implementation/tests only.** No implemented-state,
migration, installation or production activation approval. Owned by narrowly accepted
[ADR030](../../docs/adr/030-packaged-typed-action-workers.md), with [root recovery IPC](admin-recovery-ipc.md).
Existing Proposed ADR027, accepted ADR028/029 and [journal v1](action-journal.md) are not rewritten. Versioned v2 construction is consumed
only by the paired worker/broker slice; no standalone unused recovery library or automatic migration release.

## Files, versions and consumed operations

Preserve fixed action.sqlite/.action-lock/action.sqlite-journal, Linux owner-private ancestry/modes/ACL/link checks,
exclusive lifetime lock, validated rollback-header handling and unsupported-platform refusal. ProvisionV2 requires
explicit fresh empty installation; OpenExistingV2 requires existing v2. application_id=0x484C414A, user_version=2.
v1 readers/construction remain unchanged; v2 refuses v1, unknown versions and non-exact schema before mutations.
No delete/recreate, repair, copyback, quarantine, schema autodetection or in-place migration. Existing v1 developer
evidence is not disposable merely because no production worker was installed. Bundle compatibility minimum2 and
paired remote reader/source floor precede activation; rollback below2 refuses while preserving artifacts.

Broker owns versioned provision/open, bound execution admission, exact retained lookup, immutable ACK commit,
bounded prune and close. Recovery/lookup never call Execute/Acquire to bypass expiry. Each execution binds its
complete remote16 receipt to a separate32-byte remote digest BEFORE the core-facing journal adapter admits it.
No caller-settable allow/clock/SQL callback; that adapter retains the exact request/digest for this one invocation.
Existing core Key.Fingerprint and CanonicalJournalMetadata bytes stay v1-identical. Compare both fingerprints;
same private key plus changed remote digest is conflict, not replay. Recompute remote digest from the complete
canonical paired receipt (including original correlation/idempotency fields), never from a subset or private key.
The v2 remote projection is representable only when Owner==Actor, all external revisions are positive<=LongMAX,
and original request expiry is exactly requested_at+300s. Reject otherwise; do not narrow/cast broader v1 values.
On restart the stored remote digest cannot reconstruct the full16 receipt from private metadata. Obtain a fresh
authenticated exact full16 retained readback, recompute its digest and compare all representable private bindings
before reporting/recovery. No cached partial reconstruction, default correlation or inferred idempotency field.

## Exact proposed schema

The definitions below are the complete six named sqlite_schema objects (four tables/two indexes). No extra
views/triggers/indexes. Schema comparison is source-owned exact SQL, never first-live adoption. SQLite builtin
checks are structural; pure validation below provides canonical/relational/authority meaning.

```sql
CREATE TABLE actions (
action_id TEXT PRIMARY KEY CHECK(length(action_id)=36),
target_id TEXT NOT NULL CHECK(length(target_id)=36),
engine_generation TEXT NOT NULL CHECK(length(engine_generation)=32),
container_id TEXT NOT NULL CHECK(length(container_id)=64),
metadata BLOB NOT NULL CHECK(length(metadata) BETWEEN 1 AND 2048),
digest BLOB NOT NULL CHECK(length(digest)=32),
remote_receipt_digest BLOB NOT NULL CHECK(length(remote_receipt_digest)=32),
origin TEXT NOT NULL CHECK(origin IN ('ADMITTED','RECOVERY_ONLY')),
recorded_at TEXT NOT NULL CHECK(length(recorded_at) BETWEEN 20 AND 30)
) STRICT, WITHOUT ROWID;
CREATE TABLE events (
action_id TEXT NOT NULL REFERENCES actions(action_id),
ordinal INTEGER NOT NULL CHECK(ordinal IN (1,2)),
at TEXT NOT NULL CHECK(length(at) BETWEEN 20 AND 30),
code TEXT NOT NULL CHECK(code IN ('INTENT','STARTED','STOPPED','RESTARTED','ALREADY_RUNNING','ALREADY_STOPPED','TARGET_STATE_CONFLICT','DENIED')),
reason TEXT NOT NULL CHECK(reason IN ('','EXPIRED_UNDISPATCHED')),
PRIMARY KEY(action_id,ordinal),
CHECK((ordinal=1 AND code='INTENT' AND reason='') OR (ordinal=2 AND code!='INTENT')),
CHECK((code='DENIED' AND reason='EXPIRED_UNDISPATCHED') OR (code!='DENIED' AND reason=''))
) STRICT, WITHOUT ROWID;
CREATE TABLE metadata (
singleton INTEGER PRIMARY KEY CHECK(singleton=1),
server_id TEXT NOT NULL CHECK(length(server_id)=36),
installation_id TEXT NOT NULL CHECK(length(installation_id)=38),
owner_id TEXT NOT NULL CHECK(length(owner_id)=36),
pruned_expiry_through TEXT NOT NULL CHECK(length(pruned_expiry_through) BETWEEN 20 AND 30)
) STRICT;
CREATE TABLE recovery_slots (
action_id TEXT PRIMARY KEY REFERENCES actions(action_id),
ack_length INTEGER NOT NULL CHECK(ack_length BETWEEN 0 AND 2048),
ack_payload BLOB NOT NULL CHECK(length(ack_payload)=2048),
ack_sha256 BLOB NOT NULL CHECK(length(ack_sha256)=32),
CHECK(ack_length>0 OR (ack_payload=zeroblob(2048) AND ack_sha256=zeroblob(32)))
) STRICT, WITHOUT ROWID;
CREATE INDEX actions_target ON actions(target_id);
CREATE INDEX actions_physical ON actions(engine_generation,container_id);
```

The action's recorded_at is actual local admission time for ADMITTED, actual import time for RECOVERY_ONLY;
there is no second admitted_at or fabricated original admission. Events retain exactly the v1 ordinal/code/reason
shape. RECOVERY_ONLY has no events forever and must already be ACKED in every committed state. One recovery slot
per action is mandatory; RESERVED derives from length0, ACKED from positive length (no redundant state column).
An ADMITTED row and all-zero reserved slot are inserted in one transaction before admission is exposed. Every
positive slot contains strict canonical unpadded bytes followed only by zeros; SHA256 hashes those actual bytes.
Validate count<=1000/actions and slots, <=2000/events, no orphan/missing/duplicate relations, every metadata/index/
fingerprint/binding/time invariant and operation-compatible terminal using shared v1 pure validators where identical.
SQL pre-materialization bounds use byte lengths CAST(TEXT AS BLOB), including NUL-tail/multibyte negatives.
All-record validation uses three fixed ordered streams (actions/events/slots), no per-record SQL. Single/related
loads share the same validators; bound all fields before allocation. Preserve two-second open/operation deadlines.

## Canonical ACK and private evidence

ACK object order is exactly `version,recoveryId,prepareId,ownerDecisionId,actionId,remoteReceiptDigest,evidenceKind,
evidenceDigest,mappingRevision,remoteLaneRevision,ownerUserId,connectorInstanceId,challengeNonce,preparedAt,
approvalExpiresAt,acknowledgedAt,observedState,observedAt,observedMappingDigest`. All fields required; version number1;
IDs/digests follow paired closed grammars, positive revisions LongStrings, no duplicate/unknown/case aliases.
Four time fields use exactly nine fractional UTC digits, calendar-valid. HTTP six digits are expanded with three
zeros, not rounded. observedState CREATED/RUNNING/EXITED only. Observed time cannot exceed acknowledgedAt; ACK
time cannot precede local recorded_at/original event times. Remote approval expiry is enforced by the paired fresh
checked_at/elapsed rule, not local UTC equality. Backward local time refuses/defer, never invents chronology.
The paired maximum vector is988 ASCII bytes plus1060 zero padding to2048; actual semantic valid vectors remain
required. No LF/escapes/whitespace in canonical payload. Size arithmetic is not DB occupancy proof.

evidenceKind is MISSING_RECORD, ADMITTED_WITHOUT_INTENT, INTENT_WITHOUT_TERMINAL or RETAINED_KNOWN_TERMINAL.
evidenceDigest is SHA256 of ASCII `observer-action-evidence/v1\n` plus compact canonical object in order
`origin,metadata,events`; metadata is the actual canonical v1 metadata object, events an ordinal-sorted array of
objects ordered `ordinal,at,code,reason` using original exact stored time spelling. For MISSING_RECORD only,
origin is literal MISSING and metadata is the exact proposed private key reconstructed from the full remote
receipt plus protected mapping, events empty. This immutable pre-import digest remains the ACK's evidence after
RECOVERY_ONLY import; verifier reconstructs the MISSING preimage, not a fictitious prior admission. Other origins
are ADMITTED. Canonical JSON serializer must embed the actual metadata object, not double-encode its JSON string.

observedMappingDigest is SHA256 of ASCII `observer-action-mapping/v1\n` plus canonical ordered object
`serverId,ownerUserId,connectorInstanceId,targetId,engineGeneration,containerId,policyRevision,capabilityRevision,
operation,parameters,mappingRevision` for this one exact operation binding. Revisions LongStrings; operation and
parameters use the paired exact HTTP literals/object shape. Never hash unspecified whole policy files or unordered
allowsets. Missing exact current/retained mapping refuses; no unbounded history feature is introduced.
Journal v1 metadata has no mappingRevision. Resolve its mapping through the unique immutable semantic tuple
`server/owner/installation,targetId,engineGeneration,containerId,operation,policyRevision,capabilityRevision,grace`.
Across ALL current/retained policy records, reject that same semantic tuple with different mappingRevision;
unchanged semantics reuse the original immutable mapping record. Journal references and safe mapping cleanup
derive from actual canonical metadata, not a nonexistent field. ACK then records that uniquely resolved revision.
These two digests and their private preimages remain local. Remote receives only opaque ACK digest; root/public
proposal ID contains only public full receipt digest/evidence kind, never these private hashes. None proves
causality, absence of side effect, hostile-root integrity or server-recomputable private evidence.

## Local transaction CAS and non-execution rules

1. Root ingress quiesces/join execution and holds exclusive lifecycle/policy ownership; journal transaction does
   not span network/Engine I/O. Read exact original record/events/mapping, inspect fixed retained fullID, and obtain
   paired fresh current-owner recovery decision for selected approval. Caller cannot manufacture those facts.
2. BEGIN IMMEDIATE revalidates singleton binding/floor, original metadata/private+remote digest, exact event set,
   slot and unchanged mapping revision; no live attempt/preclaim reservation. Recheck original BOOTTIME commit
   deadline immediately before COMMIT. Existing slot update CAS requires action_id, ack_length=0, zero payload/hash
   and prior exact action state. Every row count must match; changed state rolls back. An already ACKED slot returns
   its original bytes only after full identity validation, never writes another ACK or samples a replacement time.
3. Missing record additionally requires expiry strictly above floor and capacity<1000 after normal eligible prune.
   Insert RECOVERY_ONLY action plus ACKED slot atomically; no observable empty import, admission or INTENT. Existing
   admitted/intent/terminal cases never change original rows/events. Existing unknown releases exactly its opaque,
   physical and global permit by deriving unresolved = INTENT without terminal AND RESERVED, never by deleting intent.
4. Explicit context-bound COMMIT followed by BOOTTIME check; failed/ambiguous/late completion poisons the handle.
   Reopen validates retained ACK to discover committed state, never repeats Engine or falsely declares rollback.
   Expose digest for remote FINALIZE only after confirmed local durable commit. Independent bounded rollback/close
   must finish before releasing lock; an unjoined execution never loses storage underneath it.
5. Remote HELD-to-release remains separate CAS. Local ACK never means remote finalized. Expired prepare/new owner
   decision renews remotely against the SAME immutable local ACK; no local approval history/finalized flag/table.
   Remote cap8 approvals/FULL and source-integrity rules remain paired authority, not local permission substitutes.

RECOVERY_ONLY and ACKED ADMITTED-without-intent can never return a dispatch-capable Attempt, regardless of fresh
lease. ACKED INTENT remains UNKNOWN for lookup; known terminal retains its original outcome. Old action never
redispatches; broker refuses it before core execution, with retained reporting through dedicated lookup. A different
action needs new key/current authority/local approval and both available local/central capacities. UNKNOWN from
another action's target fence is NOT evidence that this action dispatched. Preclaim reservation handoff counts
once with existing two permits, never adds a parallel capacity pool. ACKed unknowns may coexist with a new unresolved
action at the same physical target; only RESERVED unknowns count in duplicate unresolved-fence validation.

## Retention, capacity and compatibility acceptance

UnACKed intent never prunes. Non-ACKed known terminals keep v1 terminal-time AND original-expiry30d rule. ACKed
records require acknowledgedAt AND original ExpiresAt <= actual now-30d; known terminal additionally satisfies its
terminal-time rule. No live attempt/reservation may prune. Delete at most50 actions, their <=100 events and50slots
in one transaction; no cascading evidence deletion. Advance singleton floor monotonically to the maximum committed
eligible cutoff, separately from actual deletion eligibility (never clamp cutoff forward after clock rollback).
Missing/import/admission with expiry<=floor refuses, even after external old request replay; no floor reset.

Keep4096-byte page size,3072-page/12MiB DB and32MiB DB+journal caps, DELETE+EXTRA, cache_size3072/cache_spillOFF,
temp_storeMEMORY/trusted_schemaOFF/FKON/mmap0/busy250ms, one connection/no ATTACH/savepoints/vacuum. Reuse privacy,
header and fail-closed durability checks only where identical. Reserve one2048 slot and two logical event slots
per action; ACK updates may still allocate/split pages. Physical ENOSPC can prevent ACK and must not clear fences.
At1000 records, missing recovery may use only ordinary retention-eligible prune; otherwise FULL, never steal slots.
The conditional derived25,321,472-byte combined ceiling relies on unchanged no-spill/page/header assumptions.

packing_proven=false: earlier Windows research WITHOUT remote32-byte column observed2401 reserved/2402 updated
pages; this is not current-v2 proof. Required actual maximal valid schema:1000actions,2000events,1000reserved then
all maximal ACK slots, adversarial insertion orders, before/after-COMMIT samples plus separate derived bound.
Samples are not exhaustive intermediate peaks, semantic/crash/ENOSPC or physical power-loss evidence. If it cannot
fit, revise packing under review; never raise caps/lower1000/skip cases. Capacity/fault proof is implementation
acceptance, not a prerequisite to writing the consumed implementation after the complete spec freeze.

Production seams: current internal/actionjournal/schema_linux.go exact v1 definitions/verify stay explicit; versioned
v2 schema/load/admission/lookup/ACK/prune share only proven-identical pure helpers. Tests prove v1 bytes/fixtures and
rejection of cross-version writes, not a generic multi-version migration framework. Deliver with actual paired
broker recovery consumers, effective-profile negative tests and source/release compatibility identity, not an
unreachable optional parser. No source file edits or executed schema follow merely from this Proposed contract.
