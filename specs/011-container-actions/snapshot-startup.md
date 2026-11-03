# Approval snapshot startup and pre-claim readiness

2026-11-02. Accepted for local paired-runtime implementation and tests after root/independent design review.
This is a narrow supplement to the typed action protocol, not installation, release or live-control approval.

## Missing composition edges

The worker must recover the current publication CAS head after restart without assuming that a lost POST failed.
The networkless broker must bracket a genuinely fresh authenticated server sample BEFORE issuing an execution
nonce. An old snapshot's advertised_at, HTTP Date, GET head, browser time or serialized monotonic timestamp
cannot supply that evidence. This supplement adds no authority ticket, durable worker CAS file or clock offset.

## Observational worker head read

GET /v1/connectors/home-lab/servers/{serverId}/container-actions/approvals returns the existing SnapshotView.
Require current ACTIVE ACTION credential, ACTIVE registration and exact server/current owner/installation binding.
This read is binding-only: no renewed positive human permission/account check and no execution permission.
It remains installed after admission is disabled. It performs NO publication, TTL refresh, pruning, retirement,
cancellation or other housekeeping. Invalid authentication has zero side effects.

A verified missing row returns UNAVAILABLE with null revision/digest/advertised_at/expires_at and empty entries;
the three binding fields remain exact and nonnull. Only that response permits expected_snapshot_revision=null.
An expired existing row retains all four head values and exposes no entries. Never translate a 404, timeout,
unparseable response or dependency error into missing/zero. An existing head belonging to another owner or
installation conflicts rather than being adopted. A concurrent change can still make the following POST conflict.

POST /approvals retains its existing fresh positive current-owner/account checks and admission/readiness gate.
The GET supplies only the CAS input. It cannot establish current permission or usable clock/readiness evidence.

## Two new closed worker control kinds

Envelope remains observer-action-local/v1, with version,kind,conversationNonce,ordinal,payload in existing order.
Existing messages/fields retain their encoding. Add exactly these two kinds; reject unknown kinds.

- SNAPSHOT_QUERY: broker to worker, fresh nonzero control nonce, ordinal "1". Payload has exactly, in order:
  server_id,owner_user_id,connector_instance_id,publication_id,entries. IDs and Entry objects use the HTTP
  snake-case grammar, canonical order and full-Long strings. publication_id equals the control nonce.
  Entries are the broker's complete canonical sorted approved set (0..32 operation entries), never full Docker IDs.
- SNAPSHOT_REPLY: worker to broker, same control nonce, ordinal "2". Payload exactly outcome,receipt.
  outcome is OK/DENIED/UNAVAILABLE/CONFLICT. OK requires the exact typed SnapshotReceipt; every other outcome
  requires explicit null receipt. No headers, bearer, URL, raw HTTP errors or caller-provided clock observations.

The worker makes one GET above, validates the full response/binding, then makes at most ONE POST with query's
publication_id and unchanged entries plus the fetched expected revision. No filtering, reordering, substitution
or partial publication. It returns the authenticated POST receipt unchanged. Fresh publication uses a fresh ID;
an idempotent old receipt is not a fresh clock sample. A CAS conflict or failed/ambiguous POST is a finite failure,
not same-cycle retry. A later independently initiated control cycle reads the actual head and uses another fresh ID.

## Branch-exclusive connection state

The existing worker-originated BEGIN with all-zero nonce, ordinal "1" and empty payload is a stateless greeting.
If fresh readiness, unchanged policy and an execution reservation are available, broker emits existing
APPROVAL_SNAPSHOT with a fresh DISTINCT execution nonce; existing execution sequencing follows.
Otherwise, when the policy/profile is valid but timing/publication readiness needs refresh, broker may emit
SNAPSHOT_QUERY instead. This selects CONTROL for that connection, allocates no execution permit and accepts
ONLY its matching SNAPSHOT_REPLY. Control completion or failure closes the connection; it can never upgrade
to execution. A NEW BEGIN independently rechecks readiness, policy and capacity. Execution never accepts a
control reply; the control nonce cannot be used in CLAIM or as an execution continuation.

Bound undecided plus selected connections to three total, including at most two execution conversations and
one control conversation. No waiting queue. Unsafe/missing policy or profile refuses before query/claim.
A saturated control slot refuses rather than allocating another slot. Root quiescence invalidates readiness
and pending controls; a reply arriving after it cannot install readiness.

