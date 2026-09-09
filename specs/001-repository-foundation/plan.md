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

## Migration

The existing private prototype and private GHCR package remain unchanged during foundation work. A future release specification will publish the new package coordinate, verify anonymous download and installation on clean machines, update Platform Demo to the new digest, and only then mark the old artifact deprecated.
