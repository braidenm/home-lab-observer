# ADR 021: Connected-worker activation evidence

Status: Accepted for implementation after independent review; no new activation authority.
Date: 2026-09-17.

## Context and recommendation

ADR 020 requires packaged negative acceptance before activation, but must also
define how start/restart/refresh reject later drift. Use the
[activation design](../../specs/012-connected-observation/activation-design.md):
release-time disposable-VM proof, current root-owned artifact/manager validation,
and fixed checks in each actual worker before normal work. Do not substitute a
durable success receipt, systemd security score, owner checkbox or generic test
binary for the actual supported profile. Root remains trusted.

## Alternatives and consequences

Keeping installation stopped is safe but incomplete. A durable receipt alone
creates stale/replay authority; a generic test binary can diverge from the worker.
Actual-process checks preserve namespace fidelity and fail closed, but add bounded
startup work and require explicit platform-specific fixtures. A bounded volatile
root request/commit and fresh worker challenge bind proof to the same paused
invocation. Initial boot/crash auto-start remains disabled; a later root oneshot
must repeat the same gate. No accepted
ADR is superseded, no Docker/action capability is added, and no installed safety
or cross-platform claim follows from this proposal.

The linked design records primary sources, use cases, tests, diagnostics, refresh,
rollback and remaining gates. Production activation remains unavailable until
those requirements are implemented and demonstrated.
