# Tasks

## Slice A

- [x] Define explicit producer profile and research the existing receiver/portable JSON boundary.
- [x] Add strict schema, synthetic fixture and negative schema tests.
- [x] Implement pure encoder with privacy, bounds and unavailable-data regressions.
- [x] Run Go tests/vet, contracts and repository policy; independent review and normal CI (PR35).
- [x] Record merge evidence and receiver compatibility limitations: observer PR35 merged at
  `8e417d292630e1344574bef2ff792bfd2666a6d4`; Platform Demo PR365 merged at
  `3a9f54c58073e35339db84cb32625395b15f4a8b`. This proves only the numeric-host profile, not richer remote sections.

## Connected delivery

- [x] Prove fixture acceptance in the actual Platform Demo parser (PR365, including source binding and three OS labels).
- [ ] Specify and review credential lifecycle and OS isolation; implement without observation/control authority mixing.
- [ ] Implement private handoff, bounded uploader and connection health with failure tests.
  - [x] Define bounded disposable operational status and its private single-slot Linux writer; not upload or activation authority.
- [ ] Integrate catalog, enrollment, OS-aware installation and authorized hosted views.
- [ ] Verify packaged platforms, migration/rollback and an explicitly authorized owner canary.
  - [x] Define and test the bounded pure activation request/response/commit records; no service activation authority.
  - [x] Add pure fixed-stage byte admission for the proposal, CA bytes and exact predecessor chain; this does not establish filesystem ownership or durability.
  - [x] Classify complete staged evidence without treating a retained predecessor receipt as new completion; this does not permit abort, cleanup or worker startup.
  - [x] Add independent anchored ext4 stage inspection and exclusive publication primitives with root synthetic interruption tests; not wired to installer commands.
- [ ] Specify richer remote quality-aware read models; keep Kubernetes deferred and actions in Spec 011.
