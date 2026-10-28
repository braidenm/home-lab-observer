# Full-system dashboard delivery plan

Updated 2026-10-26 after connected-beta publication. This is the sequence, not a completed-feature claim. Each phase
requires its own accepted specification, executable contracts, independent review and verified release evidence.

## 1. Native observation: released

Spec 009 is released in preview 3; its traceability records packaged acceptance and publication evidence. Host/process
observations and optional Docker inventory, including stopped containers, work independently of native logs. Logs
remain metadata-only, with the accepted Windows continuation limitation. Preview publication is not GA certification.

## Deferred: Kubernetes visibility

Use a separate read model and dashboard view for explicitly scoped pods, their containers and controller status.
Do not confuse cluster workloads with this machine's Docker containers or host processes. Report resource usage only
when an authorized metrics source actually supplies it; unavailable is not zero. Avoid guessed application identity
or process-to-container relationships.

The owner deferred this phase on 2026-09-10. Its unresolved choices do not block the two active phases below.
Proposed eventual first slice: one explicit HTTPS cluster, namespace allowlist, bounded polling and read-only RBAC. No automatic
kubeconfig discovery, credential plugins, TLS bypass, Secrets, pod exec or mutation. Status reads can return sensitive
pod specification fields; the owner must accept a bounded ingress-and-discard policy before implementation. Freeze
the API, credential and data policy in an ADR/specification. Initial native versus in-cluster deployment is also an
open owner preference. Metrics, Events, Jobs and node-management extensions remain separately specified.

## 2. Connected Linux beta: published; real owner/share rollout next

[Spec 012](../specs/012-connected-observation/spec.md) now has a published
[Ubuntu 24.04/systemd 255/amd64 beta](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-canary.1).
Source `5dc7bfa3280287431878c778eb9faf9f86cd9152`, hosted candidate run `40402397517` attempt `1` and artifact
`12689263049` are bound by the [canonical acceptance record](acceptance/connected-linux/0.1.0-canary.1.json).
Both exact-artifact bootstrap and activation VM scopes passed; immutable publication and anonymous verification
of all four download assets completed. These are release proofs, not a real owner-host or sharing acceptance claim.

Next: enable the deployment-owned catalog through reviewed infrastructure and the normal root reconciler,
install a separate owner-bound native canary, prove useful fresh acknowledged readings, then verify ordinary-user
grant/view/revoke and direct API/URL denial. Preserve legacy Compose connectors, identities and snapshots.
The [operator guide](connected-linux.md) documents the supported My servers flow and manual lifecycle.
The rich local Windows/macOS/Linux preview remains independent; connected Windows/macOS are not delivered yet.

Remote fields require explicit approved data policy: enrollment alone does not make local-only Docker, process or log
metadata upload-eligible. Verify owner/delegate authorization on every backend view. The numeric beta uses
separate restricted collector/uploader workers and hidden one-use enrollment, but leaves services disabled/stopped
after install and reboot. Automatic boot/crash recovery, upgrades/removal and broader remote profiles remain open;
do not present the beta as unattended or production-certified.

## 3. Separately permissioned container control: after connectivity

The owner approved start, stop and restart for individually approved Docker container IDs, with a separate grant
for each action. The owner delegated production-oriented architecture choices; resolve and review the concrete
broker/security policies in an ADR before implementation rather than treating draft defaults as proven isolation.
Control uses a separate local opt-in, credential, action boundary and resource grants. Persist audit intent before
dispatch; recheck authorization and immutable target identity at execution. Require revocation, expiry, replay
protection, per-target serialization and explicit ambiguous outcomes. Never blindly retry a timed-out restart.
Docker socket authority is broader than restart, so meaningful containment needs a constrained broker or daemon
authorization policy, not just a hidden UI button.

Kubernetes management is a later separate slice: begin with an allowlisted Deployment rolling restart, not an
arbitrary container restart or pod deletion. Specify UID binding, patch restriction, paused-target refusal, rollout
status and availability safeguards. No shell, exec, arbitrary API forwarding, images, volumes, deletion, privilege
changes or node reboot is implied by either first action slice.

## Completion evidence

Use the [open-source alignment review](architecture-research/007-open-source-alignment.md) as a design checklist,
not a feature-parity promise or permission to copy another project's security defaults.

- Verified downloadable packages; accurate install, manage, rollback and capability documentation.
- Independent code/security/UI review and passing required CI without bypass, targeting under 15 minutes.
- Real platform fixtures as well as unit/contract tests, with private-data-safe output.
- Honest freshness, disconnected, unsupported, partial and permission states on desktop and mobile.
- Owner-bound hosted views, negative authorization tests, revocation and legacy compatibility.
- Separate action authorization and audit; no implicit elevation or generic command execution.
- Signing/notarization and other external prerequisites reported honestly; preview evidence is not GA certification.
