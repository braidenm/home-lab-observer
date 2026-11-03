# Journal v2 capacity-only acceptance plan

2026-11-03. **Proposed test implementation plan; root and independent review before code.**
Consumes the accepted [v2 journal contract](action-journal-v2.md) and its existing paired broker. No new runtime
API, schema, codec, limit, migration, permission, installer, workflow, skip or production activation is proposed.
Physical ENOSPC and subprocess/crash acceptance are separate later cuts, not implied by this capacity proof.

## Exact scope and source boundary

Add one Linux test file, `internal/actionjournal/v2_capacity_linux_test.go`, with package-private deterministic
fixture helpers and two sequential insertion-order subcases. Preserve all production bytes and v1 test fixtures.
The existing `capacity_linux_test.go` uses v1 `Provision`, MaxUint64 revisions and no recovery slots; its evidence
remains valid for v1 only. This test uses `ProvisionV2`, the exact six source-owned schema definitions and the
actual v2 loader/open/lookup/CAS/prune paths. No temporary schema/PRAGMA weakening or unvalidated file copy.

Ordinary Linux execution uses private `t.TempDir` directories and the existing Go test/race tier. No Docker,
mount, root account, real Engine, remote HTTP, credential, network or host-state operation is needed for this cut.
All actual journal operations retain their existing two-second context bound, 4096-byte pages, 3072-page ceiling,
DELETE/EXTRA, no-spill cache, one connection and 32MiB combined file cap. No timeout increase on failure.

## Maximum valid fixture, not arbitrary padded metadata

Build exactly 1000 unique ADMITTED actions, each with two valid original events (INTENT then RESTARTED), one
separate 32-byte remote receipt digest and one zero-filled RESERVED recovery slot. IDs, target and physical
bindings have their fixed canonical lengths and are distinct where needed; they do not create unresolved fences.
Use CONTAINER_RESTART/grace30, Owner==Actor, positive policy/capability revisions at LongMAX, request instants
with six nonzero fractional digits, and expiry exactly requested_at+300s. Canonical private metadata is produced
by the unchanged v1 codec and must pass the narrower v2 validator. Never substitute the v1 MaxUint64 fixture,
an invalid paired maximum vector, 2048 bytes of arbitrary metadata or synthetic trailing padding in metadata.

Local recorded/event/observation/ACK times use the longest allowed canonical spelling and valid chronology;
recorded_at precedes INTENT, which precedes terminal, which does not exceed ACK/observation. Request instants
remain exact microseconds, whereas local stored event/ACK timestamps may have nine fractional digits. Each
record is a valid retained known terminal; no expiry or new execution is invented by bulk setup.

