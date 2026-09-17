# Compatible code selection and rollback: remaining design gate

Status: proposed implementation boundary, 2026-09-17. Independent read-only review
identified these gaps; no upgrade, rollback, release or activation approval is implied.
This completes neither [F1](installed-composition.md) nor its packaged acceptance.

## What exists and what does not

The installer retains verified immutable releases and pins one manifest digest in
the installed configuration. This is a useful basis for code rollback, but there is
no completed code-transition history identifying a previous compatible release.
Directory order, timestamps and version strings must not select a rollback target.
Initial installation has no previous release to roll back to.

Three implementation gaps must be closed before claiming safe rollback:

1. The connected manifest identifies a package/profile but does not prove compatible
   ledger, pending wire bytes, credential, configuration and handoff semantics.
2. The existing offline `validate-ledger` operation checks pristine enrollment state.
   A used installation correctly fails that check. Preserve this promotion safeguard;
   add a separately named existing-state compatibility check, never weaken it.
3. Endpoint refresh completion binds the full installed configuration, including its
   artifact digest. Changing that digest independently would leave an inconsistent
   refresh journal. Do not delete or fabricate a refresh completion to permit it.

## Proposed smallest coherent transition

Use a bounded root-owned transition journal for either endpoint refresh or compatible
code selection, with exact previous/next configuration, operation type and expected
prior completion identity. Specify migration from the existing refresh journal before
implementation; accepted ADRs must not be silently rewritten.

A code-selection transition may change only the verified artifact identity and a
monotonic transition generation. Preserve owner/server/connector identity, dedicated
numeric principals, current endpoint policy, credential location and the exact ledger.
No ledger reset, copy-back, re-enrollment or backup restoration is a code rollback.
Record one explicit previous verified artifact only after a successful code selection.

The trusted current installer must verify both bundles against a frozen state-compatibility
contract. That contract must cover the durable ledger schema and sequence rules, pending
request bytes, credential/configuration formats, handoff and activation protocols.
Version labels or a package's own compatibility assertion are insufficient evidence.
The target's fixed offline existing-state validation and cross-version release fixtures
must also pass. Unsupported compatibility leaves both workers stopped for recovery.

Keep the installation lease through stop/join, preparation, exact artifact/unit/config
selection, manager reload, offline compatibility checks and durable completion. Restart
requires a fresh activation transaction; an older success record grants no permission.
Define every interrupted state and allowed recovery operation before implementing writes.

## Required review and proof

- Freeze the compatibility contract and journal migration in a focused ADR/spec update.
- Test each write/sync/reload interruption, mixed generations, unknown prior records,
  same-ledger preservation, retained pending bytes, terminal state and sequence monotonicity.
- Test compatible forward/back selection with two exact synthetic packaged releases;
  reject incompatible state and prove that initial enrollment validation remains strict.
- Include stop/refresh/code-selection contention, failed target validation, unavailable
  endpoint policy and cleanup failures. Never activate a partially switched generation.
- Run packaged VM restart, simulated power-loss and recovery checks. These are still
  prerequisites for the owner-server canary, not replaced by the activation unit tests.

Docker controls, Kubernetes and smart-device providers do not expand this work item.
