# ADR 017: Fail-closed one-shot enrollment coordination

Status: Accepted for the unused pure D2b kernel. Exchange/storage adapters and activation remain gated.
Date: 2026-09-11.
Supersedes: ADR 016's proposed enrollment retry boundary only; snapshot transport policy is unchanged.

## Context

Platform Demo creates a ten-minute, one-use enrollment grant and exchanges it for a server-scoped connector bearer. The
receiver commits activation before returning the only plaintext copy of that bearer and provides no connector-side status,
retrieval or reissue operation. It also converts aggregate activation failures to HTTP 401, so a 401 does not prove that
activation was absent. A crash or lost response can therefore leave a live server credential that the observer never
received.

The accepted upload state machine and D1 ledger keep credentials separate from monotonic request identity. Enrollment must
establish the credential, binding and initial ledger before any worker starts without turning several storage systems into a
new general transaction framework.

## Decision

Implement a pure single-use coordinator with four narrow ports: private grant source, durable attempt marker, exclusive
credential store, and new-ledger provisioner, plus one exchange port. Generate and durably mark the expected
server/connector binding as ATTEMPTED before the one allowed exchange call. Persist no grant, grant hash, receiver body or
credential in that marker.

After a valid correlated response, exclusively and durably persist the credential bound to both IDs; then provision and
validate the new zero-watermark ledger; then durably mark the exact attempt READY. READY is the only worker-start authority.
Any failure, cancellation or crash after ATTEMPTED and before READY is recovery-required. Do not resume from partial facts,
delete evidence, overwrite credentials, recreate ledgers or retry exchange automatically.

If the attempt-store Begin call reports failure, no exchange request has occurred, so server revocation is not implied.
However, the marker may have reached durable media before the failure was observed; preserve that local root and do not
automatically reuse or repair it. Secret-bearing byte slices are length-checked before cloning and cleared as
defense-in-depth hygiene, without claiming that Go/compiler/runtime copies are securely erased.

HTTP 429 is the only retryable receiver result because its fixed admission check occurs before grant lookup/activation. The
kernel still does not retry or sleep; it returns a fixed result and leaves explicit owner-controlled re-attempt mechanics to
the later installer. HTTP 400 is rejected. HTTP 401 and every transport, 5xx, unexpected or malformed-success outcome are
ambiguous and require owner recovery. Go HTTP trace callbacks are diagnostic timing hooks, not durable receiver-transaction
proof, so D2b does not use them to infer safe replay.

Owner recovery is revoke the possibly activated server through the existing authenticated Platform owner boundary, create a
fresh enrollment, and use a fresh local provisioning root. The observer never receives the owner's user credential. Partial
local state remains private and untouched until a later explicit stop/join/removal workflow.

## Alternatives

| Alternative | Benefit | Consequence |
| --- | --- | --- |
| Multi-phase resumable enrollment journal | Fewer owner recoveries | More secret-adjacent states, migrations and crash transitions before the first canary |
| Persist a grant hash and automatically replay selected failures | Correlates retries | Still cannot prove receiver commit on lost/401 responses and creates an automatic enrollment retry engine |
| Provision ledger before network | Less post-response work | Leaves authorized-looking durable identity without a credential and still cannot make stores atomic |
| Credential and ledger before final READY marker | Small, explicit start gate | Partial success requires revoke/fresh enrollment; selected for the first release |

## Evidence and limitations

Evidence inspected 2026-09-11:

- Platform Demo PR 367 head `500f02d3bdd292c291326f7060c58816fe05df61` proves raw exchange/acknowledgement
  snake-case fields, credential/ID shapes and `Retry-After: 60`. `HomeLabEnrollmentEndpoint.kt` performs admission before
  lookup, sends `ActivateHomeLabServerConnectorCommand` before returning the credential, and maps activation exceptions to
  401. `HomeLabServerRegistration.kt` permits activation only while pending and stores only the connector-secret hash.
- [Go 1.27.1 `net/http/httptrace`](https://pkg.go.dev/net/http/httptrace@go1.27.1) states trace hooks may run concurrently
  and some may run after a request completed or failed; `WroteRequest` reports the transport's write result and may run more
  than once. It does not attest the receiver's durable command result.
- [systemd service credentials](https://github.com/systemd/systemd/blob/v255/docs/CREDENTIALS.md) distinguishes
  `LoadCredential=` from literal unit credentials and exposes service credentials through a private runtime directory.
  This supports the later Linux adapter boundary; D2b does not implement or claim that filesystem protection.
- ADR 015 and `durable-ledger.md` define exclusive Provision/OpenExisting and recovery refusal. D2b uses only new Provision
  after credential durability and does not weaken those storage rules.

The pure kernel proves ordering and classification, not durable media, OS ACLs, HTTPS behavior or installed isolation.
Actual grant/credential adapters, D2a exchange transport extension, Linux service credentials, authenticated owner recovery,
disposable receiver acceptance and worker activation remain separate reviewed gates. The v1 connector bearer is replayable
until revocation; sender constraint and rotation require receiver migration.