Construct maximal semantically valid canonical ACKs for RETAINED_KNOWN_TERMINAL, RUNNING, full-length IDs/digests,
19-digit mapping and remote-lane revisions and all required nine-digit UTC timestamps. Use a representable held
lane below LongMAX so a hypothetical exact successor does not overflow. Derive the exact private evidence and
mapping digests for that record and revision, not all-zero stand-ins. The unchanged ACK encoder/decoder and
record validator must accept every payload. Independently assert the expected canonical byte length (the owning
contract's 988-byte maximum), exact SHA256, slot length2048 and zero-only remainder; do not hash the padding.
No claim is made that maximum local ACK fields authenticate an actual remote owner decision.

## Bounded setup versus actual API evidence

Setup is an explicitly labelled packing fixture, not 1000 executions or 1000 server-authorized recoveries.
Prepare deterministic canonical records in memory once. Insert actions/events/RESERVED slots using closed
parameterized SQL in fixed batches of at most16 records, each through the existing actual transaction helper
and a context capped at two seconds. Keep normal constraints/FKs/durability settings enabled. Fail a slow batch;
never retry, enlarge its deadline or bypass production validation. This avoids repeated growing global scans.

Use two fixed orders: lexicographically ascending IDs, and deterministic alternating low/high IDs. Fill ACKs
in reverse order to the corresponding inserts. These exercise different B-tree insertion/update histories,
not an exhaustive proof of every SQLite page allocation order. No random seed, permutation sweep or pool.

At RESERVED maximum, close and reopen with actual `OpenExistingV2`; require full strict three-stream validation,
1000 actions/2000 events/1000 slots, all ACK lengths zero and no orphan relation. Compare every loaded record
against its canonical fixture identity, metadata, private/remote digest and original two events. This is actual
loader/reopen evidence, distinct from direct fixture insertion.

Fill 999 slots using bounded fixture CAS updates with the exact RESERVED predicate and one affected row;
require each canonical payload, SHA256 and padding. Leave one deterministic sentinel (middle key) RESERVED.
Use actual `InspectEvidence` and guarded `CommitRecovery` for the sentinel, with a fresh original native recovery
timing context and AcknowledgedAt omitted. Match its expected actual sampled ACK bytes/digest, unchanged original
events and the final full-row counts. This is the real guarded API/CAS interpretation proof at maximum occupancy;
bulk fixture updates do not masquerade as that API proof. Existing before/after-COMMIT fault tests remain separate.

Close/reopen after all 1000 slots are filled. Validate all canonical ACKs and exact identities through the actual
loader, not just SQL counts. Actual `Lookup` on first/middle/last keys must return their known terminal and exact
ACK. A duplicate actual `CommitRecovery` for the sentinel must retain its original bytes and sampled time;
one changed ACK identity and one changed remote digest must refuse without replacing evidence or poisoning a
valid store. ACKed actions must not return a dispatch-capable Attempt. Exercise that existing `Acquire`
refusal on a separately reopened handle, then close it: its `ErrRecovery` intentionally poisons that handle.
Do not reuse it for later FULL/prune assertions or weaken the production poison behavior.

Before retention eligibility, actual admission of a fresh 1001st valid action and actual missing-record recovery
import must return FULL, not BUSY/identity conflict, success or poisoned storage. Give the fresh action distinct
opaque/physical identities, live valid chronology and no conflicting permit; use a correct MISSING_RECORD
template and timing context for import so an unrelated guard cannot masquerade as the capacity assertion.

## File-size samples, hard caps and retention

Use existing private beforeCommit/afterCommit hooks only to sample owned DB plus optional rollback-journal file
sizes and page_count at each fixture/API mutation boundary. Hooks introduce no permission and do not alter the
transaction result. Check actual private-file properties and query page_count only when the connection has no
open rows. Capture RESERVED maximum, filled maximum and reopen sizes; report finite counts, lengths and peaks.
Require DB<=12MiB/3072 pages and combined samples<=32MiB. Assert the separate existing conservative derivation
12MiB + (12MiB/4096)*(4096+8) + 2*65536 = 25,321,472 bytes below32MiB under unchanged no-spill/header assumptions.
Before/after samples are NOT exhaustive intermediate peaks or a replacement for that conditional derivation.

Advance only the private fixture clock to exact retention eligibility, after every original expiry, terminal
and actual ACK is at least30 days old. One real `Prune` must remove exactly50 actions, their100 events and50
slots, leaving950/1900/950. Reopen validates the remaining relations/ACKs. Roll the fixture clock back; the actual
retained floor must not decrease, and old pruned execution admission and missing recovery import must refuse.
No cutoff reset, cascade, VACUUM, unknown eviction, cap increase or external file deletion is permitted.

## Execution and review gates

First author the missing-capacity assertion as RED against absent v2 acceptance, then implement only this test
fixture/proof. Execute one targeted ordinary native run, targeted race, relevant journal regression and vet,
serially with existing GOMAXPROCS2/GOMEMLIMIT256MiB/offline toolchain controls. Record commands, source hashes,
finite counts/peaks, elapsed times and any conditional existing pressure skip honestly. No duplicate full-suite
repetitions merely to inflate evidence. If valid data cannot fit, or existing bounded CI cannot finish, stop and
report the measured cause; do not lower1000/2000/1000, remove checks, skip, widen time or modify production packing.

The later pressure cut reuses `.github/workflows/ledger-storage-pressure.yml` and its FROM-scratch UID65532,
network-none/read-only/no-capability/no-new-privileges, CPU2/memory256MiB/no-extra-swap/PID64, fixed private64MiB
`/fixture` tmpfs and unchanged90s test bound. Current ordinary runs must not enable HLO_ACTION_PRESSURE or fill a
host filesystem. Local container execution still needs separately frozen reviewed source/commands and explicit
approval. The native Engine orchestrator's clean-source guard is not a journal-pressure bypass: no Docker
fixture may be built from this shared dirty runtime tree by suppressing or weakening source admission. Use an
approved clean committed closure when that later cut is ready; retain all existing guards and failure evidence.

Capacity PASS alone does not close actual v2 physical ENOSPC/hot crash/prune fault tests, paired Platform
finalization/authorization, installed identity/isolation/lifecycle, release floor or disposable three-operation
acceptance. No worker, buttons, flags, live server or existing workload is enabled by this test plan.
