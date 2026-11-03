# Fixed Linux action process identity and files

2026-11-02. Accepted concrete local-runtime profile under [ADR031](../../docs/adr/031-nonroot-action-listeners.md), after root and independent source review.
Not an installer, release or activation approval. ADR030's separate action authority and Docker residual remain.

## Static identities and paths

Static non-login users are home-lab-observer-action-worker and home-lab-observer-action-broker, with matching
private primary groups. Exclusive home-lab-observer-action-ipc contains only those two service users. Prove actual
unique nonzero UIDs and exact memberships. No caller path/UID/GID flags, DynamicUser, user namespaces or inherited listeners.
Below worker/broker/IPC abbreviate those exact identities, never configurable names.

| Path | Owner/group and mode |
| --- | --- |
| /run/home-lab-observer-actions | root:root0755 |
| /run/home-lab-observer-actions/worker | broker:IPC0750 |
| /run/home-lab-observer-actions/worker/worker.sock | broker:IPC0660 |
| /run/home-lab-observer-actions/admin | broker:broker0700 |
| /run/home-lab-observer-actions/admin/recovery.sock | broker:broker0600 |
| /etc/home-lab-observer-actions | root:IPC0750 |
| /var/lib/home-lab-observer-actions | root:root0755 |
| /var/lib/home-lab-observer-actions/journal | broker:broker0700 |
| /var/lib/home-lab-observer-actions/worker | worker:worker0700 |

Root provisions directories and assigns leaves to their creator; the worker never has listener-directory write
access. Require exact type, ownership/mode, no extended/default ACL, links or untrusted ancestor writers; validate
opened descriptors/witnesses, not check-then-open alone. The broker creates its fixed listeners only after identity,
policy and exclusive journal validation. Refuse pre-existing/foreign endpoints rather than unlinking/adopting them.
After close/join, unlink only the captured owned listener if its current witness still matches. No broad cleanup.

