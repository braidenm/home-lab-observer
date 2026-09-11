# Private projection validation

This prerequisite implements ADR 012's unchanged numeric-host policy for a future private handoff reader, not a general
HTTP receiver. `remoteprojection.Validate(data, expectedServerID)` is pure and accepts only exact bytes that the current
`Encode` can produce for the expected exchanged `srv_` identity. It does not authenticate that identity or consent.

Input is capped at 16 KiB before JSON decoding. Typed integer decoding preserves precision. Reconstruct only eligible
numeric observations, invoke the existing encoder's bounds and policy, and compare the complete canonical bytes.
This avoids a second independent policy implementation. Canonical byte equality intentionally rejects whitespace,
reordered members, alternative number/time/string spellings, unknown or duplicate members (including nested members),
case aliases, missing members, trailing JSON, BOM and invalid UTF-8. These stricter rules apply only to the code-owned
private file; they do not narrow the public legacy receiver's compatibility contract.

Available host data must satisfy every existing producer bound. Unavailable overview must contain only its fixed reason
and false availability. Excluded sections must retain their exact unavailable reason and empty arrays. Generated aliases,
uncollected kernel and null loads are regenerated rather than trusted. Any mismatch returns one fixed error without input,
path, parser diagnostics or partial data. No filesystem, persistence, networking or worker activation is introduced.

Proof uses synthetic encoder round trips for all OS values, unavailable and empty inventories, numeric bounds and maximum
inventory, plus malformed/ambiguous/private-field documents and wrong identities. A future producer format change must
update this private contract and its compatibility tests together; stale incompatible files fail closed.
