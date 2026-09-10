# Full-system dashboard delivery plan

Updated 2026-09-10 from the owner's expanded request. This is the sequence, not a completed-feature claim. Each phase
requires its own accepted specification, executable contracts, independent review and verified release evidence.

## 1. Complete native observation

Finish Spec 009's owned Windows fixture, Linux missing-runtime proof, reproducible paired builds and packaged
lifecycle tests. Merge explicit source configuration and publish only after acceptance. Keep host/process observations
and optional Docker inventory, including stopped containers, working independently of native logs. Logs remain
metadata-only, with the accepted Windows continuation limitation.

## 2. Add Kubernetes visibility

Use a separate read model and dashboard view for explicitly scoped pods, their containers and controller status.
Do not confuse cluster workloads with this machine's Docker containers or host processes. Report resource usage only
when an authorized metrics source actually supplies it; unavailable is not zero. Avoid guessed application identity
or process-to-container relationships.

Proposed first slice: one explicit HTTPS cluster, namespace allowlist, bounded polling and read-only RBAC. No automatic
kubeconfig discovery, credential plugins, TLS bypass, Secrets, pod exec or mutation. Status reads can return sensitive
pod specification fields; the owner must accept a bounded ingress-and-discard policy before implementation. Freeze
the API, credential and data policy in an ADR/specification. Initial native versus in-cluster deployment is also an
open owner preference. Metrics, Events, Jobs and node-management extensions remain separately specified.

## 3. Connect installed observers to the hosted home-lab dashboard

Audit Platform Demo enrollment, owner/delegation permissions, installer catalog and legacy upload compatibility.
Deliver OS-aware downloads, one-use enrollment, scoped renewable credentials, bounded authenticated outbound delivery,
revocation and visible connection health. Preserve existing agents during migration. Reuse the observer's
transport-neutral UI through an authorized hosted data source, not a browser iframe to another machine's localhost.

Remote fields require explicit approved data policy: enrollment alone does not make local-only Docker, process or log
metadata upload-eligible. Verify owner/delegate authorization on every backend view. Test installation, connection,
dashboard, retries and revocation with synthetic resources before an explicitly authorized owner-server canary.

## 4. Separately permissioned container control

Start with restart of individually approved Docker container IDs; start/stop inclusion awaits owner preference.
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

- Verified downloadable packages; accurate install, manage, rollback and capability documentation.
- Independent code/security/UI review and passing required CI without bypass, targeting under 15 minutes.
- Real platform fixtures as well as unit/contract tests, with private-data-safe output.
- Honest freshness, disconnected, unsupported, partial and permission states on desktop and mobile.
- Owner-bound hosted views, negative authorization tests, revocation and legacy compatibility.
- Separate action authorization and audit; no implicit elevation or generic command execution.
- Signing/notarization and other external prerequisites reported honestly; preview evidence is not GA certification.
