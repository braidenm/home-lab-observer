# Repository instructions

These instructions apply to the entire repository.

## Delivery workflow

1. Work from an approved specification under `specs/`; update the specification when scope or behavior changes.
2. Record durable architectural choices in `docs/adr/` and evidence-heavy investigations in `docs/architecture-research/`.
3. Implement one coherent specification slice per pull request. Do not push feature work directly to `main`.
4. Run the checks listed by the active specification and keep pull-request CI below 15 minutes.
5. Use squash merge with auto-merge after required checks pass. Never bypass a failing required check.

## Engineering standards

- Keep the observer headless-first, read-only by default, and useful without Platform Demo.
- Treat every observation as potentially sensitive. Collect the minimum safe fields by default, make sensitive fields opt-in, redact before persistence or upload, and bound retention.
- Keep OS, container, storage, API, presentation, and upload concerns behind explicit interfaces.
- Prefer versioned schemas and additive compatibility. A consumer must be able to distinguish unsupported, unavailable, permission-denied, and zero values.
- Never expose the Docker socket, an arbitrary file reader, an arbitrary command executor, or an unauthenticated non-loopback listener.
- Do not log secrets, enrollment tokens, raw environment variables, or HTTP authorization headers.
- Pin release inputs, generate checksums and attestations, and use GitHub-hosted runners for public pull requests.
- Add tests for behavior and security boundaries, not just implementation details.

## Public repository boundary

This repository must not contain private infrastructure configuration, private hostnames or addresses, credentials, production enrollment tokens, private repository archives, or deployment secrets. Examples and fixtures must use reserved domains and synthetic data.

See [the coding standards](docs/coding-standards.md) and [the constitution](.specify/memory/constitution.md).
