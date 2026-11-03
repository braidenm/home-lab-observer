# Simplified first-party product delivery

2026-11-03 owner clarification; governed by [ADR033](../../docs/adr/033-first-party-dashboard-and-management.md).
This replaces the Portainer destination/provider plan, not the safety rules of an activated action worker.
It does not restore a ten-layer prerequisite rollout.

## Keep the working path

Continue native observation, existing installation/downloads, the local dashboard, connected Linux inventory,
Platform-owned server views and read sharing. Improve these without another monitoring hub or identity database.
No collection-policy widening, automatic package replacement or changes to an existing server in this planning slice.

Reuse existing gopsutil collection and typed `actionengine`/`actioncore` first.
Other projects supply design/test examples or focused compatible licensed code,
not another management product. Review maintenance, license/attribution, provenance
and contract tests for any new utility dependency. No new library or fork is selected.

## Visible milestones

1. **Useful monitoring:** owner can install/connect, see fresh host/process/container readings and share read-only.
   Verify the deployed version and stale/partial/denied states. Fix only measured gaps in that existing journey.
2. **One real operation:** finish only missing consumed wiring between current owner authorization, one exact
   locally approved disposable target and existing typed execution. Prove denied users/resources and one real
   operation before exposing a button. Retain separate action credentials, durable intent and truthful unknown
   outcomes; no automatic mutation retry. No external management UI, login or product API.
3. **Our in-app controls:** expose fixed start/stop/restart through our own local and hosted UI/API adapters.
   Server-side authorization must bind the current user, registered server and exact container. Enforce independent
   operation grants; prove wrong-user/server/container and read-only-share denial. No shared administrator token,
   arbitrary provider URL/HTTP proxy, terminal, container removal/update or generic command endpoint.
   Local adapters enforce current authentication and explicit policy too; loopback is not action authority.
4. **Our container log view:** separately specify opted-in source permissions, redaction and byte/line/time,
   concurrency and retention bounds before collecting bodies. No unrestricted third-party or raw host-log UI.

Do not claim delegated writes, cross-platform control parity or production readiness from a helper,
mocked response or green unit suite. Require exact-package disposable acceptance and rollback instructions.

## Immediate tasks

- [x] Inventory existing working components and assess upstream products using primary docs and licenses.
- [x] Record keep/modify/integrate/fork decisions and supersede the default custom-runtime delivery plan.
- [x] Withdraw unmerged Platform Portainer navigation PR530; no installation or activation occurred.
- [ ] Independently review this clarification and merge its small documentation PR through required checks.
- [ ] Verify current owner install/view/share acceptance without reinstalling or widening data collection.
- [ ] Audit only missing wiring for one owner-authorized disposable operation; reuse existing code, not new layers.
- [ ] Deliver milestone2 before adding our own separately permissioned start/stop/restart buttons.
- [ ] Specify bounded container logs separately, then implement our own view.

## Test and scope budget

Use pure tests for permission/validation/error edges, adapter contract tests, and a small real install/view/share
and disposable-container journey. Keep one configured application context; fresh integration IDs and data per test.
Keep established resource caps and required checks. Do not repeat a full suite for unchanged source or introduce
custom process supervisors/fault frameworks solely for this integration. Existing CI failures still need diagnosis;
this scope correction does not permit bypassing them.

Out of this release: broad custom worker/broker packaging, root-local/remote two-sided recovery rollout, new storage-fault
orchestration, service/host reboot or arbitrary process controls, Kubernetes, smart-home integrations and Windows/macOS connected-control
parity. Retain their existing specs/code without activating them. Revisit only for a demonstrated product gap.
