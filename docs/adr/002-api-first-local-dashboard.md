# ADR 002: Use an API-first, same-origin local dashboard

**Status:** Accepted  
**Date:** 2026-09-09

## Context

The observer needs an optional local dashboard, while Platform Demo needs a remote multi-user management view. A hosted browser cannot reliably or safely discover and call a user's loopback service.

## Decision

Serve the optional local dashboard and `/api/v1` from the same explicit loopback origin. The UI consumes only sanitized versioned APIs. Platform Demo uses its own authenticated server APIs and a transport adapter for the same normalized view model; it neither iframes the local UI nor calls localhost from the hosted page.

The local web role is on-demand by default, bearer-protected except for detail-free health, rejects foreign Host/Origin values, emits no CORS grant, and supplies strict CSP and anti-framing headers. LAN binding is out of v1.

## Consequences

- Local use remains offline-capable and independent.
- Platform Demo keeps ownership, delegation, enrollment, and future action concerns in its own shell.
- Shared presentation code can be versioned without coupling authentication or routing.
- Remote viewing uses Platform upload or an operator-managed SSH tunnel, not an accidental unauthenticated LAN product.
