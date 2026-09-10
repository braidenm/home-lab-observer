# Spec 011: Permissioned Docker container actions

**Status:** Proposed for full architecture, security and owner review; not approved for implementation or activation

## Outcome and sequencing

After connected read-only observation is delivered, an owner can explicitly enable a separate action broker and grant
start, stop and restart independently for selected existing Docker containers. The owner has accepted these three
actions as the first management scope. The security, delivery and operational policies below remain proposed.

This does not change Spec 006's GET-only collector or any accepted ADR. Kubernetes Deployment rollout restart requires
a separate deferred specification. No Kubernetes pod deletion, shell/exec, generic Docker proxy, container creation,
configuration changes, image pulls, daemon restart, host reboot or automatic remediation belongs to this slice.

## Requirements

- **R1 Separate authority:** Disabled by default. Observation credentials never authorize actions. The broker runs in
  a separately permissioned process with protected credentials/state and a fixed typed transport. The web/uploader
  roles cannot acquire its Docker handle. A shared executable alone is not isolation. No unauthenticated listener,
  arbitrary command/path/URL, inherited Docker context discovery or forwarded socket is permitted.
- **R2 Grants:** Bind each grant to owner user ID, installation ID, owner-configured engine connection generation,
  full immutable Docker container ID, grantee user ID, one of `docker.container.start`, `.stop`, `.restart`, expiry and
  policy revision. Never authorize through email, administrator role, display name, short ID, image, labels, or a
  public inventory alias alone. Recreated containers require a new grant; reconnecting/replacing the engine invalidates
  grants until owner confirmation. Grant administration is separate from execution; delegates cannot redelegate.
- **R3 Execution check:** Authenticate the request, validate installation/owner binding, resolve a broker-owned target
  reference, persist admission, then recheck current grant/revocation/expiry and authoritative target state immediately
  before dispatch. The broker also enforces a local owner allowlist. Failure to obtain fresh authorization is denial,
  not use of cached permission. Record the checked policy revision. Revocation prevents not-yet-dispatched work; it
  cannot undo an accepted daemon operation. Do not claim atomic revocation across a remote authority and Docker.
- **R4 Fixed operations:** Issue only the code-owned Engine version negotiation, bounded target state reads and exact
  `POST /containers/{full-id}/start`, `/stop`, `/restart` calls in a tested API range. No caller-provided suffix, query,
  headers, body, signal, checkpoint, attachment or method. Start has no body/options. Stop/restart use the owner-approved
  grace value, proposed default 30 seconds and range 1..120 seconds. Never allow zero/immediate force or negative/infinite
  grace. The container's configured stop signal remains in effect; Docker can force termination when grace expires.
- **R5 Semantics:** Apply the state table below. A restart grant is explicitly permission to cause a stop/start cycle,
  but does not grant the independent start or stop endpoints. Stop/start grants together do not automatically expose
  the restart endpoint. Reject paused, restarting, removing, dead, missing and unknown states. Target state can change
  outside this broker between inspection and dispatch; Docker has no transaction with our authorization check.
- **R6 Durable idempotency:** A request has an authenticated actor, unique request ID, creation/expiry time and
  idempotency key bound to its canonical action/target/policy parameters. Persist intent before any mutating call;
  storage failure means no dispatch. Same key and same payload returns the existing record without another mutation;
  same key with different payload is a conflict. Serialize all actions for a target and bound global concurrency.
  Proposed limits: two in-flight actions per installation, no queued actions in the initial slice, 30-second per-target
  cooldown. Reject excess work with a stable retryable code rather than an unbounded queue.
- **R7 Ambiguity:** Persist dispatch intent before sending. A crash, lost response or timeout after dispatch may mean
  the daemon acted. Recover such records as `OUTCOME_UNKNOWN`; never automatically replay them, including start/stop.
  Reconcile through bounded read-only state inspection, but current running/stopped state cannot prove which request
  caused it. Keep ambiguous requests and target locks until explicit owner reconciliation; a fresh operation requires
  explicit acknowledgement, fresh authorization and a new key. Cancellation after dispatch is not rollback.
- **R8 Audit:** Append durable admission/denial, dispatch, outcome, reconciliation and grant-change records, including
  actor/owner/install opaque identifiers, protected target identity, action, policy revision, request ID, UTC times,
  effective grace and fixed result codes. Never store credentials, environment, raw inspect bodies, raw daemon errors
  or unrestricted user text. Fsync/transaction durability semantics must be proven before activation. Audit is not a
  forensic tamper guarantee against the host owner. If final audit persistence fails, retain unresolved dispatch intent
  and inhibit further work rather than report unaudited success.
