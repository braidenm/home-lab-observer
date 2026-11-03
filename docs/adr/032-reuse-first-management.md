# ADR032: Reuse-first management instead of a bespoke control platform

Status: Superseded by [ADR033](033-first-party-dashboard-and-management.md) after the owner's explicit clarification on 2026-11-03. No Portainer integration was deployed.
Supersedes ADR030/ADR031 as the default product delivery path, not their safety contracts for dormant code.

## Decision

Keep the working Observer collector, local UI/API, bounded storage, Linux connection and Platform ownership/sharing.
Retain maintained libraries already in use. Do not replace working monitoring with another hub merely to claim reuse.

Select optional upstream Portainer integration for general container operations/logs. First prove an independently
authenticated owner-only management destination; then a small provider adapter for in-app operations, if the
selected edition and resource permissions support it. No fork, copied upstream UI, shared admin-token proxy,
mandatory Beszel/Glances/Dozzle stack or new custom distributed recovery platform is selected.

The hosted app remains our product UI. Linking out is an explicitly labelled interim capability, not completion
of the requested in-app start/stop/restart buttons. Observation and shared read-only access remain independent.
An upstream login does not satisfy Platform authorization, and Platform login does not grant upstream access.

## Security and completion boundary

- No generic shell/exec, unrestricted Docker proxy, implicit write grant, credential-in-URL or unauthenticated listener.
- No commands in an observation upload. Management stays separately opted-in and permissioned.
- Independent start/stop/restart permissions and exact resource binding remain requirements for embedded controls.
- Keep current authorization checks and audit. A timeout is not success and never triggers automatic mutation replay.
- Do not erase unknown outcomes, drop journal state, bypass existing gates or activate the dormant custom worker.
- Pin/review the chosen upstream release, edition, license, deployment exposure and resource mapping before installation.
- Test with a labelled disposable container before any existing workload; prove denied users/resources too.

The [reuse assessment](../architecture-research/009-reuse-first-product-scope.md) records primary evidence and
the CE/BE permission distinction. The [simplified plan](../../specs/011-container-actions/reuse-first-plan.md)
owns acceptance criteria. The constitution is unchanged.

## Consequences

The ten-layer custom-runtime rollout is no longer the default roadmap. Preserve the existing branches, libraries,
specs and evidence as dormant R&D; no deletion, migration, reset, release or weakening is implied. Do not merge
further foundation layers solely to finish that rollout. Reusable changes still need an independently useful scope.

This reduces custom execution/recovery/installer maintenance, but does not make provider integration free.
Portainer needs its own installation/account and its authorization model may not match ours. If the narrow
adapter cannot meet requirements without paid features or broad credentials, stop and record that measured gap;
do not expand scope or start a fork automatically. Windows/macOS monitoring stays supported as documented;
connected management parity and Kubernetes remain later work.
