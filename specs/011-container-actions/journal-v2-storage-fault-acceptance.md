# Journal v2 storage-fault acceptance: two bounded cuts

2026-11-03. **Proposed test-only plan; independent/root review before either implementation cut.**
Consumes [journal v2](action-journal-v2.md) and the separately reviewed
[capacity proof](journal-v2-capacity-acceptance.md). No production source, schema, cap, permission, migration,
dependency, installer, container profile, workflow, timeout or activation change. Capacity PASS does not establish
physical ENOSPC, process-death recovery or durable server finalization. Author/approve cut A before cut B.

## Cut A — actual v2 physical ENOSPC

Files: new `internal/actionjournal/v2_pressure_linux_test.go` with a package-private v2 fixture runner, plus one
explicit invocation from the EXISTING `TestPrivateTmpfsENOSPC` in `pressure_linux_test.go`, after its unchanged
v1 cases. Keep that entry point's existing HLO_ACTION_PRESSURE gate and single conditional skip; add no new skip,
top-level opt-in test, alternate tier, dispatch or job. Ordinary runs compile the runner without filling storage.

The actual runner first rechecks TMPDIR==/fixture, actual TMPFS filesystem and mount size<=64MiB, the existing
fixed profile; it never accepts a caller path or a different filesystem. Use fresh sequential private subdirectories,
creator-exclusive0600 filler files, bounded4096-byte writes and an80MiB maximum write attempt. Establish the
actual kernel ENOSPC error, not SQL page exhaustion, logical FULL, quota inference or a fabricated error callback.
The filler shares this one bounded tmpfs with the journal. Never fill a host directory, sparse/truncate a file,
mount anything, use root or touch Docker/socket/network from the test binary.

Five sequential fresh v2 stages:

| Stage | State before fill | Actual operation while physically full | Required retained readback after bounded cleanup/reopen |
| --- | --- | --- | --- |
| admission | Fresh v2, no action/event/slot | Bound guarded execution Acquire | No partial action or reserved slot; no event/ACK |
| INTENT | Actual admitted action + RESERVED slot, no event | The same attempt's guarded Intent | Original admission/digests/slot preserved; no committed INTENT or invented UNKNOWN |
| terminal | Actual admission + committed INTENT + RESERVED slot | Guarded Finish(RESTARTED) | Original INTENT remains UNKNOWN, no false known terminal/ACK, same-target new action fenced |
| existing ACK | Actual INTENT, no active attempt, RESERVED slot, exact evidence/template | Guarded CommitRecovery | Original events/digests remain, slot still RESERVED, UNKNOWN/fence retained, no successful ACK digest |
| missing import | No local action; exact valid full-receipt/private binding and MISSING template | Guarded CommitRecovery import | No partially imported action/slot/event/ACK and no invented admission or dispatch |

Use the existing actual timing-kind/native-clock fixture semantics, not an optional raw production path or a
manufactured remote permission. Execution and recovery contexts are separate. INTENT/terminal reuse the ONE
original execution context from admission across filling; never recapture/reset T0. Admission may create its
first context after filling. Recovery captures one original operation stamp, then its fresh check/ACK sub-budget
after filling while retaining that original stamp. Filling consumes the original budget; if it expires before
the target write, fail the fixture rather than call a timing refusal ENOSPC. Confirm the baseline transaction
is complete with no leftover rollback journal before filling. A fixture context is timing correlation, never
owner authentication. Keep all production two-second SQL and
rollback/close bounds. Each otherwise valid input must pass its pure/binding checks, so an unrelated expiry,
wrong kind, malformed ACK or full logical store cannot masquerade as ENOSPC refusal.

Each failed actual write must return finite ErrRecovery, poison the open handle and expose no successful result
or ACK. Verify further mutation refuses on that handle. Close/join its owned attempt/store, then release only
the exact saved filler inode in its private fixture directory (revalidate regular type/owner/link/device/inode).
Do not delete/truncate/copy back a journal, database, lock, slot or evidence row to recover. Reopen with normal
`OpenExistingV2`, validate exact schema and all three load streams, compare immutable original bytes/digests/
events and the table counts above. No automatic retry of the failed mutation or Engine execution follows.

The successful test consumes every stage and removes only its filler/private scratch after stores are closed.
Any unmet condition fails at its finite named stage; never retry, silently release uncertainty or report PASS
from cleanup. Preserve command/source identity, bounded finite test output and failure phase in the external
execution receipt. Ephemeral tmpfs and normal test-directory cleanup are not retained database forensics; do not
promise failed database files survive container exit. If a process/owned cleanup cannot be joined, fail and retain
its ownership information for the reviewed orchestrator rather than proceeding to another fixture.

### Build/run admission — unchanged closed pressure profile

Use `.github/workflows/ledger-storage-pressure.yml` and `scripts/fixtures/action-journal/Dockerfile` unchanged:
CGO_ENABLED=0, FROM scratch, UID/GID65532, network none, read-only root, no capabilities, no-new-privileges,
CPU2/memory256MiB/no-extra-swap/PID64 and only private64MiB /fixture tmpfs. No socket, host mount or network,
credential, proxy, registry login or inherited production configuration. Keep its existing `-test.run=^Test`,
`-test.count=3`, `-test.timeout=90s` and job bound; no race binary inside that pressure profile.

Before any local image build/run, separately freeze/review the exact executable source closure and commands on
a CLEAN committed checkout containing the consumed runtime and these tests. Verify pinned Go1.27.3, commit,
all raw closure hashes and empty tracked/untracked status before and after compilation; no dirty-source marker,
force option or weakened guard. Pin the resulting test binary/image identity and the exact owned container scope.
The current shared working tree or source-only GO is not build/run authority. Root approves one isolated run
and its validated cleanup separately. No container creation or host operation is authorized by this plan.