- **R9 Bounded persistence:** Proposed separate action store budget 32 MiB including WAL and 30-day terminal audit
  retention, capped at 10,000 requests. Unresolved intents are never automatically evicted. Admission reserves capacity
  for terminal records; exhausted storage rejects new actions without deleting unresolved evidence. New requests expire
  within five minutes of creation; reject expired requests before treating a missing retained key as new. Identity and
  freshness evidence must be authenticated, not supplied by an untrusted browser clock. Document that this is bounded
  replay suppression, not infinite exactly-once delivery. Explicit owner acknowledgement cannot erase audit history.
- **R10 Outcome and UX:** Separate action progress from subsequently observed container state/application health.
  `SUCCEEDED` means the daemon acknowledged the operation; it does not mean Jellyfin is healthy or remains running.
  UI shows only permitted actions, the exact selected target, grace/possible forced-termination warning, and explicit
  confirmation for stop/restart. Backend checks remain authoritative. Provide accessible pending/denied/failed/unknown
  states, double-submit protection and mobile controls. Show no batch operations in this slice.
- **R11 Compatibility:** Keep existing read-only local APIs, bearer scopes, inventory contract and upload projection
  unchanged. Define a separately versioned action request/result/audit contract and separate credential audience before
  implementation. Platform Demo owns remote user authentication and delegation; the standalone owner profile remains
  usable without Platform Demo. Remote delivery is outbound authenticated installation-bound transport, not exposing
  the Docker API on the internet. Browser-to-local control is not implied.
- **R12 Operability:** Explicit broker enrollment, enable, disable, credential rotation, revoke, upgrade and uninstall
  instructions are required. Disable stops admission and joins in-flight work up to the configured deadline, recording
  ambiguity if needed; no implicit kill/retry. Upgrade must preserve durable unresolved intents. Unsupported daemon/OS
  combinations stay unavailable until disposable-container native tests pass. Installers must not enable actions,
  broaden socket permissions or reconfigure Docker silently.

## State semantics

| Action | Fresh observed state | Behavior |
| --- | --- | --- |
| Start | created or exited | One fixed start call |
| Start | running | Audited `ALREADY_IN_STATE`, no mutation |
| Stop | running | One fixed stop call with approved grace |
| Stop | created or exited | Audited `ALREADY_IN_STATE`, no mutation |
| Restart | running | One fixed restart call with approved grace |
| Restart | created or exited | Reject; require a separate start grant/request |
| Any | paused/restarting/removing/dead/missing/unknown | Reject; no unpause, retry or substitution |

The restart precheck is a product safeguard, not a daemon-side state precondition. A concurrently stopped container can
still be started by an already-authorized restart. Require explicit owner acknowledgement of this limitation in the
restart grant workflow; if that is unacceptable, do not offer restart on that installation. Do not silently require or
consume a separate start grant after dispatch, or claim the check eliminates external-controller races.

## Proposed architecture decisions requiring review

1. Approve a small local authority-bearing broker as the trusted computing base. Docker's ordinary socket access is
   broader than these actions. A daemon authorization plugin or additional restrictive proxy can add containment but
   is not silently installed, and must not be represented as already present. See the research note.
2. Finalize standalone owner authentication, remote execution-time revocation protocol and bounded audit-store durability
   before code. No offline remote-grant fallback in the first slice. Reusing observation bearers is prohibited.
3. Approve proposed grace, cooldown, request expiry and retention limits, plus explicit ambiguous-outcome reconciliation.
   These are defaults for review, not additional user commitments inferred from accepting the three actions.

## Acceptance and evidence

Acceptance requires synthetic policy/state/transport/storage tests covering every requirement, all three independent
permission matrices, tenant/install confusion, name reuse, revocation/expiry after admission, malformed and oversized
requests, URL traversal/redirect/header smuggling, response loss, process death at each dispatch boundary, deduplication
across restart, pruning/replay expiry, storage-full/fsync failure, cooldown/concurrent callers and secret canaries.
Native tests use disposable synthetic containers only, never a developer's existing workload. Prove fixed API calls,
grace behavior, already-state handling and honest post-action health. Independent architecture, security, code and
responsive UI reviews are required. Required PR checks stay parallel and below 15 minutes; longer lifecycle/fault tests
run separately. No action is implemented, enabled, released or certified by accepting this documentation alone.

References: [research](../../docs/architecture-research/006-permissioned-container-actions.md),
[plan](plan.md), [tasks](tasks.md), [read-only container spec](../006-container-observations/spec.md).
