# Profile-bound upload state and Linux ledger

Status: Pure state machine and unused Linux ledger implemented locally, 2026-10-29; exact-head CI and activation pending.
Decision: [ADR 024](../../docs/adr/024-immutable-rich-installation-profiles.md).
Build on the unused [combined envelope](delivery-envelope.md), not a second upload stream.

## First implementation boundary

Reuse `uploadstate` and `uploadledger`. The code-owned kinds are numeric (the zero/default value) and rich-processes
(explicit opt-in); reject unknown kinds. Do not accept caller-defined validators, SQL, DSNs, limits, groups or wire
schemas. Keep current `New`, `ValidateRecord`, `Provision` and `OpenExisting` strictly numeric. New explicitly named
profile-aware constructors/validation accept only those closed kinds and an immutable policy binding.

Before this change, extract only normalized container types/constants into `containerobs/model`, preserving public
`containerobs` aliases/helpers and exact JSON. Production workload/combined validation must not import Engine HTTP,
Docker acquisition or host collectors. The existing numeric-worker closure must remain strict; only audited pure
projection/model packages may be added when profile-aware state is actually introduced.

The rich-processes kind accepts only canonical combined envelopes with process consent enabled and container consent
disabled, including the canonical disabled container section. It uses existing numeric and workload validation,
source/version/cycle binding, quality semantics and privacy filters. A valid combined document with Docker enabled is
still refused by this installed kind. The wire envelope's general profile is not authorization for every group.

## Binding through the entire state machine

Introduce one small `ProfileBinding` value: closed kind plus a 32-byte policy-record digest. Numeric requires the zero
digest; rich requires a nonzero expected digest. The caller establishes trust in that expected value through the later
installed policy gate; a digest is not authentication. Add that value to logical records, defaulting to the numeric
zero value so old call sites remain numeric. Compare it even when no pending bytes exist.

`Machine` holds its immutable expected binding. Every Load, source admission, pending retry and Commit validates both
profile and policy binding. Default numeric record validation refuses rich records. Profile-aware validation checks
the selected wire schema, body cap, SHA-256, canonical collection time and server identity; no alternate profile can
be inferred from payload contents. Clone/CAS comparison includes the binding. Keep pure validators filesystem-free.

Retain the existing sequence/ack/expiry rules, bounded send deadline, detached immutable request bytes and uncertain
commit recovery. A pending request is persisted before first send and is replayed byte-for-byte with the same
sequence/digest after reopen. Watermark never decreases and an expired/acknowledged sequence is never reused.
Policy mismatch is recovery, not normal expiry, silent fallback to numeric or permission to drop/re-encode pending.

This slice does not load policy files, credentials or start workers. Later installed composition must check the
root-owned immutable record before credentials/network and before every retry; state/ledger tests alone do not
complete that authority boundary or the independent persisted receiver approval.

### Implemented unused pure API

`ProfileKind` has only `NumericProfile` (zero) and `RichProcessesProfile`; `ProfileBinding` contains `Kind` and
`PolicyDigest [32]byte`. `Record.Profile` is part of every logical expected/next value. The explicit APIs are
`NewForProfile(binding, profile, source, clock, transport, ledger)` and
`ValidateRecordForProfile(record, expectedBinding, expectedProfile)`. Existing APIs supply an exact zero numeric
binding. Rich maps to persisted server approval `RICH_PROCESSES_V1`; the old numeric wrapper remains its existing
legacy/native-compatible wire validator, not a claim that old registrations have a newly persisted server profile.
`LEGACY_COMPATIBLE` and `NUMERIC_HOST_V1` remain distinct server-side policies, not newly inferred Go kinds.

Each Step validates loaded/cached records before terminal/expiry decisions, then revalidates before Send. Every Commit
validates both bound records before passing detached values to the existing Ledger CAS interface. No existing installed
ledger or transport caller selects the rich kind; default numeric persistence and HTTP limits remain unchanged.

### Implemented unused Linux storage API

`ProvisionForProfile(ctx, directory, binding, profile)` and `OpenExistingForProfile(ctx, directory, binding, profile)`
select only the same closed code-owned kinds. Default `Provision`/`OpenExisting` supply zero numeric binding. Invalid
profiles are refused before filesystem mutation. Both reuse the existing adapter, lifetime lock, transaction and
recovery latch. Non-Linux profile-aware APIs return unsupported without touching storage. This is not installer,
enrollment, handoff, transport, ledger-witness or worker activation.

## Exact per-kind storage budgets

| Property | Numeric, unchanged | Rich-processes, unused library |
| --- | --- | --- |
| Pending-body ceiling | 16,384 bytes | 294,912 bytes (288 KiB) |
| Database schema/user_version | Exact existing v1 | Separate exact v2 |
| Page size / max page count | 4,096 / 64 | 4,096 / 128 |
| Database ceiling | 262,144 bytes | 524,288 bytes |
| DB + journal + lock envelope | 1,048,576 bytes | 2,097,152 bytes |
| Expected record profile/digest | Numeric / zero | Rich-processes / exact nonzero digest |

