# ADR 019: Private Linux enrollment persistence

Status: Accepted for an unused Linux current-user adapter only.

## Decision

Implement ADR017 with immutable ATTEMPTED, credential, and READY records under a
dedicated owner-private directory. READY is a commit witness for the same binding,
not a replacement of ATTEMPTED. A fixed exclusive lease serializes setup and ready
access. Publish each bounded record using exclusive staging, file synchronization,
no-replace rename and directory synchronization. Never repair, overwrite, resume
an incomplete attempt, or automatically exchange again.

Keep the mutable upload ledger in its own fixed child using ADR015. A new SQLite
enrollment database would introduce credential-bearing pages and rollback journals
without making the separate ledger and remote activation one atomic transaction.
Immutable records keep this one-shot transition small; SQLite remains the selected
adapter for frequently changing admission state. No generic transaction framework
or additional dependency is introduced.

OpenReady requires all records, matching credential and binding, and a valid existing
ledger, followed by successful synchronization before exposing either credential or
ledger. A reported publication failure can still leave a valid commit witness. The
failed call never activates anything; a subsequent fully validated READY may be
authoritative. ATTEMPTED or a credential alone never authorizes recovery.

## Limits and evidence

The directory is current-user-owned, not the future root-owned installation layout.
This does not implement systemd LoadCredential promotion or service isolation.
Root and hostile same-UID interference, network filesystems, hardware that lies
about synchronization, and restored old state are outside this boundary. Files are
plaintext owner-private credentials, not encrypted or securely erasable secrets.

[Linux fsync](https://man7.org/linux/man-pages/man2/fsync.2.html) requires a separate
directory sync for directory entries. [Linux rename](https://man7.org/linux/man-pages/man2/rename.2.html)
provides RENAME_NOREPLACE; unsupported filesystems fail closed rather than falling
back to overwriting rename. [SQLite synchronous](https://www.sqlite.org/pragma.html#pragma_synchronous)
documents EXTRA's additional directory synchronization; it does not solve the remote
activation/multi-store transaction. Sources consulted 2026-09-11.

See [the implementation contract](../../specs/012-connected-observation/enrollment-persistence.md)
for exact APIs, bounds, recovery and acceptance. Installed power-loss and service
acceptance remain separate gates.
