# Connected Linux activation acceptance

Date: 2026-10-26. Scope: explicit-start beta, not unattended or general availability.

## Exact inputs

The three-role package built with Go 1.27.3 from merged commit
`4b13efd43f3ff063e95bd4c2b7609e223a272853`, version `0.1.0-canary.1`, passed the
manually invoked `--scope activation` disposable Ubuntu 24.04/systemd 255/amd64
journey. The independently reviewed PR #113 source tree is identical to this
merged tree; the merged build has its own embedded commit and artifact pins.

| Input | SHA-256 |
| --- | --- |
| Canonical Ubuntu 24.04 cloud image | `04e8f2461a16a44e78a370284471f29616cdd9baf1893df25807b71f985b6c94` |
| Connected manifest | `eceee2960909e3f8143e4c906998979a4ed6a7dd0260bef170f6641e7d234845` |
| Connected archive | `90607e974e56c08ce370e6a7e229f40bc0fa8417c23b2e91fae7053084d27c41` |
| Synthetic receiver | `4789ec2cabf7ad5895a4f7ca3329b91e13b1a7ecc8067cf81225304de4edf9cd` |
| Exact production-policy preflight probe | `095b563e1bc389a9fbc42a0e9ddd131cd36db8f0a18f677ff72aa23a1a6aa92c` |
| Canonical six-file harness manifest | `9162710d6b76cff4f62454ca140df40c26249416e62e427b686e0440e3e5bf43` |

Image source: [Canonical Noble amd64 cloud image](https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img),
with its published checksum verified before staging. The harness digest binds,
in order, `run_vm.py guest.sh drive_pty.py assert_guest.py preflight_probe.py
activation_guest.py`. The VM had two vCPUs, 3 GiB RAM, a fresh 12 GiB copy-on-write
disk, no NIC, no host share and no production credentials. Synthetic DNS/TLS and
receiver traffic stayed inside the guest. The original server services were not
stopped, replaced or enrolled by this test.

## Bounded results

The runner ended with `HLO_VM_CASE_PASS activation` and `HLO_VM_ALL_PASS`.
The actual packaged commands and receiver assertions established:

- A fresh install passes exact preflight and leaves both workers stopped.
- Explicit start completes the invocation-bound activation checks and receives
  `ACKNOWLEDGED_FRESH` with useful quality-aware native host metrics.
- Stop joins both workers; start reconnects with a strictly greater sequence.
- Reboot leaves both units disabled/inactive. A new explicit start reconnects
  with a sequence greater than the pre-reboot value.
- Installed configuration/credential fingerprints and the ledger inode remain
  unchanged across these operations; ledger file bytes stay below 1 MiB.

Only fixed allowlisted assertions and public artifact pins are recorded. No
credential, grant, observation body, raw console or private topology is published.
The test source is [the manual harness](../../scripts/fixtures/connected-first-install/README.md).

## What this does not prove

The historical [stopped-install/power-cut record](first-install-acceptance.md)
uses a different source and image; those three cases were **not** rerun here.
This record does not establish production endpoint ingestion, owner registration,
viewer sharing/revocation, an owner-host install, or anonymous public release
availability. Those are separate release/Platform Demo acceptance gates.

Response-loss/revocation fault acceptance, endpoint refresh, compatible code
rollback, removal and unattended boot/crash recovery remain open. This is not
permission to enable units directly or claim those lifecycle operations work.
The beta intentionally requires an explicit verified `start` after reboot and
keeps `Restart=no`. Docker/process/log remote views and controls are not included
in this numeric-host connected profile. Windows/macOS connected parity is not
implied. A future public release must pin the exact tested artifact inputs, not
substitute a rebuild from a later source commit.
