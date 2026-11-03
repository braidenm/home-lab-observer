# Tasks

**Status:** Local consumed ADR030 paired-runtime design accepted for implementation/tests only. Unchecked items
are future work, not delivered capabilities; installation/production activation and broader Spec011 remain gated.

- [x] Record owner's accepted first action set: independently granted start, stop and restart.
- [x] Draft proposed scope, safety semantics, boundaries and primary-source research.
- [x] Draft coordinated finite-grace/immutable-target/unknown-dispatch proposal in
  [ADR 027](../../docs/adr/027-bounded-action-dispatch-and-uncertainty.md); no implementation or activation.
- [x] Review and implement the [unused typed coordinator](worker-core.md) with synthetic ports;
  this does not complete durable storage, a Docker adapter or installed action authority.
- [ ] Independently review/accept that proposal and freeze parameter codecs, authenticated reconciliation and
  corresponding crash/deadline/rebinding tests before any executable action profile.
- [ ] Complete independent architecture/security and owner review of proposed decisions and numerical limits.
- [ ] Accept spec and create a new ADR without rewriting accepted read-only decisions.
- [ ] Freeze versioned action/grant/result/audit contracts, credential audiences and standalone-owner authentication.
- [ ] Specify remote freshness/revocation protocol and request replay-window semantics.
- [ ] Implement/test synthetic policy matrix and durable ledger with crash/storage/retention failure injection.
- [x] Approve the exact [dormant journal contract](action-journal.md)/ADR 028 and private core-metadata/fingerprint extension
  before production code; do not claim the full Spec 011 broker is accepted.
- [x] TDD/implement locally the complete Linux-only SQLite adapter with fixed Provision/OpenExisting, canonical audit metadata,
  two uncertainty-retaining permits, 1,000 records, 30-day retention/floor and no unknown-clear method.
- [x] Local proof: derived DB+journal ceiling plus honestly bounded before/after samples, maximal terminal completion,
  actual private-fixture ENOSPC, commit uncertainty, hot/subprocess crash recovery and no replay/new-key bypass.
- [x] Obtain exact final journal root/independent code review and fresh CI: PR161 merged as
  cb5331dc6a5efbf0ed55d94490bcdad64031ecaa after all18 checks/all10 workflows passed. No action activation follows.
- [ ] Prove unknown outcomes cannot auto-retry, unlock silently or lose their dispatch audit.
- [ ] Implement/test separate broker process, protected state and fixed local Docker adapter.
- [x] Obtain exact root/independent review of [fixed Engine contract](engine-adapter.md)/ADR029 for unused Linux
  implementation only; journal all-green merge is complete. Actual native mutations remain separately gated.
- [x] Locally TDD the unused Linux adapter's fixed1.45 HTTP, full-ID/selected-field/witness checks, bounds and no-retry
  faults; preserve observation APIs/dependencies and prove Worker+real-journal replay safety. Native execution is separate.
- [x] Obtain exact final adapter/native-harness root/independent code review before real fixture execution; fresh PR CI
  and publication remain separate delivery gates.
