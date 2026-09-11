# D1: Linux durable upload ledger

Status: Accepted for the uninstalled Linux D1 adapter only; installation and other-platform support remain pending.
Decision: [ADR 015](../../docs/adr/015-durable-upload-ledger.md).

## API and lifecycle

Package `internal/uploadledger` implements `uploadstate.Ledger` and `Close() error`.

- `Provision(ctx context.Context, directory string, binding uploadstate.Binding) (*Ledger, error)` accepts only an
  existing dedicated private directory and creates fixed `upload.sqlite` and `.upload-lock` exclusively. It creates
  exactly the initial zero-watermark record for that binding. Any existing artifact is refused, never repaired.
- `OpenExisting(ctx context.Context, directory string, binding uploadstate.Binding) (*Ledger, error)` requires both
  artifacts. It opens the existing DB with mode=rw and validates identity/schema/state. It never initializes tables.
- `Load(ctx)` returns a detached validated record. `Commit(ctx, expected, next)` validates both, compares the complete
  logical current record inside BEGIN IMMEDIATE, writes next and commits. Binding cannot change or watermark decrease.
  A next pending sequence at or below the expected watermark must exactly equal the existing pending record; retired
  sequence numbers cannot be reintroduced and existing pending bytes cannot be replaced. C1 owns remaining transitions;
  these small safety invariants are not a second independent uploader state machine.
- `Close()` joins serialized operations, closes SQL before releasing the separate lifetime lock, and is idempotent.

All public failures are code-owned sentinels: unsupported platform, unsafe storage, busy ownership, or recovery required.
No SQL/path/OS error strings escape. Concurrent calls fail promptly as busy before starting. Other Load/Commit failures latch recovery on the adapter; no later operation sends or
repairs state. Close/reopen is required. Context cancellation is a fixed failure; bounded caller context plus configured
busy timeout limits SQL waits. SQL connection disposal/rollback cannot be skipped on uncertain commit.

Use explicit BEGIN IMMEDIATE/COMMIT statements on the dedicated connection: modernc v1.58.0 driver.Tx.Commit/Rollback
use a background context internally. ExecContext retains cancellation; rollback gets an independent two-second cleanup
context. Busy timeout is 250 ms. Arbitrarily stalled kernel/device synchronization cannot be forcibly bounded by these
deadlines; do not claim recovery from defective storage through cancellation alone.

Provision is an explicit library operation, not a worker fallback. The later enrollment coordinator must persist trusted
install/credential state and call it only for new enrollment; this API alone cannot prove authorization to start again.
Interrupted provisioning leaves artifacts for explicit operator recovery and must not be retried as automatic initialization.
No credential, HTTP client, scheduler, service registration or runtime activation is added.

## Exact schema v1

Use application_id `1213156420`, user_version 1, one STRICT table `upload_record` and no user indexes, triggers or views.
Columns (in this order):

| Column | Storage and constraint |
| --- | --- |
| singleton | INTEGER PRIMARY KEY CHECK(singleton=1) |
| server_id | TEXT NOT NULL, exact C1 server binding |
| connector_id | TEXT NOT NULL, exact C1 connector binding |
| watermark | INTEGER NOT NULL CHECK(watermark>=0) |
| has_ack | INTEGER NOT NULL CHECK(has_ack IN (0,1)) |
| last_ack | BLOB NOT NULL CHECK(length(last_ack)=32) |
| stopped | TEXT NOT NULL, closed C1 terminal set or empty |
| pending_sequence | INTEGER NULL, positive when present |
| pending_body | BLOB NULL, 1..16384 bytes when present |
| pending_digest | BLOB NULL, 32 bytes when present |
| pending_at | TEXT NULL, canonical UTC RFC3339Nano, maximum 30 bytes |

All four pending columns are jointly NULL or jointly present. No-ack digest is exactly 32 zero bytes. Read using exact
typed columns, require exactly one singleton, verify the closed schema and `uploadstate.ValidateRecord`. SQL constraints
are defense in depth; full body validation/hash/timestamp/binding checks remain mandatory. No float sequence conversion.
CAS compares every field including bytes and terminal state; timestamps use canonical UTC value, never pointer identity.

## Filesystem and transaction envelope

