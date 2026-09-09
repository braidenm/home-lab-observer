# Coding and delivery standards

## Design

- Use small packages with one reason to change and constructor-injected dependencies at process boundaries.
- Keep collection adapters independent of transport and UI concerns.
- Model observations with a timestamp, source, schema version, support state, and collection quality.
- Use UTC internally and render the viewer's local time at presentation boundaries.
- Prefer additive fields. Unknown JSON fields must not break compatible clients.
- Return typed problem details for API errors and stable machine-readable error codes.

## Security and privacy

- Bind the local API to explicit `127.0.0.1` by default and validate `Host` and `Origin` headers. IPv6 is a future separately tested listener extension.
- Require explicit authentication and TLS configuration before non-loopback binding can be enabled.
- Use allowlists for log sources, filesystem paths, container operations, remote destinations, and future actions.
- Normalize and redact before storing or transmitting. Tests must include tokens, URLs with credentials, email addresses, and common secret formats.
- Keep enrollment secrets out of command history when the platform permits it; exchange one-use tokens for renewable scoped credentials.
- Treat access to Docker-compatible sockets as root-equivalent. Use a constrained read-only proxy when containerized.

## Quality

- Unit-test domain and policy behavior; contract-test adapters; integration-test local API/storage boundaries; smoke-test packaged artifacts on every supported OS.
- Prefer deterministic clocks, synthetic fixtures, and platform capability fakes.
- Race detection, static analysis, vulnerability scanning, license checks, and contract compatibility run in CI.
- Pull-request CI excludes long-running end-to-end and release publication jobs.

## Repository workflow

- Specifications live under `specs/<number>-<name>/` with `spec.md`, `plan.md`, and `tasks.md`.
- ADRs are immutable after acceptance except for status and supersession links.
- Work on feature branches, open a pull request, enable auto-merge, and let required checks merge it.
- Keep release workflows tag- or manually-triggered. Production signing credentials are restricted to protected release environments.
