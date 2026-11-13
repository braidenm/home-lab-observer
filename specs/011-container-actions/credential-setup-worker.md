# Consumed ACTION bootstrap and quiet worker renewal

Status: Root/independent R2 design accepted for this local cut1 implementation/tests only; source-first commit
d3aaf3d preceded runtime. Full source review/current-head CI, package and installed/paired gates remain open.
Parent [package plan](action-package.md), local docs commit1db863c; Observer base4193c3d / paired API sourceb07a2335.
This cut consumes the existing RunWorker, protected role/settings/credential readers and fixed-origin HTTP client.
It is not an installer, live enrollment procedure, new authority, recovery workflow or an unused client-only PR.

## Deliverable and fixed role boundary

Add `observer-action-setup exchange`: exactly the executable name plus the literal exchange; no URL, identity,
path, secret, expected-UID or provider arguments. This mode requires the actual existing WORKER role/host namespace,
secret-process hardening and identical manager-environment admission before reading stdin or opening a secret.
It uses protected worker.json and bounded existing profile paths. It never opens Engine, journal or broker IPC.
No extra service user. Package cut2 supplies root setup/lifecycle and networkless broker-only Provision modes in
this SAME third binary; this cut implements no placeholder root mode, arbitrary dispatcher or new service.

The future fixed transient enrollment unit runs that mode as worker:worker with IPC membership ONLY, protected
settings/CA/DNS/first-party egress and worker state, no Engine/journal/policy/observation credential mounts.
Main worker/broker remain exactly two services, stopped and disabled during bootstrap. No secret in unit
properties, argv, environment, diagnostics or stdout/stderr. Input is only one81-byte ACTION enrollment token,
optionally followed by one LF, with EOF required; at most82bytes, cancellation/deadline aware. Never consume the
observation hle_ grammar. Normal exchange input comes from the future supported once-only owner disclosure/no-echo
installer prompt, NOT a documented curl/manual-file workflow. Uninstalled source does not authorize live use.

One setup/renewal invocation has an ORIGINAL30s budget, including stdin, HTTP, local transitions and any status
resolution. Capture using the existing private worker captureStamp/elapsed (UTC, retained Go-monotonic, BOOT);
use the original Go deadline and check cancellation before AND after native samples. Each HTTP leg retains its
existing5s child context and native pre/post check; perform at most ONE new-then-old status pass (one GET each,
no polling). Do not restart30s after a reply/reopen within that invocation. An explicit later invocation samples
a new operation budget but cannot repeat an attempted POST or resume a Docker action.

Stdin EOF must arrive strictly before ORIGINAL T0+5s. Pin fd0 as FIFO, reopen ONLY /proc/self/fd/0 read-only,
nonblocking/CLOEXEC with the same pipe witness; never alter caller flags or accept an arbitrary file/socket/TTY.
Poll at fixed25ms, rechecking original native/Go/cancellation bounds; no reader goroutine. Refuse over82bytes,
missing EOF, cancellation, drift or late input before any HTTP. Close only the owned reopened descriptor.
Use existing pre/post native timing plus contexts, not a new HTTP polling monitor/framework. Suspension or an
uninterruptible filesystem/network operation can delay RETURN; every late acceptance check still refuses/poisons
without READY/claim, retaining attempted evidence. No hard OS-return deadline, detachable writer, execution permit
or atomic deadline/syscall guarantee is claimed. Credential lifecycle timing does not widen Engine/COMMIT guards.

## Four typed HTTP operations; no transport widening

W=/v1/connectors/home-lab/servers/{exact_settings_server}/action-credentials.
Exchange(EnrollmentSecret) POST /exchange; Prepare(CurrentActionSecret) POST /prepare;
Activate(PreparedActionSecret) POST /activate; Status(ActionSecret) GET /status. All bodies absent, exact existing
instance header, existing correlation/privacy behavior. Schemes ActionEnrollment vs ActionConnector are distinct.
Use existing private fresh HTTP/1/TLS/no-proxy/no-redirect, five-second whole request/body, response/header/depth/
byte/concurrency bounds and joined cancellation. No exported arbitrary path/scheme/request API, idempotency header,
GetBody replay, POST retry, credential fallback or generic SDK. Existing execution/snapshot/recovery methods and
their validators retain signatures/semantics. The lifecycle client cannot create an execution principal/permit.

EnrollmentSecret validates exactly hlae_+32lowerhex+dot+43 canonical base64url characters decoding to32bytes.
ACTION uses existing exact hlac_ validation. Secret/disclosure types redact String/GoString/errors and do not
implicitly serialize; trusted private file translation necessarily handles the raw secret. Bound its lifetime,
zero owned byte buffers where possible and keep LimitCORE/nondumpable; do not promise Go string/SSD erasure.

