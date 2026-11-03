# Packaged Linux action worker and broker

Status: Design accepted by root/independent review for local consumed paired-runtime implementation/tests only.
Installed profile, release and production activation remain gated; no broader Spec011/ADR027 acceptance.
Historical runtime base: merged Observer `c2f11b2`; `5fd9e335` subsequently delivered native race acceptance only.
Decision proposal: [ADR030](../../docs/adr/030-packaged-typed-action-workers.md).
This is a complete executable vertical cut, not another unused primitive or authority to enable production actions.

## Product and trust boundary

First installed profile is Linux/amd64, rootful local Docker with API1.45 compatibility, systemd255-compatible
isolation, owner-only remote execution, and three independent start/stop/restart approvals. Other platforms,
delegates, generic commands, schedules, Kubernetes and bulk actions are unavailable. Observation stays independent.
Standalone/local-only action authentication is deferred; this first action profile requires Platform connectivity.
The standalone read-only observer remains useful without Platform and is unchanged.

The action worker holds only its own separate action credential and fixed-origin HTTPS access. It has no Engine
socket, private mapping or journal access. The broker holds the fixed `/engine/docker.sock` bind, root-provisioned
approval files and its private writable journal; no Internet/IP sockets, credential mount or arbitrary host view.
Both processes are trusted for remote-authority delivery; compromised worker assertions are not cryptographically
independent proof. Broker-enforced local scope, fresh policy reads, cooldown and unresolved fences still apply.
The installer never changes global Docker socket permissions or adds observation users to a Docker/action group.

## One closed local protocol

Proposed exact [root-only recovery IPC](admin-recovery-ipc.md) and [journal v2](action-journal-v2.md) own the detailed
recovery codec/state/CAS. Paired Platform Spec075 container-execution-v1/storage/composition contracts own HTTP
members/current authority; these selected contracts govern over this summary. All remain Proposed, not runtime acceptance.

