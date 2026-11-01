# Dormant Linux action journal

Date: 2026-11-01. Status: accepted for unused Linux implementation only after exact-contract coordinator/independent review.
Decision: [ADR 028](../../docs/adr/028-dormant-linux-action-journal.md). The full broker in Spec 011/ADR 027,
authenticated recovery, installed isolation, credentials and all Engine mutation remain separately gated.

## Closed ports and private evidence

New package internal/actionjournal implements actioncore.Journal/Attempt plus Provision(ctx, directory, binding),
OpenExisting(ctx, directory, binding), Prune(ctx) and Close(). Binding is the exact server/installation/owner tuple;
the directory is already dedicated/private. Limits are code-owned, not parameters. Prune has no caller cutoff.
Other platforms return a fixed unsupported error before touching storage. No listener, command or installer imports it.
Close invalidates live handles/new operations, closes SQL before releasing the lifetime lock and is idempotent; it never
cancels/replays an Engine operation or clears evidence. A closed/poisoned handle cannot subsequently persist intent.

Extend actioncore.Key with fixed JournalMetadata containing Request plus approved EngineGeneration and ContainerID.
Keep existing ActionID, TargetID and Fingerprint fields and require equality with that metadata. Add a pure bounded
canonical-metadata validator/encoder shared with fingerprint construction. It validates structure, not authentication
or permission. Worker still proves request/binding/authority before Acquire. Preserve the exact existing version-1
fingerprint preimage/bytes; delivery lease IDs/deadlines, receipt times and claim timing remain excluded.

The canonical metadata is at most 2,048 bytes: existing scalar identifiers, operation, grace, revisions and immutable
UTC CreatedAt/ExpiresAt. Revisions retain full uint64 precision inside typed canonical metadata bytes, never an SQLite
INTEGER or float conversion. Persist no credentials, display names, raw errors, request text or Engine response bodies.
Load rejects unknown/noncanonical keys, malformed/bound-exceeding values, wrong digest and disagreement with indexed
identity columns. UTC RFC3339Nano bytes define stored time; never Go time ==, monotonic payload or pointer identity.

## Durable state and concurrency

| Evidence | Meaning and permitted next step |
| --- | --- |
| Admission only | Provably undispatched. Same identity may resume only through fresh Worker validation/authority/lease. |
| Dispatch intent, no terminal | OUTCOME_UNKNOWN forever until a later separately approved authenticated reconciliation workflow. Never replay. |
| Known terminal | Return the same result without another inspect/mutation. No terminal rewrite. |

Admission, intent and terminal changes are short transactions with complete expected identity/state comparison.
Acquire stores admission before returning a new attempt. Intent appends exactly once before Engine dispatch. Finish
before intent permits only matching ALREADY_RUNNING/ALREADY_STOPPED or TARGET_STATE_CONFLICT; after intent permits only
the operation's STARTED/STOPPED/RESTARTED acknowledgement. Errors never become raw persisted outcomes. A lost/failed
terminal commit leaves intent unknown unless reopening proves a complete terminal commit; no second Engine call.

Hold at most two permits across active undispatched handles plus durable unresolved intents, counting each once.
Serialize both opaque TargetID and (EngineGeneration, full ContainerID), preventing a second alias from bypassing the
physical fence. Retained terminal/unknown lookup precedes capacity/cooldown refusal. New excess work fails promptly;
no queue, goroutine backlog or lock held across authority/Engine I/O. Closing an undispatched handle frees only its
in-process reservation; its admission remains. Closing after intent never frees durable uncertainty. After a known
mutating terminal result, apply the proposed fixed 30-second physical/opaque-target cooldown; exact replay bypasses
it without dispatch. No cooldown is invented for non-mutating outcomes or expired-undispatched maintenance.

Expired admissions that have no intent and no live attempt may append DENIED with fixed internal reason
EXPIRED_UNDISPATCHED. This is provable non-dispatch, not clearing uncertainty. No first-slice API acknowledges,
deletes, releases or reconciles an unknown request/target/global permit, even after remote expiry/cancellation.

## Fixed files, schema and SQL envelope

Require trusted ancestry, current-owner directory 0700, regular single-link artifacts 0600 and no links/privileged
mode bits. Reuse ownerfs validation and reviewed Linux checks. Root/same-user hostile replacement is not an isolation
claim. Never chmod/chown existing entries. Fixed files: action.sqlite, .action-lock and action.sqlite-journal only;
bounded enumeration rejects extras, WAL/SHM, unsafe types/modes/ownership and excessive size. No sensitive error text
escapes: fixed unsafe/busy/full/recovery/unsupported sentinels only; existing core mappings remain finite.

