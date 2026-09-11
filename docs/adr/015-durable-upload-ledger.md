# ADR 015: Separate durable upload sequence ledger

Status: Accepted for the uninstalled Linux D1 adapter only. Installed and other-platform profiles remain gated.
Date: 2026-09-11.

## Decision proposed

Use existing modernc.org/sqlite v1.58.0 (SQLite 3.53.4), separate from history, for C1's one-record ledger.
Select rollback DELETE with synchronous EXTRA, not history's WAL/NORMAL or recovery-by-recreation.
The first adapter supports Linux only; other OSes return a fixed unsupported error without touching disk.
This does not activate remote upload or establish installed service isolation.

An atomic JSON file is viable but would require new durable replacement, directory synchronization,
cross-process CAS and ambiguous-write recovery. SQLite already provides transaction machinery with no new dependency.
Its guarantees still depend on correct persistent local filesystem/device synchronization. Neither option authenticates
an externally restored older valid record; restoring backups requires revocation/re-enrollment.

Use explicit Provision versus OpenExisting operations, a lifetime exclusive lock on a separate fixed file,
one dedicated SQL connection, full-record transactional CAS and closed logical validation. Never quarantine, delete,
reset, migrate or recreate lost/corrupt state automatically. An uncertain commit poisons that adapter instance.

Linux paths must be dedicated, current-user private and non-linked. Check database handles before SQL opens, not during
transactions: a raw file close can disturb POSIX SQLite locks. The separate lifetime lock serializes cooperating adapters
before any database handle checks. Same-user hostile processes and root remain outside the confidentiality boundary.
Windows/macOS filesystem proof is deferred rather than copying existing ACL implementations into this slice.

## Evidence and limits

Primary sources accessed 2026-09-11:

- [SQLite synchronous](https://www.sqlite.org/pragma.html#pragma_synchronous): EXTRA synchronizes the journal directory
  after DELETE commits; FULL rollback mode may lose a just-committed transaction after power failure.
- [SQLite transaction semantics](https://www.sqlite.org/lang_transaction.html): IMMEDIATE takes the write transaction
  before reading; COMMIT errors can leave transactions active. Cleanup must not assume automatic rollback.
- [SQLite journal limits](https://www.sqlite.org/pragma.html#pragma_journal_size_limit): retained journal size is not
  peak transaction size. D1 must prove bounded database plus journal usage separately.
- [SQLite atomic commit](https://www.sqlite.org/atomiccommit.html): hot-journal recovery and hardware assumptions.
- [SQLite URI modes](https://www.sqlite.org/uri.html): rw opens existing state without implicit creation.
- [modernc v1.58.0](https://pkg.go.dev/modernc.org/sqlite@v1.58.0): bundled SQLite version, immediate transactions and
  POSIX/OFD lock caveats. No new process-wide locking setting is selected here.

See [D1 contract and proof](../../specs/012-connected-observation/durable-ledger.md). Existing C1 remains storage-independent.
Installation, enrollment/credential coordination, VM power-loss evidence and cross-platform support remain separately gated.
