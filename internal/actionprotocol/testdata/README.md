# Paired execution codec test vectors

`paired-container-execution-v1.json` is the exact reviewed, sanitized paired
Spec075 fixture. It preserves the original codec examples from source commit
`c933bf6d18af280ebbf417060c97110f1ae19827` and adds the separately reviewed
post-finalization observational cases described in
[`post-finalization-status.md`](../../../specs/011-container-actions/post-finalization-status.md).
SHA-256: `7dec7af8fe21301534442d2ef55654c70dde83fda267ad7866c20df21d65612e`.

It contains synthetic public protocol examples only. Identifiers and HMAC material
are test data, never deployment keys, configuration, authority or acceptance
receipts. Public CI uses this checked-in copy; it must not fetch another repository.
The hash-bound fixture does not replace the typed production codec, state checks,
fresh remote authorization or the independently enforced local policy.
