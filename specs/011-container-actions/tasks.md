# Tasks

**Status:** Proposed; unchecked items are future work, not delivered capabilities.

- [x] Record owner's accepted first action set: independently granted start, stop and restart.
- [x] Draft proposed scope, safety semantics, boundaries and primary-source research.
- [ ] Complete independent architecture/security and owner review of proposed decisions and numerical limits.
- [ ] Accept spec and create a new ADR without rewriting accepted read-only decisions.
- [ ] Freeze versioned action/grant/result/audit contracts, credential audiences and standalone-owner authentication.
- [ ] Specify remote freshness/revocation protocol and request replay-window semantics.
- [ ] Implement/test synthetic policy matrix and durable ledger with crash/storage/retention failure injection.
- [ ] Prove unknown outcomes cannot auto-retry, unlock silently or lose their dispatch audit.
- [ ] Implement/test separate broker process, protected state and fixed local Docker adapter.
- [ ] Prove start/stop/restart, grace and external-state-race semantics with disposable native fixtures.
- [ ] Integrate explicit enrollment, grant/revoke and execution transport without observation-credential reuse.
- [ ] Add accessible permission-filtered UI and independent responsive UX review.
- [ ] Document enable/disable/rotate/upgrade/recover/remove and known authority/health limitations.
- [ ] Complete requirement-to-test traceability, independent code/security review and release activation evidence.

The repository's connected observation work remains ahead of this implementation. Kubernetes rollout actions require a
separate future spec. No checklist completion here authorizes manipulating existing host containers.
