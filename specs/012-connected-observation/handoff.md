# Slice B: credential-free latest-snapshot handoff

Status: implementation scope approved under the owner's connected-delivery request; no worker activation.

The collector publishes only a numeric-host projection into an explicitly supplied, existing dedicated directory.
The future uploader reads only that projection. This library stores no credential, pending HTTP request or sequence.

- B1: Pin an `os.Root` handle and validate the opened directory's owner/private permissions, not just pathname shape.
  Accept only Linux/macOS/Windows. Existing unsafe directories fail closed without chmod/ACL repair or creation.
  The directory must be owned by the current process user and inaccessible to other ordinary users. Windows SYSTEM
  and local administrators remain trusted. Same-user compromise and privileged mount changes are outside this boundary.
- B2: Use code-owned `snapshot.json`, `.snapshot-next` and `.writer-lock` only. Keep one writer via an OS file lock;
  concurrent readers see a validated owned copy or a fixed unavailable/missing/invalid error. No arbitrary filename API.
- B3: Publish accepts the local observation plus enrollment identity, invokes the explicit projection, creates staging
  exclusively, validates its handle, writes at most 16 KiB, syncs and closes before replacement. Existing
  staging is removed only after bounded regular-file/owner/link validation while holding the writer lock.
  Prove new-file creation with O_EXCL before initializing its owner. Elevated Windows tokens may otherwise assign
  Administrators as default owner; initialize only the newly created empty handle to the current user, then validate
  it before writing. Opening an existing lock after EEXIST never changes its owner or ACL. No process token mutation.
- B4: All reads use the pinned root, check opened handles (regular, single link, private ownership/permissions, size),
  read at most 16 KiB plus one sentinel byte and revalidate the canonical producer profile and expected server ID.
  Reject links/reparse points and unsafe existing files. Never return raw filesystem errors or snapshot contents in logs.
- B5: Unix same-directory rename plus directory sync provides durable replacement when the filesystem supports it.
  Windows replacement can report sharing/transient failures; reads fail closed during absence or invalid data. Do not
  promise universal Windows/network-filesystem atomicity or power-loss durability. This is disposable latest state:
  restart may require recollection. Durable admitted upload bodies/sequences belong in a separate later ledger.
- B6: Bound retained named files to latest+staging+empty lock (at most 32 KiB payload). No unbounded retries or histories.
  Sync/replace failure is reported even if a new snapshot became visible. Close releases only owned handles/locks.
- B7: Test private versus unsafe existing permissions, missing/oversized/corrupt/symlink/hardlink input, competing
  writers, concurrent readers, staging leftovers and injected write/sync/replace failure. Native CI covers all three OSes.

Receiver proof is merged in Platform Demo PR365. This slice implements a local privacy boundary, not OS-enforced
collector/uploader separation: separate accounts, egress policy, credential storage and installed worker lifecycle
require the following spec/ADR gate before any canary enables upload.
