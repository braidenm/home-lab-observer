# Connected transition staging and durability research

Status: research for proposed ADR 022; not an installed-profile approval.
Reviewed: 2026-09-18. Target: Linux/amd64, Ubuntu 24.04, systemd 255,
root-owned ext4 state and fixed connected-worker files.

## Decision pressure

Endpoint refresh and compatible code selection must retain the same used
upload ledger, pending request bytes, credentials and server binding. A crash
between replacing a journal, CA, units, hosts and config must never make a
mixed generation look complete or start a worker. The exact stopped-worker
and previous/next resource checks are specified in
[transition staging](../../specs/012-connected-observation/transition-staging.md).

## Primary-source findings and design consequences

- Linux `fsync` of a file does **not** ensure its directory entry is durable;
  the containing directory also needs `fsync`. Stage publication therefore
  synchronizes every staged file and its directory, then the config parent
  after stage-directory creation and authoritative journal publication.
  Completion must be synchronized last. [Linux fsync(2)](https://man7.org/linux/man-pages/man2/fsync.2.html).
- `renameat2(RENAME_NOREPLACE)` atomically refuses an existing destination and
  is supported on ext4, but that namespace property alone is not a crash-
  durability claim. Use it for exclusive first publication and still sync the
  parent; replacement requires exact previously verified bytes and a separate
  re-open/compare after rename. [Linux rename(2)](https://man7.org/linux/man-pages/man2/rename.2.html).
- Go `os.Root` confines paths to its root but follows in-root symlinks and does
  not prohibit crossing mounts or opening device files. The installer must use
  its existing anchored root-owned directory walk plus fixed basenames opened
  with no-follow flags, then verify type, owner, mode, link count, ACL, size and
  exact bytes. `os.Root` alone is not an ownership boundary.
  [Go os.Root documentation](https://pkg.go.dev/os#Root),
  [Linux openat2(2)](https://man7.org/linux/man-pages/man2/openat2.2.html).
- systemd's unit-file view may differ from backing files until
  `daemon-reload`; `reload` of a service is not a unit-file reload. After
  replacing fixed unit files, reload the manager and recheck the exact loaded
  properties before publishing completion or allowing a later fresh start.
  [systemd v255 systemctl manual](https://github.com/systemd/systemd/blob/v255/man/systemctl.xml).

## Alternatives rejected for this profile

- Direct in-place edits or rename without parent sync cannot support the
  required interruption/power-loss proof.
- Copying a prior SQLite ledger back to roll back code would undo committed
  sequence/acknowledgement state; only code selection may be reversed.
- Trusting `os.Root` path containment without separate metadata and mount
  audits would not enforce the private root-owned stage boundary.
- Reconstructing missing CA or predecessor bytes from current host state
  would turn recovery into a different proposal. Unknown evidence refuses.

## Required evidence before installation

1. Fault-inject each create, write, file sync, directory sync, rename,
   manager reload and completion/cleanup boundary; reopen in a new process.
2. Prove no-follow, owner/mode/link/ACL and exact fixed-membership refusal,
   including foreign files, symlinks, mounts and incomplete stage contents.
3. On a disposable ext4 VM, exercise normal reboot and abrupt power loss at
   selected in-flight boundaries with a used pending ledger, then verify
   forward-only recovery and unchanged ledger identity/bytes.
4. Independently review the exact staged implementation and evidence before
   an owner-server canary. This research does not claim those tests passed.
