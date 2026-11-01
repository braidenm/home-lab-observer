# Tasks

**Status:** Proposed; unchecked items are future work, not delivered capabilities.

- [x] Record owner's accepted first action set: independently granted start, stop and restart.
- [x] Draft proposed scope, safety semantics, boundaries and primary-source research.
- [x] Draft coordinated finite-grace/immutable-target/unknown-dispatch proposal in
  [ADR 027](../../docs/adr/027-bounded-action-dispatch-and-uncertainty.md); no implementation or activation.
- [x] Review and implement the [unused typed coordinator](worker-core.md) with synthetic ports;
  this does not complete durable storage, a Docker adapter or installed action authority.
- [ ] Independently review/accept that proposal and freeze parameter codecs, authenticated reconciliation and
  corresponding crash/deadline/rebinding tests before any executable action profile.
- [ ] Complete independent architecture/security and owner review of proposed decisions and numerical limits.
- [ ] Accept spec and create a new ADR without rewriting accepted read-only decisions.
- [ ] Freeze versioned action/grant/result/audit contracts, credential audiences and standalone-owner authentication.
- [ ] Specify remote freshness/revocation protocol and request replay-window semantics.
- [ ] Implement/test synthetic policy matrix and durable ledger with crash/storage/retention failure injection.
- [x] Approve the exact [dormant journal contract](action-journal.md)/ADR 028 and private core-metadata/fingerprint extension
  before production code; do not claim the full Spec 011 broker is accepted.
- [x] TDD/implement locally the complete Linux-only SQLite adapter with fixed Provision/OpenExisting, canonical audit metadata,
  two uncertainty-retaining permits, 1,000 records, 30-day retention/floor and no unknown-clear method.
- [x] Local proof: derived DB+journal ceiling plus honestly bounded before/after samples, maximal terminal completion,
  actual private-fixture ENOSPC, commit uncertainty, hot/subprocess crash recovery and no replay/new-key bypass.
- [x] Obtain exact final journal root/independent code review and fresh CI: PR161 merged as
  cb5331dc6a5efbf0ed55d94490bcdad64031ecaa after all18 checks/all10 workflows passed. No action activation follows.
- [ ] Prove unknown outcomes cannot auto-retry, unlock silently or lose their dispatch audit.
- [ ] Implement/test separate broker process, protected state and fixed local Docker adapter.
- [ ] Obtain exact root/independent review of [fixed Engine contract](engine-adapter.md)/ADR029; journal all-green merge
  precedes implementation. Drafting is not approval for code or native mutations.
- [ ] TDD the unused Linux adapter's fixed1.45 HTTP, full-ID/selected-field/witness checks, bounds and no-retry faults;
  preserve observation APIs/dependencies and prove Worker+real-journal replay safety.
- [ ] Separately approve/review then execute one exact image/full-ID/label-owned disposable native fixture with fixed
  caps; record actual tested daemon/API1.45 evidence and cleanup without touching any other workload.
- [ ] Prove start/stop/restart, grace and external-state-race semantics with disposable native fixtures.
- [ ] Integrate explicit enrollment, grant/revoke and execution transport without observation-credential reuse.
- [ ] Add accessible permission-filtered UI and independent responsive UX review.
- [ ] Document enable/disable/rotate/upgrade/recover/remove and known authority/health limitations.
- [ ] Complete requirement-to-test traceability, independent code/security review and release activation evidence.

The repository's connected observation work remains ahead of this implementation. Kubernetes rollout actions require a
separate future spec. No checklist completion here authorizes manipulating existing host containers.