Use one fixed pathname Unix socket in the broker-owned private listener leaf under root-owned ancestors defined by
[ADR031's fixed process profile](process-profile.md). Distinct static worker/broker UIDs
and an exclusive IPC group are provisioned explicitly. No abstract sockets, TCP listener, socket activation, arbitrary
paths, descriptor transfer or inherited listening descriptors. Authenticate both peers using SO_PEERCRED and
validated socket/path ownership; verify actual process identity/group policy at startup. Owner-root remains trusted.

Protocol `observer-action-local/v1`: length-prefixed canonical UTF-8 JSON, maximum 16KiB per message, depth <=8,
strict required/unknown/duplicate/case-sensitive keys and bounded finite enums. No raw HTTP, bearer, headers,
container ID, socket address or arbitrary error text crosses this IPC. Full IDs remain broker-private.
At most two active execution conversations, no waiting queue, and a small separately bounded control lane for
lookup/status/reconciliation. Oversized, unsolicited, malformed or out-of-order messages terminate the conversation.
Neither endpoint accepts ancillary data; close any received descriptors before refusing.

Root administration uses a DISTINCT protected local admin socket/path and a strict root-only peer allowset, not the
action-worker ingress. Worker UID must never invoke approve/change/revoke/reconcile-initiation/installation operations;
root CLI cannot use the worker channel to bypass its remote checks. Exact framing/operations/path are selected in
the linked admin contract. Reconciliation requires both this local initiation and remote-owner proof.

Execution uses the paired BEGIN/all-zero sentinel then a fresh broker-generated128-bit nonce; admin uses its HELLO.
Protocol version is an exact wire member; build compatibility belongs to verified package identity, not an invented field.
Every message binds that nonce and a monotonically increasing conversation ordinal. Closed kinds are execution,
authority challenge/reply, finite result, retained lookup, approved capability snapshot, and reconciliation.
There is no generic authorize/HTTP call.
Freeze exact JSON vectors with the paired Platform protocol before implementation, including integer precision and
canonical UTC microseconds; external revisions are canonical positive signed-Long decimal STRINGS, never JSON numbers.

An execution carries the complete immutable remote receipt: action/server/installation/owner/actor/opaque target,
operation, parameter version/value/digest, policy/capability revisions, created/expiry instants; plus replaceable
lease ID/deadline and local claim timing. Broker compares parameters with its local approval and existing core
fingerprint. Lease identity/timing never changes durable action identity. Missing journal is not an alternate executor.

The broker issues a fresh one-use conversation nonce BEFORE a genuinely new remote claim starts and records a
suspend-aware local start T0. Execution delivery must arrive within T0+30s, including HTTP and IPC delays. Never
start another allowance at IPC receipt, accept a reclaimed/old claim as new, or serialize a Go monotonic timestamp.
The accepted server claim must provide the full 120s lease inside the action expiry, with the current claim
credential valid strictly beyond lease expiry plus the existing 30s reserve; equality refuses.

The conservative timing candidate uses lease horizon T0+119s and receipt lower bound T0+measured claim RTT
(RTT nonnegative and no greater than the broker's elapsed time). The existing core horizon becomes
min(T0+90s, receipt-lower-bound+60s, T0+119s-30s), at most T0+89s. This does not add 90s after delivery or widen
the 45s Docker-call cap. Exact conversion into the existing time seam, inequalities and rounding vectors remain
paired-protocol pre-code gates, not an already implemented timer contract.

Linux CLOCK_BOOTTIME must constrain controlled phase boundaries and the actual transport write boundary;
ordinary Go monotonic/context timers can pause during system sleep. Awake cancellation and a late acknowledgement
retain UNKNOWN. Neither a BOOTTIME check nor cancellation makes the check and write atomic: suspension or process
pause between the final check and syscall, and later daemon execution, remain explicit races. Do not promise an
absolute no-write-after-deadline guarantee, add a host suspend inhibitor, or invent a clock-offset/ticket framework.
These are trusted-bridge assertions, not browser authority. Freeze the chain before code; prove it in acceptance tests.

## Two genuine current-authority checkpoints

The reviewed [startup supplement](snapshot-startup.md) closes initial server-time delivery and restart snapshot
CAS recovery with one observational worker head read and two closed control kinds. Control is a separate branch
of a BEGIN connection, never an execution reservation or nonce; its completion always requires a new connection.

Broker actioncore.Authority.Check sends a challenge tied to the entire immutable receipt, delivery lease and local
approved binding revision, with checkpoint ordinal and fresh nonce. The worker obtains a NEW fixed-origin TLS
decision for that challenge, not a cached claim result. Its closed reply contains the exact action/server/installation/
owner/actor/target/operation/parameter digest/revisions/lease, check nonce, active credential revision, and the server's
finite decision. Broker requires exact correlation and a response inside the original execution budget. Transport
errors, absent reply, expired lease, local approval drift and remote deny refuse before dispatch.

Remote authority must read current registration, current owner/actor permission, credential, capability and approved
policy under the owning server fence; the Platform slice must prove both race orders. No local UID, stale claim,
endpoint URL or bare `allowed=true` establishes those facts. A decision linearizes at that server check, not at Docker
mutation. Revocation after the final check can race dispatch; disclose that limit rather than claim distributed atomicity.
Do not hold a database transaction across IPC, Docker or the full claim lease. No mutation retry exists at any layer.

## Local approval and engine identity

Root CLI accepts an explicitly selected full 64-hex container ID, reads only bounded selected metadata, and requires
confirmation before minting an installation-bound opaque target. Persist server/installation/owner, explicit random
engine generation, full ID, operations, grace and immutable approval revisions in protected canonical records.
No names/short IDs/labels/observation hashes become authority. No target auto-discovery or implicit grant migration.
Only broker-produced approved capability snapshots may be advertised remotely: opaque target, independent operations,
approved grace and immutable policy/capability revisions with exact server/installation/owner binding. No full ID or
Engine generation leaves the broker. Publishing this snapshot does not grant a remote actor execution permission.
Worker cannot invent or alter it legitimately; broker rejects any subsequent receipt that differs from local policy.
The representation below permits32 active operation entries, not32 targets, with no paging/truncation. It retains
only bounded exact mappings needed by journal records; absent mapping refuses rather than reconstructing history.

Approve/change/revoke is an atomic durable policy operation. Recheck current local policy at both authority checkpoints;
retain immutable prior mappings referenced by retained actions/unknown evidence. Changing operation/grace increments
policy revision; target or engine replacement mints a new opaque target/generation and requires fresh approval.
Socket metadata is only drift detection. Drift refuses all new dispatch until root explicitly validates the intended
engine binding; no automatic same-inode/generation inference, credential/journal replacement or fence bypass.

### Closed bounded policy representation

Fixed root-provisioned `/etc/home-lab-observer-actions/policy.json`, root-owned0640 with ONLY the dedicated broker
read group, parent0750 under root-controlled ancestry; no worker membership, extra/default ACL, links or privileged
mode bits. Broker read-only mount; updates are explicit root lifecycle operations with execution stopped/joined and
exclusive policy/journal coordination, not recovery IPC commands. No generic policy path or caller file loader.
Canonical JSON envelope ordered `version,serverId,ownerUserId,connectorInstanceId,revision,current,mappings`;
version `observer-action-policy/v1`, revision positiveLongString. current is sorted unique mappingRevision strings
(numeric ascending) selecting at most32 entries. mappings is mappingRevision-sorted array of at most1032 immutable
operation records, each exactly the ordered mapping object in journal-v2's observedMappingDigest definition.
Each compact record<=1024ASCII bytes; complete canonical file<=2MiB/depth8; unknown/duplicate/case/encoding refusal.
All bindings equal envelope. Envelope revision is the persisted allocation high-water mark, never lowered by pruning.
For a publication introducing N fresh semantic mapping records, sort those records by their canonical semantic
tuple bytes (the mapping object order excluding mappingRevision, closed ASCII fields); allocate oldHead+1 through
oldHead+N. New head=oldHead+max(1,N), checked for overflow<=9223372036854775807 before any write. Initial oldHead=0:
an empty first policy has head1; a nonempty first policy allocates1..N and headN. Reused unchanged records retain
their original mappingRevision; all record revisions must be<=head. Deleting the highest retained record never
lowers head or permits reuse. PolicyRevision/capabilityRevision remain independent immutable semantic fields,
not renumbered by this allocation. Counter exhaustion refuses the entire publication, including empty/revoke changes.
Current entries are unique by targetId+operation and satisfy paired alias/parameter consistency when advertised.
Across ALL current and retained records, the semantic tuple server/owner/installation,targetId,engineGeneration,
containerId,operation,policyRevision,capabilityRevision,grace must be unique: reject the same tuple under different
mappingRevision and reuse its original immutable mapping record when unchanged. Journal metadata contains no
mappingRevision; derive references/retention eligibility by this unique actual metadata tuple, never an invented field.
At most1000 records may be noncurrent, retained for the at most1000 journal actions; deduplicate identical referenced
mappings. A now-unreferenced record may remain until the next explicit policy publication, still within that cap.
No additional historical registry, enumeration fallback or automatic recreation.
Root change preserves every referenced record byte-for-byte, advances head by the rule above, and selects a complete new
current set. A current record may remain referenced after revocation. Discard a noncurrent mapping only after
its last referencing journal action is durably pruned; interrupted cleanup merely retains within the fixed cap.
If required records cannot fit, refuse FULL without changing either file/journal; never drop referenced evidence.
Publish through one fixed exclusive stage file `policy.json.stage`, fsync file, atomic rename, fsync parent;
both old+stage bounded to4MiB total, no backup chain. Existing stage/ambiguous publication requires explicit recovery
and validated old/new snapshot, not auto-delete/adopt. Same-root hostile rollback is outside this mechanism.
Exact policy vectors and stopped-update/crash/privacy tests accompany the consumed implementation; these are fixed
selected limits, not claims that current installed helpers already implement this action-specific policy.

## Journal composition and retained reporting

Broker exclusively owns the existing actionjournal for the exact server/installation/owner. Provision only on explicit
fresh install; normal startup only OpenExisting. Preserve current 32MiB total, 1,000 records, two permits, cooldown,
30-day terminal retention, canonical fingerprint and unknown fences. The worker cannot replace or copy back state.

Execution still goes through actioncore without weakening expiry. Add a bounded broker-owned read-only retained-result
lookup after current ACTIVE machine credential/registration and immutable binding/source-integrity authentication,
not renewed positive human grant/account eligibility. This is required because Execute refuses
expired requests before journal lookup. Lookup cannot create an admission, execute, release a fence or rewrite a result.
Result transport retries the exact retained result only while the remote result contract permits; after lease/action
expiry, record/report unconfirmed state through its separately defined read/reconciliation path, never a fabricated
successful terminal update. Credential rotation may report an existing valid lease only for the same installation.

The execution horizon does not consume the reserved reporting window. A read-only reporting timing check stays
bound to the original execution conversation's native T0, immutable action/receipt and conservative T0+119s lease
horizon (deadline equality refuses), including the original continuity checks. It may admit a joined retained
result for IPC/HTTP reporting after the active execution horizon ends; it never creates an execution context or
extends permission for Engine mutations, journal writes, another claim or another action. Use the original parent
cancellation scope for this phase, not an already expired core execution context. Report transport remains bounded
and non-retrying; neither successful IPC handoff nor EOF proves remote acceptance.

For this reporting check only, continuity from the original snapshot-query send must cover the complete
less-than5s control exchange, up-to15s readiness age after its receipt, and less-than119s reporting horizon:
less than139s in total. Preserve the original origin and both continuity comparisons; do not reset the clock
or widen existing execution/mutation/journal limits. Cancellation is rechecked after native sampling too.

Worker execution receives distinguish bounded broker processing from frame transfer: waiting for a checkpoint
challenge or the final result may include the original bounded inspection/Docker call. Bound that idle wait by
the original conversation/lease phase, then retain the existing five-second whole-frame limit from the first
header byte through the remaining header/body. Only challenge ordinals3/4 and result ordinal5 use this receive
path; control/admin framing and all byte, ancillary-descriptor, cancellation and native-phase checks are unchanged.
Do not extend Engine call limits or add a general caller-configurable idle timeout.

The broker's initial claim delivery has the same processing-versus-transfer distinction, but a separate closed
receive path: ONLY CLAIMED_EXECUTE or NO_CLAIM, worker ordinal2, under the existing execution nonce. Its idle wait
uses the ORIGINAL preclaim T0+30s deadline, including approval send, HTTP claim and IPC; never read-start+30s.
After the first header byte, complete the remaining header/body within min(original deadline, first-byte+5s).
The fixed HTTP client's5s request limit and all other frame kinds/limits are unchanged. A named DeliveryContext
and CheckDelivery retain the original UTC/Go-monotonic/BOOT samples, check continuity and parent cancellation
before and after native sampling, and refuse deadline equality conservatively. They create no execution permit
or authority and cannot restart the clock. Recheck before approval send/receive and after the complete frame,
including NO_CLAIM before releasing its anonymous reservation; bindClaim still validates the one-use original
claim/RTT. EOF, partial frames, timeout or cancellation never substitute for authenticated bodyless HTTP204.
Test delayed first byte beyond5s processing (with shortened private test caps), partial header/body, original
delivery boundary, cancel during native sampling, suspend/discontinuity, ancillary descriptors and branch misuse.

## Explicit owner acknowledgement of UNKNOWN

First version uses a root-local CLI ceremony PLUS authenticated current remote-owner approval. A root shell is not
remote-owner identity. Root selects an exact full remote receipt; the admin contract handles missing local record,
admitted-without-intent, unresolved-intent and retained-known-terminal cases. Broker derives immutable proposal
correlation and fresh current-check nonce; owner approves exact remote action/evidence/lane through Platform,
acknowledging that the old
operation may have happened and that inspection cannot establish causality. No delegate/worker-issued approval.
Approval lifetime <=5min; equality expired. Worker carries the bounded receipt, never the owner's session secret.
This first recovery path requires host console/SSH access for the root operator, plus the owner's existing authenticated
Platform session for the narrowly scoped approval. It is a deliberate rare-recovery usability limit; no general browser
recovery dashboard, SSH transport through Platform, or root-only replacement for owner authorization is included.

Broker performs bounded fixed Engine inspection of the retained full ID, then repeats current-owner/reconciliation
authorization via a fresh correlated TLS bridge check. Under the exclusive journal transaction, compare the exact
still-unresolved action/fence and unused acknowledgement identity; append acknowledgement plus finite observed state
and checked revisions, and release ONLY that action's unresolved permit/fence. Preserve original intent and UNKNOWN
unaltered within its documented audit lifecycle; never recast it as success/failure or redispatch it. Inspection failure
refuses acknowledgement in this version; no force-clear fallback. Missing/replaced containers remain recovery-required.
The journal has TWO GLOBAL permits, not two per target. Two unknown intents whose targets cannot be inspected can
therefore disable ALL installation actions indefinitely, including unrelated approved targets. This is an explicit
availability limitation pending a separately reviewed recovery extension; never clear a fence, replace the journal,
infer no effect from missing containers, or hide the limitation behind a retry instruction.
Repeated acknowledgement is idempotent only for identical identity/payload, with no new Engine mutation. Concurrent
new execution cannot pass a fence until that transaction commits. A new action requires a new key and fresh authority.

The same ceremony must also complete the Platform admission-lane release. Proposed order is remote current-owner
approval PREPARE while the server lane remains HELD, local atomic acknowledgement, then a remote exact-identity CAS
release. These are separate commits, never distributed atomicity. Read-only status and idempotent completion must
recover a lost reply or crash between them without redispatching the old operation or releasing another lane.

A lost claim response can leave EXPIRED_UNCONFIRMED with no local record; lost results may leave a known terminal.
The linked admin/v2 contracts select all four cases, including atomic RECOVERY_ONLY import with ACK and refusal
without exact protected mapping. Missing state is never no-effect proof, journal replacement or automatic release.

This requires a versioned journal schema/reader update: explicit reconciliation evidence, immutable original outcome,
and bounded retention/capacity reservation. The linked v2 DDL/CAS is the proposed code contract; capacity/crash tests
are implementation acceptance, not circular prerequisites to writing tests/code. Do not overload Finish or mutate
old terminal events. Acknowledged UNKNOWN may become retention-eligible only under a separately documented timestamp
rule at least 30 days after acknowledgement AND original expiry; preserve anti-replay floor. No arbitrary prune/clear.
Lost credentials require explicit same-owner/same-installation action re-enrollment, validated against existing journal;
no observation-token fallback. Ownership/installation change, unavailable remote owner or lost journal cannot use this
ceremony to transfer/recreate authority. Those states remain disabled with preserved evidence and an honest limitation.

## Credentials, package and shutdown

Implement existing Platform ADR052 end to end: one-use 10min bootstrap, separate 30-day action secret, renewal seven
days early, 10min PREPARED successor, durable local successor before activation, predecessor CAS, no overlap grace,
auth-only status after ambiguous activation, and no claims with prepared/uncertain credentials. Store raw secrets only
in the action-worker secret boundary; never IPC, arguments, output, observation paths or audit. No automatic re-enroll.

Package installer, worker, broker, fixed unit/socket definitions, closed manifest/compatibility identity, instructions
and source-restricted checksums/attestations. No change to native-v2 or observation install profiles. Explicit commands:
install-stopped, enroll, approve/change/revoke target, enable, status, disable, renew, reconcile, upgrade/rollback, remove.
No mode accepts arbitrary commands, URLs or Docker paths. Resource limits and effective-profile negative tests are
part of the package, not merely template strings. Installer validates root-controlled artifacts, actual UID/group
membership, no observation access, exact binding and fixed manager properties before enabling.
Reuse reviewed installed-package filesystem, credential and lifecycle helpers only where their policy is identical;
do not copy an entire privileged installer framework. The broker's RootDirectory binds the Engine socket at the
existing adapter's fixed `/engine/docker.sock` inside its private view; never create a host-global `/engine` directory.
Action credentials/mappings/journal still require their own exact boundaries, not observation-helper defaults.

Disable first stops claims/new IPC admissions, then cancels authority checks and joins active executions. Core's
bounded phase and storage cleanup must finish before closing Engine/journal. Proposed stop budget 75s, covering
60s active phase plus bounded joins/cleanup; confirm with deterministic and actual package tests. systemd uses
control-group termination after the configured deadline. A forced stop preserves durable intent as UNKNOWN; it
does not prove Docker stopped. Never close journal under an unjoined execution or launch a replacement while its
exclusive lock/process remains. Shutdown/result transport cannot retry a mutation. No background unbounded goroutines.

Upgrade/rollback stops and joins both, verifies exact compatibility, preserves journal/approval/credential files,
then revalidates the complete profile. Reject older readers after new acknowledgement schema/facts exist. Removal
defaults to disabling services while retaining protected audit/state; destructive evidence deletion is not routine
uninstall. Observation keeps running during action failure, revocation, disable and rollback.

## Coherent delivery and acceptance

Before implementation: accept ADR030/full relevant Spec011 decisions; freeze paired Platform claim/current-check/
result/owner-reconciliation contracts and compatible reader floor, IPC vectors, journal schema/size proof, approval
storage and service profile. Existing dormant lifecycle/readers are not production authentication endpoints.

Deliver two SMALL consumed vertical PRs, each independently reviewed:

1. Runnable two-process path using the actual TLS protocol against an isolated real Platform protocol fixture,
   existing core/journal/Engine, private/admin ingress, retained lookup and authenticated reconciliation. Prove complete
   execution/recovery on owned disposable targets; no installed production activation or allow-stub binary.
2. Installed bundle with enrollment/renewal, fixed profile and lifecycle, compatible upgrade/rollback, and disposable
   VM proof of the exact packaged path. Neither production claim activation nor existing-workload use is permitted
   until BOTH slices and their paired Platform prerequisites pass.

No production allow stub or `--accept-any-owner` mode may ship. Synthetic ports remain test seams, not deliverables
substituted for the real protocol. Exact paired wire/clock/profile/journal-ack DDL and bounds remain pre-code gates;
these two consumed cuts are not permission to create a generic wire framework or split into unused helper PRs.

| Area | Required adversarial evidence |
| --- | --- |
| Remote/current authority | Two checkpoints, both revocation race orders, wrong actor/owner/install/target/revision/lease, expired equality, unavailable denies |
| IPC/isolation | Wrong peer, observation UID, stale/cross-request nonce, oversized/duplicate/case keys, ancillary FD cleanup, disconnect, worker cannot reach Engine/journal, broker cannot reach IP/credentials |
| Approval | Full-ID recreation, engine/socket drift, alias reuse, changed grace/revisions, immutable old mapping retained, no automatic adoption |
| Durability | Kill each intent/commit/report boundary, lost response/result, actual full disk, same/new key after restart, no second mutation |
| Renewal | Prepared denial, local persist failure, ambiguous activation, predecessor races, rotation during lease, instance replacement |
| Reconciliation | Root without owner denied, delegate denied, stale/forged challenge, fresh owner loss, inspection failure, replay/CAS races, audit retained and old action never dispatched |
| Lifecycle | Stop at every phase, unjoined execution, lock contention, rollback reader incompatibility, missing/corrupt state, observation unaffected |
| Actual package | Disposable VM and owned full-ID containers only; three actions/no-ops, permission denial, external-state races, real service identities/confinement, exact cleanup |

Finite status distinguishes disabled, unavailable, denied, active, credential-recovery-required, and unknown-fenced.
Daemon acknowledgement is not application health. Redact tokens, IDs/private binding, raw HTTP/inspect/errors from
logs; count bounded reason categories without identity labels. Root-private inspection is a deliberately separate view.

This design permits local implementation/tests, not installation, host mutation, remote grant or publication. The initial canary remains
one owner-controlled disposable target, only after independent review of exact package and isolated acceptance.
