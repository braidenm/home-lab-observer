# ADR034: Ship bounded controls with locked uncertain outcomes

Status: Accepted product scope by the owner on 2026-11-04; implementation and live acceptance remain gated.
Supersedes ADR030's complete two-sided recovery prerequisite for the first connected release only.
Preserves ADR028/029/030/031's durability, authentication, process separation and isolation requirements.

## Question and context

Can the first owner-only start/stop/restart release defer the larger recovery workflow without mistaking an
ambiguous Docker operation for a safe retry? The owner explicitly selected locked uncertain outcomes with manual
investigation, never automatic retry or clear. ADR033 keeps our own dashboard/API. PR170 records a real disposable
Engine/core/journal proof; PR167/171/172 deliver HTTP, protected credentials and the worker command. None establishes
a matching runnable broker or installed authority. The journal has no unknown-clear API; uncertain delivery latches
the worker off. Reuse those conservative behaviors instead of requiring the full recovery subsystem first.

## Decision

The first connected Linux release supports only the current registered owner and individually approved Docker
START, STOP and RESTART. Keep independent operation permissions, source-backed current ownership, distinct action
credentials, exact protected local mappings and both fresh execution checks. Observation remains read-only.

Uncertain claim, dispatch, result or acknowledgement preserves evidence and refuses further controls for the affected
target. Retain remote HELD lanes and local intent/unknown fences across restart, deployment and expiry. Missing local
evidence after a lost claim cannot release the remote lane. A local terminal with a lost result acknowledgement does
not establish remote completion. The worker may conservatively stop all execution; two retained global journal
permits may stop the whole installation. Availability of unaffected targets is not guaranteed in this first release.

Manual investigation means authorized inspection of bounded status and retained evidence, NOT an unlock ceremony.
There is no supported reset/reconcile/force-clear or uncertain mutation retry. Never infer causal success from current
container state or prune history to regain capacity. New keys, aliases, policy revisions, credentials, replacement
target IDs and re-enrollment must not bypass unresolved evidence for the same native resource. Unsupported binding
or state changes refuse rather than create an empty replacement journal. Missing/corrupt/full state fails closed.

Defer root-local/remote-owner recovery routes, ACK and finalization as a separate enhancement. Dormant recovery code
is neither deleted nor installed/exposed by this choice. There is no supported unlock even after manual inspection;
recoverability is deliberately reduced, not solved. No new schema, event, enum or credential format is selected.

## Alternatives and consequences

| Option | Tradeoff |
| --- | --- |
| Finish full two-sided recovery first | Better recoverability, but delays usable controls; deferred by owner |
| Keep uncertain targets locked | Existing evidence and safety at lower initial complexity; selected, possibly installation-wide unavailability |
| Retry/clear on timeout or observed state | Can duplicate a restart or erase uncertainty; rejected |
| Leave all controls disabled | Safe default and rollback until remaining acceptance passes |

Docker access remains root-equivalent. Keep the trusted outbound worker separate from the networkless typed broker,
protected identities/IPC/settings, deadline/concurrency/storage bounds and compatible reader floor. This scope choice
does not authorize broad socket access, a one-process shortcut or weaker installation.
These persistence rules constrain supported product flows. As in ADR030, host-root compromise or out-of-product
manual destruction of all retained state is outside the trust boundary; this is not a claim to prevent a trusted
host owner from wiping a machine. Product tools must never offer that wipe as recovery or adopt detected missing state.

## Evidence and pitfalls

Primary sources checked 2026-11-04:

- [RFC9110 section9.2.2](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2.2): non-idempotent automatic retries
  need knowledge that repetition is safe or the first request was not applied. Connection failure supplies neither.
  Recovering the original request receipt by its key is not a second Docker dispatch.
- [Docker API1.45](https://docs.docker.com/reference/api/engine/version/v1.45/) and its
  [official schema](https://docs.docker.com/reference/api/engine/version/v1.45.yaml): distinct lifecycle responses
  include already-started/stopped, not already-restarted. They do not establish our cross-system exactly-once outcome.
  Keep the adapter's fixed API1.45 compatibility profile, not the observed host's maximum API1.52.
- [Docker Engine security](https://docs.docker.com/engine/security/): daemon control can confer host authority;
  trusted access and strict parameter checks remain necessary. Typed methods do not sandbox a compromised broker.

The sources support no-retry and the trust boundary, not this particular recovery UX. The owner selected the
availability tradeoff. A running container, clean client exit or fixture cleanup cannot release an ambiguous lane.

## Required proof, rollout and rollback

The [delivery plan](../../specs/011-container-actions/reuse-first-plan.md) must match Platform's first-release scope.
Before activation, exercise the real owner API/credential/worker/guarded broker/Engine journey on one disposable
target, with separately denied operations/users/servers/read-only shares. Inject lost claim/report and restart the
consumers: prove no second mutation or new-key/native-alias bypass. Prove both durable locks survive expiry, restart
and supported upgrades. Preserve storage-failure, installed identity/isolation, exact-package and reader-floor proof.

Show pending and unconfirmed/locked states separately from success or application health. Expose only bounded
diagnostic state/references, never secrets/raw Docker bodies/host logs. Lock or capacity exhaustion is unavailable,
not healthy zero activity. Install disabled; enable only after these proofs, independent review and current-head CI.
Rollback stops admission, joins action processes and retains compatible credentials, policy, journal and lanes.
Never downgrade below readers of emitted facts or retained storage. No data migration is authorized here. A later
unlock workflow needs its own accepted design and end-to-end proof before either fence is released.
