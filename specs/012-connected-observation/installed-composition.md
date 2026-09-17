# F1: first installable Linux connected worker

Current implementation/evidence checkpoint: [installed-implementation.md](installed-implementation.md).

Status: accepted for implementation after independent review, 2026-09-11. This is an end-to-end implementation plan, not activation approval or
evidence of installed isolation. Complements ADR 014 and the existing installed-acceptance research; does not
replace its negative tests. Requires reviewed D2d persistence and C2 pacing to land before integration.

## Deliverable and supported scope

One verified Linux release bundle and one explicit elevated installer provision two system services, enroll once,
publish numeric observations, and produce a receiver-acknowledged status. No further disconnected helper phase.
Start with the exact Ubuntu 24.04/systemd 255 amd64 disposable-VM image proven by acceptance; do not advertise
all distributions merely because their manager version is newer. ARM64 support requires equivalent native evidence.
Define a separately versioned connected-bundle manifest/profile naming exactly these executables and unit/config
templates, checksums and build identities. Do not add optional files to or relax the existing exact-content native-v2
manifest validator. Verification rejects missing, extra or mismatched connected artifacts before privileged mutation.
Existing local Observer and legacy Compose installations stay untouched. Windows/macOS remote installation,
Docker/process/log export, container actions and Kubernetes are not enabled by this slice.

No product question is needed to implement the conservative defaults below. Independent review accepted the
bounded canary network residual and installed-state transition with the effective-policy and IPC prerequisites below;
a failed proof blocks that profile, not
a request to the owner to disable safeguards. Broader distribution/endpoint support is a later release decision.

## Executable composition, not a mode of the rich observer

| Release executable | Composition and allowed responsibilities | Must not initialize/import |
| --- | --- | --- |
| `observer-connected-collector` | numerichost -> remoteprojection -> sharedhandoff writer; serial 15-second collection; bounded fixed diagnostics | rich collector/scheduler, process/network/container/log adapters, enrollment, HTTP, credentials, upload ledger |
| `observer-connected-uploader` | installed READY/credential validation -> sharedhandoff reader -> D1 OpenExisting -> C1 -> C2 -> platformtransport; separate explicit one-shot enrollment subcommand composes D2b/D2c/D2d | any collector, host/process/log/container observation or local API |
| `observer-connected-install` | fixed-layout preflight, verified artifact provisioning, typed enrollment handoff, root promotion, exact service operations and fixed status | arbitrary URLs, shell snippets, plug-ins, package installation, generic file-copy or command-execution API |

Separate `cmd` roots are mandatory: branching in the existing rich command's main does not prevent imported package
initializers. Production `go list -deps` assertions enforce explicit internal-package allowlists for both workers.
The collector's gopsutil common utilities currently import os/exec; absence of arbitrary execution also needs the
installed capability/syscall/filesystem proof, not an import-graph claim. Do not reuse the rich scheduler just for
its loop. Reuse its tested serial/cancel/join conventions in a small command-local collection runner.

Workers accept only a fixed root-owned installed-config path; no configurable provider, origin, path, proxy,
interval or command arguments. Bindings and exact numeric UIDs/GIDs are data, not executable text. Enrollment
receives the one-use grant by a bounded private handle/no-echo prompt, never argv, environment or a copied command.
Nonsecret instructions can be copy-pasted. The installer uses exact executable paths and argument vectors for a
small allowlist of required system tools; it does not pass user content to a shell or expose these operations remotely.

## Principal and installed-state transition

Use dedicated static collector and uploader service accounts with no login, no extra group memberships and a
dedicated shared-read GID. Refuse preexisting unknown users, groups, unit names and paths. Numeric identity is
recorded in root-owned metadata and verified on every start; names alone do not confer trust. Root remains trusted.

1. Preflight is read-only. Before the first installation mutation, acquire an installation-wide root-owned exclusive
   lease at a fixed validated coordination location; creating that validated lease is the only bootstrap operation.
   Never unlink/recreate its inode to bypass contention. Keep it through provisioning, promotion and service start;
   refresh, rollback, recovery and uninstall acquire the same lease before any mutation and hold it through completion.
   The explicit elevated install verifies the release, baseline and exact target paths, then
   exclusively provisions a root-owned installation directory in PREPARING state, disabled units and private subtrees.
   This record is not D2d READY and never permits worker startup. No automatic sudo or repair of unknown resources.
