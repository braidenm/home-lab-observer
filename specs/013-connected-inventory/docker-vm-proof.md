# Docker-rich packaged VM proof plan

Originally approved 2026-10-29; final packaged proof completed 2026-10-30 at source
`0c7f8806224517f511292d342e361c11e404a887`. Both cases below passed first boot and reboot;
the [canonical receipt](../../docs/acceptance/connected-linux-docker/0.1.0-canary.3.json) binds the exact inputs.
Immutable prerelease `0.1.0-canary.3` is published. Hosted rollout and owner-host acceptance remain open.
The dated diagnostic sections retain earlier failures; none is substituted for this final proof.

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
  final guest loader/runtime validation passed in the exact packaged cases.
- [x] Implement closed Docker scope, guest journey, receiver/state siblings and cheap negative tests; wire only
  explicit runner/guest/PTY modes and extend the independently pinned harness manifest.
- [x] Independently review fixture source and correct all pre-launch findings; 73 focused Python tests and real Linux
  receiver race/vet checks passed at that review; later corrections and their tests are recorded below.
- [x] Independently review exact same-source four-binary package and all input/hash manifests before VM execution.
- [x] Run both fresh-overlay cases; record finite passes/failures and preserve any failure disk unchanged.
- [x] Record resource/disk/daemon/isolation/retry bound checks through the pinned harness and closed case markers;
  keep public-release and owner rollout evidence separate. This is not a representative host-capacity benchmark.

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
bytes must match the installed policy digest, and socket replacement must never silently adopt the new inode.
The original same-active-reader assertion was incorrect for an unlinked old socket; the correction below requires
its exact fail-closed exit before explicit Stop/Start recovery. Every fixed engine target is checked with lstat before the first
write, including dangling entries. The namespace probe also checks the target root device/inode immediately after
setns and before dropping identity. Focused positive/negative tests cover these gates; real VM acceptance remains pending.

## Socket replacement fixture correction (2026-10-30)

The exact-source `df43ef6e8fb22df8ffd413dcbbc8e47d138792b6` packaged journey reached the fault exercise and reported
generic `FAULT`. Source review identified an inconsistent same-active-reader requirement; the exact failing subphase
was not captured. Production requires the pinned socket's link count to remain 1; unlinking its old pathname makes
that pinned inode's count 0. Preserve this guard and CLI exit22, not a surviving
reader or automatic socket adoption. The retained failure is not complete VM/release acceptance.

Before replacing the fixture daemon socket, capture the reader's same active PID/invocation and old bound inode,
and prove `Restart=no`, `NRestarts=0`, no job. While awaiting termination, reject a different PID/invocation,
automatic restart or adoption. Require the old invocation's exact terminal metadata: `ActiveState=failed`,
`MainPID=0`, `Result=exit-code`, `ExecMainCode=1` (`CLD_EXITED`), `ExecMainStatus=22`, `NRestarts=0`, no job.
The retained failed unit must still identify the captured invocation; empty/new IDs, clean exits, signals and other
states are not evidence. A bounded `deactivating` wait is permitted only for the same old invocation, PID old/0,
exit-code/CLD_EXITED1/status22, zero restarts and no job; it is never terminal proof. Require the exact stable failed
state before and after a fresh unavailable receipt, without silently restarting the reader. Then only the supported
installer `stop`/`start` may recover it. Supported Stop preserves that already-failed reader tuple while collector
and uploader become inactive/PID0. A recovery-specific check must prove fixed disabled unit identities, no
enrollment, unchanged numeric state and exact `RECOVERY_REQUIRED` status, not pretend normal `STOPPED`. Preserve the
failed old invocation until explicit Start; no direct systemctl/reset-failed workaround or generic stopped relaxation.
Recovery must have a fresh active invocation, bind the current root socket, retain `Restart=no`/zero restarts/no job,
and produce current healthy rows at a sequence strictly beyond the unavailable receipt. Recheck the same recovered
manager/socket binding after health so historical rows, PID reuse or a later replacement cannot stand in for recovery.

Add only closed `FAULT_` subphases for initial binding, replacement, terminal reader, unavailable receipt,
supported Stop/Start and fresh recovery. Never emit raw errors, paths, status records or logs. TDD includes normal
terminal/recovery and changed PID/invocation/inode, restart, clean/signal/wrong-code exit and stale-health negatives.
Production Go, units, definition, budgets, profile admission and old numeric/process assertions remain unchanged.
Merge only after source review and exact-head CI; the next full proof needs fresh same-source protected/local bytes
and both no-overlay cases. No VM/build/publication/owner operation is authorized by this correction.

