# F1 packaged VM subset: synthetic stopped state and collector

Status: the reviewed synthetic subset passed in a disposable VM. A later packaged
quality-aware collector/used-ledger run and a stopped-state reboot/power-off
identity check also passed, as detailed below. No steady uploader or production
exchange occurred. This is partial acceptance, not a successful real enrollment/
Install transaction, uploader activation, transition recovery or completed F1.

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
bounded cleanup. Baseline exited 22 after the parent closed empty input. Adding only the fixed
`MemoryPressureWatch=skip` property sampled an active process and passed
the loaded offline policy gate; closing the empty input then stopped it. This is
diagnostic evidence, **not** successful enrollment or offline validation. That
initial sample had no sustained-alive check and did not establish the sole cause.
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

## Third execution and startup race (2026-09-17)

Source `855cc49` package archive SHA256:
`3d17d7ae20bf7b8107076373a1e434cbc82db964250eea38707403f4ea315c0e`;
manifest SHA256:
`0c7f4fc05d9ad69d43ceec5d5bc92109d13b352a6862fbc692f30bfa336b67e7`.
Full subset again failed at offline validation after 1.96 seconds. A separate
empty diagnostic root, with no credential/state/handoff binds and a test-only
executable under the same offline syscall and identity profile, confirmed
process hardening, exact runtime identity and closed environment checks passed.
Only fixed diagnostic codes were reported. This probe is not packaged acceptance.

A subsequent no-input invocation of the unchanged packaged validator captured
the actual manager transition: loaded, exact nonce/Transient identity, inactive,
empty InvocationID. A following read supplied the real active invocation; the
process remained alive through a 250ms held-input dwell and passed loaded policy.
The prior immediate ownership predicate had refused the legitimate pending
state, then closed stdin itself. Exit 22 following that close was therefore not
evidence of a pre-input worker boundary failure.

The correction waits boundedly only for the exact nonce-owned inactive/activating
unit to acquire an invocation. Malformed IDs, foreign units and ended states are
refused; a known invocation cannot be cleared/replaced; input remains withheld
until nonempty active/activating identity plus the existing policy/recheck gate.
Deterministic tests cover pending-to-ready, replacement, malformed/foreign state,
cancellation and timeout. All units were confirmed absent/inactive/PID zero
after diagnostics and the VM powered off pending the fresh rerun.

## Fourth execution: reviewed subset passed (2026-09-17)

Source `5e90b5a` package archive SHA256:
`a70d8342f8bc673e9cdb3ee061c9c54c7142b3decf0a57f86ae7e6d4c1450ec1`;
manifest SHA256:
`08c989805a0e013fa958774dfc01780c0e41f4ed7f0a02ab24e9e52388b13924`.
The actual packaged fixture passed in 3.92 seconds, including deferred cleanup,
not merely its pre-cleanup success log. Proven subset: exact bundle preflight,
dedicated principals, synthetic owner-private seeding, unchanged packaged offline
enrollment validator under the loaded manager gate, exact directory and database
inode-preserving ledger promotion, unchanged packaged pristine-ledger validation,
and packaged collector current-invocation COLLECTING plus shared handoff read
under the uploader identity. Numeric overview remained explicitly
`HOST_DATA_UNAVAILABLE` because no filesystem-coverage proof was asserted.

Independent post-test manager reads confirmed both steady units inactive,
disabled and PID zero; enrollment unit absent/inactive/PID zero; activation
directory absent. No steady uploader start, HTTP exchange, real enrollment grant
or production credentials were used. The temporary guest was shut down for
removal after evidence capture. This **does not** complete F1: real enrollment and
promotion transaction acceptance, complete uploader activation/FD/TLS rendezvous,
compatible code transitions, crash/reboot/power-loss recovery and live canary
remain separate unexecuted gates.

Cleanup completed and independently verified: the exact owned domain, NAT
network/filter and bridge, guest/base/seed/NVRAM images, staging directory and
per-domain diagnostic log were removed; the ephemeral local private key was
deleted. No failed overlays were retained. Existing host networks, runner
resources and production workloads were not modified by these fixtures. The
test VM is no longer running or defined. Only sanitized repository evidence and
local non-secret build/test artifacts remain; the temporary guest state and key
are intentionally not recoverable.

## Quality-aware collector and used-ledger follow-up (2026-09-18)

The draft collector now publishes the native quality-aware host contract. The
next packaged run therefore validates that exact canonical contract and server
binding, requires Linux CPU and uptime availability plus useful RAM (complete or
degraded), and refuses any assertion of complete filesystem coverage or a known
whole-host filesystem total. The old legacy-unavailable assertion is removed;
historical evidence above continues to describe the older package truthfully.

The acceptance predicate has Linux-native fixture tests covering useful partial
data, false complete coverage, foreign server binding, invalid metric values and
the legacy response. These tests alone do not prove hosted transport.

That fresh run also exercises the unchanged packaged `validate-existing-ledger`
mode twice through the real offline manager profile. It compares private results
to the synthetic pristine record fingerprint, retains anchored ext4 directory and
database identity across both opens, and checks that the named paths still point
to the promoted objects. Fingerprints and filesystem identifiers never enter test
output. This adds no steady uploader, HTTP request, steady-uploader activation or real
credential.

The fresh package from source `ab9d7d9` (current branch later advanced with
documentation and fixture changes only) was built as Linux amd64 with Go 1.27.1,
CGO disabled. Its archive SHA256 is
`513d0eecc0a0ed40cc989e634a2a5ba811c31bf5c65910b42d54c70f81442116`;
canonical manifest SHA256 is
`f43f9672f5e1683945426f70e1e9d9f435d0da50a8dcf209b96e6b7206fdc0e9`.
The VM test binary SHA256 is
`340aae413b7fbcfb6e3e3e4f0b2b40a5a6c8d8d6a917971504c2dcab19a8b7a2`.
The same packaged fixture passed on two fresh Ubuntu 24.04/systemd255 QEMU guests,
including the quality-aware current collector output, explicit partial filesystem
coverage, both offline existing-ledger opens, physical identity anchoring and
logical pristine-data equality. The second guest completed in 4.45 seconds.
Both units were then inactive, disabled and PID zero; enrollment unit and
activation directory were absent. The independent VM network filter allowed only
the exact task routes and denied unrelated public/private destinations. No
production request or real grant was made.

The second guest ran a separate test-only reboot witness binary (SHA256
`c829e4b2f3bae3e4107ff446c71f2883bd8ea0de037714febbfb65400278ee07`).
It recorded root-private canonical config, ext4 directory/database identity and
the logical synthetic ledger fingerprint while both workers were stopped and
disabled. After a normal VM reboot, then after an abrupt VM power-off and start,
the boot IDs changed and the exact witness comparison passed. This proves the
stopped synthetic state and ext4 identity survived those two events on this guest.
It does **not** test an in-flight refresh journal, failed upload recovery,
credential promotion under power loss, a filesystem restore/clone, a running
uploader, real enrollment or owner-visible hosted canary. Those gates remain open.

After evidence capture, both exact disposable domains were powered off and
undefined, their owned images/staging files and task-only network filter were
removed, and the temporary local SSH private key was deleted. Independent host
inspection found no remaining defined guest or task-owned staging path. The
guest ledger is intentionally not recoverable; the sanitized test record and
non-secret local package artifacts remain.