Listener creation additionally requires the Linux6.6-or-newer syscall capability for Fchmodat with AT_EMPTY_PATH:
pin the created socket using O_PATH|O_NOFOLLOW, change its owner/group with Fchownat(AT_EMPTY_PATH), then set its
exact mode with Fchmodat(AT_EMPTY_PATH) on that same inode. Recheck descriptor/path identity and protected ancestry;
never fall back to path-following chmod, a procfs alias or extra service capabilities. Unsupported syscalls refuse
controls as unavailable; observation is unchanged. Kernel-version text alone is not capability proof. Private
temporary-socket tests and later actual package tests must prove success, unsupported-capability refusal and no
foreign-endpoint cleanup. Reviewed2026-11-02: [Linux chmod](https://man7.org/linux/man-pages/man2/chmod.2.html)
documents AT_EMPTY_PATH since6.6 and O_PATH support; [Linux chown](https://man7.org/linux/man-pages/man2/chown.2.html)
documents empty-path descriptor ownership changes. The pinned x/sys0.48 Fchmodat wrapper refuses unsupported flags
without a path/procfs fallback; this is a first-profile compatibility requirement, not an observation-platform floor.

Both peers authenticate exact opposite service identity using SO_PEERCRED and protected socket/witness checks.
Admin requires initial-host UID0, sufficient actual DAC access and exact broker peer identity; UID0 in another
user namespace or without traversal authority is unsupported. Keep broker's explicitly approved Docker group
separate from observation/worker membership. Socket metadata does not prove remote permission or Engine identity.

The worker and root CLI share only two named outbound profile capabilities: DialWorkerBroker and DialAdminBroker.
Each resolves/revalidates its actual fixed host role, requires the unchanged expected identity witness, chooses
ONLY its code-owned socket/ancestor plan, and verifies the opposite broker using SO_PEERCRED before protocol
bytes. Root cannot use the worker capability and a worker cannot use the admin capability. The shared protected
descriptor/witness/ACL walker and temporary-test seams remain private: no public path, expected-peer callback,
UID override or generic dialer. Preserve the distinction between an absent/refused broker and unsafe profile,
path or peer drift; only the former is eligible for the worker's existing paced reconnect. Retain ancestor FDs
and recheck the same node/ancestry/role before and after connecting. A fixed read-only procfd alias may query
socket ACLs with no-follow semantics and bracketed inode checks; it never changes permissions or accepts a path.
This is reuse of the existing admission policy, not a new authority mechanism or proof of installed isolation.

Initial-namespace provenance is not inferred from UID-map spelling or self/PID1 equality inside an arbitrary
container. The supported installation is the trusted host systemd255 manager with PrivateUsers=false and no
private PID/proc namespace. Root acceptance verifies that manager against actual host PID1 before activation.
Root provisioning creates ONE read-only host NSFS bind of the fixed trusted HOST /proc/1/ns/user at
/run/home-lab-observer-actions/host-userns; both private service views receive that same protected handle as a
read-only bind, and the host root CLI can access the fixed host path too. Before role/peer/secret/Engine use,
open the protected reference and
own /proc/self/ns/user, verify NSFS and NS_GET_NSTYPE=CLONE_NEWUSER, and compare their device/inode identities.
Missing, unreadable, foreign-type or mismatched reference refuses; never substitute a regular marker, copied
number, environment flag or UID-map fallback. Root controls the bind source; hostile host root is out of scope.
No namespace FD is transferred through IPC. Effective units must prohibit namespace creation/entry and carry no
ambient namespace/DAC privilege. The root CLI's host-proc/UID assertion is complemented by the broker's host-
namespace identity and kernel-mapped SO_PEERCRED UID0; an unprivileged nested-namespace UID0 cannot substitute.
Actual installed bind/proc/namespace tests remain a package gate, not proof supplied by this document.
The separate package cut must define root preparation and reboot ordering for this ephemeral reference. Services
never mount, repair, adopt a regular marker or create it themselves; missing reference leaves controls unavailable.

## Protected Engine approval witness

/etc/home-lab-observer-actions/engine.json is root:broker0640 under the same root:IPC0750 configuration parent.
Exact ordered fields: version,engine_generation,socket_device,socket_inode,socket_group_id. Version is
observer-action-engine/v1; generation is32lowerhex. Device/inode are canonical uint64 decimal STRINGS (device
may be0; inode must be positive); group is a positive canonical uint32 decimal STRING. Same4096-byte/depth8,
closed parsing and protected-FD requirements below apply. There is no caller-selected socket path/URL.

Read this root-approved source before VerifyBroker/OpenGuarded; never inspect the current node and silently call
that approval. OpenGuarded independently rechecks the fixed /engine/docker.sock against the captured witness.
Require active policy mappings to match the approved generation. Explicit stopped/joined root approval owns
generation/witness changes; retained mappings/unknown evidence are not adopted into a replaced engine. Missing
old engine/mapping can keep recovery fenced indefinitely, as the existing first-profile contract discloses.

## Root-owned worker settings

/etc/home-lab-observer-actions/worker.json is root:worker0640 under the root:IPC0750 configuration parent.
Both services can traverse that shared parent, but only the private worker group can read this file; the existing
policy.json remains root:broker0640 and is unreadable to the worker. Neither group can write the parent or files.
Its exact ordered fields are version,origin,server_id,owner_user_id,connector_instance_id; version is
observer-action-worker/v1. Origin is one explicitly selected canonical HTTPS origin without userinfo/path/query/
fragment; IDs use paired grammar. Worker cannot modify origin/binding. No URL/identity CLI overrides.

## Bounded active credential reader

/var/lib/home-lab-observer-actions/worker/active.json is worker:worker0600, single-link regular file in its0700
directory. Exact ordered fields: version,server_id,owner_user_id,connector_instance_id,credential_id,
credential_revision,state,expires_at,authorization_secret. Version is observer-action-credential/v1; state ACTIVE
only; full binding and public credential ID must match the actual ACTION secret. Revision is a positive signed-Long
decimal string; expiry canonical six-fraction UTC. No invented issued_at/activated_at absent from server metadata.

Each document is <=4096 actual UTF-8 bytes/depth8 with strict required, duplicate, unknown/case, encoding and trailing
value checks. Bounded protected-FD reads compare witnesses before/after and honor cancellation. No links, broader
ACL adoption, credential/rejected-value output or error causes. Harden only the worker process against core dumps
before reading secrets. The provider checks exact requested binding, ACTIVE metadata and expiry; these are local
filters, never remote permission. Server still authenticates current credentials and each operation's authority.

This first runtime cut reads already provisioned active state for isolated acceptance. Installer/enrollment,
prepared successor persistence/activation/rotation, atomic updates and rollback belong to the following package
cut; do not fabricate them, auto-enroll on failure or publish a manual production-secret workflow.

### Fixed worker launch

The local-runtime worker command accepts no arguments beyond its executable name. Reuse the existing connected
service environment admission only because the policy is identical: at most32 bounded, unique manager metadata
entries; required `GODEBUG=netdns=go`; no proxy, trust-store, provider, Docker, credential or arbitrary Go overrides.
This is not observation-credential reuse. Do not print rejected environment/arguments or underlying errors.
SIGINT/SIGTERM cancel the single conversation and join it before the fixed HTTP client closes. The command is a
local consumed runtime entry point, not enrollment, a package identity, a service installer or activation approval.

### Fixed broker launch

The distinct local-runtime `observer-action-broker` command accepts only its executable name and calls the
concrete RunBroker with the original SIGINT/SIGTERM cancellation scope. Reuse the same manager-environment
admission as the worker only because its entire policy is identical; broker metadata is inert, never a source
of paths, Docker/provider destinations, trust or credentials. Reject arguments, environment overrides and runtime
errors with finite exit22 and zero stdout/stderr; no generic command/provider switch. RunBroker alone owns
installed role admission, fixed resources, bounded admission/cancel/join and cleanup. The launcher cannot
provision, repair, enroll, restart or resume either service; unsupported platforms refuse without fallback.
Prove the actual compiled invalid-argument entry and cancellation forwarding; package/service activation remains gated.

## Root-initiated relay branch

Only an authenticated root operation owning one pending fixed query permits RECOVERY_QUERY on a fresh worker
BEGIN. INIT/CONTINUE additionally own live quiescence; standalone STATUS is read-only and never newly quiesces.
Fresh relay nonce is independent of execution/control nonces. Existing
query/reply ordinal pairs1..5 use the single non-execution control/relay slot; no execution or snapshot publication
on that connection. Accept only matching RECOVERY_REPLY; close on completion/failure and re-evaluate any new BEGIN.

Quiescence stops execution, not worker relay availability. Worker cannot initiate a root ceremony/query.
Disconnect/cancellation invalidates pending replies. No reverse listener, new kind, arbitrary HTTP proxy or auto-
resumption after admin EOF. Existing5s relay and90s whole-admin budgets include quiescence/join time.

Between validated recovery replies, wait for the next RECOVERY_QUERY under the SAME original worker relay90s
deadline, then complete its header/body within min(that deadline, first-header-byte+5s). This processing idle
interval includes the broker's independently guarded local ACK commit (up to10s after CHECK); it is not an HTTP
leg and cannot consume that next frame's5s transfer allowance. A named recovery-continuation reader accepts only
worker RECOVERY_QUERY ordinals2..5; the existing exact nonce/action/digest and permitted next-operation graph
remain mandatory in the worker. Initial selection, each actual HTTP leg/reply and every other frame retain5s.
Native original90 checks surround the receive/next dispatch; malformed/partial/canceled/late/wrong-branch input
refuses, never retries or restarts either clock. Prove actual delayed CHECK-to-FINALIZE continuation beyond5s,
partial-frame bounds, cancellation and unchanged transition refusals without widening a timeout constant.

Keep a fixed-paced reconnect loop so quiesced/no-pending refusal does not strand a later root CONTINUE.

Before any broker selection is received, a clean EOF with ZERO header/body bytes and ZERO ancillary data on
the already role/path-authenticated connection is a transient unselected BEGIN, not a claim outcome. Retry only
through the existing paced reconnect and retain the same process latch; do not create a claim, enable execution
or translate it into NO_CLAIM. Partial frames, malformed/cross-branch messages, ancillary data, timeout, canceled
reads and unsafe peer/profile failures remain refusals, not this EOF category. A named initial-selection reader
accepts only the three existing worker ordinal1 selection kinds and the unchanged five-second whole-frame cap.
The worker loop may keep recovery relay available after this category even when no prior claim set its latch.

After an uncertain claim/check/report or the first non-STATUS recovery query, the worker latches execution disabled and
accepts only broker-initiated recovery relay, not publication/claim/execution. INIT/CONTINUE begin with RETAINED;
standalone STATUS uses only STATUS and does not set this latch. A later STATUS never clears an existing latch.
Failure after a valid selected SNAPSHOT_QUERY also conservatively sets that same latch and keeps paced authenticated
recovery relay available. Quiescence can cancel an in-flight snapshot publication; terminating the otherwise unlatched
worker would strand the root ceremony. This is not an automatic snapshot/claim retry or evidence of remote rollback.
Malformed initial selection and unsafe profile/peer failures remain terminal. Prove the actual snapshot-close failure
then next authenticated recovery conversation using the same worker/latch, with zero claims and no resumed publication.
Broker independently blocks execution under quiescence. ACK success cannot resume either process: explicit
stop/join/restart BOTH services is required. No same-action HTTP/Engine retry, unbounded queue or arbitrary RESUME.

## Acceptance

Test file bounds/identity/ACL/link/drift, secret canaries, literal names, worker-vs-root Unix access, wrong peers,
foreign/stale endpoints, exact cleanup, no inherited FDs, role mounts/egress and cancellation/join. Actual packaged
systemd/VM probes remain mandatory before release. Unsupported platforms remain read-only. No host file, flag,
resource cap, privileged membership or observation service is changed by accepting this local design.
