# Open-source alignment for connected observation and Docker control

Research date: 2026-09-10. Scope: inform the next connectivity specification and
[Spec 011](../../specs/011-container-actions/spec.md), not approve a wire protocol or enable remote actions.
Kubernetes is deferred. The owner requests production-oriented choices aligned with comparable open-source projects.

## Context and alternatives

The released observer is local-first: authenticated loopback UI/API, bounded numeric history, opt-in Docker inventory
and native log metadata. Upload and actions are not implemented in the native preview. Preserve those trust boundaries.

| Approach | Fit | Cost or limitation |
| --- | --- | --- |
| Keep local-only preview | Useful offline; smallest remote attack surface | Does not meet hosted monitoring or delegated actions |
| Adopt an existing dashboard as the product | Mature operational workflows; less custom implementation | Must verify edition/license, tenant authorization and cross-platform fit; cannot assume our resource grants map directly |
| Extend our agent with established hub/agent patterns | Preserves local UI, versioned contracts and Platform Demo integration | We own protocol, recovery, release and security testing; avoid inventing cryptography |

Recommendation: use the third approach within this project's accepted modular architecture. Reuse maintained libraries
and standards when appropriate; compare adoption again before building a general-purpose orchestrator, terminal or log
search engine. This is a project-fit decision, not a claim that a custom dashboard is safer than established products.

## Primary-source comparison

Sources below are rolling documentation accessed on the research date, not proof of behavior in every release or edition.
Any implementation dependency needs an exact version/license review separately.

- **Beszel** (documentation identifies 0.19.0): its [security guide](https://beszel.dev/guide/security) describes
  agent-initiated WebSocket connections and authenticated hub/agent association. Its
  [installation guide](https://beszel.dev/guide/agent-installation) provides hub-generated setup commands and multiple
  distribution choices. Apply the simple add-system workflow and outbound topology. Do not copy its handshake as our
  cryptographic protocol, use machine fingerprints as ownership, or put reusable secrets in copied commands.
- **Portainer**: [Edge Agent architecture](https://docs.portainer.io/advanced/edge-agent) avoids an Internet-facing
  agent listener through outbound polling and a reverse tunnel. Its
  [connectivity security guide](https://docs.portainer.io/faqs/getting-started/how-does-portainer-secure-connectivity-to-and-from-agents-and-edge-agents)
  describes agent association and additional Edge certificate protections. Apply explicit pairing, connection health
  and permission-aware management. Our three actions do not need a generic reverse tunnel or forwarded Engine API.
  Do not copy certificate-verification bypass examples. Edition availability requires separate verification before reuse.
- **Netdata**: [parent/child reference](https://learn.netdata.cloud/docs/netdata-parents/parent-child-configuration-reference)
  separates local collection from centralized viewing and describes reconnection, retention and replication. Apply
  offline continuity, visible freshness and bounded delivery. Its custom streaming protocol is not an HTTPS upload API;
  adopting it would introduce a protocol/server dependency. Certificate verification stays mandatory here regardless
  of another tool's documented defaults. Do not promise lossless history when our queue has dropped data.
- **Docker**: [authorization guidance](https://docs.docker.com/engine/extend/plugins_authorization/) establishes that
  ordinary daemon access is broad authority. A fixed-operation broker reduces exposed application functionality but
  does not remove the broker's underlying daemon privileges. A socket mounted read-only is not an HTTP operation policy.
  Record the residual authority and test the separation; do not silently reconfigure an owner's daemon.
- **Cockpit** ([main-branch privilege guide](https://docs.cockpit-project.org/cockpit-guide/main/guide/privileges.html)):
  effective privileges are visible and derive from a host login session. Apply clear privilege indicators, but do not
  map a hosted dashboard account to a broad host login or administrative session. Our resource grants stay narrower.

The [Beszel path-traversal advisory](https://github.com/henrygd/beszel/security/advisories/GHSA-phwh-4f42-gwf3)
(affected through 0.18.3, fixed in 0.18.4) demonstrates why authenticated read-only access still needs strict target
validation. Validate opaque references at the hub and resolve full immutable IDs locally; construct only fixed Engine
paths and reject traversal/query injection at the agent too. Never forward unrestricted inspect responses.

## Required product and verification consequences

1. Onboarding selects OS/architecture and explains observation versus optional control, downloaded artifact trust,
   enrollment expiry, installation scope, data sent, update/rollback and complete uninstall. No GitHub token for public
   downloads. Do not say connected until an authenticated upload is accepted.
2. Outbound authenticated TLS connectivity must work without inbound host ports. Freeze credential issuance, rotation,
   revocation, audience/installation binding and platform-specific isolation in an evidence-backed ADR. Shared process
   identity and encrypted storage alone must not be described as isolation from a compromised local owner account.
   The current GET-only adapter is application-level read-only, not containment against a compromised socket-owning
   process. Strong containment needs restricted local mediation as well as isolated credentials. Observation replies
   must use a closed, bounded contract with no command envelope; outbound networking is not one-way authority.
3. Preserve local collection through hub outage; bound spool bytes/age and retries with jitter. Test full disk,
   reconnection, expired/revoked credentials, replay, duplicate delivery and legacy-agent coexistence. Surface gaps,
   last collection versus last receipt, authorization failure and unsupported sources distinctly.
4. Approve a remote-safe field contract separately from local data. Negative tests reject accidental process arguments,
   raw logs, environment variables and Docker configuration. A rich dashboard does not imply unrestricted upload.
5. Controls require both local opt-in and separate per-resource start/stop/restart grants. Observation credentials cannot
   dispatch actions. Test revocation, recreated targets, duplicate requests and crash-ambiguous outcomes with synthetic
   transports before disposable-container tests. Never retry ambiguous mutations automatically.
6. UI review covers phone/desktop layouts, keyboard access, explicit server selection, stale/partial data and clear
   action confirmation/results. A successful Engine response is not proof of application health.

## Rollout and unresolved work

Start with additive contracts and synthetic end-to-end connectivity; retain the legacy agent path during migration.
Gate native connection independently from action activation. Rollback disables new enrollment/delivery or control
without erasing local history or unresolved audit records. An owner-server canary is separate from fixture tests.

This comparison does not settle the exact auth transport or certify production readiness. The next specification/ADR
must close those choices with tests and operational limits. Signing/notarization gaps remain explicit release gates.
No new owner product question is needed for this comparison; detailed architecture choices remain subject to review.