2. Run the one-shot enrollment executable as the eventual uploader UID in an owner-private 0700 staging directory.
   Here "owner-private" means the D2d store's service principal, not the interactive login account. The installer
   gives it only a bounded private grant handle; the collector is not started and never receives the grant. Apply the
   reviewed endpoint/root policy to this process too. D2b performs at most one exchange and D2d durably reaches READY.
3. Join the enrollment process. A fixed validation operation under that same UID calls D2d OpenReady, validates the
   exact expected binding and pristine ledger, and returns only bounded canonical enrollment data through a private
   pipe to its root parent. No machine uploads before promotion. A non-pristine ledger or uncertain/partial enrollment
   is recovery-required; do not reconstruct sequence one. The installation-wide lease remains held.
4. After all child handles are closed, root removes service-UID traversal to the staging parent and validates pinned
   source objects. This freezes the cooperative store; the threat model does not claim to revoke already-open handles
   held by a hostile same UID/root. Only the installed tools may run as these dedicated principals. Root promotes the
   credential record into a new root-owned 0600 source directory 0700, bounded and exactly bound to both IDs. It moves
   the existing closed `ledger/` directory within the same filesystem to the final uploader-private state location,
   preserving its complete bytes/ownership, not copying only the SQLite file or calling Provision again. Sync both
   changed parents. Root validates each final handle/metadata before publication; no symlink following or recursive chown.
5. Reopen the relocated ledger through D1 as uploader and prove unchanged binding/pristine record, then close it.
   Only after credential, ledger, config, units and immutable artifact identity are durable may root publish
   `INSTALLED_READY` atomically and sync its parent. Its versioned fixed metadata binds both IDs, principals, artifact
   identity, layout version and endpoint-policy generation. It contains no secret and is not writable by either worker.
6. Start collector and uploader only after installed negative acceptance. Steady-state startup requires root
   INSTALLED_READY, exact LoadCredential record binding and D1 OpenExisting. D2d staging is neither mounted into the
   worker nor used as the permanent installed trust authority. Its old READY is not sufficient to start a worker.

Crash at any promotion boundary leaves PREPARING and no start permission. Preserve bounded evidence and report
RECOVERY_REQUIRED; first release offers explicit revoke/new-enrollment cleanup, not automatic promotion resumption.
If final InstalledReady publication reports an uncertain sync, require explicit revalidation of all final objects;
do not blindly retry enrollment. Keep partial credential copies root-inaccessible to workers; remove only the exact
owned staging artifacts after verified successful promotion/owner cleanup. Secure erase is not promised.

This is a new narrow installation transaction, not permission weakening in D2d. Do not open the same SQLite state
through extra raw handles while D1 holds it. Do not migrate a previously running registration through this fresh-install
path. Backup restoration of an older ledger still requires revocation/re-enrollment, as D1 cannot detect every rollback.

## Endpoint policy: no runtime DNS privilege in the first canary

Recommended small supported model: the reviewed release selects one canonical HTTPS origin and its fixed routes.
Root preflight resolves that origin using the host's configured resolver, validates a bounded set of globally routable
unicast addresses, and tests TLS hostname validation to the selected destinations. Reject loopback, private, link-local,
multicast, unspecified and other reserved destinations for this profile. No arbitrary owner origin or IP override.
The policy stores only exact /32 or /128 destinations and an exact hostname mapping in a minimal read-only `/etc/hosts`.
The uploader uses the ordinary transport with certificate/hostname verification; it does not replace HTTPS URLs by IPs.
Use a fixed hosts-first pure-Go resolver setup; no functional external or host-loopback DNS resolver is supplied.
Prove resolution makes no DNS calls and a missing hosts entry fails closed under the final root and packet filter.

