# Packaged process-only VM acceptance — 2026-10-29

Result: **PASS for the staging `RICH_PROCESSES_V1` package only.** This clears the prerequisite for implementing
ADR 025's process-and-Docker package. It is not a public release, owner-host installation, Docker acceptance or GA.

## Exact identity

Tested source: `24153fa278eea907cb0987ae96c26137c9aae909` (PR 139, subsequently squash-merged as
`97466795777be763c4ae2942452519246a86c960`). The table identifies locally built VM inputs, not downloadable release assets.
The later documentation commit recording this result was not the tested artifact source.

| Input | SHA-256 |
| --- | --- |
| Exact source archive | `29d6565170bba88959310e5b3c4394283df79d63483a049f631782830971ed9d` |
| Numeric package manifest | `87d6ce86144083d377226e36c2f69f57efc972038b05f8fefd03b27bab63bafa` |
| Numeric package archive | `0aec9232d7faa0b949dc21c475e7ab504ed2e379f02efedce42fa8d1e94e2f9e` |
| Process-only package manifest | `55ac66683c03de50c12e6d7ad225434e2ed71d69ac75349f31fb49f0103475d0` |
| Process-only package archive | `27d091ac4e3d69046b0f34868bca6304eb8ad414c3f64de918ae2d7959ec8d3f` |
| Synthetic receiver | `f23951e7e96d7cb82c14c2a03a50dc5f970843a1e2a82781e9fdcdaadcbf3ed8` |
| Source-bound preflight probe | `7c05d8f4a7a4cc3db7e56eb9d6aa1a240436693f60cd02a044e62a2a5ee8f12a` |
| Canonical nine-file runtime harness | `f2e3886635edb47abfdd63bda9186292be64ecb8bd9e8825a35e772859d1e110` |
| Official Ubuntu 24.04 amd64 image, release 20260911 | `612b2c0cc1bc413a6cb8c38fd611794caf0f2b436c50013d8b3794db12ad7354` |

Both packages identify version `0.1.0-canary.2`. Go 1.27.3 built static Linux/amd64 executables with trimpath and
VCS embedding disabled; the explicit release identity binds the source commit. Independent review rehashed the
source/PAX commit, build record, all twelve recorded artifact pins, archive member names/sizes/modes/root ownership,
embedded manifests, extracted binaries and harness. The image checksum signature verified with Ubuntu's installed
cloud-image keyring. These are local exact-byte acceptance checks, not publication provenance or a signing claim.

## Executed result

The reviewed runner used KVM, two virtual CPUs, 3 GiB RAM, a disposable overlay, read-only seed/payload disks,
QEMU sandboxing, no network interface, no host shares and synthetic enrollment/TLS only. The exact runtime harness
order is `run_vm.py guest.sh drive_pty.py assert_guest.py preflight_probe.py activation_guest.py bootstrap_guest.py
rich_scope.py rich_guest.py`.

The parent runner exited zero and emitted:

```text
HLO_VM_CASE_PASS rich-processes
HLO_VM_ALL_PASS
```

Independent read-only verification found exactly one `HLO_VM_RICH_FIRST_BOOT` in the first console and exactly one
`HLO_VM_RICH_REBOOT_PASS` in the second, neither in the wrong boot, no `HLO_VM_FAIL_` marker, unchanged input pins,
and no remaining QEMU or owned VM runner. All 19 triggered exact-head PR checks also passed before merge.

The complete journey proved:

- Stopped numeric installation remains byte-identical and inactive beside the separately enrolled process package.
- Real distinct principals, installed/effective units and host foreign-user process visibility; bounded process body
  larger than 16 KiB; collector credential/numeric-state masks and actual namespace denials at the same invocation.
- Lost acknowledgement leaves a durable pending body. Policy tampering refuses activation without changing pending
  bytes or receiver state. Stop/reboot preserves identity and pending sequence/digest.
- Explicit restart delivers the identical pending sequence/hash within the unchanged two-minute expiry, receives an
  acknowledgement and reaches fresh acknowledged status. Reboot isolation, final stop, identity and disk bounds pass.

The no-NIC fixture masks only its verified fixed network-wait unit before reboot. It checks the real pending
collection age and reboot interval below 90 seconds before Start and the original 120-second limit during retry.
There is no clock substitution, production TTL extension or acceptance of a replacement sequence as exact retry.

## Limits and next gate

Earlier failures and their bounded diagnoses remain in the
[process proof history](../../specs/013-connected-inventory/installable-process-proof.md). The successful run does not
erase those failures, and synthetic fixture tests alone did not establish this result.

No owner-host install, production rich-profile writer, public rich catalog, artifact publication, Docker authority,
container control, responsive hosted UX, delegated live read/revocation, automatic recovery, upgrade or removal was
accepted by this VM. Real SQLite crash/ENOSPC tests are separate evidence, not substituted by a reboot.

Next: the [coherent process-and-Docker installation](../../specs/013-connected-inventory/installable-docker-observation.md),
reader-first hosted integration, exact four-executable artifact/VM/Docker fault and resource proof, and then explicit
owner installation with owner/delegate/outsider/revocation acceptance. Keep the existing numeric installation intact.
