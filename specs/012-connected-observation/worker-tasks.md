# Proposed worker delivery tasks

C1 contracts/implementation have received independent code review; ordinary CI/merge remains pending. All later slices remain
proposed. Implement one bounded slice per PR; do not infer installation approval from library completion.

## C1 — actual receiver contracts and pure state machine

- [x] Record request/enrollment/ack/revoke/error facts from actual receiver and add synthetic translated-outcome fixtures;
  executing the real HTTP adapter against the JVM remains D2, not claimed by C1.
- [x] Add typed Source/Clock/Transport/Ledger interfaces; credential provider remains transport-owned, never in state.
- [x] Test admit-before-send, exact-body retry, duplicate latest, lost response, expiry retirement without sequence reuse,
  revocation, clock skew, cancellation and signed-64-bit exhaustion. Prove no collector imports/network activation.
- [x] Independently review privacy and ambiguity; source failures remain distinguishable from idle, record validation is
  shared with adapters, and mutable buffers are detached. Full local Go tests/vet passed on 2026-09-11.
- [ ] Pass relevant checks in ordinary CI and record merged C1 evidence.

## D1 — durable admission ledger

- [ ] Compare existing SQLite one-row FULL/rollback-journal adapter with a new atomic-record adapter; freeze selected
  representation and bounded recovery policy before implementation. C1 does not select a disk format.
- [ ] Implement one pending body, monotonic allocation/ack/retirement and single writer; exclude credentials.
- [ ] Test crash at every commit boundary, corrupt/missing initialized state, sync failure, disk-full, total 1 MiB bound
  including temporary files, and incompatible downgrade. Never rebuild pending data from latest handoff. Document
  mandatory revoke/re-enroll after external ledger backup restore; do not claim detection of every valid old record.

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
