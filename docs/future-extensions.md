# Future extension backlog

Status: proposed direction, recorded 2026-09-17. This is not an implementation,
installation, compatibility or permission claim. Existing accepted ADRs remain in
force. Each extension needs a focused specification and independent review before
implementation or activation.

## Deliver the current product first

1. Complete the Linux connected dashboard: verified installation, isolated collector
   and uploader, explicit enrollment, honest per-group quality and freshness,
   owner/delegate access, recovery, and packaged positive/negative acceptance.
2. Deliver the separately permissioned Docker start, stop and restart slice described
   by [Spec 011](../specs/011-container-actions/spec.md). Reading observations grants
   none of these actions; each action requires its own approved resource grant.
3. Keep Kubernetes and smart-device control deferred. They do not expand the current
   numeric-host upload policy or block completion of the two priorities above.

## Keep the observer a standalone product

Headless collection and an optional authenticated local UI remain useful without a
Platform Demo account or cloud connection. Platform Demo is an authorized consumer
of versioned APIs/read models, not the owner of native collection internals. Reusable
presentation components may share a transport-neutral model; hosted pages must not
iframe or directly reach another person's localhost dashboard.

Future provider adapters should expose explicit, versioned capabilities and normalized
observations. Unsupported, denied, partial, disconnected and known-zero values remain
distinct. A new provider does not automatically gain remote-upload eligibility,
credentials, local filesystem access, or permission to perform actions. Define bounds,
redaction, freshness, retention and compatibility tests per provider.

## Resource-scoped permissions

Separate the ability to discover an explicitly configured resource in the product,
read its permitted observations, administer its grants, and invoke each fixed action.
Bind grants to the owner, installation/provider connection generation and stable
resource identity; display labels and dashboard aliases are not authorization keys.
Delegated access cannot silently become administrator or redelegation authority.

The UI shows only permitted areas/actions, but every backend read and action must
enforce the same resource policy. Future action adapters retain durable intent/audit,
execution-time authorization, expiry/revocation and explicit unknown outcomes. No
automatic retry of an ambiguous physical or operational action is implied.

## Deferred smart-device dashboard

A possible later slice is an opt-in dashboard for selected devices already managed
by an owner-configured integration platform, with simple separately granted typed
controls. Home Assistant is one candidate to evaluate, not a selected integration or
supported dependency. This backlog does not prescribe its API, token model, release
baseline or deployment topology; those require primary-source research and a new
provider-specific spec/ADR when prioritized.

Start that future design by choosing a small resource and observation allowlist,
then separately deciding which controls are safe enough to expose. Do not assume a
generic device service call is an acceptable control boundary. Safety-critical devices,
unattended automations, third-party account linking and credential custody need their
own explicit decisions and acceptance evidence.

There is **no automatic discovery, LAN scan, arbitrary URL/API forwarding, arbitrary
command execution, new inbound listener, or device control in the current slice**.
Existing owner-managed integrations are a possible future source, not permission to
probe a network or reuse credentials today.

## Entry gate for any later extension

- A small user outcome, supported provider/version and explicit local versus remote
  data policy, with documented unsupported cases.
- Closed bounded contracts, per-resource read/action grants and negative authorization
  tests; separate credentials and isolation where required by accepted architecture.
- Synthetic provider fixtures plus actual packaged integration evidence, safe audit
  and diagnostics, responsive/accessible UI and honest degraded states.
- Install, revoke, rotate, disable, upgrade, rollback and uninstall behavior, including
  crash/ambiguous-action handling before calling the feature production-ready.

See [the delivery sequence](delivery-plan.md),
[connected observation](../specs/012-connected-observation/spec.md), and
[the separate Docker action proposal](../specs/011-container-actions/spec.md).
