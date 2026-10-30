# Docker reader: physical scaffold versus runtime propagation denial

Status: proposed production correction; source implementation and exact-package acceptance pending.
Accessed: 2026-10-30. Owns no new collection scope; follows [ADR025](../adr/025-isolated-installed-docker-observation.md)
and [ADR026's same-invocation barrier](../adr/026-docker-reader-startup-barrier.md).

## Evidence and its limit

The retained-sandbox test probe returned `READ_RUN_INCOMING_MODE`, with recovery `COMPLETE`; original startup still
failed at `START_RECOVERY_READER_SERVICE_START`. It passed the preceding `/sys` check. The retained console is92545
bytes, SHA-256 `566117fffa630c003c5aae8e23e3e4195b15c1c730f470e6ab31e73f4b50711f`.
This observes a later probe invocation in preserved state, not the original worker's first cause. It neither proves
the actual mode by itself nor accepts Docker-rich installation, release or an owner-host upgrade.

The physical installer scaffold expects an empty root-owned0755 `reader-root/run/systemd/incoming`. The live reader
currently applies that same readable-directory expectation. This conflates two independently audited objects.

## Primary version-matched sources

- [systemd v255 `exec-invoke.c`](https://github.com/systemd/systemd/blob/v255/src/core/exec-invoke.c),
  `apply_mount_namespace`: system scope selects `/run/systemd/propagate/<unit-id>` as source and
  `/run/systemd/incoming` as destination. It passes both into namespace setup.
- [systemd v255 `namespace.c`](https://github.com/systemd/systemd/blob/v255/src/core/namespace.c),
  `setup_namespace`: creates the propagation source with0600, adds a read-only bind and makes the destination a
  slave mount. Its mount ordering/deduplication also makes competing entries at one target nontrivial.
- [systemd255 execution documentation](https://www.freedesktop.org/software/systemd/man/255/systemd.exec.html),
  filesystem namespacing/`InaccessiblePaths=`: explicit denial hides a subtree; read-only mounts alone do not prevent
  later propagated child mounts. Capability and mount-syscall denial remain necessary independent controls.

Upstream code explains the proposed distinction; it does not replace actual Ubuntu255 package tests or prove a
distribution has no patches. Root must verify the selected source, denial and binding rather than infer them.

## Alternatives

| Choice | Consequence | Decision |
| --- | --- | --- |
| Require live0755/readable/empty as now | Rejects the native propagation view; making it readable is not the desired boundary | Reject |
| Retain native exact0600 denial and audit the one fixed source/inode | No unit/resource change; stricter provenance and actual read/traverse denial | Select, subject to proof |
| Add explicit `InaccessiblePaths=` or another bind | Changes mount selection/identity and duplicates an existing denied boundary | Reject for this correction |
| Accept either0755 or0600, ignore mode or merely skip the check | Broadens the accepted view without explaining its source | Reject |

Retain the physical0755 empty scaffold check unchanged. For this Docker reader only, the runtime guard must instead
require exact root:root0600, ACL absence and denied directory access; the root audit proves emptiness and exact
unit-specific source identity. This does not turn a denied listing into proof of emptiness.

## Security and implementation boundary

Reuse the existing root-only `checkNamespaceView` pinned-root callback. Bind the source to the compiled exact reader
unit, never a caller-supplied unit/path. Walk/pin each fixed parent no-follow and reject group/world-writable or
non-root-owned ancestors without assuming their exact modes. Open the unit relative to that pinned parent; check exact metadata,
ACL absence via its descriptor, bounded empty contents and non-procfs/non-sysfs. Compare source path to the pinned FD
before and after the namespace audit (including each parent path/object), and compare the callback's `run/systemd/incoming` to that source.

Return only a tiny Dev/Ino identity to the Docker reader's existing `ops.audit`. Capture before request publication
and require the same identity after the fresh matching response, before commit; retain PID/InvocationID, socket,
descriptor and policy fences. No general manager abstraction, arbitrary path argument, repair/chmod or adoption.
The unprivileged reader cannot read/traverse the directory; root's audited source establishes its empty identity.

No accepted ADR is rewritten: this applies ADR026's existing two paused audits to native sandbox machinery, without
a new channel or authority. Numeric/process contracts, collection fields, definition digest, units and budgets stay
unchanged. [The scoped implementation/proof plan](../../specs/013-connected-inventory/reader-incoming-runtime.md)
requires failing tests first and a complete new exact-source packaged VM before release or owner use.
