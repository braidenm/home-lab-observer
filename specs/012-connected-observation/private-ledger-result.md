# Private existing-ledger result

Status: protocol primitive; installed offline validation remains open.

The fixed `observer-existing-ledger-result/v1` worker-pipe response contains only
the expected server ID, connector ID, and a SHA-256 fingerprint of the private
existing-ledger witness. It is not operator status, telemetry, or an API response.
The receiver supplies the expected binding independently; it must not infer
that binding from the response.

Encoding validates the binding and emits one canonical JSON object. Decoding
accepts at most 512 bytes and refuses missing or additional fields, duplicate
keys, noncanonical hex, foreign bindings, and trailing bytes. Fingerprint
bytes must not enter diagnostics. A successful decode is comparison evidence,
not permission to promote a credential or start an uploader.

The installed caller must still validate the exact confined offline process,
its private ledger ownership and physical identity, then compare the result
with a before/after witness. Those checks and packaged VM acceptance are
separate from this codec.
