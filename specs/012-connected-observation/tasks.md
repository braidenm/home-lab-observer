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
- [ ] Integrate catalog, enrollment, OS-aware installation and authorized hosted views.
  - [x] Implement a stopped-first Linux installer and synthetic packaged fixture locally; this is not a published or owner-installed connected release.
  - [ ] Finish review and installed acceptance before making the connected package downloadable or enrolling the owner server.
  - [ ] Show the registered owner server in Platform Demo and verify owner-controlled read/write sharing without granting global administration.
- [ ] Verify packaged platforms, migration/rollback and an explicitly authorized owner canary.
  - [x] Implement and locally test the bounded [existing-ledger witness](used-ledger-witness.md); no activation or rollback authority.
  - [x] Integrate the separately named [offline existing-ledger command](existing-ledger-offline-mode.md), private binding-checked result and exact confinement policy with local focused tests.
  - [ ] Prove cross-version packaged compatibility; local command/policy tests do not establish this gate.
  - [x] Merge pure ext4 identity, installed-profile, transition-record and recovery-classifier foundations (PRs 50-53); none authorizes activation.
  - [x] Add pure fixed-stage byte admission for the proposal, CA bytes and exact predecessor chain; this does not establish filesystem ownership or durability.
  - [ ] Integrate the bounded preparation witness, exact predecessor/staging retention and forward-only interrupted transition recovery.
  - [ ] Prove compatible code selection and rollback across two packaged releases without restoring or changing the used ledger.
- [ ] Review and implement [activation evidence](activation-design.md), including bounded same-process checks and the parent/worker enforcement rendezvous; no manual bypass.
  - [x] Implement bounded root request/response file helpers, closed manager-property parsing, dual-stack owned baseline fixtures, and internal transaction composition.
  - [x] Exercise malformed manager evidence, record metadata/size/link refusals, unexpected fixture traffic, cancellation, and uncertain-start identity capture in repeated Linux tests.
  - [x] Integrate the principal audit and public start/restart entry points locally; these remain unreleased and acceptance-gated.
  - [ ] Complete transaction-wide fault injection and independent security review.
  - [x] Run the packaged stopped synthetic VM subset and its reboot/abrupt-power-off identity witness.
  - [ ] Prove the full packaged installed-principal, egress, TLS, enrollment and running-uploader transaction, including in-flight interruption recovery, in a disposable supported VM before live activation.
- [ ] Specify richer remote quality-aware read models; keep Kubernetes deferred and actions in Spec 011.
