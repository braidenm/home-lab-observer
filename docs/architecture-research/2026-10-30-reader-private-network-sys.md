# Preserve the Docker reader's inert `/sys` under systemd 255

Status: implementation source independently reviewed; packaged acceptance remains pending.
Access date: 2026-10-30. Owning decision: [ADR 025](../adr/025-isolated-installed-docker-observation.md).

## Question and observed boundary

How can the fixed Docker reader retain private networking while exposing only its approved minimal filesystem?
The supported installation is Ubuntu 24.04/systemd 255/amd64. This is an unreleased rich-profile correction, not a
change to the connected numeric beta or a reason to give the collector/uploader more authority.

The packaged reader fails startup. A separately pinned, mixed-source diagnostic in a no-network disposable VM first
identified `READER_VIEW`. A second diagnostic invokes the actual private `checkDockerReaderView` through unchanged
filesystem functions and identifies `READ_SYS`: the fixed `/sys` read or validation of its returned metadata refuses.
These are observations of the preserved post-failure sandbox, not direct proof of the original worker's first error.
No production guard was relaxed; no Engine request or owner-host action was used to obtain this evidence.

The current validator requires an empty, root-owned `0555` directory and rejects sysfs. The installer creates that
physical directory, and the parent namespace audit requires its original inode. The reader unit also selects
`PrivateNetwork=yes` and `MountAPIVFS=no`. Unit tests alone did not expose their runtime interaction.

## Primary evidence and pitfalls

- [systemd 255 execution environment](https://www.freedesktop.org/software/systemd/man/255/systemd.exec.html),
  `PrivateNetwork=`: private networking also remounts `/sys`. `MountAPIVFS=no` is not a promise that this independently
  requested mount is absent. Address-family restriction and a private network namespace are separate protections.
- [systemd v255 namespace implementation](https://github.com/systemd/systemd/blob/v255/src/core/namespace.c):
  `setup_namespace` adds `MOUNT_PRIVATE_SYSFS` for private networking. `MountMode`, `mount_path_compare` and
  `drop_duplicates` give an explicit bind priority over the competing sysfs mount at the same target, retaining
  restrictive flags. This is upstream v255 evidence, not proof that every distribution patch behaves identically.
- The same manual's `BindReadOnlyPaths=` documents host-source/service-destination binds with `RootDirectory=`.
  `TemporaryFileSystem=` can hide a hierarchy, but creates a different filesystem/object. Neither setting justifies
  allowing arbitrary host paths or changing an existing identity check to accept unknown objects.

## Alternatives and recommendation

| Option | Consequence | Selection |
| --- | --- | --- |
| Explicit read-only bind of the already audited `reader-root/sys` onto `/sys` | Preserves empty-directory identity and all existing runtime guards; narrow unit change | Proposed |
| Empty read-only tmpfs on `/sys` | Loses priority to private sysfs in v255 and replaces the audited inode | Not reliable here |
| Remove private networking and retain AF_UNIX-only restrictions | Smaller unit change but removes an independent containment layer | Rejected |
| Accept sysfs or suppress the failed guard | Exposes additional machine data and weakens the accepted minimal view | Rejected |

Use `BindReadOnlyPaths=/var/lib/home-lab-observer-connected-rich/reader-root/sys:/sys:norbind`.
The nonrecursive bind avoids importing nested mounts. Use only the fixed root-owned physical scaffold as the source,
never host `/sys`. Do not use an optional `-`
prefix, a caller-supplied path, writable bind or a general mount setting. Preserve private networking, AF_UNIX-only
socket creation, capability/syscall denial, null output, resource ceilings, existing read-only checks and inode audits.
This restores the existing profile's intended minimal view; it adds no collection field, permission, credential or
new profile definition. The same source artifact must prove that claim before release.

## Proof, operation and rollback

1. A focused rendering regression must fail before the fix, then prove the exact mandatory bind plus unchanged
   isolation/resource settings. Keep namespace tests rejecting substituted `/sys` identity and unexpected authority.
2. Run focused Linux race/vet and unit/package/contract checks. No additional dependency or broad CI suite is needed.
3. Build an exact-source complete package and rerun the existing disposable Docker acceptance cases, including first
   start, same-invocation audit, reboot/retry, numeric coexistence and existing-rich-root refusal. Require the empty
   `/sys` guard to pass unchanged. Further refusal is a new investigation, not permission to weaken another check.
4. Keep diagnostic output to closed stages; never publish directory listings, private consoles or raw errors.
   Build/probe success alone is not packaged, release or owner-host acceptance.

Before any rich enrollment, rollback is removal of unused code; no data migration is required. Do not modify a live
installation or daemon to test this proposal. Later owner activation still requires accepted immutable assets and
the existing enrollment/consent gates. There is no new owner decision: the proposed correction preserves the already
approved authority boundary, and Kubernetes/management remain separate work.