Linux only initially. Reuse ownerfs path/link traversal checks, add Linux current UID/mode verification, private directory
0700, regular single-link artifact permissions 0600, no privileged mode bits. Parents must not permit unrelated users to
replace the directory; trusted root/current-owner ancestry and sticky-directory semantics must be explicit in tests.
No chmod/chown of existing entries. POSIX ACL effective permissions remain bounded by group mode mask zero.

Acquire nonblocking flock on `.upload-lock` before opening/closing any raw database handle. Hold it until SQL is closed.
Validate fixed artifact types/sizes and combined DB+journal cap before SQLite opens; no raw database handle reopen while SQL lives. Bound directory
entries and refuse unknown files, links, WAL/SHM or extra state; permit the fixed DELETE recovery journal after validating
its private type/size. Never remove a hot journal: let SQLite recover it. Root/same-user malicious mutation is not isolated.

Construct the fixed DSN using URL encoding with no caller-provided options. One dedicated connection with no implicit
reconnect, private cache, mode=rw, BEGIN IMMEDIATE, journal_mode=DELETE, synchronous=EXTRA,
page_size=4096, max_page_count=64 (256 KiB), cache_spill=OFF, bounded cache covering the DB, temp_store=MEMORY,
trusted_schema=OFF and fixed bounded busy timeout. Verify effective settings including page count and mode. Provision sets
page size before schema creation; OpenExisting refuses incompatible settings/schema rather than silently migrating.

Total fixed state including journal is capped at 1 MiB. A 256 KiB DB leaves journal/header headroom; the implementation
must demonstrate the peak envelope under the exact bounded update pattern, not equate journal_size_limit to a hard cap.
No VACUUM, ATTACH, DDL migrations, large sort, growth maintenance, backup file or unbounded queue. If peak proof fails,
revise the supported storage envelope before acceptance. Initial provisioning must synchronize directory entries before
returning success. Durability requires persistent local storage; tmpfs tests prove behavior, not power-loss persistence.

## Acceptance tasks

- [x] Implement Linux only and side-effect-free unsupported stubs; fixed errors and no runtime imports.
- [x] Provision/open distinctions: missing/empty/corrupt/partial/foreign-version/wrong-binding state never recreated.
- [x] Round-trip signed maximum sequence, pending bytes/hash/time, terminal outcomes and detached records.
- [x] Exact full-record CAS; independent process ownership contention; busy and cancellation bounds.
- [x] Private paths/artifacts, hardlinks/symlinks, unsafe ancestors, unknown files, oversized DB/journal refused.
- [x] Effective DELETE/EXTRA/page/cache settings verified, including reopening.
- [x] Repeated maximal-profile admission/ack/expiry exercises and transactional DB+journal bound evidence.
- [x] Synthetic subprocess death during pending update and after successful commit preserves old-or-new complete state;
  restart never fabricates zero watermark. Failed commit/ack uncertainty latches recovery even if commit took effect.
- [x] Hot journal recovery uses SQLite, not cleanup; corrupt state remains untouched except SQLite's valid recovery.
- [x] Full Go tests/vet and focused Linux tests, with unexecuted power-loss/installed/cross-platform claims explicit.

The future installed acceptance owns actual disk-full and VM power-loss fixtures; adapter seams can exercise fixed failure
and uncertainty now but must not be labeled physical storage failure proof. Backup restore still requires re-enrollment.

## Local proof notes

Linux amd64 tests execute the crosscompiled static test binary under Ubuntu WSL with owned native temporary directories,
not host logs or service access. A test-only child enables SQLite cache spill to create a verified hot-journal header, exits
without cleanup, and proves production OpenExisting rolls back to the prior valid state. This is a recovery-format fixture;
production retains cache_spill=OFF. Other children exit immediately before/after the adapter's actual commit.

The production envelope is one bounded row/UPDATE per transaction, at most 64 database pages, and no cache spilling,
savepoints, attached DBs or vacuum. The pinned SQLite source caps sector/header alignment at 65536 bytes. Even budgeting
two such headers and one journal copy plus eight-byte overhead for each 4096-byte page gives 655872 bytes including the
full 256 KiB database, below 1 MiB. This envelope depends on retaining those exact transaction restrictions; additional
SQL operations require re-review. Tests repeatedly sample actual journal and DB sizes before commit using maximal eligible
filesystem/numeric fields; current canonical profile bodies are smaller than the defensive 16 KiB payload ceiling.
