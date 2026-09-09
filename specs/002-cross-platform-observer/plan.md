# Implementation plan

Spec 002 is delivered in independently releasable slices after Spec 001 merges:

1. Domain contracts, capability model, configuration, and deterministic test fixtures.
2. Cross-platform host/process/filesystem/network collectors and bounded local storage.
3. Read-only local API, OpenMetrics output, and embedded responsive overview/trend UI.
4. Linux services/containers/logs; Windows services/Event Log; macOS launchd/unified log adapters.
5. Outbound enrollment/upload compatibility and Platform Demo migration.
6. Native installers, optional container, release attestations, clean-machine canaries, and operations documentation.

Each slice updates the API/schema compatibility fixtures and publishes no artifact until its platform acceptance checks pass.