Apply `IPAddressDeny=any` plus those precise allows to the service, with no inherited socket activation/descriptors.
Before activation and refresh, inspect the effective unit, drop-ins and every ancestor slice's IP allow/deny policy.
An inherited allow can override deny-all: refuse any effective allow broader than the reviewed exact destination set,
including a broader CIDR, symbolic allow or unrelated exact address. Refuse unrecognized policy composition instead
of assuming the generated unit controls the entire effective policy. Inspect exact resolved properties, not only the
packaged unit file; test hostile ancestor/drop-in allows. Never modify a global slice to make this profile pass.
Actual IPv4 and IPv6 deny/allow probes are mandatory on the final supported kernel. Do not change a global slice or
the host firewall. This reuses systemd cgroup filtering rather than adding a privileged network daemon or proxy.
The existing WSL dual-stack probe demonstrates feasibility only; it does not certify this installed policy.

**Residual, requiring review:** IP filtering is not a TLS-origin or destination-port sandbox. A compromised uploader
could contact other ports or tenants sharing an approved public address; TLS/fixed routes constrain the honest adapter,
not compromised code. No blanket CDN subnet allows are permitted. The canary protects host/local networks and limits
egress to exact reviewed addresses; it must not claim exclusive origin egress. If exclusive-origin isolation is required,
this model is rejected and a separately reviewed enforcement design is necessary; do not silently add a proxy.

Pinning trades automatic CDN/DNS rotation for a smaller privilege boundary. Provide one explicit root endpoint-refresh
operation: stop/join uploader, resolve and validate the same compiled origin, stage a complete hosts/filter policy,
sync and switch one generation, re-run connection/denial checks, then start. Failure stays stopped or preserves the
previous complete policy without widening it. No background root resolver/updater in this first canary. UI/help must
identify endpoint-policy staleness as one possible cause of a fixed connectivity failure, not assert a DNS diagnosis
from every timeout. This is an operator-managed canary constraint, not the general-purpose one-click release promise.

For the unattended/headless release, extend that same bounded refresh operation with a reviewed root-owned systemd
oneshot/timer. It resolves only the compiled control-plane hostname, accepts at most a small fixed address count,
validates public destinations and TLS, and coordinates stop/join, complete generation application and readiness
before returning. A failed refresh never leaves hosts entries and filters from different generations or broadens
access. Bound runtime, retries and diagnostics; use timer jitter and document refresh interval/expiry behavior from
the control-plane DNS contract. No generic command daemon, arbitrary resolver/hostname or new networking dependency.
Do not claim this future timer exists in the manual canary; successful refresh must also accommodate CA/artifact
updates through their separate verified update path. Avoid stopping a healthy uploader merely because a lookup fails
when the previously verified complete generation is still usable within its defined validity policy.

A dedicated network namespace plus veth/routing and per-destination firewall could additionally constrain ports and
separate loopback, but needs privileged link/address/rule lifecycle, conflict-free naming, dual-stack routing and DNS
maintenance. It does not by itself distinguish tenants on a shared destination IP. That larger operational surface
is not justified for this first exact-IP canary unless the installed filtering proof fails or review requires port
isolation. Prefer the small systemd refresh path for subsequent reliable headless operation; neither option is a
substitute for testing the actual supported host. Do not ask users to select firewall internals.

## Actual filesystem and manager profile

