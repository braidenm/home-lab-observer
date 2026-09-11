# ADR 020: First installed Linux connected-worker composition

Status: Proposed; no implementation or installation approval.
Date: 2026-09-11.

## Context and proposed decision

ADR 014's unused adapters now need an installable, tested vertical slice, not further disconnected libraries.
Follow the [installed composition specification](../../specs/012-connected-observation/installed-composition.md):

- Package separate numeric collector and uploader executable roots; never use rich-main branching to assert isolation.
- Enroll using owner-private D2d staging owned by the eventual dedicated uploader UID. A narrow root installation
  transaction seals LoadCredential input and installed metadata while relocating the exact closed ledger without
  reset or cross-UID migration. Only durable installed READY permits service activation.
- Propose systemd 255 exact-public-IP cgroup filtering plus a matching fixed hostname map, no worker DNS authority,
  and the transport's existing TLS/fixed-origin policy for the bounded canary. It is not exclusive-origin or
  destination-port enforcement against a compromised uploader; shared-IP exposure requires reviewer acceptance.
- Manual stopped-worker endpoint refresh is a canary limitation. A later narrow root-owned oneshot/timer reuses that
  transaction for unattended reliability; it is not a general command daemon. Do not claim one-click unattended
  production support before its refresh/expiry and failure behavior are implemented and proven.

## Alternatives and consequences

Reusing the rich command imports excluded adapters before flags are parsed. Chowning a live enrollment tree weakens
the tested ownership/lifetime boundary. Reprovisioning a migrated ledger can reuse sequences. Broad DNS/network
access weakens local-host isolation. A dedicated routed namespace/firewall adds privileged link/rule lifecycle and
still does not provide TLS-origin identity; reserve it for a demonstrated enforcement need.

The proposed native canary is numeric-only, with unproven filesystem coverage reported unavailable, explicit
credential/endpoint recovery, and immutable compatible code rollback. It leaves the legacy connector unchanged.
Its manual refresh, shared-IP residual, architecture/native target limitations and unexecuted installed proof must
remain visible. This proposed ADR does not supersede ADR 014, D1/D2d durability contracts or their acceptance gates.