Provision requires a new empty dedicated directory, exclusively creates lock/database, commits initial binding/floor
and syncs directory entries. Any partial artifacts refuse subsequent Provision. OpenExisting requires both files,
acquires lifetime nonblocking flock first and opens mode=rw, never initializes schema. Before SQL, check file/header
format, fixed application_id 0x484C414A, user_version 1 and 4 KiB rollback-mode format. SQLite may recover only a vetted
first-header journal envelope: empty/all-zero cold headers or exact hot magic, bounded record count (including SQLite's
-1 sentinel), original page count, 4 KiB pages and power-of-two 512..65,536-byte sector. Malformed nonzero/truncated hot
headers refuse before SQL. This is not full body authentication or bitrot certification; SQLite alone validates body
checksums/segments and performs recovery. Then validate exact schema, binding and all bounded logical records,
including impossible duplicate unresolved opaque/physical fences. No migration, salvage,
quarantine, implicit recreation or data adoption. Do not open/close a raw DB handle while SQLite owns a connection.

Use three STRICT tables: singleton metadata (binding and pruned_expiry_through), immutable actions (key, canonical
metadata/digest, indexed target/physical binding and admitted UTC time), and append-only events (action ID, fixed ordinal
1 intent or 2 terminal, UTC time, closed code/reason). Reserve at most two event slots for every one of at most 1,000
actions. Events cannot outlive their action or overwrite earlier evidence. Freeze exact DDL/indexes/constraints in TDD;
refuse extra tables/indexes/triggers/views, invalid relation/cardinality, terminal/op mismatch and invalid time ordering.

One dedicated connection, private cache, BEGIN IMMEDIATE, DELETE, synchronous EXTRA, page_size 4096,
max_page_count 3072 (12 MiB), cache_spill OFF, cache covering the bounded DB, temp_store MEMORY, trusted_schema OFF,
foreign_keys ON, mmap_size 0 and busy_timeout 250 ms; verify effective values on each open. Use explicit context-bound
COMMIT and independent bounded rollback/disposal; uncertain commit poisons the instance, and reopening must revalidate.
Short operations use caller deadline capped at two seconds, cleanup separately two seconds. Kernel/device stalls are
not forcibly interruptible. No ATTACH, savepoints, vacuum, backup, bulk import or SQL chosen by callers.

Revalidation reads at most 1,000 actions and 2,000 events through two fixed ordered query streams, including the
related-target history variant used for admission. Bound raw fields in SQL before materialization (TEXT uses byte
length after CAST AS BLOB, never character-count length) and share the same
pure metadata/event validators with single-action reads. Query count must not grow per retained record. Preserve all
canonical bytes, digest/index/binding, event ordering/operation/expiry, orphan and unresolved-fence checks; batching
does not relax the two-second operation deadline or the schema and storage limits.

## Capacity, retention and time floor

Cap combined logical DB+journal bytes at 32 MiB; lock is zero length. Never equate journal_size_limit with peak growth.
With at most 3,072 pages, no spill/savepoints and one main DB, the candidate bound budgets the 12,582,912-byte DB plus
3,072 journal page images with eight-byte overhead and two 65,536-byte alignment/header allowances: 25,321,472 bytes.
Pinned SQLite's MAX_SECTOR_SIZE is 65,536. Require both the derived worst-case bound and before/after-COMMIT samples
of the maximal-schema state; these samples are not an exhaustive intermediate-peak measurement. If the schema/update
pattern violates the derivation's assumptions or the hard ceiling, revise the contract before acceptance rather than loosen tests.

At admission reserve logical room for both future event slots. Complete all 1,000 maximum-sized actions under the
fixed DB cap in tests. This is NOT physical free-space reservation: external exhaustion/flush failure can still prevent
Finish, requiring retained unknown intent. Refuse record 1,001 if no retention-eligible terminal record can be removed.
Never evict recent terminal, admission/intent uncertainty or live attempts to admit new work.

Prune at most 50 eligible requests per transaction, with their at-most-two events. Delete only known terminal records
whose terminal time AND immutable authenticated ExpiresAt are <= current UTC minus 30 days. Atomically advance
pruned_expiry_through to the maximum committed cutoff. Missing-key admission additionally requires ExpiresAt strictly
above that floor and normal five-minute request validity; retained known/unknown lookup never creates a new attempt.
The expiry comparison prevents pruned-key resurrection after clock rollback. Large forward jumps can conservatively
block new admissions; there is no floor reset/recreate API. An older externally restored valid DB is not detectable
by this local mechanism; authenticated recovery remains a later gate.

## Audit scope, rollback and required proof

This records admitted action/intent/outcome evidence only, not all authentication/grant denials before Acquire or an
unbounded audit stream during storage failure. Fixed audit timestamps/results are not a forensic tamper guarantee.
No public audit/export endpoint is added. Removing unused code leaves evidence intact; future rollback disables new
work and preserves compatible readers/unknown fences, not automatic old-state adoption.

