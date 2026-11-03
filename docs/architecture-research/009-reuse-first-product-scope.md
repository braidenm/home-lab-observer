# Reuse-first reassessment

Reviewed 2026-11-03 against the working repository and upstream primary documentation.
This supersedes research007's recommendation to extend a custom management platform by default.
It does not certify an upstream version or authorize an installation.

## Keep, modify, integrate, or fork

| Area | Decision | Reason / boundary |
| --- | --- | --- |
| Host/process/Docker observation | Keep existing Go collectors and gopsutil | Already working; replacing them would repeat collection, privacy, installation and compatibility work. |
| Local UI/API, bounded history and outbound upload | Keep | These make Observer useful independently and already support the connected Linux flow. |
| Platform ownership, sharing and feature permissions | Keep | An upstream account is not automatically the same principal/resource as a Platform account. |
| Dashboard UX and setup | Modify incrementally | Improve the existing views and instructions; do not introduce a second monitoring hub just for charts. |
| Broad container operations and logs | Integrate Portainer as an optional upstream product | Start with an explicitly separate authenticated management destination. A narrow embedded API adapter follows only after permission mapping is proven. |
| Beszel | Reference / optional alternative, not a required service | Strong monitoring product, but another hub, history store and identity system duplicate the working path. |
| Glances | Alternative collector, not the default replacement | Cross-platform observation/API; Python deployment and no replacement for our ownership model. |
| Dozzle | Alternative to Portainer for a logs-first deployment | Do not install both by default. Its action switch enables more than our three operations. |
| Dockge | Not selected | Compose editing/deletion/terminal management exceeds the initial product scope. |
| Forks | None initially | No demonstrated required feature justifies maintaining an upstream fork. Prefer documented interfaces and upstream fixes. |
| Custom packaged broker/recovery framework | Defer | Preserve its reviewed code/evidence, but stop making completion of it a prerequisite for useful monitoring or upstream management. |

## Why Portainer, with limits

Portainer documents per-user API access inheriting that user's permissions. Its resource ownership model must
be checked for existing containers: resources deployed outside Portainer are external and default to admin-only.
Fine-grained RBAC is a Business Edition feature; do not promise that Community Edition natively implements
our independent start/stop/restart grants.

The initial upstream UI is a separate product, with its own login and permissions. An owner-only link is not
SSO, an iframe integration, delegated management or our in-app buttons. Never manufacture that appearance by
forwarding a shared administrator token, trusting browser-supplied user headers, or treating a display filter
as authorization. Remote reachability must be designed explicitly; no public Docker API or automatic tunnel.

The existing typed `actionengine` and `actioncore` are the first reuse candidates if a measured provider gap
requires a narrow custom operation. Do not replace their tested Docker verbs with another SDK just for fashion.
Compare that small path against upstream identity/deployment integration cost before choosing a fallback.

For embedded buttons, use one small server-side provider adapter with fixed operations, server/resource binding,
current Platform authorization and appropriately scoped upstream identity. Both layers must deny unauthorized
access. Keep credentials out of browser responses, logs and URLs. If the chosen edition cannot enforce the
required boundary, leave delegation/buttons disabled and revisit the provider rather than silently widening it.

## Primary sources and licensing

- [Beszel overview](https://github.com/henrygd/beszel): hub/agent, Docker metrics, history and multi-user sharing.
  [API](https://beszel.dev/guide/rest-api) is not promised stable across minor versions; pin a tested release if integrated.
- [Glances](https://github.com/nicolargo/glances): cross-platform system/process/container monitoring and REST API.
  Its current develop branch describes beta functionality; evaluate a stable release rather than copying develop examples.
- [Dozzle actions](https://dozzle.dev/guide/actions): start/stop/restart plus remove/update; not a three-operation grant.
  [Authentication](https://dozzle.dev/guide/authentication) needs an explicit protected boundary.
- [Dockge scope](https://github.com/louislam/dockge): Compose-oriented stack management and terminal features.
- [Portainer API](https://docs.portainer.io/api/access), [resource ownership](https://docs.portainer.io/advanced/access-control),
  and [Business Edition RBAC](https://docs.portainer.io/admin/user/roles) establish the integration constraints above.
- [Portainer CE license](https://raw.githubusercontent.com/portainer/portainer/develop/LICENSE) permits reuse with
  origin/alteration/notice requirements. Beszel, Dozzle and Dockge declare MIT; current Glances declares LGPLv3.
  These are repository-level observations, not a complete dependency or redistribution clearance. Review the
  exact selected release and notices before packaging anything; no upstream code is copied by this decision.

Do not recreate our custom lease/journal/recovery protocol around the upstream tool: that would retain the
same complexity while adding another service. Existing custom-action components remain dormant, with their
uncertainty and compatibility safeguards intact. Choosing integration is not permission to weaken those components.