Success reply keysets (JSON member order irrelevant; duplicate/unknown/case/type/trailing/UTF8/depth refusal):

| Call | Exact successful shape and required checks |
| --- | --- |
| Exchange201 | authorization_secret, metadata; ACTION public ID equals metadata credential_id; state ACTIVE |
| Prepare201 | same shape, new ACTION ID/revision distinct from and greater than captured predecessor; PREPARED |
| Activate200 | metadata ONLY; exact prepared ID/revision; ACTIVE; new expiry replaces the pending expiry |
| Status200 | metadata ONLY; exact queried ID when ACTIVE/PREPARED; INACTIVE must be null ID, numeric0, null expiry |

Metadata keyset is credential_id,revision,state,expires_at. Parse revision losslessly as a canonical JSON integer
(no string, float, exponent, sign or leading zero),0 only for INACTIVE, otherwise1..9223372036854775807.
Credential IDs are hlac_+32lowerhex. ACTIVE/PREPARED require a valid UTC Instant string ending Z, year0001..9999,
0..6 fractional digits and no leap second/offset/rounding; normalize NEW local expiry to exact six-fraction UTC.
Include actual server-produced zero/millisecond/microsecond forms in adapter fixtures and paired compatibility
proof; never reuse the execution decoder’s mandatory six-fraction/LongString grammar. Local current expiry
equality refuses. The server independently authenticates current binding/expiry on every request; local metadata
is not owner permission. Do not infer a new issued_at from an expiry or serialize local monotonic clocks.

400/401/403/409/429/503 and missing keys/schema remain finite distinct failure classes, not fabricated INACTIVE.
Successful/mapped responses retain no-store and correlation; no body/error/header/secret logging. Bootstrap and
PREPARED are10min, ACTIVE30days; one ACTIVE+one prepared predecessor-CAS, no overlap. These are SERVER constraints,
not client-minted lifetimes. Client receives no owner JWT or server verifier key. No automatic re-enrollment.

## Exact private store, lease and CAS

Use only existing worker0700 directory. Files: active.json (existing exact ordered nine-field contract),
handoff.json (below, <=4096 UTF8 bytes/depth8), credential.lock (worker:worker0600, regular/single-link/zero/noACL).
One nonblocking exclusive flock on the pinned lock inode serializes each supported bootstrap/RunWorker lifetime;
CLOEXEC, protected ancestry/recheck before/after acquisition, no unlink/replacement/wait queue. Busy is unavailable.
The main worker holds it until its conversations and clients have joined. It is mutual exclusion, not authority.

handoff.json exact ordered canonical compact JSON keys:
version,server_id,owner_user_id,connector_instance_id,phase,active_credential_id,active_credential_revision,
active_sha256,predecessor_secret,predecessor_expires_at,candidate. Version observer-action-handoff/v1; binding equals protected settings. Nullable active
fields are ALL null together, otherwise exact hlac_ ID, positive LongString revision and64lowerhex SHA256 of
the canonical active.json bytes. Hash is private consistency only, never authentication or remote evidence.
candidate is null or ordered credential_id,revision,state,expires_at,authorization_secret; revision LongString,
expiry six-fraction UTC, exact public ID/secret equality, state PREPARED or ACTIVE. No arbitrary extension fields.

predecessor_secret/predecessor_expires_at are BOTH null for UNENROLLED/EXCHANGE_ATTEMPTED/READY and BOTH required
for PREPARE_ATTEMPTED/PREPARED/ACTIVATE_ATTEMPTED. Retain the exact old ACTION secret and its existing expiry only
after matching the old file/ID/revision/hash; expiry may later pass but is never invented or extended. Reconstruct
the old canonical nine-field ACTIVE record from binding, old ID/revision, these two fields and its fixed version/
state; require its SHA256==active_sha256 on every renewal-state load. Thus the old authenticated status query is
still possible after active.json became the successor. This does NOT create overlap grace; the server revokes old
authority atomically. Never log/copy this secret to backups/IPC/argv/environment; clear it only when READY durably
commits. Ambiguous READY fsync preserves the actual old/new state for reopen, not a promise of secure disk erasure.

| Phase | Active reference | Candidate | Allowed next step |
| --- | --- | --- | --- |
| UNENROLLED | all null; active.json absent | null | Setup exchange only |
| EXCHANGE_ATTEMPTED | all null | null or ACTIVE | Null: locked disclosure uncertainty; ACTIVE: status-only resolution/promotion |
| READY | exact existing active file | null | Execution credential reads; quiet due renewal |
| PREPARE_ATTEMPTED | exact old active file | null | Locked until operator investigation; no prepare retry or claims |
| PREPARED | exact old active file | PREPARED | Fresh new-then-old status; first activation only if exact new PREPARED/old ACTIVE |
| ACTIVATE_ATTEMPTED | exact old active reference | PREPARED or ACTIVE | Read-only new-then-old status; only exact new ACTIVE/old INACTIVE resolves |

