# Proposed slices C–G: uploader and isolated Linux canary

Status: C1 pure state-machine implementation accepted; installed worker/profile slices remain Proposed and gated.
Decision: [ADR 014](../../docs/adr/014-linux-connected-canary-workers.md). Keep slice A/B APIs and all local behavior intact.

## C: portable admission and upload state machine

Dependencies are narrow interfaces: SnapshotSource.Read(expectedServerID), Clock, bounded Transport
and transactional Ledger. The future Transport adapter owns its CredentialProvider; the state machine never receives
credential bytes. The state machine never imports collectors, Docker, process/log readers or filesystem walkers.
Test it with in-memory fakes before implementing disk or HTTP adapters. Production worker has no configurable arbitrary
command, upload path, proxy or collector endpoint.

- Admit only canonical numeric-host bytes, maximum 16 KiB, for the enrollment exchange's server ID. Collection time must
  be no more than two minutes old or 30 seconds ahead. Use UTC and injected clocks; never rewrite timestamps to appear fresh.
- One writer owns a private durable ledger. Persist connector/server binding, last allocated sequence and at most one
  pending exact body plus SHA-256 and collection time before any send. Sequences are positive signed-64-bit values,
  monotonically allocated; overflow stops with a fixed reason. Repeated latest bytes do not allocate duplicate work.
- A retry reuses exact pending bytes and sequence, including after process death. Commit acknowledgement before admitting
  new work. An uncertain response is not success. Do not claim server-side body-hash acknowledgement: the current receiver
  accepts equal sequence idempotently without comparing the previous body. Local immutable pending state is mandatory.
- Pending observation expiry is not an action ambiguity: atomically retire expired pending data without reusing its
  sequence, record a fixed expired/unknown-delivery outcome, then admit a fresh observation at a higher sequence. Never
  claim the expired request was not received. This prevents permanent blockage because the existing receiver validates
  collection age before its sequence check. Control/action retries remain entirely separate.
- Transport result classes: acknowledged, transient/ambiguous, rate limited, credential rejected, invalid/conflicting request. Terminal
  auth rejection stops uploads until explicit re-enrollment. Invalid/conflicting responses stop for operator diagnosis;
  do not reset sequence automatically. Frozen receiver facts and conservative mappings are in [receiver-contract.md](receiver-contract.md).
- Initial proposed timings: collect/poll every 15 seconds, request deadline five seconds; exponential jittered retry
  bounded to 1–60 seconds. A `RATE_LIMITED` result waits at least the receiver's fixed 60 seconds. Only one request at a
  time; cancellation joins the worker. No unbounded retries or sleeps inside a transport call.
- Persist at most one pending body and fixed bookkeeping. Bound ledger including temporary files to 1 MiB; no history
  queue. Corrupt or missing-after-enrollment ledger must not silently restart sequence at one. Require explicit
  re-enrollment/reconciliation. An externally restored older but valid ledger is not reliably detectable locally:
  backup restore requires revoking/re-enrolling rather than resuming the old credential. Reuse existing bounded
  diagnostics (10 MiB/seven days); never log request bodies,
  credentials, raw URLs/errors or per-server metric labels.

## D: disk/HTTPS/enrollment adapters, still uninstalled

The [D1 contract](durable-ledger.md) and [ADR 015](../../docs/adr/015-durable-upload-ledger.md) select existing
`modernc.org/sqlite v1.58.0` (no new dependency), a separate bounded one-row database with synchronous EXTRA and DELETE
rollback journal. This uninstalled adapter is Linux-only; Windows/macOS support remains explicitly unavailable pending
filesystem/durability proof. Prove locking, total database/journal bounds, full sequence precision, corrupt-state refusal,
crash recovery and transaction uncertainty. Do not put upload state into the host history
database or treat disposable handoff durability as sufficient. The logical record contains binding, allocation watermark,
last acknowledged body hash, optional pending sequence/body/hash/collection time, and terminal outcome. On uncertain commit,
stop the operation and reopen/reconcile before sending; never assume rollback. Freeze adapter format and migration tests
in D1, independently of C1's pure Ledger interface. The ledger never contains the credential. Distinguish initial enrollment
from lost initialized state using root-provisioned install metadata and credential presence; do not infer from missing files.

HTTP adapter behavior is frozen in [transport.md](transport.md): fix the documented connector routes beneath one reviewed HTTPS origin. Verify certificates, reject redirects,
userinfo/fragments and unexpected origins, disable inherited proxy settings, cap response bytes at 16 KiB and use the shared
deadline. Do not accept arbitrary remote URL configuration in the canary. Compile/test the actual receiver request headers,
body and accepted/duplicate/expired/revoked/conflict response mappings; do not invent an acknowledgement schema.

