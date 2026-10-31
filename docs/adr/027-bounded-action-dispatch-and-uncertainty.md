# ADR 027: Bounded action dispatch and persistent uncertainty

Status: Proposed for independent review; no executable action broker or activation.
Date: 2026-10-31. Owner: maintainers; scope: the future connected action profile in Spec 011.

## Question and existing boundary

How should a separately credentialed outbound broker fit a remote execution lease without confusing observation
aliases with action authority or repeating an ambiguous Docker operation? [Spec 011](../../specs/011-container-actions/spec.md)
already proposes exact local identity, independent start/stop/restart grants and a durable unknown-outcome fence.
The current `internal/containerobs` adapter issues GETs only; observation credentials, installed observation
definitions and native log metadata remain unchanged. This proposal does not approve broker OS/IPC isolation,
standalone-owner authentication, remote wire codecs, numerical storage/cooldown limits or a public package.

## Proposed coordination rules

1. **Finite grace inside existing budgets.** Narrow the former proposed 1..120-second grace to integer **1..30**,
   default **30 seconds**, for stop/restart; start has no options. Preserve the configured container stop signal.
   Reject zero, negative/infinite grace, caller signals and delegate overrides. Remote claims need a full 120-second
   lease; reject execution when claim-response latency exceeds 30 seconds. Keep the Docker call at most 45 seconds
   and the full lock/inspect/policy/call/durable-journal phase at most 60 seconds after receipt and 90 seconds from
   request start. Reserve time for every non-call phase; refuse dispatch if the remaining monotonic budget cannot
   accommodate it. Grace is not an HTTP timeout or a completion guarantee. No automatic mutation retries.
2. **Pin the parameters and private target.** Resolve grace from owner-approved policy, never the browser. Bind a
   closed versioned canonical parameter value (no options for start, exactly `stop_grace_seconds` for stop/restart)
   and policy/capability revisions to admission, request fingerprint, claim, journal and audit; recheck equality
   immediately before dispatch. The broker alone maps opaque target ID to installation, owner-approved engine
   connection generation and full immutable container ID. Names, short IDs, labels and observation hashes confer
   no authority. Changing/recreating/rebinding the engine or container creates a new target and fresh approval.
   Engine generation is an explicit approved binding, not an inferred PID/socket inode. Missing mapping refuses;
   never reconstruct it from a display alias or copy a prior installation's journal/credentials.
3. **Lease recovery is delivery recovery.** Reoffering the same action ID/fingerprint after a lease expires only
   permits lookup in the installation's exclusive durable journal. A different/missing/unreadable journal cannot
   authorize an alternate executor. Persist/flush dispatch intent before the Engine call. On restart, an intent
   without durable terminal outcome is `OUTCOME_UNKNOWN`, even if the crash might have preceded the actual write.
   A known terminal result can be reported again without another call; an unknown request never runs again.
   Only a provably undispatched admission may execute under a newly valid lease and fresh authorization.
4. **Unknown outlives expiry.** Server expiry/cancellation, lease replacement, pruning or a new idempotency key
   cannot clear the local unresolved target fence. Refuse new actions for that target until a separately
   authenticated owner reconciliation records acknowledgement and bounded read-only inspection. Inspection does
   not prove causality, erase audit, redispatch the old request or grant authority. A subsequent action needs a
   new key/current authorization. Loss of registration/credential authority cannot trigger implicit re-enrollment;
   the authenticated recovery ceremony must be specified before enabling that path. Remote late results cannot
   rewrite a terminal expired action. This is conservative duplicate suppression, not exactly-once execution.

## Alternatives and consequences

| Option | Benefit | Cost / conclusion |
| --- | --- | --- |
| Leave actions disabled | No new mutation authority | Safe status quo while proof is incomplete |
| Retain 120-second grace by widening the lease | Longer graceful shutdown | Changes coordinated timing/recovery policy; not selected |
| Finite grace and a durable uncertainty fence | Reuses typed outbound boundaries and accepted finite budgets | Availability may require owner reconciliation; proposed choice |
| Retry after timeout or infer success from running state | Simpler recovery | Can duplicate a mutation or claim false causality; rejected |

Daemon access remains broad authority inside the trusted broker; typed code does not install a daemon authorization
plugin or make socket access intrinsically least-privileged. No daemon restart, shell, generic proxy, Kubernetes,
new dependency or widening of the current read-only package follows from this proposal.

## Proof, migration and rollback

Freeze bounded request/result/audit/parameter codecs, state/result mapping and OS/IPC confinement before code.
Future persisted/wire readers precede writers. An existing opaque target plus capability revision may bind parameters
through a complete immutable approved-policy snapshot retained for outstanding/replayed/unknown requests. Compare
that bounded reference design with a proposed explicit parameter receipt/new fact; the latter adds reader/rollback
cost, the former requires proven immutable retention. A revision number or mutable latest row alone is insufficient.
No new fact or broader retention is approved here, and this choice does not block dormant credential lifecycle work.
Synthetic tests must cover grace 0/1/30/31/negative, policy/fingerprint substitution, exhausted phase budgets,
engine/container rebinding, cross-install/forged observation IDs, storage/fsync failure and crash at both sides of
dispatch intent. Reclaimed delivery, response loss, restart, new keys and retention must produce no second call
after intent/unknown; expiry never silently unlocks the target. Prove exact independent operation grants and both
orders of revocation races, then use only one disposable container for native/package acceptance.
No tests for this new contract or live actions are claimed by this document. Rollback disables new admissions and
claims, preserves unresolved journals/audit and compatible readers, and leaves observation running. Standalone
owner authentication and authenticated unknown-outcome recovery remain explicit implementation prerequisites.

## Primary evidence and limits

Checked 2026-10-31: [Engine API v1.45](https://raw.githubusercontent.com/moby/moby/v26.0.2/docs/api/v1.45.yaml)
defines distinct existing-container operations and stop/restart timeout parameters. Current
[stop](https://docs.docker.com/reference/cli/docker/container/stop/) and
[restart](https://docs.docker.com/reference/cli/docker/container/restart/) guidance describes force after grace and
the infinite timeout our policy excludes. Neither supplies an application-level idempotency transaction.
[Docker authorization](https://docs.docker.com/engine/extend/plugins_authorization/) documents broad default daemon
authority; a plugin is a separate daemon configuration decision, not part of this proposal.
[PostgreSQL 16 row locking](https://www.postgresql.org/docs/16/explicit-locking.html) can serialize remote authority
changes but cannot atomically commit local Docker effects. [OWASP authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)
supports fresh least-privilege checks, not these product-selected time limits. The Engine adapter's actual supported
version range and all degraded behavior still require executable, disposable-fixture proof.