- TDD canonical identity/digest and full uint64/UTC round trips; exact replay, conflicts, detached metadata and SQL CAS.
- Real Linux provision/open, ownership/links/ACL-effective privacy, exclusive-process lock and foreign/corrupt/partial refusal.
- Two active/unknown permits, opaque/physical target serialization, cooldown and no new-key bypass after Close/restart/expiry.
- Kill subprocesses before/after intent and terminal commit; retained evidence produces no second synthetic Engine call.
- Inject before/after-commit failure and cancellation; distinguish uncertainty even when a transaction actually committed.
- Fill all 1,000 maximal requests/events; observe DB+journal peaks, rejected 1,001, retained lookup at full capacity,
  original-expiry/floor boundaries, backward/forward clock cases and no unknown pruning. Prune does not erase recent audit.
- Crosscompile Linux CGO-disabled tests and execute only a test-owned scratch image: no network/socket/host mounts,
  read-only root, no capabilities, two CPUs, 256 MiB RAM/no extra swap and bounded private tmpfs. Kernel ENOSPC tests fill
  only that fixture's bounded file and prove no dispatch on admission/intent failure and unknown on terminal failure.
  No root/production-server action is implied. Extend existing pressure CI narrowly; preserve its resource/time bounds.
- Full Go tests/vet, Linux race CI, contracts and independent code/security review. Static native execution is not race
  proof; process death/tmpfs/ENOSPC is not physical power-loss or dishonest-device fsync proof. Final independent acceptance
  and current-head CI remain separate from local evidence below.

## Local implementation evidence (2026-11-01; not runtime acceptance)

The specs were committed before code. Initial core-metadata/journal contract tests failed before implementation.
The unchanged version-1 fingerprint has an independently frozen original-preimage golden; typed canonical metadata
round-trips full uint64 revisions and rejects alternate UTC bytes/duplicates/unknowns. Native tests exercise one-shot
intent, stale/closed handles, full expected state, two retained unknown permits, physical/opaque fences, cooldown,
cross-process exclusivity, corrupt/foreign refusal and Worker+journal synthetic no-second-mutation behavior.

All 1,000 maximum-schema requests complete both reserved event slots, reopen, retain results at full capacity and
refuse record 1,001. Maximum before/after-COMMIT DB+journal sample is 1,528,440 bytes; sampling does NOT observe every
intermediate COMMIT instant or prove a measured 12 MiB database peak. The separate conservative 25,321,472-byte derived
ceiling depends on the documented page/no-spill/no-savepoint/single-DB restrictions. Logical SQLITE_FULL is tested
separately from actual private-tmpfs kernel ENOSPC at admission/intent/terminal phases.

Subprocess death before/after intent/terminal commit, genuine SQLite hot-journal rollback, commit uncertainty and
atomic prune/floor recovery pass. The hot-format fixture deliberately enables cache spill only in its child to produce
a real hot journal; production always verifies spill OFF. Root review found and TDD reproduced a retention-clock bug:
deletion eligibility now uses actual now-minus-30-days, while the persisted floor advances separately. Header/collision
refusal regressions likewise preceded their corrections. SQLite's [rollback format](https://www.sqlite.org/fileformat2.html#the_rollback_journal)
defines the bounded header fields and leaves body recovery to SQLite.

Native proof uses only a new FROM scratch test image and private 64 MiB tmpfs: network none, read-only root, UID65532,
no capabilities/socket/host mounts, two CPUs, 256 MiB RAM/no extra swap and 90-second test deadline. Existing pressure CI
adds that complete package three times with explicit ENOSPC opt-in; normal Linux race coverage remains in runtime CI.
Local ordinary-user Linux race, complete Windows Go tests/vet, Node22 contracts (80), workflow guards (10), Foundation43
and workflow syntax/context checks passed. Exact final code review and current-head CI remain required. No physical
power-loss, installed broker, authenticated authority, Engine adapter or live mutation is claimed.

## CI revalidation correction (2026-11-01; local proof, current-head CI pending)

PR 161's Linux race job failed the maximal-schema reopen, without a reported data race. The original loader issued
one ID query plus three queries per action (3,001 at capacity), within the unchanged two-second open deadline.
Replace that N+1 pattern with the bounded streams above, first reproducing statement-count growth on real SQLite
through test-only driver instrumentation. The deterministic regression reproduced 3,001 queries before correction;
all-record, single-record and related-target loads now use exactly two streams, and maintenance uses four queries
including its binding/floor check. Shared pure validators retain the same canonical metadata and event decisions.
Actual SQLite corruption tests cover all three scopes, orphan events and pre-scan raw-field refusal, including
NUL-tail/multibyte TEXT values that defeat SQLite character-count length but not the byte-count projection.

The unchanged maximal-schema reopen/prune passes with the two-second operation deadline. Final local Linux race
passes (actioncore 1.021 s, actionjournal 43.699 s), as does focused vet; optional kernel-pressure fixtures are proved
separately, not by their normal-race skips. All 23 native package tests pass three times in the same bounded scratch
fixture with pressure enabled, including ENOSPC, hot recovery and crash/commit cases. Complete Windows Go tests/vet
also pass. These are local source checks, not successful current-head CI or authorization to activate actions.
