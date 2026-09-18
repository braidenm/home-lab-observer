# ADR 022: Compatible connected code transitions

Status: Proposed; implementation and independent review required.
Date: 2026-09-17.

## Context

F1 must support code rollback without restoring an older upload sequence, pending
request, credential or endpoint policy. Its immutable release directories are not
transition history. The existing refresh completion also binds the full installed
configuration, so changing an artifact digest beside it would break that evidence.
The pristine enrollment validator deliberately cannot validate a used ledger.

## Proposed decision

Implement explicit verified code selection and one recorded previous-code target,
not version inference. A separate connected bundle/identity v2 binds an exact
compiled compatibility contract. The current installed installer accepts only its
known contract and unchanged supported sandbox/resources. A separately named
offline existing-state validator runs each compatible uploader against the same
ledger, preserves every logical field and pending byte, and emits only a bounded
fingerprint. It receives no credential, handoff, network or arbitrary path.

Use one bounded typed transition journal for endpoint refresh and code selection.
Its before/after configurations, predecessor completion and operation-specific
changes are strict. Completion records the previous artifact only for a successful
code selection. Recovery can finish only the exact recorded transition forward,
with workers stopped; it never rolls back durable upload state. Every subsequent
start requires a fresh activation transaction, and no completion record is
activation authority.

The [detailed transition plan](../../specs/012-connected-observation/compatible-code-transition.md)
defines compatibility, legacy journal admission, interruption behavior and tests.
ADR 020 remains authoritative on isolation and ledger preservation; this proposal
supersedes no accepted ADR until reviewed and implemented.

## Alternatives and limits

Selecting the newest/oldest directory, trusting semver or accepting a target's
unrecognized compatibility assertion are rejected. Copying back a database is
not code rollback. Independent refresh/code journals would require coupled
receipts and permit contradictory current identities, so one small fixed
transition protocol is preferred over a generic migration framework.

The first contract intentionally excludes schema migration and sandbox changes.
It cannot downgrade into a bundle with no known contract. Compatibility fields
are not a mathematical proof: reviewed builds, exact trusted digests and two-release
forward/back fixtures remain release requirements. Unsupported state stays stopped.
The first canary still has no automatic boot or crash restart. Packaged VM and
power-loss/recovery acceptance remain open and cannot be waived by this ADR.
