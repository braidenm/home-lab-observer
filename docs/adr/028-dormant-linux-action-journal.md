# ADR 028: Private bounded Linux action journal

Status: Accepted for unused Linux implementation only after exact-document coordinator and independent review.
No installed broker, Docker mutation authority or activation is accepted here.
Date: 2026-11-01. Scope: Spec 011's next unused adapter after the typed coordinator.

## Decision candidate

Use the already pinned modernc.org/sqlite v1.59.0 (embedded SQLite 3.53.4) for a separate action journal. Follow the
reviewed principles of ADR 015, without changing that accepted ADR or refactoring the upload/enrollment stores.
The exact [journal contract](../../specs/011-container-actions/action-journal.md) owns APIs, transitions and proof.

Select explicit Provision versus OpenExisting, a dedicated current-owner-private directory and three fixed filenames,
a lifetime separate exclusive lock, one dedicated SQL connection and short BEGIN IMMEDIATE transactions. Use rollback
DELETE with synchronous EXTRA, cache_spill OFF and verified fixed settings. Never recreate, adopt, reset, migrate,
rename or quarantine missing/corrupt/foreign state. Valid hot-journal recovery is SQLite's operation, not cleanup.
Other platforms refuse without filesystem access. No caller-supplied SQL, limits, schema, paths inside the directory
or policy plugins. Reuse ownerfs and individually reviewed filesystem checks, not a new generic storage framework.

Persist immutable canonical private action metadata, admission, dispatch intent and finite outcome evidence. Preserve
the core's version-1 fingerprint bytes; rotating delivery leases never become identity. Two global permits include
unresolved intent, and both opaque target and engine-generation/full-container binding are serialized. Intent without
a terminal result is always OUTCOME_UNKNOWN. Close, crash, expiry and new keys cannot free that permit/fence. This
first journal has no owner acknowledgement/reconciliation/clear method; that authenticated workflow remains gated.

Choose 1,000 retained requests, minimum 30-day terminal retention, at most two events reserved per request and no
unresolved eviction. This narrows the former 10,000-request proposal for the first dormant profile. A 12 MiB database
candidate plus rollback journal must remain below 32 MiB total. Logical terminal capacity is reserved at admission;
it cannot guarantee filesystem space or successful future synchronization. Finish failure retains unknown intent.

Pruning atomically advances one monotonic pruned_expiry_through value while deleting only terminal records whose
terminal time AND authenticated immutable expiry are at least 30 days old. Missing-key admissions require expiry above
that floor. This prevents a forward-clock prune followed by rollback from resurrecting deleted keys, without a general
per-operation clock high-water mark. Current Authority supplies no authenticated retention clock. A very large forward
jump can conservatively prevent admissions; no implicit floor reset is allowed. Root/same-user mutation or restoring
an older valid journal is outside this anti-replay guarantee and requires a future authenticated recovery policy.

## Alternatives and limits

Keeping actions disabled remains safe. Atomic JSON would require new replacement/fsync, multi-record CAS and crash
machinery. WAL/FULL could be durable but introduces WAL/SHM/checkpoint accounting with no need for concurrent readers;
WAL/NORMAL may lose recent committed state on power loss. Existing rollback-mode patterns are smaller here. Typed code
does not constrain an eventual broker's broad Docker socket authority or make audit tamper-proof against its owner.

This journal audits admitted actions/intent/outcomes only. It does not promise durable pre-admission authentication or
grant-change denials, unlimited denial logging under full storage, physical power-loss proof, remote wire compatibility,
credential isolation, standalone authentication or installed lifecycle. No real Engine adapter precedes the required
native crash, capacity, actual private-filesystem-full and commit-uncertainty tests. Rollback leaves private evidence
intact and disables use; it never clears unknown records or rewrites observation state.

## Primary evidence checked 2026-11-01

- [Pinned modernc v1.59.0](https://pkg.go.dev/modernc.org/sqlite@v1.59.0): bundled 3.53.4 and connection/locking caveats.
  Its actual tx.go uses background contexts for driver Commit/Rollback; explicit ExecContext statements preserve the
  adapter's bounded SQL context. Do not change the process-wide OFD-locking switch as part of this slice.
- [SQLite synchronous](https://www.sqlite.org/pragma.html#pragma_synchronous): EXTRA adds directory synchronization
  after DELETE commits; guarantees still depend on correct filesystem/device flush behavior.
- [Transactions](https://www.sqlite.org/lang_transaction.html): IMMEDIATE acquires the write transaction; commit or
  I/O/full errors need deliberate rollback/disposal, not an assumption that every error ended the transaction.
- [Page cap](https://www.sqlite.org/pragma.html#pragma_max_page_count) and
  [journal size limit](https://www.sqlite.org/pragma.html#pragma_journal_size_limit): a database cap is not a total
  active-journal cap; journal_size_limit concerns retained files, not the required peak proof.
- [Atomic commit](https://www.sqlite.org/atomiccommit.html), [corruption hazards](https://www.sqlite.org/howtocorrupt.html)
  and [URI modes](https://www.sqlite.org/uri.html): hot journals must not be removed, raw closes can affect POSIX locks,
  and mode=rw avoids implicit creation. Subprocess/tmpfs tests do not simulate physical power loss.
