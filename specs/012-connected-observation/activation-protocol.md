# Bounded connected activation records

Status: implementation slice under Spec 012 and ADR 020. This specifies only a
pure message contract. It does not enable service start, credential access, or
network activation.

## Contract

- Root creates a fresh request for one activation attempt. Its canonical JSON
  binds a random nonce, exact artifact and configuration SHA-256 digests, a
  nonzero policy generation, and two root-selected nonprivileged fixture ports.
  It carries no arbitrary address, URL, path, command, or credential.
- The paused uploader responds with the exact request digest, an independently
  obtained manager invocation ID, a fresh process-local challenge, and a closed
  PASS value. Root must verify the actual invocation and the underlying runtime
  probes separately; a correctly encoded response is not proof of their success.
- Root commits only the matched request, invocation, and challenge. The worker
  must compare it with its retained in-memory response in the same startup and
  apply a monotonic deadline and one-time consumption. A status-file response
  reloaded after restart cannot grant activation.
- Each record is at most 2 KiB, has one fixed version, lowercase fixed-length
  hexadecimal bindings, and exact canonical JSON bytes. Unknown or extra fields,
  reordered fields, malformed values, or oversize records refuse without
  echoing untrusted input.

## Boundary and acceptance

`internal/connectedactivation` contains no filesystem, service-manager,
credential, transport, or collector imports. Tests cover round trips, limits,
noncanonical records, wrong request and invocation, and challenge mismatch.

The installer, manager-owned volatile directory, same-process probes, effective
network enforcement, crash/restart handling, full packaged VM acceptance, and
release authorization remain separate open gates. This codec must never be
interpreted as a durable acceptance receipt or a standalone permission to start
an uploader.
