# ADR 025: Fixed Docker reader for a separately enrolled rich installation

Status: Accepted for engineering design only; implementation, packaged acceptance and activation are gated.
Date: 2026-10-29.

## Context

The owner wants useful hosted process and Docker inventory before separately permissioned controls. ADR 024's
`RICH_PROCESSES_V1` package is a process-only disposable-VM prerequisite, not an interim owner installation. Its
collector cannot create sockets; its uploader alone holds the delivery credential. Neither role may acquire Engine
authority to make the dashboard look complete. Numeric and process-only definitions remain immutable.

The reusable `internal/containerobs` adapter already performs bounded, fixed GET requests and preserves unknown
measurements. No installed Docker mediation role exists yet. Read-only acquisition code is not a daemon authorization
policy: possession of an ordinary rootful Docker socket is root-equivalent authority, even through a read-only mount.

## Decision

Deliver one coherent, fresh-install process-and-Docker package after the process-only VM proof. Its independently
approved profile is `RICH_PROCESSES_DOCKER_V1`, with processes and containers both enabled. Use a new frozen collection
definition, explicit local approval and independent persisted server approval; the enrollment response must match
the approved profile and definition digest. Do not change the meaning or implementation definition of an old kind.

Add one fixed credential-free Docker reader, with its own principal and unit. Only this reader can open the one
explicitly approved local Unix socket. Reuse `containerobs`; do not run Docker CLI commands, discover environment/
contexts, connect over TCP/SSH, expose a listener, or accept caller-chosen HTTP methods/paths. The reader writes only
a bounded typed latest file to a narrowly shared directory. The collector validates that file and constructs the
existing combined envelope. It retains socket denial and has no upload credential. The uploader retains only its
approved handoff, ledger, credential and allowlisted HTTPS destination, with no Docker endpoint or host process view.

Constrain socket visibility, address families, writable paths, capabilities, inherited descriptors and process
visibility in the exact installed units. Private networking alone does not hide pathname Unix sockets. Prove actual
role separation and same-invocation lifecycle behavior, rather than inferring it from separate executable names.
The helper remains trusted authority-bearing code: compromise can reach daemon authority despite these restrictions.
Do not describe GET-only code as daemon-enforced read-only access. Do not install an authorization plugin, alter socket
permissions, or reconfigure/restart the user's daemon. Container controls require Spec 011's separate broker and grants.

Keep the combined/workload wire schemas and privacy rules unchanged; the enabled-container policy is new authority,
not a new raw-inspect field allowance. Add only closed local upload kind `2` and an exact schema/user_version `3`
ledger. Numeric kind `0`/schema `1` and process kind `1`/schema `2` stay unchanged. A distinct header version rejects
cross-kind opens before SQLite hot-journal recovery. Preserve existing rich byte/page/disk budgets and exact retries.
No generic profile registry, second stream, queue or ledger implementation is needed.

Use a fresh separately enrolled identity and exclusive state. Never copy a credential, grant, sequence or used ledger.
Preserve existing numeric installations. Replacement of an existing rich installation still requires ADR 024's
stop/join/fence and confirmed revocation; an uncertain outcome blocks replacement and preserves bounded private state.
There is no automatic upgrade, policy toggle or rollback by selecting an older binary against new state.

## Tradeoffs and alternatives

- Giving the process collector a socket is smaller in source but broadens acquisition authority and violates its
  accepted isolation. A fixed file boundary keeps the root-equivalent code smaller and credentials separate.
- A general socket proxy/listener adds a remotely callable request surface and configuration without helping this
  fixed observation use case. The helper is a producer, not a forwarding service.
- Daemon-side authorization can further restrict operations but adds administrator-managed daemon integration.
  It is not silently installed as part of a monitoring package.
- Adopting another dashboard does not remove enrollment, owner/delegate authorization or daemon trust requirements.
  Reuse the existing tested adapters and nullable contracts instead; do not replace dependencies in this feature.

Costs are another small worker/unit, explicit re-enrollment and a separately reviewed artifact. The benefit is useful
inventory without widening old profiles or exposing socket authority to the collector/uploader. Names and images remain
sensitive after redaction; authorized server viewers can see them, and owners must be told this before enrollment.

## Delivery, evidence and rollback

Follow [the installation plan](../../specs/013-connected-inventory/installable-docker-observation.md): receiver readers
first, then one complete artifact/installer/VM slice, then approved writer/release/UI activation and owner/delegate proof.
All gates are pending. No new profile, reader, daemon access or release is implemented by accepting this ADR.
Any persisted new profile/feature event establishes a permanent reader rollback floor, including disabled feature
events; later revocation does not erase event history. Before those writes, unused code can be removed. After activation,
disable new admissions, stop/fence workers and retain/revoke identity through supported paths; never reset state.

## Primary evidence (accessed 2026-10-29)

- [Docker socket protection](https://docs.docker.com/engine/security/protect-access/) and
  [daemon authorization](https://docs.docker.com/engine/extend/plugins_authorization/): ordinary Engine access is broad;
  granular authorization is a separate daemon facility, not a filesystem mount property.
- [Engine API version reference](https://docs.docker.com/reference/api/engine/): check advertised minimum and maximum
  against the adapter's tested `1.41..1.45` intersection. A newer daemon version alone does not prove incompatibility.
- [systemd 255 execution environment](https://www.freedesktop.org/software/systemd/man/255/systemd.exec.html): private
  networking, mount visibility and socket restrictions are distinct; inherited descriptors require separate attention.
- [SQLite header identity](https://www.sqlite.org/fileformat2.html#database_header) and
  [atomic commit](https://www.sqlite.org/atomiccommit.html) support the existing pre-open kind fence and recovery model,
  not a claim that an unexecuted schema-v3 storage or installed fault test has passed.

This decision extends [ADR 024](024-immutable-rich-installation-profiles.md); it does not rewrite accepted numeric,
process-only or [local read-only container](006-specialized-container-read-model.md) contracts.

## Dated gate update — 2026-10-29

The prerequisite process-only packaged VM gate subsequently passed at exact source
`24153fa278eea907cb0987ae96c26137c9aae909`; [input pins, independent result and limits](../acceptance/connected-rich-processes-2026-10-29.md).
This permits implementation under the decision above. It does not accept Docker, public release, owner rollout or controls;
the earlier decision-time pending language is historical, not evidence that those remaining gates passed.