Uploader RootDirectory contains only the exact static executable, root-owned public install metadata, reviewed system
CA bundle, minimal hosts/resolver files, read-only shared handoff, private D1/diagnostic paths and systemd's service
credential mount. LoadCredential names the root-owned credential source; the executable reads the bounded record from
the manager-provided credential directory and correlates both IDs. Do not relax the transport trust constructor.
CA updates use the stopped validated installation-update path, never an uploader-writable trust store. No shells,
host home/root directories, procfs/sysfs, Docker socket, journal socket or local API socket. Verify final mounts, not
just unit directives: some systemd hardening options implicitly create API filesystems. Do not add procfs to silence
optional Go runtime cgroup-discovery failures. Standard output/error are discarded; core dumps are disabled.
Restrict uploader socket creation to AF_INET and AF_INET6 only. Do not allow AF_UNIX for logging or notification;
test both filesystem and abstract AF_UNIX denial, because an abstract socket is not hidden by RootDirectory.
Apply `SystemCallArchitectures=native` and explicitly deny `socketpair` for uploader and enrollment execution:
RestrictAddressFamilies filters socket() only, and a UNIX datagram pair can otherwise reach an abstract peer.
The abstract-socket fixture must exercise both socket() and socketpair()-based paths. Deny `io_uring_setup`,
`io_uring_enter` and `io_uring_register` on both workers/enrollment when unused by the pinned static Go/SQLite
runtime, and prove that normal epoll/futex-based collection, TLS and durable ledger operations still work. This
closes an alternate kernel socket-operation path rather than assuming syscall family filtering covers io_uring.
Do not claim these restrictions are compatible/enforced until the exact packaged profile passes its positive and
negative tests; if a dependency actually requires one, revisit the boundary instead of silently allowing it.
Both workers use PrivateIPC plus explicit denial of unnecessary System V/POSIX IPC syscalls under the tested syscall
policy. Do not treat chroot, PrivateIPC or socket-family filtering alone as complete IPC isolation. Prove denial
against owned IPC fixtures and inspect inherited descriptors; the workers receive no inherited IPC/network sockets.

Collector has PrivateNetwork plus denied IPv4/IPv6 socket creation, no inherited network descriptors, empty capability
sets, NoNewPrivileges, read-only host view apart from handoff and bounded diagnostics, and protection against access
to uploader process memory/environment/descriptors and all credential/state paths. Its read-only host view is not a
general host-file confidentiality guarantee. Use ProtectProc=invisible with a proven numeric-source view; do not use
ProcSubset=pid, which hides needed numeric inputs. No privileged Docker or host-control mounts are added.

Set explicit resource and stop limits for both processes; initial tested budget targets are 128 MiB MemoryMax and
64 TasksMax per worker, 15-second TimeoutStopSec with kill of the entire owned control group on escalation. Tune only
from synthetic/installed acceptance evidence, not by disabling limits. Syscall policy must permit actual Go threads,
TLS entropy and SQLite synchronization. Prove denied execution/ptrace/mount/device actions rather than claiming an
untested allowlist. Use Type=exec initially: process startup is deliberately not end-to-end readiness. Terminal C1
auth/conflict/recovery results exit with distinct fixed non-restart codes; transient HTTP failures remain in C2.
Any configured crash restart retains C2's full 60-second startup cooldown and bounded manager restart burst.

Filesystem completeness defaults false. A sandboxed namespace often cannot represent every host mount. CPU/memory/
swap/uptime may remain locally measurable, but the existing numeric-host/v1 `projectOverview` makes the **entire hosted
overview HOST_DATA_UNAVAILABLE** when filesystem input is unavailable or incomplete. The first no-coverage-proof
canary therefore proves identity, freshness and delivery only; it does not provide hosted CPU/memory charts with
only disk omitted. Do not set coverage true just to make charts appear. Enable the existing complete hosted numeric
overview only after independent eligible-mount/capacity reference comparisons for the actual namespace profile,
including hidden mounts and drift invalidation. Never expose a user checkbox that bypasses this proof.
Alternatively, a subsequent separately specified quality-aware per-field remote profile, receiver/backend projection
and frontend view can support partial numeric data. That end-to-end contract change is not implemented or implied
by this installed composition slice.

## Readiness, operator experience and rollback

Provide a fixed local status command reading bounded status records, not an HTTP listener or arbitrary file endpoint.
States distinguish PREPARING, ENROLLED, COLLECTING, WAITING_FIRST_UPLOAD, PENDING, RETRYING, RATE_LIMITED,
CREDENTIAL_REJECTED, RECOVERY_REQUIRED and ACKNOWLEDGED_FRESH/STALE. Compose C2's closed outcomes/counters and
handoff age/validation with D1 facts; no raw errors, payloads, credentials or unbounded per-identity metrics. Atomic
bounded records are not new durable delivery authority. Print remote acknowledgement time separately from process
liveness. The first successful connection can take at least one minute plus collection/network time.

Platform Demo's logged-in owner server page should show received freshness, the numeric-only profile and unavailable
**whole numeric overview** when coverage is unproven, plus exact install/status/stop/restart/refresh/revoke/uninstall
instructions. Do not describe this initial canary as a usable partial hosted system dashboard. An offline worker
cannot send its current local failure status; do not invent remote live diagnostics from an old snapshot. Existing
server authorization stays unchanged. No new rich-data or management entitlement is implied by this installation.