Privately read a one-use enrollment grant only in uploader-owned enrollment mode. Exchange once, durably commit the resulting
binding/credential before workers start, and never provide the grant to collector argv/env/files. Ambiguous exchange or a
crash before safe persistence requires a new grant/revocation workflow unless the receiver explicitly supports recovery.
Re-enrollment replaces credentials/registration only by explicit operation, never automatic retry. Credential source staging,
root-owned protection, service restart and revocation require tests; no custom crypto. Existing v1 bearer replay-until-revoked
limitations must remain visible. Stronger credential protocols are a separate migration.

The unused D2d owner-private adapter is specified in
[enrollment-persistence.md](enrollment-persistence.md) and ADR019. Its fixed private
layout is not root-owned installation metadata or systemd credential promotion;
those installed steps remain explicitly separate from this library.

## E: cross-principal handoff and credential-free worker

Implement a separate installed-handoff API with provisioned collector UID/group and reader UID. Do not relax owner-private
Store. Root-owned configuration binds exact numeric principals. Directory 0750, data 0640, no extra/default ACL grants;
uploader cannot create/replace/remove children. Use pinned roots, fixed names, whole-document validation, single writer,
maximum latest+staging 32 KiB, and the existing safe atomic replacement pattern. Tests must use genuinely distinct principals.
Unexpected filesystem/identity policy is refused without repair. Reader never receives a write handle or cleanup authority.

The bounded uninstalled library and explicit owned DAC/ACL fixture are defined in [E1's contract](shared-handoff.md)
and [ADR 018](../../docs/adr/018-shared-read-handoff.md). This does not implement root provisioning or worker activation.

The new collector worker samples only required CPU/memory/swap/uptime/filesystem sections; no default broad collector run
followed merely by redaction. It writes the handoff, exposes no HTTP endpoint and never accesses credentials. Mount namespace
and permissions may hide filesystems: report unavailable/incomplete instead of quietly publishing a complete host inventory.
Keep the existing rich local observer separate and unchanged; do not advertise this canary as complete rich remote telemetry.

## F: Linux packaging and negative isolation acceptance

See [F1 installed-boundary findings and evidence matrix](installed-acceptance.md) before unit design;
uploader network enforcement and collector namespace-coverage proof remain explicit unresolved gates.

Baseline proposal: Linux amd64/arm64, system-level systemd >=255, required namespaces/seccomp available; test exact supported
images/kernel behavior. Root provisions distinct static service users, dedicated reader group, immutable artifact selection,
private state, minimal uploader root and LoadCredential configuration. No automatic sudo, package installation or fallback
to a per-user profile. Preflight and explicit elevated install are separate owner steps. Refuse name/resource collisions.

Collector: PrivateNetwork, socket-family denial, no inherited sockets, empty capabilities, NoNewPrivileges, bounded resource
limits and fixed writable paths. Uploader: minimal root with only executable/CA/DNS inputs and owned state/credential,
read-only handoff, no host procfs, host roots, Docker or local collector API. Use capped internal diagnostics and discard
manager stdout/stderr. Test resolution/TLS and service credentials from inside the actual root; no insecure TLS fallback.

Mandatory disposable-VM acceptance (synthetic accounts/data only):

1. Actual installed collector cannot create/connect IPv4/IPv6 sockets or reach a fixture sink; normal collection continues.
2. Collector cannot read uploader credential, ledger, process environment, memory or descriptor paths; uploader cannot read
   collector history, host process/secret fixtures, Docker socket or local observer API.
3. Uploader reads canonical handoff but cannot alter it; unrelated third user can neither read nor write; ACL/group drift
   prevents startup. Demonstrate real denied operations, not just asserted unit property strings.
4. Invalid/oversized/changed-server handoff is not transmitted. Reboots, kill/restart, response loss, TLS failure, expired
   snapshots, disk-full, revoke and downgrade never leak or reuse allocated sequence with different bytes.
5. Exact packaged binary under immutable artifact identity performs enrollment and accepted upload; readiness distinguishes
   collecting, pending, retrying, revoked and failed. Evidence prints fixed results only, with no raw telemetry or secrets.

Privileged VM acceptance is a bounded manual/release gate; ordinary PR jobs remain synthetic and below 15 minutes. Do not
run public PR code on private infrastructure. Missing enforcement is a failed profile, not a warning-and-continue mode.

## G: explicit owner canary and operation

After review and all acceptance gates, verify the exact target/resource scope under the owner's existing canary authorization,
create a separate canary registration and install
alongside the unchanged legacy connector. Do not retire or overwrite it. Verify hosted identity/freshness and sustained
bounded storage; publish install support only after evidence. Print fixed status/stop/restart/revoke/uninstall commands.
Uninstall stops/joins both workers before owned resource cleanup; credential/ledger removal is explicit and recoverability
is disclosed. Roll back by stopping the canary and revoking its registration, preserving the working legacy installation.
Program rollback is side-by-side and refuses incompatible ledger versions rather than deleting state.

No Windows/macOS remote installer, Docker action, Kubernetes, rich logs or local UI embedding is included in these slices.