UNENROLLED is provisioned explicitly by package setup with the exact binding before the worker-mode exchange;
missing store is NOT UNENROLLED. Existing active-only files are not adopted by this package lifecycle. Setup marks
EXCHANGE_ATTEMPTED durably BEFORE its one HTTP POST. A valid reply is persisted as candidate ACTIVE in that phase
before publishing active.json. With no candidate after loss/failure, do not re-exchange or invent a usable secret.

READY renewal records PREPARE_ATTEMPTED before its one POST. Persist the returned successor atomically as PREPARED
before recording ACTIVATE_ATTEMPTED and sending the one activation POST. On a successful activation reply, persist
candidate ACTIVE with its actual new expiry before active-file promotion. An expired/changed predecessor, missing
key, slot conflict, malformed/lost disclosure or file uncertainty blocks claims; it never selects another grant.

For promotion, verify the expected state/phase/binding/old active witness under the SAME lease; publish exact
active.json, then publish READY with its new ID/revision/hash and candidate=null. Crash between files is recognized
by the retained candidate plus old reference; it is not an automatic fallback to either secret. Read-only status
must prove the new credential ACTIVE and predecessor INACTIVE before recovery promotion (no predecessor for first
exchange). A contradictory or unavailable read remains blocked. A phase that never attempted activation may make
its first POST after the required fresh statuses; ACTIVATE_ATTEMPTED never repeats it, even if statuses show pending.
Status reads are sequential, not one atomic authority decision; later revocation still denies the next authenticated
request. Promoting a file never grants execution permission or weakens the server’s fresh credential checks.
No post-uncertainty Docker execution or supported fence clear follows from a credential status resolution.

Filesystem witnesses per retained phase (all other combinations refuse; no absence means fresh installation):

| Phase/candidate | Required active.json witness |
| --- | --- |
| UNENROLLED; EXCHANGE_ATTEMPTED with null candidate | Absent |
| EXCHANGE_ATTEMPTED with ACTIVE candidate | Absent OR exact canonical candidate ACTIVE record |
| READY | Exact current ID/revision/hash; candidate and predecessor fields null |
| PREPARE_ATTEMPTED; PREPARED; ACTIVATE_ATTEMPTED with PREPARED candidate | Exact reconstructed predecessor record/hash |
| ACTIVATE_ATTEMPTED with ACTIVE candidate | Exact predecessor OR exact canonical candidate ACTIVE record |

A renewal active-file absence/foreign value always refuses. Candidate ACTIVE must be durably recorded BEFORE
active rename. After a post-rename/pre-READY crash, retain/reconstruct the predecessor from handoff, authenticate
new THEN old exactly once, and finish READY only on the required positive pair and matching candidate/file.
If the active file already equals the verified successor, do not manufacture an old file or repeat activation.
No prior secret must be recoverable by ID/hash lookup: the exact bounded retained predecessor supplies it.

Every store transition validates the pinned worker-directory/file identities, exact owner/mode/link/ACL and old
canonical bytes/phase/reference immediately before publication. Stage one owned exclusive0600 temporary file,
write-all/fsync, atomic same-directory rename, then fsync parent. Check cancellation before and after; an ambiguous
post-rename/fsync error poisons this owning lifecycle and preserves actual retained state for reopen/status, never
claims rollback. No automatic cleanup of unknown files, whole directory, local intent or central lane. Compare+
rename is not atomic against hostile same-UID/root interference; the lease serializes supported writers only.

## Consumed quiet-loop integration

RunWorker acquires the lifecycle lease and validates/positively resolves state BEFORE constructing its execution
credential provider; no raw ActiveCredentials fallback. Keep the existing narrow protected reader available for
its existing callers/tests, but the runnable path wraps it with READY/hash/binding/expiry admission. UNENROLLED,
attempted-without-secret, pending/ambiguous or corrupt/missing handoff refuses execution. Constructor/metadata
checks alone are not server authentication. Setup and RunWorker never share a live writer.

Before each new BEGIN, when the previous conversation/report actually joined, sample current UTC and begin renewal
when expiry-now<=7days. Fresh successful activation/promotion can supply the next conversation; no parallel timer,
mid-lease rotation, restart, replay or new clock allowance. Recheck after wake; server expiry remains authoritative.
Keep the SAME conversationClient and its process-local disabled latch: successful renewal/status must never clear
an uncertainty latch. Failed lifecycle work returns finite credential-recovery-required and stops new claims.
No generic callbacks grant execution; existing broker/current-check/native/COMMIT/Write budgets stay unchanged.