- [x] Separately approve/review then execute at most two sequential exact image/full-ID/label-owned native fixture
  modes (graceful and ignored signal), one at a time with fixed caps and verified cleanup between; record actual
  tested Docker29.1.3/API1.45 evidence without touching any other workload. Both exact fixtures independently absent
  after acknowledged removal; see [bounded proof and limitations](engine-adapter.md#supervised-disposable-linux-proof).
- [x] Prove distinct start/stop/restart, graceful/forced stop and retained replay without another restart on disposable
  native fixtures. External-state checks remain non-atomic as documented in ADR027; this is not remote authorization,
  installed confinement, a release/canary gate or cross-platform parity.
- [x] Exercise controlled external-state races on owned native fixtures: three assertions over two sequential fixtures
  passed on reviewed source593f0999. Actual non-204 refusal is proven, not an exact HTTP304 or atomic state change.
  - [x] Root reviewed/froze the separate test-only external-state profile in `engine-adapter.md`; spec-first
    commit `0b2825324283cba1b2138380147cd489758ad2df` precedes test code. Native execution remains separately gated.
  - [x] Add the closed harness profile and real core/journal/Engine checkpoint cases: stale no-op, Stop race and
    Start race; preserve both immutable fixture modes, ownership/caps, existing timeouts and real cooldown.
  - [x] Synthetic Unix tests prove finite ordering/counters, retained replay after reopening the same journal and
    new-key UNKNOWN fence; ordinary Linux race tests and 18 Node policy cases pass. This is not native Docker proof.
  - [x] Obtain exact source/harness root/independent approval before separately authorized native execution;
    record three race assertions over two sequential fixtures plus independently verified exact-ID cleanup.
    See [actual evidence and limitations](engine-adapter.md#actual-external-state-race-evidence); installed worker,
    remote permission, application health, release activation and current-head delivery are not asserted.
- [x] PR163 (`5fd9e335`) delivered three controlled external-state assertions on two owned sequential native fixtures,
  with independent exact cleanup; actual non204 uncertainty, not an exact304 or atomic-state-change claim. No rerun
  or action activation is authorized by this completed acceptance evidence.
- [ ] Integrate explicit enrollment, grant/revoke and execution transport without observation-credential reuse.
- [ ] Add accessible permission-filtered UI and independent responsive UX review.
- [ ] Document enable/disable/rotate/upgrade/recover/remove and known authority/health limitations.
- [ ] Complete requirement-to-test traceability, independent code/security review and release activation evidence.

The repository's connected observation work remains ahead of this implementation. Kubernetes rollout actions require a
separate future spec. No checklist completion here authorizes manipulating existing host containers.

## Proposed complete packaged-worker delivery

- [ ] Review/freeze [root admin codec](admin-recovery-ipc.md) and [journal v2 exact DDL/CAS](action-journal-v2.md)
  with paired Platform vectors/authority before runtime TDD; preserve Proposed ADR027, accepted ADR028/029 and v1 evidence.
- [ ] Implement consumed v2 admission/full16 binding, dedicated retained lookup and four-origin immutable ACK
  transaction; test equal/changed replay, reserved-slot/fence counting, restart and no old-action redispatch.
- [ ] Prove three bounded load streams and malformed/NUL/orphan/version refusal; prune50 plus ACK/expiry/terminal
  retention/floor rollback cases; keep two-second SQL bounds and no mapping-history fallback.
- [ ] TDD policy allocation after highest mapping is pruned: head stays monotonic, next fresh mapping cannot reuse
  its revision; cover empty publication, unchanged reuse, multi-record canonical order and atomic LongMAX refusal.
- [ ] Prove exact updated1000/2000/1000 max packing including remote32B digest, actual private ENOSPC, hot recovery,
  lost/late COMMIT and every localACK/remoteHELD crash boundary without raising limits or claiming power-loss proof.

- [ ] Review/accept ADR030 and [packaged-worker.md](packaged-worker.md), including trusted-worker residual and rare
  root-plus-remote-owner reconciliation; freeze paired remote contracts before implementation.
- [ ] Freeze exact IPC vectors, local approval storage, journal acknowledgement schema/CAS/size/rollback contract,
  compatibility manifest and installed identity/filesystem/network profile; no signed-ticket framework or generic proxy.
- [ ] First small consumed PR: runnable worker/broker against isolated real Platform protocol, actual TLS current checks
  at both core checkpoints, separate root-admin ingress, retained lookup and owner acknowledgement using existing
  core/journal/Engine; prove authority/revocation, IPC, crash/storage/result ambiguity and recovery on owned targets.
- [ ] Second small consumed PR: installed enrollment/ADR052 renewal/profile/lifecycle bundle; prove disposable VM
  identity/isolation, credential/policy drift, bounded stop/join, upgrade/rollback and unchanged observation. Both PRs
  and paired Platform prerequisites pass before production activation; no unused-helper or giant all-feature PR.
- [ ] Finish exact-source independent review, normal CI and release evidence; install disabled and explicitly gate
  one disposable target before any existing workload or UI action enablement. No draft checklist authorizes execution.

### Local implementation checkpoint (2026-11-02; not delivered or activated)

ADR030's local paired-runtime decision and [ADR031's concrete non-root process profile](process-profile.md) have
root and independent source approval. The installed identity, host-namespace bind, effective service confinement,
reboot ordering and release gates remain open; accepting these documents does not satisfy those proofs.

The fixed-origin worker HTTP/codec component and protected settings/active-credential reader passed independent
source review and focused Linux race tests. The HTTP tests include lost-response/no-retry, claim ambiguity,
cancellation/join and wrong snapshot-CAS revision refusal. Native file tests cover bounded reads, exact file
properties, ACL/link/FIFO refusal and descriptor cleanup using private synthetic roots. They do not prove actual
installed role/namespace admission or deterministically replace a pathname during the read.

Broker/control, execution-commit guards and the fixed-role profile passed independent component review. The
profile's foreign primary-group gap has three corrected regressions; the broker-returned BindExecution journal
rejects acquisition without its original execution context. Mandatory protected-source tripwires now surround
authority, Engine, journal and retained-report boundaries, with real temporary SQLite mapping-retention tests.
A native reader test exposed a same-size in-place rewrite missed by coarse stat timestamps: the unchanged test
failed before a bounded second-pread comparison fix, then passed ten race-enabled runs. Both buffers remain
bounded and metadata/path/ACL checks remain required; this is not an atomic root-publication guarantee.

After local environment recovery, the full native broker race suite passed on these sources (2.628s), as did
native vet. Root independently reported the seven paired-package race suite plus journal/Engine race and vet
passing. These use private synthetic Unix/SQLite/TLS/Engine fixtures, not new native Docker effects or installed
role/namespace proof. The initial claim-receive processing/transfer distinction above remains a source-first
implementation task. None of these receipts completes the runnable paired worker, real API permissions,
packaged lifecycle, actual storage fault/maximum-packing acceptance or new disposable-container acceptance.

The worker now composes its protected settings/active credential with the actual fixed-origin HTTP client and
one sequential authenticated broker conversation. The same execution-disable latch survives paced reconnects;
unsafe profile/peer drift terminates while an absent/refused broker retries at a fixed cancellation-aware pace.
Protected ancestor descriptors, socket witnesses, ACL checks and kernel peer admission surround every dial,
before any greeting. Cancel closes and joins the active conversation before closing the HTTP client. Independent
four-file source review and the native worker race suite passed (1.680s); a later test-only ordering correction
removed a cancellation scheduling race in the fixture and passed ten focused race runs (1.286s).

The worker recovery consumer uses only the five fixed typed remote operations and the accepted root-initiated
query graphs. Non-STATUS recovery permanently latches execution off; lost finalization permits only lookup,
not mutation retry. Full receipt/binding/nonce/ordinal checks and cumulative native 90s/per-relay 5s limits remain.
Independent three-file review and native race tests passed (1.589s), including denial, changed binding, canceled
remote work, exact timing equality, suspend discontinuity and bounded query closure. These are component proofs:
the broker still must retain its original admin-operation budget across quiescence and reconnects, and actual
broker command entry point, root-admin orchestration and real paired API/Engine acceptance remain unfinished.

The fixed worker command consumes RunWorker, admits no arguments or transport/trust/credential environment
overrides, and reuses the identical existing manager-environment guard without observation authority. Its launch
contract was committed before code; independent three-file review and native command/worker race tests passed
(1.009s/1.652s). An actual compiled native invalid-argument launch under an empty environment plus the required
safe GODEBUG returned22 with zero stdout/stderr bytes before profile access. This negative smoke is not installed
identity, action enrollment, release identity or two-process/API/Docker acceptance.

### Local recovery integration checkpoint (2026-11-03; not released)

The fixed root CLI and broker launcher now consume the reviewed role admission and closed commands. The CLI
requires a complete bounded receipt on stdin; it exposes neither a transport selector nor a generic command.
Actual native invalid-argument smoke tests refuse before profile/socket access. Independent source review,
native race tests and vet passed; these negative launch tests are not installed-profile acceptance.

The broker's actual root ingress now consumes one read-only STATUS through the worker's authenticated BEGIN and
the same single control capacity as snapshot publication. INIT/CONTINUE fence and join real non-relay owners,
but still refuse before proposal/ACK/finalization. Cleanup releases capacity only after actual handler completion;
canceling an operation or finishing its reply is not a join. Native budget/IPC/broker race tests passed with no
skips, as did vet and independent thirteen-file review. This is local implementation, not a delivered capability.

Consumed worker regressions also prove that an interrupted valid snapshot exchange retains only paced recovery,
and that malformed initial frames remain terminal in either latch state. The same original recovery receive budget
now permits bounded between-query ACK processing without widening frame transfer or HTTP limits. None of these
changes automatically resumes execution. Four-origin SQLite/root recovery, paired real API/Docker acceptance,
installed isolation/lifecycle, capacity/crash proofs and the stronger deployment reader floor remain open.
