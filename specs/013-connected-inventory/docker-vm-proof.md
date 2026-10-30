# Docker-rich packaged VM proof plan

Date: 2026-10-29. Status: approved engineering scope; implementation/input verification in progress. No Docker-rich
VM acceptance, public release or owner-host acceptance is claimed. The separately accepted process-only gate does not prove Docker.

## Closed scope and inputs

Add explicit `rich-docker` manual acceptance only; existing default, `all`, numeric and process-only scopes retain
their current semantics. Reuse the existing verified Ubuntu24.04/systemd255/amd64 base and no-NIC QEMU runner with
2 vCPU, 3 GiB RAM, 12-GiB private overlay, read-only payload, no host filesystem share and bounded serial markers.
Each case gets a fresh overlay. Keep failures for read-only diagnosis; cleanup only proven owned fixture paths.

Independently pin the same-source numeric package, complete four-executable Docker package/manifest/checksums,
receiver, prerequisite probe, expanded harness, Docker inputs and source-built synthetic image before VM execution.
The Docker package must use its own exact profile/definition; never accept a three-binary manifest as equivalent.

Use Docker's signed Noble stable repository, not an unsigned static-archive first-download hash. In a new private
local staging directory verify the fixed Docker release primary-key fingerprint
`9DC858229FC7DD38854AE2D88D81803C0EBFCD88`, then the `InRelease` signature, its exact Packages SHA-256/size and the
closed package records before fetching/extracting any binaries. The out-of-band fingerprint is retained in
[Docker's immutable official installation documentation](https://github.com/docker/docs/blob/be718ce24184c8994a42bb327c5ac97c7c669f10/engine/install/ubuntu.md#L94-L103);
the [current Ubuntu instructions](https://docs.docker.com/engine/install/ubuntu/) identify the current official
key and Noble repository. Record signer, metadata hashes/date and every package's version/architecture/size/hash.
Do not change the workstation's APT sources, trust store, packages, Docker daemon or network.

The [verified input record](docker-vm-inputs.md) binds Docker CE and CLI
`5:29.9.2-1~ubuntu.24.04~noble`, containerd.io `2.4.2-3~ubuntu.24.04~noble`, amd64. CE requires containerd.io>=2.1.5,
CLI, iptables, nftables, libc6>=2.34 and libsystemd0; containerd requires libc6>=2.38 and libseccomp2>=2.5.0.
Verify the exact guest dependency/tool closure without network; retain AppArmor/seccomp rather than disabling them
to hide a missing prerequisite. Do not run package maintainer scripts or auto-start a default daemon during staging.
Any additional Ubuntu package needs its own signed metadata and an explicit reviewed manifest update.

The daemon runs only in the disposable guest with dedicated data/exec/pid paths and fixed `/run/docker.sock`.
Disable bridge, iptables, ip6tables, forwarding, masquerading and userland proxy using the
[documented daemon options](https://docs.docker.com/reference/cli/dockerd/). Independently prove only loopback and no
IPv4/IPv6 default route before/after preparation; each container uses `--network none`, no mounts/capabilities,
read-only root, no-new-privileges, bounded memory/CPU/tasks, no restart and no logging. Do not install/restart Docker
on an owner/CI host. Validate the daemon's actual advertised min/max intersection with the adapter's `1.41..1.45`;
do not assume compatibility from its marketing version or override negotiation.

Reuse the existing CGO-disabled Go sleeper and scratch Dockerfile; pin the image input and executable from the
reviewed source, without pulling an external image or using a network-enabled build. No synthetic raw observation,
credential, key or daemon output is copied into serial logs or host acceptance artifacts.

## Two independent overlays

1. **Numeric plus Docker-rich:** install numeric stopped and retain its deep byte/inode/unit/process fingerprint.
   Install Docker-rich into absent rich roots with its own synthetic one-use admission. Verify all three locked
   principals, exact service-only groups, schema3/kind2/policy binding, four binaries, disabled/inactive units and no
   enrollment transient. Explicit Start must prove the same-invocation reader barrier and root socket/FD/namespace
   audit before any Engine I/O. Receive real running and exited rows, warmed nullable CPU/memory, foreign process
   rows and a valid body above16KiB; stopped metrics remain null. Secret/raw-ID canaries must not escape the frame,
   upload, status or bounded evidence. Prove actual collector socket/credential/activation denials and uploader
   process/frame/socket exclusion. Exercise helper stale/absent and daemon failure/replacement without auto-adoption;
   only supported Stop/Start may bind a replacement. Preserve numeric state throughout. Then lose the response,
   stop all three workers, fingerprint the exact pending request, reboot inside the unchanged TTL and prove the
   identical sequence/digest retry before a fresh acknowledgement. Check resource/disk/log bounds and exact cleanup.
2. **Existing process-only rich:** install the separately pinned process-only package stopped, fingerprint all rich
   configuration/policy/credential/ledger/release/NSS/unit state, then attempt Docker-rich installation without a
   grant. Require pre-mutation refusal, no new reader/frame principals, no activation/enrollment and no changed
   existing bytes/inodes/units. Do not invent migration or two simultaneously installed rich roots.

Reuse bounded existing numeric snapshots, process sleepers, no-NIC reboot guard, nanosecond pending-age checks and
exact retry gates. Keep new Docker profile/receiver admission explicit rather than mutating old module globals.
The receiver validates raw bodies only in memory and persists closed hashes/sequences/counters/booleans, not rows.

## Implementation and acceptance tasks

- [x] Read current fixture/Spec013/ADRs and primary Docker input/network documentation; identify reusable seams.
- [x] Verify signed repository/key/Packages chain and the three exact package downloads in private staging
  ([input record](docker-vm-inputs.md)); no host package/runtime changes.
- [x] Freeze researched guest dependency closure and separately pinned source-built synthetic image input;
  actual guest loader/runtime validation remains required.
- [x] Implement closed Docker scope, guest journey, receiver/state siblings and cheap negative tests; wire only
  explicit runner/guest/PTY modes and extend the independently pinned harness manifest.
- [x] Independently review fixture source and correct all pre-launch findings; 73 focused Python tests and real Linux
  receiver race/vet checks pass. The separate artifact review below is still required before execution.
- [ ] Independently review exact same-source four-binary package and all input/hash manifests before VM execution.
- [ ] Run both fresh-overlay cases; record finite passes/failures and preserve any failure disk unchanged.
- [ ] Record measured resource/disk/daemon/isolation/retry evidence and remaining public-release/owner rollout gates.

Cheap fixture tests are not VM acceptance. A real Docker VM pass does not authorize owner installation, publishing,
remote actions or Kubernetes, and is not a substitute for hosted owner/delegate authorization and responsive UI proof.

## 2026-10-30 reader startup diagnostic correction

The first exact-source packaged run failed closed before reader startup completed. A separately pinned diagnostic
harness retained the installed artifacts unchanged and narrowed the result to `START_RECOVERY_READER_STARTUP`.
Neither run is accepted; original failure disks and bounded evidence remain preserved. The coarse phase does not
identify the cause, and teardown can remove causal runtime files. No safety assertion or sandbox is relaxed.

The next bounded correction must expose only code-owned `READER_` startup phases: initial state, activation-directory
pinning, response clearing, service start, both paused audits, request construction/publication, response wait/match,
commit publication, status wait and final identity. Cleanup failure must preserve the original phase and sanitized
primary sentinel while conservatively requiring recovery. Existing CLI format and exit22 remain unchanged; arbitrary
OS/manager errors, paths and record contents must never become diagnostics. Fault-inject each operation, cancellation,
replacement, malformed responses/status and cleanup failure before a fresh reviewed artifact/VM attempt.

Local source proof: the new seam/cleanup tests failed against the preceding implementation, then passed with the
diagnostic correction. Full Linux Go tests/vet, focused installer/command race checks, 75 Python fixture tests,
Python syntax, guest Bash syntax and the 43-file repository foundation check passed. Independent review, exact-head
CI and a new pinned packaged run remain required. This changes diagnostics, not the unknown underlying startup cause.

Pre-launch independent review found five fixture correctness gaps, corrected before execution: the frame grant is
service-only (collector NSS remains SharedGID-only and the persistent frame group is empty), actual live worker
UID/GID/groups are checked separately, the fixture daemon is exactly static rather than disabled, pending policy
bytes must match the installed policy digest, and socket replacement must leave the same active reader invocation
bound to the old inode until explicit Stop/Start. Every fixed engine target is checked with lstat before the first
write, including dangling entries. The namespace probe also checks the target root device/inode immediately after
setns and before dropping identity. Focused positive/negative tests cover these gates; real VM acceptance remains pending.