## Implementation ownership and meaningful proof

One coherent source PR, not a separate unused transport/store release:

- `internal/actionremote/credential_lifecycle.go` +tests: typed purpose-bound lifecycle client; only private shared
  HTTP plumbing adjustments if necessary, preserving all existing delivery tests and no-retry semantics.
- `internal/actionworker/credential_lifecycle{,_linux,_other}.go` +tests: closed codecs/store/lease/phase-CAS,
  quiet renewal and bootstrap consumption; bounded existing protected-file helpers reused privately.
- `internal/actionworker/run_linux.go` +tests: mandatory lifecycle in actual RunWorker/loop; no reset of the
  existing conversation latch, no broker/Engine/journal import or optional runtime bypass.
- `cmd/observer-action-setup/main_{linux,other}.go` +Linux tests: only exchange mode now, fixed role/environment/
  stdin/deadline/cancellation; unsupported platforms refuse. Root/package modes follow cut2, not stub success.
- Owning profile/contracts/tasks document these new finite records/modes; no new domain event/SQL schema/module,
  public owner API, observation change, workflow tier, dependency or installer is implemented in cut1.

| Actual injected boundary | Required observation |
| --- | --- |
| Attempt-marker write/fsync fails | Zero lifecycle POST and zero action claim |
| Marker durable, before POST / response lost after server success | Restart cannot repeat exchange/prepare; no secret fabrication or claim |
| Disclosure validation or candidate durability fails | No activation; no execution; last actual phase/evidence retained |
| PREPARED durable, before activation marker / marker fails | Zero activation until durable attempt; exact first activation only, never a new prepare |
| Activation committed, response lost | New then old status exact; promote only new ACTIVE/old INACTIVE; no second POST |
| New pending/old active, revoked/expired/stale instance/key503 | Preserve blocked handoff; never downgrade failure to INACTIVE/READY |
| Candidate-active durable, active rename/parent fsync/READY commit fails | Poison owner; reopen old/new witnesses, reconstruct predecessor hash/secret and exact new-then-old status; no claimed rollback or fallback |
| Duplicate supported processes / changed inode/ACL/link/phase | Nonblocking lease/CAS refusal; no foreign cleanup or secret exposure |
| Conversation/report still owns work when renewal becomes due | No prepare/activation before actual join; no claims during pending handoff |
| Successful renewal after prior uncertain execution | Original disabled latch stays set; local/central target evidence unchanged |
| Withheld EOF, exact5s/30s boundaries, cancellation during capture, suspend/discontinuity | No late accepted input/HTTP/state/READY or claim; no new clock, reader/monitor goroutine, hard-return claim or retry |

Use ordinary-user private files/SQLite-independent state and synthetic TLS fixtures, deterministic cancellation/
fault seams and original resource/time limits. Exercise the ACTUAL worker-loop consumer, not only helpers. Preserve
the existing full delivery/profile/IPC tests and run focused Linux race/Windows unit/vet serially. No skipped new
proof, new full context, Docker/VM/host identities or real credential. Tests must distinguish statement/write
failure from an actually completed rename/remote transition; raw secret canaries must be absent from every sink.
Paired real HTTP codec interoperability and full installed artifact/role/renewal/Engine acceptance remain package
gates. Existing server APIs are evidence of a contract, not proof this new client already interoperates.

### Local source acceptance checkpoint (2026-11-05 UTC)

The actual RunWorker now holds the credential lease and uses the READY/hash/expiry provider without an old-reader
fallback. It performs sequential lifecycle maintenance before BEGIN, after the preceding conversation/cancellation
callback/connection has joined; the same execution-disable latch remains. The third executable implements only
the closed worker-role exchange mode, not package management, broker provisioning or an installed service.

Local synthetic TLS and private-file tests cover numeric/time codecs, purpose separation, exact finite HTTP
categories/no retry, durable attempt ordering, separate-process flock exclusion, same-byte inode replacement,
all six renewal publication failures, positive new/old status resolution and actual post-active-rename restart.
The first expanded run exposed a test-only missing import (no worker test ran); it was corrected, not skipped.
Fresh full Linux race runs passed worker8.440s, remote2.396s, setup-command1.009s and worker-command1.010s; vet
passed those four packages. Source and final independent review are separate from these local receipts.
No actual Platform credential, service account, installed confinement, package artifact, VM or Docker operation
was used. Existing HTTP interoperability and exact-package gates above are deliberately not checked complete.
