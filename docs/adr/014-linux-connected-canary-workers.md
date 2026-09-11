# ADR 014: Isolated Linux connected-observation canary

Status: Accepted for C1 pure uploader state-machine contracts only. Installed profile remains Proposed and gated;
no credentials, installation or network activation in C1.
Date: 2026-09-11.

## Decision proposed

Implement a portable uploader state machine first, then one opt-in native Linux system-service canary using distinct
unprivileged collector and uploader accounts. Export only ADR 012's numeric-host profile. Preserve the existing Linux
Compose connector and local-only native product unchanged. This deliberately sequences a Linux isolation proof before
the desktop-first rollout originally described in Platform Demo ADR 044; it does not supersede or weaken that ADR's
separate-worker, credential, host-authority, signing or platform-acceptance requirements. Windows/macOS remote
installation remains unavailable until equivalent boundaries or explicitly reviewed desktop residual risks are proven.

Use a new explicit cross-principal handoff profile. ADR 013's owner-private Store remains unchanged: it cannot share
files between distinct UIDs. The installed profile provisions one fixed collector UID and a dedicated read group whose
only ordinary member is the uploader UID. The collector owns the handoff directory (0750) and data files (0640); the
uploader receives read/traverse only. The empty writer lock remains collector-private. Root provisions the users/group,
owned paths and immutable program before starting either worker. Validate exact expected UID/GID, no named/default
ACLs or extra access, regular single-link files, bounded sizes and pinned roots. Never infer trust from group names alone.
Installer-created identity metadata is root-owned and not writable by either worker. Existing unknown resources are
refused, not repaired. This is a separate API/policy, not a flag allowing arbitrary permissions in owner-private Store.

The collector receives no credential and exposes no listener. In this canary it does not load Docker, process inventory,
native log readers or action code. It reads required numeric host sources, projects and publishes every 15 seconds.
Use a system-level service with an empty capability set, no privilege escalation, private network namespace, socket
creation denied, private temporary storage and read-only host filesystem except its handoff/diagnostic locations.
Do not inherit network descriptors or provide access to another process's credentials. Unsupported restrictions fail
the installed acceptance gate; a unit file containing the right words is not proof of enforcement.

The uploader runs as a different account in a minimal filesystem root: exact executable, reviewed CA/DNS inputs,
read-only handoff, private ledger/diagnostics and its service credential only. No host procfs, Docker socket, collector
history, local API or host resource mounts. Use systemd LoadCredential, not an environment/unit-text secret; its root-owned
credential source remains outside collector reach. No inbound listener. Add resource, shutdown and retention limits.
ProtectSystem alone is insufficient because it restricts writes, not reads. RootDirectory and credential/DNS integration
must be tested together on the supported baseline, not assumed portable from documentation.

## Alternatives and limits

| Alternative | Benefit | Why not the first canary |
| --- | --- | --- |
| Existing per-user background service | Already delivered | ADR 008 explicitly provides convenience, not compromise isolation |
| Linux Go container bundle | Familiar mount/network separation | Current native provider needs explicit host-path adaptation and physical-host proof |
| Distinct system services | Native host view, no Docker dependency | Requires privileged opt-in provisioning and a separately tested shared-read profile |

Select the third for the initial numeric-only canary. Keep the container alternative viable. Matching UIDs across containers
to reuse owner-private handoff while mounting host procfs can expose uploader process metadata/credentials; read-only
mounts are not confidentiality controls. A raw Docker socket can defeat isolation through host-equivalent daemon authority.
Docker observation and separately credentialed actions need later reviewed profiles, not extra mounts in this canary.

## Evidence

Primary sources accessed 2026-09-11. systemd v255 is a proposed acceptance baseline, not a claim about every owner's OS.

- [systemd v255 execution controls](https://github.com/systemd/systemd/blob/v255/man/systemd.exec.xml): private network
  namespaces, socket-family restrictions, filesystem roots and LoadCredential. Socket filtering does not revoke inherited
  descriptors; namespace support must be checked. Minimal-root and distinct-user selection are our application design.
- [systemd v255 IP controls](https://github.com/systemd/systemd/blob/v255/man/systemd.resource-control.xml): IPAddressDeny
  may have no effect without required eBPF support. It may supplement but cannot alone prove collector egress denial.
- [Docker security](https://docs.docker.com/engine/security/): daemon control and host mounts carry substantial authority.
- [Compose network reference](https://docs.docker.com/reference/compose-file/networks/): internal networks isolate external
  connectivity; this does not establish credential/mount isolation by itself.

Repository evidence: ADR 008 local background mode; ADR 012 projection; ADR 013 owner-private handoff; Platform Demo
ADR 038 existing separated Compose connector and ADR 044 native boundary. The existing connector's operator documentation
confirms separate collector/proxy/uploader, private snapshot exchange, no published ports and bounded logging. No private
deployment topology or configuration is copied into this proposal.

## Proof, operation and rollback

Follow [the worker plan](../../specs/012-connected-observation/worker-plan.md). Before credentials or canary activation,
prove real denied network/credential/host access under the actual installed units in a disposable Linux VM. Add crash,
admission, lost-response, expiry, revocation and rollback tests using the real receiver contract and synthetic data.
Installation success requires an accepted upload, not process liveness. Keep explicit stop/status/revoke/uninstall and
immutable side-by-side version rollback. Never downgrade a durable ledger into disposable handoff state.

No additional owner product approval is needed for C1. The owner has authorized an eventual own-server canary; execute it
only after the installation/security gates pass and its exact host/resource scope is verified. General availability and
Windows/macOS remote claims remain gated separately.
