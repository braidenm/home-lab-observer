# Simplified product delivery

2026-11-03. Owner-directed scope correction; governed by [ADR032](../../docs/adr/032-reuse-first-management.md).
This replaces the default ten-layer rollout, not the safety rules of an activated custom action worker.

## Keep the working path

Continue native observation, existing installation/downloads, the local dashboard, connected Linux inventory,
Platform-owned server views and read sharing. Improve these without another monitoring hub or identity database.
No collection-policy widening, automatic package replacement or changes to an existing server in this planning slice.

## Three visible milestones

1. **Useful monitoring:** owner can install/connect, see fresh host/process/container readings and share read-only.
   Verify the deployed version and stale/partial/denied states. Fix only measured gaps in that existing journey.
2. **Optional upstream management:** install one pinned Portainer instance only after a separate deployment review.
   Test its own authentication and existing-resource access using a disposable container. Add a clearly labelled
   owner-only management destination to the server view. Explain the separate login; never advertise it as SSO
   or as our container-action permissions. Read-only viewers do not inherit this management link or credentials.
   New integration UI is feature-flagged off by default; no URL is shown without current owner authorization.
3. **In-app controls:** prove one small provider integration for fixed start/stop/restart and bounded log viewing.
   Server-side authorization must bind the current user, registered server and exact container. Enforce independent
   operation grants; prove wrong-user/server/container and read-only-share denial. No shared administrator token,
   arbitrary provider URL/HTTP proxy, terminal, container removal/update or generic command endpoint.

Milestone2 is an interim usable management route, not completion of milestone3. Do not claim delegated writes,
cross-platform connected control or production readiness from a link, mocked API response or green unit suite.

## Immediate tasks

- [x] Inventory existing working components and assess upstream products using primary docs and licenses.
- [x] Record keep/modify/integrate/fork decisions and supersede the default custom-runtime delivery plan.
- [ ] Independently review this scope correction and merge its small documentation PR through required checks.
- [ ] Verify current owner install/view/share acceptance without reinstalling or widening data collection.
- [ ] Inspect whether a suitable management deployment already exists; avoid a duplicate service. Record the chosen
  pinned version/edition, network exposure, resource ownership and credentials workflow before installing anything.
- [ ] Execute milestone2 on a disposable container; provide a genuine owner-visible route and instructions.
- [ ] Freeze only the milestone3 adapter/auth/error/audit contract needed for one actual end-to-end control flow.

## Test and scope budget

Use pure tests for permission/validation/error edges, provider contract tests, and a small real install/view/share
and disposable-container journey. Keep one configured application context; fresh integration IDs and data per test.
Keep established resource caps and required checks. Do not repeat a full suite for unchanged source or introduce
custom process supervisors/fault frameworks solely for this integration. Existing CI failures still need diagnosis;
this scope correction does not permit bypassing them.

Out of this release: custom worker/broker packaging, root-local/remote two-sided recovery rollout, new storage-fault
orchestration, service/host reboot controls, Kubernetes, smart-home integrations and Windows/macOS connected-control
parity. Retain their existing specs/code without activating them. Revisit only for a demonstrated product gap.
