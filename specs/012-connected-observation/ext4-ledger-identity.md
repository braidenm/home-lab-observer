# Anchored ext4 ledger identity

Status: bounded implementation of ADR 022's proposed identity prerequisite; no
transition, installer activation, reboot stability or restore protection claim.

`internal/ledgeridentity.Inspect(ctx, directory, database)` accepts already-open
handles only. It returns a comparable `Witness` with `FilesystemUUID` (32 lowercase
hex characters), and `Directory`/`Database` objects containing uint64 `Inode` and
uint32 `Generation`. The owning transition codec binds the fixed version
`observer-ext4-ledger-identity/v1`; it must preserve integer precision. `Validate`
rejects malformed/zero identifiers and identical directory/database inodes.
All failures return one fixed error without paths, UUIDs or OS diagnostics.

Both handles remain pinned with `SyscallConn.Control` throughout two complete
descriptor-only observations. Each observation reads fstatfs, fstat, filesystem
UUID and inode generation. Require the supported ext4 interface, directory and
regular-file types, live directory links, exactly one database link, matching
runtime device and filesystem UUID, and unchanged identity/mode/owner/link facts
between observations. Device numbers are process-local cross-checks, never the
durable identity. No path reopen, block-device scan, SQL open or mutation occurs.
No names or contents are read; the caller retains and closes its handles.

The caller must first establish trusted path traversal, directory membership,
owner/mode/ACL policy and exclusive installation authority, and join all writers.
This package does not infer those facts from two supplied handles. Repeated
inspection detects observed drift, not an atomic snapshot or hostile-root proof.
Cancellation is checked between operations; an individual kernel call is not
interruptible by the context. Unsupported kernels/filesystems/platforms refuse.
Initial implementation supports Linux amd64/arm64 only, matching the verified
64-bit ioctl encoding. Native v2 and SQLite durability policies are unchanged.

## Decision evidence (accessed 2026-09-17)

Linux 6.8's [ext4 UAPI](https://raw.githubusercontent.com/torvalds/linux/v6.8/include/uapi/linux/ext4.h)
defines GETFSUUID with an eight-byte length/flags header and flexible UUID data.
The [ext4 implementation](https://raw.githubusercontent.com/torvalds/linux/v6.8/fs/ext4/ioctl.c)
copies 16 UUID bytes and returns inode generation through GETVERSION. Use a
fixed-size buffer with length 16, flags zero, and verify the returned header.
The generation result is 32 bits despite the 64-bit ioctl's `long` size encoding.

Device/inode alone is simpler but device numbering is not a reboot identity.
Path/block-device discovery adds authority and races. Anchored UUID+inode+
generation offers a narrow replacement witness; unsupported filesystems are
explicitly excluded instead of supplying weaker evidence. Zero UUID/inode/
generation values are conservatively refused, even if a filesystem permits them.
These values are not secrets or authentication, but remain private transition
comparison data and must not enter operator logs or hosted status.

## Required proof and limits

Synthetic tests cover every failed query, before/after identity changes, type/
link/UUID/device mismatch, cancellation and fixed errors. An owned native ext4
fixture checks repeated reads and replacement detection while keeping the old
descriptor open. Ordinary unsupported hosts skip only that native fixture;
`OBSERVER_EXT4_ACCEPTANCE=1` makes missing support a failure. No root, mounts,
host ledger or service changes are needed. Non-Linux tests prove refusal.

Reboot/simulated-power-loss evidence remains a separate packaged VM gate. A
restored or cloned filesystem preserving UUID/inode/generation cannot be detected
here; backup restoration requires revoked credentials and re-enrollment, not a
claimed anti-rollback guarantee. No accepted ADR is amended by this prerequisite.

## Local evidence

On 2026-09-17 the Linux amd64 fixture passed five repetitions on WSL kernel
6.6.87.2 using only newly created `/tmp` files with required ext4 acceptance
enabled (not skipped). Windows pure/stub tests and vet, Linux synthetic/native
tests and vet, and Linux arm64/macOS arm64 test compilation passed. Cross-compilation
is not native ARM or macOS execution. No reboot, mounts, services, real ledger,
credentials or device scans were involved; packaged recovery evidence is pending.
