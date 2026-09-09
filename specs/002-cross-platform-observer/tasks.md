# Tasks: Local API contracts and compatibility fixtures

- [x] Accept the focused contract slice and retain the cross-platform follow-on roadmap.
- [x] Define the OpenAPI 3.1 read-only local surface and operational health reads.
- [x] Define versioned capabilities, current snapshot, and Problem Details JSON Schemas.
- [x] Add valid synthetic Linux, Windows, log-opt-in, and problem fixtures.
- [x] Add invalid support-state, snapshot-consistency, log-upload, and problem-internals fixtures.
- [x] Document bounds, privacy classes, log metadata/body separation, and compatibility policy.
- [x] Add requirements-to-contract/test traceability.
- [x] Add deterministic local validation and a five-minute GitHub-hosted workflow.
- [x] Implement typed Go projections and handlers in [Spec 003](../003-native-host-snapshot-preview/tasks.md) and
  [Spec 005](../005-local-data-plane/tasks.md); schema tests enforce compatibility without generated Go types.
- [x] Implement actual-host collectors and capability tests in Spec 003 and optional containers in
  [Spec 006](../006-container-observations/tasks.md).
- [x] Implement SQLite history and the embedded local dashboard in Spec 005.
- [ ] Deliver native installers and release canaries in [Spec 007](../007-native-delivery/tasks.md).
- [ ] Deliver optional outbound upload in a subsequent reviewed integration specification.
