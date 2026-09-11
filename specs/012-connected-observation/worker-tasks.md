# Proposed worker delivery tasks

C1 is merged; the uninstalled Linux D1 adapter has passed independent review. Normal CI gates remain below.
Later installed slices remain proposed. Implement one bounded slice per PR; do not infer installation approval from library completion.

## C1 — actual receiver contracts and pure state machine

- [x] Record request/enrollment/ack/revoke/error facts from actual receiver and add synthetic translated-outcome fixtures;
  executing the real HTTP adapter against the JVM remains D2, not claimed by C1.
- [x] Add typed Source/Clock/Transport/Ledger interfaces; credential provider remains transport-owned, never in state.
- [x] Test admit-before-send, exact-body retry, duplicate latest, lost response, expiry retirement without sequence reuse,
  revocation, clock skew, cancellation and signed-64-bit exhaustion. Prove no collector imports/network activation.
- [x] Independently review privacy and ambiguity; source failures remain distinguishable from idle, record validation is
  shared with adapters, and mutable buffers are detached. Full local Go tests/vet passed on 2026-09-11.
- [x] Ordinary CI passed; C1 merged in PR 37 at `fc1cea02b8ee6756ec13f62bf55c58a148612b2e`.

## D1 — durable admission ledger

- [x] Compare SQLite and atomic files; freeze [ADR 015](../../docs/adr/015-durable-upload-ledger.md) and
  [the D1 contract](durable-ledger.md) before implementation (DELETE/EXTRA; Linux only).
- [x] Implement one pending body, monotonic allocation safety and single writer; exclude credentials.
- [x] Test process death around commits, hot-journal recovery, corrupt/missing initialized state, fixed commit uncertainty,
  bounded files, and incompatible downgrade. Never rebuild pending bytes; document revoke/re-enroll after backup restore.
- [x] Independent adapter review passed on 2026-09-11; reviewer repeated the full Linux suite three times under WSL.
- [ ] Normal CI; do not claim VM power-loss or actual disk-full proof from synthetic errors.
- [ ] Installed acceptance: actual disk-full, sync/storage failure and VM power-loss tests before canary support.

## D2 — restricted transport and enrollment adapters

- [ ] Implement fixed-origin HTTPS with no redirects/inherited proxy, deadline/body limits and actual contract mappings.
- [ ] Prove TLS failures, wrong host, oversized response, replayed ack and revoked credential never become success.
- [ ] Implement uploader-only enrollment/re-enrollment staging and failure recovery; no secret argv/env/log persistence.
- [ ] Test against disposable receiver; retain v1 bearer limitation and explicit renewal/revocation semantics.

## E1 — installed shared-read handoff

- [ ] Add separate exact UID/GID policy and provisioning manifest; leave owner-private Store acceptance unchanged.
- [ ] Test distinct writer/reader/third-user access, ACL/default-ACL drift, linked files and atomic concurrent reads.
- [ ] Independently review permissions and resource cleanup. No installed daemon or credential activation yet.

## E2 — numeric-only workers

- [ ] Implement separate collector/uploader entrypoints with fixed settings, no listeners and bounded shutdown/diagnostics.
- [ ] Collector samples only reviewed required numeric sections; test no Docker/process/log collector initialization.
- [ ] Uploader receives only Source/credential/ledger/transport; test no host adapters or owner-local API dependency.

## F1 — Linux service package and acceptance

CI reliability evidence (2026-09-11): the C2 Windows check exposed a smoke
startup-cleanup gap: a child whose readiness/token check failed was not returned
to its caller for joining, and SQLite cleanup could mask the original error.
The smoke now joins that child before rethrowing and uses bounded filesystem
cleanup retries after joins. Independent static review and the Windows managed
restart/foreground smoke passed; normal CI remains required.

- [ ] Add explicit root provisioning/preflight, two accounts, reader group, minimal root, immutable selection and units.
- [ ] Validate LoadCredential and CA/DNS behavior inside actual uploader root; do not put secrets into unit text.
- [ ] Add disposable-VM negative probes for real egress/credential/host-access denial and cross-user handoff rights.
- [ ] Test exact packaged amd64/arm64 binaries through enrollment, upload, reboot, crash, revocation and rollback.
- [ ] Review fixed-output evidence and storage/resource caps before marking profile supported; no private PR runners.

## G1 — owner canary

- [ ] Verify exact host/resource scope under existing owner canary authorization; enroll a separate registration and
  leave existing connector untouched. Do not ask again for a product choice the owner has already made.
- [ ] Verify sustained hosted numeric data, honest unsupported sections, reconnect/revoke and bounded disk use.
- [ ] Document install/status/stop/uninstall/rollback and Linux-only limitations; include exact acceptance evidence.

Only after these tasks close may this profile claim installed remote connectivity. Rich telemetry, desktop remote workers,
Docker start/stop/restart and Kubernetes remain separately specified work, not hidden completion criteria here.