Keep at most one handoff/stage (32 KiB), D1's 1 MiB bound and existing 10 MiB/seven-day diagnostic limits per worker;
no host-wide journal retention changes. Status files are fixed and bounded. Test hours of synthetic retry/idle output
without storage growth. Stop/join workers before closing borrowed dependencies or moving artifacts.

Uninstall/recovery never targets an inferred broad path: verify exact owned installation identity, stop/join both
services, revoke the separate registration, and offer explicit removal of owned state. Preserve evidence by default
on failure and disclose that deleting credentials/ledger is not recoverable service rollback. Rollback of code selects
the prior immutable compatible artifact with the same ledger; incompatible state refuses startup. The legacy connector
remains working and is neither overwritten nor retired by the canary.

The first canary's `uninstall` is deliberately detach-only: under the installation lease, validate exact
owned artifacts/unit bytes, stop and join both units, disable them, move the exact unit files into a root-owned
quarantine, reload the manager and publish a bounded completion marker. Keep credential, ledger, accounts,
artifacts and diagnostics. Report explicitly that the separate registration must be revoked in Platform Demo;
local uninstall cannot assert remote revocation. A partial detach reports recovery required and preserves evidence.
Recursive state deletion and account recycling are not performed by this initial command.

## Implementation and evidence gates

Deliver one coherent installed-profile PR with command composition, installer transaction, unit templates, operator
instructions and synthetic fixtures, split into reviewable commits. Do not declare it done at command compilation.

1. PR tests: dependency allowlists, typed config and grant handling, promotion crash boundaries/foreign principal/
   collision/partial-state refusal, same-inode ledger preservation, bounded diagnostics, serial cancellation and fixed
   terminal exits. Existing C1/D1/D2c/D2d/E1/E2/C2 tests remain green; race and native CI stay below 15 minutes.
   Include installation-wide lease contention before mutation across install/refresh/uninstall, connected-manifest
   extra/missing-file refusal without native-v2 policy changes, and inherited broad IP allow rejection.
2. Bounded manual disposable-VM test: install exact verified bundle with synthetic receiver; real separate principals,
   final mount view, no inherited sockets, denied host/local-network/unauthorized-sink access, allowed TLS upload,
   abstract AF_UNIX and cross-principal IPC denial, hostile ancestor/drop-in IP allow rejection,
   wrong CA/hostname, missing host mapping, endpoint-policy refresh, collector coverage failure, drift refusal,
   kill/restart, response loss, ENOSPC, revoke and exact rollback. No public PR code runs on private infrastructure.
3. VM reboot/power-loss and exact filesystem durability evidence; native target coverage. WSL syscall/namespace probes
   are useful early evidence, not a physical-host or ARM64 substitute. All installed tests above are currently unexecuted.
4. Only after review and those gates: verify the owner's exact server/resource scope, install a separate canary beside
   legacy, confirm hosted accepted/fresh identity and bounded storage, then document actual evidence. No activation
   permission is inferred solely from the presence of approved unused libraries.

## Primary documentation consulted

Read 2026-09-11: current upstream [systemd execution reference source](https://github.com/systemd/systemd/blob/main/man/systemd.exec.xml)
and pinned [v255 execution controls](https://github.com/systemd/systemd/blob/v255/man/systemd.exec.xml), plus
[v255 IP filtering](https://github.com/systemd/systemd/blob/v255/man/systemd.resource-control.xml).
These support credential/root integration and address-based filtering semantics, not our installation proof.
The v255 execution reference explicitly excludes socketpair and inherited sockets from address-family filtering,
recommends native syscall architectures, and distinguishes IPC namespaces from AF_UNIX/POSIX shared memory.
The v255 resource-control reference combines ancestor policies with allow precedence; generated-unit text alone
therefore cannot establish a deny boundary. These caveats are acceptance requirements, not evidence of enforcement.
The v255 baseline, fixed-hosts policy, promotion transaction and budgets above are this project's proposed design.