Use the existing application_id `1213156420`, fixed filenames and lifetime flock. Schema v2 is one STRICT singleton
table retaining v1 transition fields, with immutable profile-kind and 32-byte policy-digest columns and the rich body
constraint. Verify exact SQL and exactly one row, no user indexes/triggers/views. SQL-gate body length before scanning
it, then run the correct pure record validator. Read persisted binding rather than substituting caller expectations.

Before opening SQLite, under the existing lifetime lock, check application_id and user_version in the already checked
100-byte header against the requested kind. Numeric cannot open rich and rich cannot open numeric, including a valid
hot journal; refusal must not start SQLite recovery. There is no version conversion or reset. Once a matching kind is
admitted, preserve valid SQLite recovery. A later wrong server/connector/policy may fail after that recovery, so do not
claim all refused opens are physically byte-preserving. Logical state is never deliberately adopted or rebound.

Provision remains exclusive; OpenExisting never creates state. Full-record CAS rejects kind/digest mutation before a
transaction, including empty/pending/terminal states. Retain 0700 directories, 0600 single-link artifacts, checked
ancestors, closed entries, regular private journals, no WAL/SHM, one connection, BEGIN IMMEDIATE and independent bounded
rollback on uncertainty. No filesystem permission repair, journal deletion, queue/history, DDL migration or backup.

## Peak-bound argument and required proof

Retain DELETE/EXTRA, `cache_spill=OFF`, verified 4096-byte pages/max_page_count, cache covering the selected page budget,
one bounded row UPDATE per transaction, and no savepoints, ATTACH, VACUUM or extra write statements. The pinned SQLite
source caps sector/header alignment at 65,536 bytes. Under those restrictions, the existing conservative accounting
scaled to 128 pages is `524288 + 2*65536 + 128*(4096+8) = 1180672` bytes, below 2 MiB. This is a design argument, not
completed proof: check its journal-copy/header assumptions against the pinned implementation and actual rich fixtures.
`journal_size_limit` controls retained files after transactions, not the active peak; never use it as the justification.

The 288 KiB body cap is defensive. Individual host/workload caps and canonical fields may prevent constructing a valid
document exactly that size; process-only is smaller still. End-to-end tests must use the largest eligible canonical
process-only document. Separately exercise the SQL BLOB ceiling exactly at 294,912/294,913 bytes in isolated synthetic
fixtures, and distinguish SQL capacity from logical acceptance. Do not pad invalid JSON and call it a valid profile.
Measure DB/journal metadata during transactions without reopening the database while the SQL connection is alive.

## Acceptance before merging the unused library

- [x] TDD closed-kind/default-numeric constructors and record validation, including empty and terminal bindings;
  local proof only, independent implementation review and exact-head CI pending.
- [x] Preserve numeric schema SQL, fixtures, exact retry/sequence semantics, 64-page budget and unchanged default callers.
- [x] Rich process-only canonical roundtrip above 16 KiB; reject wrong group, source, digest, timestamp, unknown kind,
  numeric body in rich state, rich body in numeric state, malformed/oversized bytes and attempted profile/digest mutation.
- [x] Maximum-field, section-byte-budget rich fixture persists, closes/reopens, retries with identical bytes/sequence/digest and acknowledges;
  ack/expiry/restart never lowers watermark or reuses retired sequence. Include a pending maximum signed sequence.
- [x] Strict SQL bounds, exact v2 schema/profile metadata and allocation-before-validation negatives; no migration/adoption.
- [x] Cross-kind opens refuse before SQLite, with byte snapshots of DB and validated hot journal unchanged; same-kind
  hot-journal recovery remains owned by SQLite and restores complete old-or-new state.
- [x] Shared real SQLite tests cover full-record CAS/lock contention, detached records, cancellation/commit uncertainty,
  process death before/after commit, repeated maximum-profile transactions and measured total under 2 MiB.
- [x] Run the existing opt-in disposable ENOSPC fixture for the rich kind; document execution separately from ordinary
  unit failures. Physical power-loss and packaged installed acceptance remain later explicit VM release gates.
- [ ] Native Windows full Go tests/vet and exact-head Linux/race CI; run real Linux ledger tests rather than treating
  non-Linux unsupported stubs as persistence proof. Existing Node numeric/workload/combined fixtures stay unchanged.

## Subsequent gates, not activated by this library

Immutable root-owned policy/enrollment binding; typed bounded handoff; per-admission/per-retry installed policy gate;
separate receiver profile approval and atomic combined projections; installer/sandbox/manual Start/status/stop; exact
Ubuntu packaged isolation/resource/crash acceptance; later Docker-only mediation and new immutable profile; owner opt-in
and ordinary recipient read/revoke/deny journey. No extra production server entry is required for process-only staging.
Replacement must follow ADR 024's stop/join/fence plus confirmed revocation before a new identity; uncertainty leaves
old workers stopped and retained state outside new binds. Numeric canary and legacy connectors remain independent.
Permit at most one bounded retired rich slot; refuse another replacement until explicit reviewed removal frees it.
Compatible artifact refresh must keep the exact collection-definition digest, not merely the same friendly profile
name. A future distinct `RICH_PROCESSES_DOCKER_V1` approval requires its own accepted implementation/VM proof; it is
not a supported kind in this library and must not be enabled by interpreting the general combined wire profile.
