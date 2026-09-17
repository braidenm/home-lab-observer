# F1 packaged VM subset: synthetic stopped state and collector

Status: bounded plan independently reviewed 2026-09-17; first execution failed
closed at the dedicated principal audit, before synthetic credential creation or
any worker start. Subsequent gates remain unexecuted.
This is partial acceptance, not a successful real enrollment/Install transaction,
uploader activation, rollback, reboot recovery or completed F1.

## Safety and admission

Use an explicitly owner-authorized disposable Ubuntu 24.04/systemd255 amd64 QEMU
VM. Its reviewed NIC filter is applied before boot: fixed guest identity, private
destination and IPv6 denial, only task-gateway DNS/host-initiated SSH and exact
control-plane TLS addresses. No production credential, enrollment grant or HTTP
request is used. The origin, CA policy and packaged binaries remain unchanged.
Private VM addresses, UUIDs, keys and host details never enter this repository.

The fixture is an opt-in `linux && connected_vm_fixture` test, not an installed
command or bypass API. Before account/path mutations require root, an explicit
confirmation and expected VM DMI UUID supplied from the separately retained private
inventory, matching actual DMI and QEMU detection. Refuse any existing connected
accounts/groups, config/state/release paths, units/drop-ins or activation directory.
Use only a fixed fixture bundle directory and trusted expected manifest digest.
Ordinary test runs neither compile nor invoke this fixture.

## Exact subset

1. Verify the actual connected bundle and call non-mutating CheckRequest. Never
   call Install or pass a syntactically valid grant to a production exchange.
2. Under the installation lease, construct explicitly fixture-created local
   state with existing directory/bundle/root helpers and the exact dedicated
   principals. This is not an installer-success result.
3. In a test-only child with exact uploader real/effective/saved IDs, explicit
   supplementary group set, no capabilities and no-new-privileges, seed synthetic
   D2d records using its existing Begin/Create/Provision/MarkReady operations.
   No exchange or transport method is invoked. Never print credential bytes.
4. Run only the unchanged packaged uploader's `validate-enrollment` and
   `validate-ledger` modes through the real renderer and loaded offline manager
   gate. Exercise the existing ledger promotion helper between them. Preserve
   exact ledger identity; compare synthetic credential data only in memory.
5. Publish exact fixture unit/config bytes and verify loaded ownership. The
   steady uploader remains inactive and disabled, with no activation request or
   commit. Start only the unchanged packaged collector, bounded to 20 seconds,
   and require its current-invocation COLLECTING status and known coverage-
   unavailable numeric projection, never fabricated healthy-zero metrics.
6. Independently bounded cleanup stops/joins the owned collector on every path;
   cleanup failure fails acceptance. Recheck uploader inactivity/disablement and
   absent activation records. Leave fixture-created state stopped for review;
   remove the entire owned disposable VM after testing, not unrelated host files.

All native changes are confined to the admitted disposable guest. Keep the host
VM powered off during substantial implementation waits. The eventual full gate
still requires real installed profiles, enrollment transaction proof, compatible
code transitions, fault recovery and reboot/power-loss tests.

## First execution evidence (2026-09-17)

The unchanged package from source `caa451ecc0bd305a84abdbcfa82a0ec8357b2dc7`
was built using Go 1.27.1, CGO disabled and trimpath, with role version
`0.1.0-f1-vm`. Its archive SHA256 was
`14d948dfcdac62361da7ceb8c6861477f501e3a0d864c2f5dc1381e1c0b894ca`;
canonical manifest SHA256 was
`66d0e1d0c26a923c4d26bea3ea7def05fb84ab3491644e8bf7782331421f385f`.
The full test failed after 14.28 seconds with `VM_FIXTURE_PRINCIPAL_FAILED`.
Bundle/preflight and fresh dedicated account creation passed; no credential,
collector, uploader or activation records were created. Independent manager
inspection confirmed both worker units absent/inactive with PID zero. The VM
was powered off while the correction was reviewed.

Ubuntu's glibc `getent initgroups` reports supplementary memberships, not an
injected passwd primary GID: collector name only and uploader shared group only
for this exact dedicated layout. The audit had incorrectly expected primary
groups there as well. Correct the expected supplementary sets only, retaining
the independent exact passwd primary-GID checks and refusal of missing shared
membership, extra or duplicate groups. Regression tests cover both wrong primary
GIDs and the actual supplementary output. Primary source:
[glibc 2.39 getent initgroups implementation](https://raw.githubusercontent.com/bminor/glibc/glibc-2.39/nss/getent.c)
(`getgrouplist` receives the sentinel primary GID and output filters it).

## Second execution and bounded localization (2026-09-17)

A fresh disposable overlay used source `6b36c39` with the corrected principal
audit. Archive SHA256:
`b4ff4ecf4c7c4ebdcf88593de5a483ab431079358d3a7fc2d16efa9720adde03`;
manifest SHA256:
`243d21119b5f5d500353708370320733a3ec436f8168551051ba9545300e3f88`.
The full test failed after 2.03 seconds at `VM_FIXTURE_OFFLINE_ENROLLMENT_FAILED`.
The corrected account audit, layout and synthetic seeding passed. No collector,
steady uploader, production exchange or activation record was started/created.

An independently reviewed empty-input diagnostic used the unchanged packaged
offline validator with all original restrictions, discarded output and owned
bounded cleanup. Baseline exited 22 before input. Adding only the fixed
`MemoryPressureWatch=skip` property kept the process waiting for input and passed
the loaded offline policy gate; closing the empty input then stopped it. This is
diagnostic evidence, **not** successful enrollment or offline validation.
Systemd 255's default memory-pressure protocol supplies environment keys outside
the worker's closed environment policy. `skip` suppresses those keys; `off` does
not. Preserve the closed environment validator and cgroup memory limits; pin and
verify `skip` in all owned profiles. Primary source:
[systemd v255 MemoryPressureWatch](https://raw.githubusercontent.com/systemd/systemd/v255/man/systemd.resource-control.xml).

The first diagnostic also exposed a safe-but-spurious cleanup refusal when
`--collect` removed an already exited unit between ownership reads. Accept exact
manager `not-found` on the second read without issuing stop; still refuse any
foreign invocation or read error. Synthetic sequencing tests cover this race.
After diagnostics all three product units were independently confirmed absent,
inactive and PID zero, with no activation directory. The VM was powered off
while the profile correction was prepared.
