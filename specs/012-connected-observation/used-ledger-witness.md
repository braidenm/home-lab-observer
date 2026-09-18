# Existing-ledger witness primitive

Status: authorized implementation slice under the F1 installed-worker workstream.
This is not rollback, installed activation or release approval.

## Scope and contract

Add a small internal, credential-free logical fingerprint and fixed-path existing
ledger inspection primitive. Validate with the existing C1 record rules, then hash
a versioned, unambiguous encoding of binding, watermark, acknowledgement state,
terminal state and every pending field, including the exact pending bytes. Do not
apply age expiry: an old pending request remains part of the witness.

The production inspection entry point accepts only context and expected binding;
the ledger path is fixed to `/state/ledger`. It only opens existing D1 state, loads,
validates and closes. It cannot provision, commit, acknowledge, enroll or send HTTP.
SQLite recovery can synchronize physical journals; logical state must not change.
No fingerprint is returned on open/load/close/cancellation failure. The witness is
private comparison data, never operator output or telemetry. It is not a credential
or authorization receipt and does not prove version compatibility by itself.

## Encoding

SHA-256 over length-prefixed domain `observer-ledger-witness/v1`, server ID,
connector ID, unsigned 64-bit watermark, one acknowledgement byte, 32 acknowledgement
digest bytes, length-prefixed terminal state, then one pending-presence byte. If
pending exists: unsigned 64-bit sequence, length-prefixed exact body, 32 digest
bytes and length-prefixed UTC RFC3339Nano collection time. Integers and 32-bit
lengths are big-endian. C1 validation precedes encoding; field bounds remain C1's.

## Acceptance and plan

- Implement the pure fingerprint and fixed-path Linux inspection separately.
- Test deterministic/golden encoding, binding and malformed-state rejection,
  changes to every meaningful field, and no source-record mutation.
- Test real D1 pristine/acknowledged/pending/terminal/exhausted states; preserve
  directory/database identity and logical data across inspection. Reject missing,
  busy, corrupt, unsupported and wrong-binding state, plus cancellation.
- Keep pristine enrollment validation unchanged. The separately named offline
  command and exact confinement policy remain a subsequent reviewed integration slice.
- Independent review before integration. The transition journal, compiled
  compatibility contract, cross-version fixtures and VM recovery gates remain open.

## Local verification (2026-09-17)

- Pinned Go 1.27.1: Windows pure tests and vet passed; Linux package vet passed.
- Linux/amd64 test binary executed under WSL Ubuntu three times: all tests passed.
  Real SQLite fixtures covered initial, old pending, acknowledged, terminal and
  exhausted state, repeated inspection and unchanged directory/database identity.
- Rejection tests covered missing, busy, foreign-binding, corrupt and unsupported
  schema state; pre-open cancellation; injected load/close failures; cancellation
  during close; invalid loaded state; clearing detached pending bytes on failure.
- Pure tests cover a separately specified initial encoding vector, distinct
  logical states, input preservation, UTC normalization and malformed records.
- Independent production-code review found no blocker and clarified that the
  inspection worker runs as the uploader while the root coordinator holds the lease.
- Independent final test review found no blocker; its suggested isolated
  acknowledgement-flag regression is included.

These are local package tests, not release CI, a cross-version packaged test,
power-loss evidence or live registration. No installed service is changed.

## Delivery against current main

Extracted without runtime wiring onto main `d7b857e` for a focused PR. Full Windows
`go test ./...` and `go vet ./...` passed with pinned Go 1.27.1; Linux package vet
and the compiled Linux/amd64 real-ledger suite under WSL passed three times.
Existing preview executables do not import or invoke this primitive.
Independent extraction review confirmed all four primitive files match the reviewed
implementation. Its documentation finding (a premature integration claim/link) was
corrected before delivery; no offline mode is included in this PR.
