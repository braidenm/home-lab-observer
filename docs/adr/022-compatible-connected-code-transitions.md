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
code selection. Before staging, a separate bounded preparation witness binds
the proposed journal digest and unchanged predecessor resources/ledger. It is
intent evidence, not transition, cleanup or activation authority. Recovery can
finish only the exact recorded transition forward,
with workers stopped; it never rolls back durable upload state. Every subsequent
start repeats ADR 021, and no completion record is activation authority.

For each active resource, use a fixed role name bound to the full proposed
journal digest for its temporary file in the same parent directory. Sync the
new file before replacement and the parent directory after rename. An
interrupted temporary file may be discarded and regenerated only after the
exact staged proposal, authoritative journal, unchanged private ledger and
previous-or-next active target are re-established. The legacy generic
`.install-next` name is not transition evidence. Unknown or substituted
resources remain stopped for explicit recovery, never guessed from current
DNS, host CA, or directory order.

The [detailed transition plan](../../specs/012-connected-observation/compatible-code-transition.md)
defines compatibility, legacy journal admission, interruption behavior and tests.
The [staging durability research](../architecture-research/008-connected-transition-durability.md)
records the filesystem and systemd evidence behind the proposed write order.
The bounded [descriptor binding slice](../../specs/012-connected-observation/compatibility-descriptor.md)
recognizes current state/wire contracts using connected v2, while explicitly
marking transitions unsupported and durable filesystem identity unasserted.
That descriptor must be revised before transition-enabled releases; it does not
promote this proposed ADR to implemented or waive any acceptance gate.
The [staging design](../../specs/012-connected-observation/transition-staging.md#legacy-predecessor-design-gate)
requires a closed predecessor format before the first new-format transition:
none, exact legacy refresh, or exact new-format transition. A completed legacy
receipt is retained and digest-bound in the stage, while the new-format
authoritative names remain absent until publication. The pure stage classifier
now admits the exact legacy bytes, but the installer has not proven the fixed
installed legacy files match those staged copies or completed VM migration
acceptance. It must not equate legacy evidence with an initial installation.
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
