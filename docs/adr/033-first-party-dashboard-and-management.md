# ADR033: First-party dashboards and library-level reuse

Status: Accepted product direction, explicitly clarified by the owner on 2026-11-03; control implementation and activation remain gated.
Supersedes ADR032's third-party management destination/provider selection, not existing safety contracts.

## Context and decision

The owner wants one Observer product with its own useful local dashboard/API and
a hosted first-party server view. Reuse means learning from solved open-source
problems and using focused libraries or appropriately licensed code behind our
interfaces. It does not mean requiring another management UI, login or product API.

Keep existing Go/gopsutil collection, bounded history, local UI/API, connected
Linux upload and Platform ownership/read sharing. Remove the Portainer product
integration from the roadmap. Its unmerged Platform navigation PR was closed;
no instance, credential, route or feature was installed or activated.

For container controls, first reuse our existing typed Docker Engine adapter and
action core. Their exact-resource, independent-operation and uncertain-outcome
rules remain applicable. Complete the smallest consumed owner/disposable-container
journey before adding UI buttons; do not restart a broad bespoke platform rollout.
Local and hosted views remain our own presentation of explicit supported contracts;
this decision does not claim they already have working controls or equal capabilities.

A focused maintained library is preferable when it solves a measured gap with
less maintenance than our existing adapter. Any copied code needs compatible
license/attribution, source provenance and security/contract tests. No new library
or fork is selected here. A future third-party service is permissible only if it
is fully wrapped behind our API/UI and a separately reviewed auth, deployment,
license and maintenance decision; it must not become a user-facing dependency.

## Alternatives and consequences

- External Portainer UI: quick general management, but creates another login and
  product experience; explicitly rejected by the owner.
- Fully wrapped upstream service: potentially reusable implementation, but adds
  deployment and authorization mapping; not selected without a demonstrated gap.
- Existing typed Engine adapter plus focused utility reuse: keeps the product
  coherent and reuses reviewed operations without another product API; selected.

Observation remains read-only by default and independently useful without Platform.
Management requires explicit opt-in and separate credentials/permissions. No generic
Docker proxy, shell, automatic mutation retry, erased unknown outcome, relaxed
authorization or implied write permission for viewers. Logs remain a separately
scoped, opted-in and bounded feature. Kubernetes stays deferred; processes remain
observation-only. Preserve dormant code/evidence rather than deleting or activating it.

The [updated delivery plan](../../specs/011-container-actions/reuse-first-plan.md)
owns the next acceptance steps. This correction adds no runtime or deployment change.
