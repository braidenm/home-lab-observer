# Independent proposal review

## Open-source alignment follow-up

On 2026-09-10 an independent security reviewer compared Beszel, Portainer, Cockpit, Netdata and Docker primary guidance.
The observation-first sequence fits the request. The [research record](../../docs/architecture-research/007-open-source-alignment.md)
incorporates the review's important qualifications: GET-only code/socket mounts are not OS containment, both hub and
agent validate targets, outbound replies cannot carry observation-authorized commands, and ambiguous actions cannot
use observation retry semantics. This review does not certify an unimplemented broker or approve draft isolation defaults.

## Original proposal review

Reviewed 2026-09-10. Scope and delivery order are coherent: the owner approved separately permissioned start, stop
and restart, after connected read-only observation. This does not approve implementation or activate daemon access.

Before the implementation checkpoint, resolve and test:

- **Authority and transport:** Freeze outbound installation-bound delivery, broker-only credential audience and holder,
  local IPC peer authentication, rotation/revocation/re-enrollment and fail-closed freshness. Observation roles must
  neither mint actions nor gain an unrestricted local broker transport. No offline execution by inference.
- **Target privacy:** Prefer opaque broker-minted remote references. Keep their full Docker ID and engine-generation
  mapping in protected local state; specify creation/invalidation rules. Remote storage of full IDs is a distinct
  privacy decision, not implicitly authorized by requesting dashboard controls.
- **OS isolation:** Specify supported accounts, IPC ACLs and peer checks, executable/update verification, lifecycle and
  threat model. Separate processes alone do not establish a security boundary against the same user.
- **Unknown outcomes:** Define an owner-authenticated reconciliation transition and exact lock-release conditions.
  Acknowledgement must never redispatch the action or erase its unresolved audit history.
- **Grace period:** Bind the owner-approved timeout to action policy/grant and the idempotency payload. A delegate must
  not independently lower it merely because the adapter accepts a bounded range.
- **Ledger exhaustion:** Specify space reserved for intent plus terminal outcome, denial-flood limits and the recovery
  behavior when unresolved non-evictable intents fill capacity. Never silently evict evidence to accept another action.

The ADR must include primary sources for the selected Engine API's operation/status semantics and the chosen OS peer
security primitives. Existing read-only ADRs remain unchanged. Proposed numerical defaults can support synthetic policy
tests, but they do not resolve these authority, privacy or operational decisions.

Proposal adjustment: R2/R8 now explicitly keep full daemon identities local and use broker-minted opaque remote
references; R4 binds grace to owner policy and denies delegate overrides. These resolve those two ambiguities in the
written proposal, not the pending executable-contract, architecture-acceptance or implementation gates.