Primary sources checked 2026-10-30: [systemd 255 systemctl](https://www.freedesktop.org/software/systemd/man/255/systemctl.html)
retains failed status for inspection; [v255 service.c](https://github.com/systemd/systemd/blob/v255/src/core/service.c)
`service_enter_dead` preserves a failure state and `service_start` obtains a new invocation before resetting exit
status; [v255 unit.c](https://github.com/systemd/systemd/blob/v255/src/core/unit.c) generates invocation IDs in
`unit_acquire_invocation_id`. Do not generalize these finite255 expectations to arbitrary manager versions.

## Balance fault scheduling across the existing boots (2026-10-30)

The `ae678614ca4e4f0b27cd4cbe9cb05af6fee818dc` first boot reached its existing deadline. Read-only retained-disk
inspection, without journal replay, found the hold marker and finite receiver state: accepted12, sequence12,
healthy_sequence12, unavailable_count2, duplicates1 and all required healthy booleans true. This establishes that
the first-boot fault journey completed and held delivery began; an absent result record in the unreplayed view
does not establish the precise interrupted operation. This is not a packaged acceptance pass.

Move only the socket-replacement exercise from first boot to late second boot, after the exact pending retry,
fresh acknowledgement and role-isolation checks, before final supported Stop. Keep the stopped-reader/stale-frame
fault on first boot. Do not change either fault implementation, assertion, retry TTL, wait/command deadline,
nine-minute boot limit, two independent cases/four boots, no-NIC resource budget or numeric/process-only proof.
Socket recovery must still finish before final stopped-state, fingerprint, disk bounds and numeric checks.
Add deterministic first/reboot call-order tests. Source review, exact-head CI and new exact-source packaged proof
remain required; balancing work is not permission to skip a gate, reuse old acceptance or extend a timeout.

Local TDD evidence: the stale-fault/no-socket and executed reboot-order cases failed before the two-line move.
All80 fixture tests then passed, including pending-then-acknowledged status before socket replacement; no VM ran.

## Match the installed-root refusal diagnostic stream (2026-10-30)

Source inspection found a deterministic refusal-fixture contradiction: the Docker installer writes its fixed
pre-grant `PREFLIGHT_REFUSED` diagnostic to stderr, but the fixture discarded stderr and required that diagnostic
on stdout. Correct only the fixture: require exit22, empty stdout and the exact known pre-grant stderr line within
the existing 1024-byte diagnostic bound. Never forward captured child output. Unknown, oversized, mixed-stream or
otherwise unsuccessful results are not the expected refusal. Production installer output and preflight stay unchanged.

The generic preflight line alone does not identify the conflicting rich root: an absent Docker socket or stopped
guest TLS/DNS receiver would produce the same line if the target guard regressed. Prepare the existing pinned,
offline Docker fixture before the process-only baseline, and retain its synthetic receiver through the attempt.
Require unchanged receiver receipt bytes as well as the existing state fingerprint, then stop the fixture daemon
in `finally` and terminate the receiver in the outer `finally`. On reboot require the fixture daemon already
inactive/static/PID0 without starting or stopping it; do not hide unexpected boot activity. No new package, host
daemon operation, production admission change or timeout extension is part of this correction.
Before the attempt and after each boot, require all three Docker users, all five fixed Docker group names, all four
unit fragments and their `.d` entries absent via direct NSS/lstat checks, in addition to the manager not-found tuples.
These are fixed absence checks, not a new arbitrary filesystem reader or a claim that one generic message alone
identifies the exact rejecting guard.

Revalidate prerequisites immediately before the Docker attempt, after capturing the state and receiver receipt:
the existing bounded container proof, root-owned mode0660 single-link GID22003 Unix socket, live receiver process,
and existing bounded DNS/TLS and parent-policy probes. These checks send no enrollment/HTTP request and must not
start a service as a health check. Any failure prevents the attempt. Check both fixed rich activation directories
with lstat, without returning early after the first absent path. Preserve the primary closed failure if either
daemon or receiver cleanup also fails; cleanup failure alone must still fail the journey, never imply acceptance.

Also cover the actual CLI producer in a Linux/amd64 subprocess test with a valid synthetic build identity and nil
context. It must exit22 with empty stdout and the same exact early stderr line. Nil-context admission refuses
before bundle, host, socket, DNS or account inspection; the child isolates secret-hardening syscalls. Bound it to
five seconds with no root, candidate execution, test-only production seam or dependency addition.

TDD must exercise the real refusal function with a correct stderr result and reject a stdout diagnostic, wrong exit,
oversized/unknown stderr or extra stdout. Retain all before/after process-only file/inode/NSS/unit fingerprints,
absent Docker principals/units, no enrollment and numeric checks on both boots. Review the remaining refusal
assertions against their actual source contracts before another run. Local tests/source review are not either
packaged VM acceptance case; exact-source CI, both independently reviewed cases and release gates remain pending.

Final local source proof: correct-stderr, wrong-channel, reboot-daemon, fixed absence, immediate-prerequisite,
common-activation and receiver-cleanup regressions failed before their corrections. All84 Python fixture tests then
passed with zero skips, including receipt changes, every absence target/manager field, each unhealthy prerequisite
preventing the attempt, exact socket metadata and primary-vs-cleanup failures. The real Linux/amd64 CLI subprocess
test passed normally and under race detection; focused command vet, formatting, AST17, Bash syntax, foundation43
and diff checks passed. No candidate build or VM execution was performed by this slice. Static comparison found
no other obvious refusal-contract contradiction; that is not proof that all runtime prerequisites will succeed
in a new guest. Independent exact-source review, actual CI and full packaged acceptance remain required.

## Final executed proof (2026-10-30)

The corrected source above passed protected candidate run `41126989574`, attempt1, and independent review of all
same-source package/fixture inputs. Both fresh no-NIC overlays completed both boots, their exact four expected
markers and final `HLO_VM_ALL_PASS`. Root and independent review verified zero owned QEMU, mount and loop resources
afterward, retaining evidence disks privately. No diagnostic overlay or prior failed case was reused as acceptance.
The canonical receipt records image, harness, helper, baseline, Docker-input and public-file bindings; the pinned
harness defines the granular resource, disk, privacy, daemon, isolation, retry and refusal assertions behind PASS.

PR154 merged only that receipt after all16 triggered checks passed, then the fixed publisher produced immutable,
non-latest [release0.1.0-canary.3](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-canary.3)
and verified anonymous exact-byte downloads. The receipt is reviewed evidence, not cryptographic VM attestation.
No hosted catalog/feature activation, owner-host richer upload, delegated viewing, container control or Kubernetes
acceptance is implied. The conservative fixture ACK-barrier follow-up is tracked separately in
[issue155](https://github.com/braidenm/home-lab-observer/issues/155).
