# D2d: Linux owner-private enrollment persistence

Status: accepted for unused library implementation; no CLI, exchange, installer or
worker activation. Implements ADR017 through [ADR019](../../docs/adr/019-private-enrollment-persistence.md).

## API and fixed layout

`enrollmentstore.OpenNew(ctx, directory)` accepts an existing empty private directory
and returns a single-use Setup implementing the coordinator's Begin, Create,
Provision and MarkReady ports, plus Close. Methods are serialized and any failed
operation latches recovery. No post-failure retry or automatic deletion is provided.

`OpenReady(ctx, directory, expectedBinding)` holds the same exclusive root lease,
validates all artifacts and opens D1 using OpenExisting, never Provision. Its Ready
handle supplies a detached Credential and an already-open Ledger; Close joins that
ledger before releasing the root lease. Callers must not use returned objects after
Close. No independently usable credential-only reader or ready Boolean exists.

Fixed children: `.enrollment-lock` (empty), `attempt.json`, `credential.json`,
`ready.json`, `.record.tmp`, and `ledger/`. Each JSON file is at most 512 bytes;
all four files including staging total at most 2048 bytes. D1's independent ledger
limits remain unchanged. Bound enumeration to one beyond the allowed entry count.
Canonical closed JSON contains version `observer-enrollment/v1`, state
`ATTEMPTED`, `CREDENTIAL` or `READY`, and exact server_id/connector_id. Only CREDENTIAL
contains the validated 47-byte hlc_ secret. Never persist a grant or its hash.
Decode by typed parsing plus exact canonical re-encoding; reject unknown/duplicate
fields, alternate encodings, invalid IDs, excessive bytes or trailing data.

## Privacy and durability

Existing root and ledger directories: exact current UID and 0700, no privileged bits
or access/default POSIX ACL. All regular files: current UID, 0600, single link, no ACL.
Reject symlinks, special files, unexpected names, unsafe ancestor ownership/write
authority and replaced pinned roots. Root/current-owner sticky temporary ancestors
are supported. Do not repair permissions or existing content. Created files are
checked before credential bytes are written. Linux-only; other OS stubs perform no I/O.

Use an exclusively created lease for OpenNew; existing lease only for OpenReady.
Hold nonblocking exclusive flock throughout the handle lifetime. Begin must complete
durable ATTEMPTED before exchange. Create requires that binding and no credential.
Provision creates only a new ledger child, validates and closes a zero-watermark D1
ledger. MarkReady independently rereads attempt and credential, reopens and validates
the pristine ledger, closes it, then publishes READY. No method accepts a caller
assertion that substitutes for artifact validation.

Publish via exclusive fixed staging, bounded complete write, file fsync, close,
renameat2(RENAME_NOREPLACE), directory fsync. All failures preserve evidence and
latch a fixed recovery error. Uncertain publication may leave a valid READY witness:
OpenReady validates every artifact and synchronizes record files and directories
before returning. It permits a valid advanced ledger, but never incomplete enrollment.
Do not open/close raw SQLite database handles while D1 owns SQLite locks.

Context cancellation is checked before and between operations; local filesystem
syscalls cannot promise cancellable kernel I/O. D1 keeps its existing bounded SQL
timeouts. No background work, network, service operations or external error text.
The no-hostile-same-UID/root threat model is explicit; flock is cooperative exclusion.

## Acceptance and remaining gates

- [x] Ordinary lifecycle, detached credentials, advanced ledger reopen, both-ID mismatch.
- [x] Existing/partial roots, missing/corrupt records or ledger never recreated.
- [x] Duplicate/unknown/noncanonical JSON, record bounds and fixed error behavior.
- [x] Link/ACL/mode/special-file refusal, root replacement and exclusive lease.
- [x] Hooks before write and after successful write/fsync/close/rename/directory-sync
  prove publication-boundary uncertainty and latching, not injected syscall errors.
- [x] Cancellation, fixed error canaries and credential-free marker assertions.
- [x] Child-process interruption at publication boundaries and subsequent validation.
- [x] Native Linux suite and separate-process Setup/Ready lease contention.
- [x] Full Go tests/vet on Windows; non-Linux unsupported behavior.
- [ ] Direct foreign-UID fixture; production checks current UID but this slice does
  not claim actual multi-UID refusal evidence. The distinct-principal E1 fixture is separate.
- [ ] Real ENOSPC/EIO/close error and VM power-loss injection; boundary hooks are not
  equivalent evidence. Every returned syscall error is nevertheless handled fail-closed.
- [x] Independent production/test review found no blockers; native Linux suite was
  independently rerun five times on 2026-09-11, including cross-process lease contention.

VM power-loss/disk-full, root-owned installed metadata, LoadCredential promotion,
service principal isolation and owner canary remain separate. This adapter is not a
drop-in implementation of the installed mount layout.
