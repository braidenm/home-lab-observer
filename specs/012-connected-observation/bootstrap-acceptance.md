# Connected Linux public-helper VM acceptance

Date: 2026-10-26. Scope: generated helper download, verification, honest-owner
privilege transition and stopped install/reboot. Not a published release,
activation proof for these exact bytes, owner-host install or sharing proof.

## Exact reviewed inputs

Go 1.27.3 built the three-role bundle/helper from reviewed PR118 source
`d16e52109c1719524ff98757b068c5583996bd5e`, version `0.1.0-canary.1`.
The merged source `0bcbfc98c453e90c12383925148825513bfd518a` has the same tree;
this record nevertheless identifies only the actual tested PR-head artifact,
not a rebuild with a different embedded commit or a future hosted candidate.

| Input | SHA-256 |
| --- | --- |
| Canonical Ubuntu 24.04 amd64 cloud image | `04e8f2461a16a44e78a370284471f29616cdd9baf1893df25807b71f985b6c94` |
| Connected archive | `2c7b1fac4b043c5de92adaff7c088fa6f3883e90997d20013c799dca1faf2b9e` |
| Connected manifest | `a2df20e53eb32fd213f912e3ab007c42975f79ad17096e6d1d0de9d9d1ca3815` |
| Generated installer | `680f46b56488a6d8f5a011a6be45980e922b9b0b4e23e5b6ac0f1426b1ad1696` |
| Synthetic receiver | `312f5097ce91ea2cbfd3211981aac57d12b3d76d42ddc73cdcb5d9bdae06e173` |
| Production-policy preflight probe | `87e9acd2377841893d1586fb717ec4be370cb0704e9410d330cb52e931c74e28` |
| Canonical seven-file harness manifest | `adc82119e0c7b4ab76f1865d0d16ca7df81c32587f0a271e418e1b3a0d93ecfe` |

Image source: [Canonical Noble amd64 image](https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img),
published checksum independently verified before staging. Harness order:
`run_vm.py guest.sh drive_pty.py assert_guest.py preflight_probe.py
activation_guest.py bootstrap_guest.py`. The independently supplied helper pin
was not inferred solely from its checksum sidecar.

The manually invoked `--scope bootstrap` used a fresh 12-GiB copy-on-write disk,
two vCPUs, 3 GiB RAM, no NIC, host share, forwarded port or production credential.
Read-only virtual media supplied only pinned public artifacts and synthetic
fixture code. All DNS/TLS/HTTP endpoints existed inside the disposable guest.
Production server services and the existing Compose connector were untouched.

## Bounded results

Runner exit 0, `HLO_VM_CASE_PASS bootstrap` and `HLO_VM_ALL_PASS`.

| Case | Result / exact request deltas |
| --- | --- |
| Changed helper | Refused by non-root hash check; helper=1, archive=0, redirects=0, enrollment=0 |
| Changed archive | Refused before grant; helper=1, archive=1, redirects=1, enrollment=0 |
| HTTP downgrade | Refused before grant; helper=1, archive=0, redirects=1, enrollment=0 |
| Redirect loop | Refused at limit; helper=1, archive=0, redirects=6, enrollment=0 |
| Valid HTTPS cross-host redirect | Installed stopped; helper=1, archive=1, redirects=1, enrollment=1 |

Every case had exactly zero port-80 accepts and zero unrecognized requests.
The actual accept trap, not absence of an HTTP handler, measured downgrade
refusal. Receiver changes joined the owned process and closed its listeners.
No root/user download staging or bootstrap/installer/worker process remained;
refusal cases created no product account/group/unit/config/state/release.

The owner-side command ran as UID/GID 22001, used private 0700 staging and a 0600
script, checked the independent hash before sudo, preserved script identity/hash
and delivered the synthetic grant only at the actual no-echo terminal prompt.
It did not claim resistance to a malicious same-UID installing owner.

The valid install passed exact installed metadata, principal, credential, unit
and ledger assertions. Reboot left workers disabled/inactive, retained installed
identity and ledger inode, and retry refused without mutation. Bootstrap did
not start or enable either worker.

## Failures and remaining gates

Earlier reviewed source `676bf2d69f7cc2c8c6aef78f3c967bf0db95cd99` passed changed-helper
refusal but failed the following archive-case counter assertion; overall VM
result failed. The minimal image lacked the helper's external `setfacl`
prerequisite. PR118 replaced that avoidable dependency with reviewed descriptor-
anchored POSIX ACL removal and real inherited-ACL/path-replacement tests. Only
the fresh rerun above passed the complete scope. Do not turn the failed run into
positive acceptance or claim this test installed packages into the guest/host.

This synthetic service is not an anonymous real-GitHub download, provenance or
immutable-publication proof. A future candidate must run bootstrap **and**
activation against its own exact bytes; the older [activation record](activation-acceptance.md)
does not establish activation for this different embedded source identity.
Applicable installed fault/lifecycle, owner-host useful upload and ordinary
viewer grant/revoke/direct-access gates remain open. No Docker/process/log
remote view, container control, upgrade/removal or unattended lifecycle is added.
Only bounded assertions and public pins are published, never raw consoles,
credentials, terminal captures, observations or private topology.
