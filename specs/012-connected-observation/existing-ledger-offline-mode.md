# Offline existing-ledger validation

Status: bounded implementation slice authorized 2026-09-17. This connects the
reviewed [witness primitive](used-ledger-witness.md) to the existing private
worker/manager protocol; it is not code selection, rollback or activation.

## Contract

Add only `validate-existing-ledger` as a separately named uploader mode. Preserve
`validate-ledger` pristine-only behavior. Reuse the canonical, bounded
`ValidationInput` containing expected server/connector IDs and uploader/shared
numeric principals. Reject malformed input, wrong identity and cancellation before
inspection. Invoke only `ledgerwitness.Inspect` at fixed `/state/ledger`; no
provisioning, credential, enrollment, handoff, sequence mutation or transport call.

After successful close and a final cancellation check, emit canonical JSON with
exact fields `version`, `server_id`, `connector_id`, `fingerprint_sha256`.
Version is `observer-existing-ledger-result/v1`; digest is 64 lowercase hex
characters; total result is at most 512 bytes. The parent verifies canonical bytes,
exact version/binding and digest syntax before accepting the private result.
No fingerprint appears in logs, operator status, errors or diagnostics. It remains
comparison data, not compatibility or activation authority. A write error or short
write returns the existing closed failure exit; parent accepts no failed output.

Render exactly the existing offline final-ledger profile: dedicated uploader
identity, private network/IPC and denied socket syscalls, minimal root, only
`/state/ledger` state bind, no credential/enrollment/handoff bindings. Keep the
held-input gate and exact loaded-policy validation before input delivery. The new
mode does not add a public installer operation or permit an arbitrary path/URL.

## Verification and remaining gates

- Protocol roundtrip, canonical/duplicate/unknown-field/version/binding/digest and
  size rejection. Witness bytes never appear in failure messages.
- Mode dispatch, exact inspection binding, malformed/principal/canceled refusal,
  inspection/close failure, cancellation after inspection and output failure.
- Use existing real D1 witness tests for pristine, old pending, acknowledged,
  terminal and exhausted state preservation; mode tests compose through a private
  inspection seam, without creating `/state` or native services.
- Renderer and loaded manager tests require the exact final-ledger-only profile
  for this mode and retain the pristine/enrollment distinction.
- Linux focused tests/vet, cross-platform pure tests and independent review.

The compiled compatibility contract, root transition coordinator, filesystem
identity witness, cross-version packaged validation, crash/reboot recovery and
real installed activation remain unimplemented/unexecuted gates. No VM or live
server changes are authorized by this slice.

## Local evidence (2026-09-17)

- Pinned Go 1.27.1 Windows pure protocol, renderer, policy and separate worker
  dependency-boundary tests passed. Only the uploader gained `ledgerwitness`;
  the collector's closed dependency allowlist is unchanged.
- Linux/amd64 uploader build and focused package vet passed. WSL executed the
  enrollment adapter, manager enrollment tests and policy tests five times, and
  real D1 witness tests three times successfully. No services were created.
- Prelaunch tests use canonical identifiers and a private manager-reader sentinel:
  malformed/noncanonical/duplicate/unknown fields, mismatched principal and invalid
  binding all refuse before any manager query; valid input reaches that boundary.
- Independent review corrected initially invalid synthetic baseline identifiers;
  the nonvacuous positive control and all rejection cases then passed.
- These are local source-level checks, not release CI, packaged cross-version or
  installed-profile acceptance. Transition/rollback and activation gates stay open.