## Cut B — v2 process crash, hot recovery, prune and load relations

Files: new `v2_crash_linux_test.go`, `v2_hot_linux_test.go`, `v2_load_linux_test.go`, and focused additions to
existing `v2_prune_linux_test.go`, all under `internal/actionjournal`. If needed, extract only the existing
test-only native execution-context construction from `v2_execution_linux_test.go` into a private helper used
by its old fixture and the crash runner; preserve its assertions and semantics, review that exact delta.
No production fault hook/SQL callback, generic child runner, new runtime API or altered v1 fixture is added.

### Actual subprocess boundaries

Use this same test binary only, a closed child-mode enum and parent-created private synthetic v2 fixture.
No shell, arbitrary executable, child HTTP/Engine call, unbounded output or inherited credential environment.
Reuse existing child8s/parent10s bounds, GOMAXPROCS2 and original fixture TMPDIR. A child uses guarded execution
or recovery timing as appropriate; reconstruction is only from the fixed synthetic test contract, not an
inferred production full receipt. The selected private beforeCommit/afterCommit hook exits23 without running
deferred storage cleanup. Earlier fixture/setup failures have distinct finite non23 outcomes.
Parent actually waits/joins that one process, checks its exact intended boundary exit and normal retained reopen.
Timeout/unjoined child fails the case; no later child starts, no broad process kill or automatic mutation retry.

Run before/after actual COMMIT for admission, INTENT, terminal, existing-intent ACK and missing-record import
(ten small child cases). Compare actual reopened states, not an exception message alone:

- Admission: old empty versus complete ADMITTED+RESERVED; never a partial relation.
- INTENT: retained no-event admission versus actual INTENT/UNKNOWN; latter still fences same-target new keys.
- Terminal: actual original INTENT/UNKNOWN versus actual known RESTARTED; never relabel a committed terminal UNKNOWN.
- Existing ACK: original immutable INTENT+RESERVED versus identical INTENT plus the exact immutable ACK; never
  invent/delete events, emit a digest from the interrupted caller or repeat execution. Remote finalization is absent.
- Missing import: empty versus complete RECOVERY_ONLY+ACK with zero events; never an empty admitted import.

Any synthetic effect counter is explicitly a fixture marker, not evidence of a real Docker effect or causality.
This proves abrupt process exit around real SQLite commits, not physical power loss, dishonest-device fsync,
arbitrary kernel I/O stalls or exactly-once Engine execution.

### Genuine v2 hot-journal recovery and prune atomicity

Prepare a small exact v2 fixture with100 valid ACKed known-terminal records/200 events/100 slots, then close it.
One child opens it, advances only its fixture clock to retention eligibility and invokes actual Prune. As in
the accepted v1 hot-format fixture ONLY, set child connection cache_size1/cache_spillON to establish a real hot
rollback journal; never change page size, max-page count, source schema or production no-spill configuration.
Before its precommit exit23, require the actual nonzero rollback-header magic from its own journal. Without that
proof the fixture fails; no fake header/copy/manual unlink. Parent joins it and uses normal `OpenExistingV2`,
which retains production settings and validated header handling. Require all100/200/100 original records/ACK
bytes and the old floor, no partial slot/event/action deletion. Label this recovery-format proof, not evidence
that the normal no-spill execution path ordinarily spills or survives physical power loss.

Add actual Prune beforeCommit-error and afterCommit-error cases with the existing private hooks. A failure must
poison the handle; after close/reopen the exact result is old100/200/100+old floor, or committed50/100/50+new
floor. No mixed deletion/floor result, cascade, vacuum or retry. Include active-admission exclusion and an
unACKed INTENT alongside eligible ACKed records; neither live ownership nor unresolved evidence may be pruned.
After owner close an undispatched expired admission may become the existing denied event, never fake INTENT.
Keep exact ACK/expiry/terminal30-day equality and clock-rollback tests already present. These hooks plus actual
hot rollback do not claim exhaustive physical failure injection between every individual SQL statement.

### Narrow v2 three-stream corruption matrix

Use fresh small synthetic fixtures per corruption, bounded mutations/reads and no source DDL change. Reuse the
existing v1 test pattern's fixture-only constraint/FK bypass solely where necessary to construct unreachable
corruption; restore it before actual API reads, never use it to admit a valid runtime record or production path.
Cover missing slot, orphan slot (whole-store/open scope), wrong32-byte remote binding on an ACKed record,
malformed ACK length/hash/canonical bytes, nonzero padding, invalid RECOVERY_ONLY origin/event/reserved relation,
and byte-length/NUL-tail malformed metadata or recorded_at. Match the corruption to the selected record for
single/related scopes; do not falsely require an unrelated orphan to be discovered by a bounded single lookup.
Actual all-record load and normal reopen must reject every global relation fault. No truncated/default valid
record, raw sensitive body, unbounded materialization or repair/adoption is allowed.

## Evidence and stop conditions

Freeze/review each cut before committing its exact tests or requesting execution. Ordinary targeted/race/vet
and existing required suites remain serial/resource bounded; no new skip/tier or timeout increase. Physical
ENOSPC runs only after its separate clean-source/binary/container admission. Keep source/command identity,
finite per-stage assertions, elapsed/exit/join/cleanup receipts and the original failure when a case refuses.
If the existing count3/90s profile cannot finish all real assertions, report measured cause for review; do not
filter out old tests, reduce repetitions, weaken assertions or widen resources/time. No reenactment of completed
native Engine races is needed. Neither cut authorizes release, installed-profile/lifecycle claims, real paired
server finalization, flags, owner buttons or any existing workload mutation.
