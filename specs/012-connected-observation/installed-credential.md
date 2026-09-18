# Installed connected credential admission

Status: implementation slice under Spec 012 and ADR 020. This validates a
worker-readable credential; it does not enroll, install, upload, or activate a
worker. The credential is private installation data, never a diagnostic field.

## Contract

- Accept only the canonical `observer-connected-credential/v1` JSON record at
  most 512 bytes. The server and connector IDs must match independently
  supplied bindings, and the secret must match the fixed credential syntax.
  Extra fields, changed ordering/encoding, malformed values and cross-binding
  records refuse with one closed error.
- On Linux, only a non-root uploader may load the fixed systemd 255
  `CREDENTIALS_DIRECTORY` location. Walk each fixed component from `/` with
  no-follow file descriptors; require exact root ownership/modes, one regular
  file link, bounded content, and the one named-user POSIX ACL grant. Refuse
  arbitrary paths, environment-selected files, broad ACLs, symlinks and root
  worker execution. Clear the temporary read buffer after decoding.
- The loader does not log, persist, return in diagnostics, or upload the raw
  credential. Default text, Go and structured-log formatting of the record
  is redacted. Explicit JSON encoding is still possible for the private
  installed record and must not be used for logging. Callers must keep the
  returned secret in memory only for the fixed-origin authenticated transport.

Tests cover canonical bounds and binding refusal plus byte-for-byte ACL
tampering. The actual packaged systemd credential mount, uploader identity,
permission denial, rotation/revocation and restart behavior still require
disposable-VM acceptance. These static tests are not a credential-release gate.
