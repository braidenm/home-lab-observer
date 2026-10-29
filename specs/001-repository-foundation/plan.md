# Implementation plan

## Phase 1: Boundary and governance

- Establish repository instructions, constitution, security policy, and coding standards.
- Record the public/private migration boundary and package naming.

## Phase 2: Architecture and contracts

- Complete primary-source research and compare runtime, packaging, storage, collection, and UI integration options.
- Accept ADRs for the runtime/distribution and API/UI boundary.
- Draft the implementation specification with explicit capability and privacy behavior.

## Phase 3: Delivery controls

- Add fast documentation/repository validation on GitHub-hosted runners.
- Configure squash auto-merge, branch cleanup, topics, and repository security settings.
- Open a pull request, review evidence, and merge automatically after checks pass.

## Contract-tool security maintenance (2026-10-29)

The existing development-only Ajv dependency resolves `fast-uri` 3.1.7. The maintainer's
[3.1.9 security release](https://github.com/fastify/fast-uri/releases/tag/v3.1.9) fixes URI normalization advisories,
including [GHSA-w74x-v6f9-j3xg](https://github.com/advisories/GHSA-w74x-v6f9-j3xg). Refresh only that transitive patch
within the declared 3.x range; keep direct dependencies and runtime binaries unchanged. Verify Node 22 clean install,
all contract/schema tests and a zero-advisory audit. This is preventive tooling maintenance, not evidence that the
installed Go agent exposes URI processing through this package.

## Migration

The existing private prototype and private GHCR package remain unchanged during foundation work. A future release specification will publish the new package coordinate, verify anonymous download and installation on clean machines, update Platform Demo to the new digest, and only then mark the old artifact deprecated.