## Native timing and immutable local policy correlation

One <=5-second BOOTTIME budget covers the WHOLE GET, single POST, IPC reply and validation, not five seconds per
leg. Broker privately captures original UTC/Go-monotonic/BOOT send and receive samples around this whole exchange.
Authenticate/correlate the reply, check exact publication ID and receipt fields, then recompute the existing
snapshot digest from the exact server/owner/installation, receipt revision/times and original query entries.
Require the complete advertised_at-to-local-send/receive bracket inside +/-4.9 seconds and existing <=100ms
wall/BOOT and BOOT/Go-monotonic continuity. Only a genuinely fresh successful POST can install clock readiness.

The private control slot binds the exact local policy head and canonical entry digest before sending; re-read
and compare both before installing readiness and again at NEW BEGIN. Changed/revoked policy or lifecycle state
invalidates it. Readiness also retains publication ID, snapshot head/digest and the native timing samples.
The existing <=15-second freshness at BEGIN and original execution budgets remain unchanged. No private mapping,
policy file, absolute monotonic clock or Engine identity crosses the channel.

## Required proof and rollback

Test verified missing, expired retained head, wrong owner/installation, restart, GET error, CAS conflict, lost POST
reply, changed policy during refresh, empty policy, all closed/oversized/duplicate/case inputs, wrong control nonce,
late reply/equality, suspend, quiescence and three-connection capacity. Prove ZERO claims before readiness, no
execution reservation for control, no control-to-execution upgrade and no same-cycle POST retry.
Keep all existing receipt/journal fingerprints unchanged. Exact new control maxima must stay within 16KiB.
No new domain event/schema is needed. The paired profile/source floor must contain both kinds and GET before
activation; old profile rollback is ineligible after activation. While unpublished/disabled, rollback removes
the unactivated runtime changes and leaves observation and durable action evidence untouched.

## Empty claim poll acknowledgement

Add one separate execution kind NO_CLAIM: worker to broker, ordinal "2", the exact execution conversation nonce
issued in APPROVAL_SNAPSHOT, payload exactly {}. Emit ONLY after the authenticated fixed-origin claim request
returns exact bodyless HTTP204. This is a trusted-worker assertion, not independent proof of server state.

Accept ONLY while that anonymous preclaim reservation is still owned and has not handed off to an action.
Release that in-process reservation once and close the connection. Do not create a journal row/fact, release
any central HELD lane, infer the absence of other server work or allow later execution on that connection.
Stale/duplicate/nonmatching messages, control-branch misuse and post-handoff NO_CLAIM refuse without release.
EOF, cancellation, timeout, HTTP5xx, malformed success, wrong nonce or lost claimed response NEVER mean NO_CLAIM;
retain CLAIM_RESPONSE_UNCONFIRMED and the existing stop/recovery policy. There is no automatic claim retry.
Tests must distinguish verified204 from a lost committed claim response, plus every reservation/branch case.

This brings the closed worker set to twelve kinds: original nine, two startup control kinds and NO_CLAIM.
Its full envelope is 138 UTF-8 bytes before the four-byte prefix when using the fixed v1 names/nonce/ordinal.
Existing CLAIMED_EXECUTE remains nonnull and byte-compatible; no nullable-claim/outcome widening is introduced.

## Evidence and alternatives

Reviewed 2026-11-02: [RFC9110 safe methods and conditional requests](https://www.rfc-editor.org/rfc/rfc9110.html)
supports an observational GET and protecting updates from stale state; the body revision CAS is this project's
existing choice, not an HTTP-mandated encoding. [Go1.27.3 monotonic clocks](https://pkg.go.dev/time@go1.27.3#hdr-Monotonic_Clocks)
explains why local monotonic samples cannot be serialized as portable timing evidence.
[PostgreSQL16 READ COMMITTED](https://www.postgresql.org/docs/16/transaction-iso.html) permits state to change
between GET and POST; the existing fenced POST must check its expected head after waits.

Retaining the incomplete startup handshake leaves controls unavailable. A durable worker head/pending-publication
file is viable but adds another crash/retention/permission boundary and still needs fresh broker timing. The selected
observational read plus two finite control kinds reuses the existing server CAS and native clock checks, at the
cost of two HTTP requests per refresh. A generic time/HTTP RPC or treating GET/old receipts as authority is rejected.
No additional product decision or host permission is needed; implementation, exact vectors, review and isolated
acceptance remain separate gates.
