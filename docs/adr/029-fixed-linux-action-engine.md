# ADR 029: Fixed dormant Linux Docker action adapter

Status: Draft for independent review; no implementation, real fixture mutation or installed authority approved.
Date: 2026-11-01. Scope: the next unused Spec 011 adapter, after complete dormant journal delivery.

## Question and current evidence

How can the existing typed Engine port issue only approved start/stop/restart without exposing arbitrary Docker API
authority or repeating an ambiguous POST? The [core contract](../../specs/011-container-actions/worker-core.md) and
[journal](../../specs/011-container-actions/action-journal.md) already own authority ordering, parameters, durable
intent, uncertainty and target fences. `internal/containerobs` remains a broader read-only observation adapter;
do not reuse its short-ID/endpoint policy for action authority or refactor its behavior as part of this slice.
The pinned runtime is Go1.27.3; no official Docker SDK is currently a dependency.

Read-only runtime evidence is deliberately separate: the existing owner daemon was reported as Docker29 with
advertised API minimum1.44/maximum1.52; independently queried local Docker Desktop29.9.1 reports minimum1.40/maximum1.56,
Linux/amd64. These advertisements contain1.45 but are not mutation compatibility tests. No private endpoint metadata
belongs in public source. Journal PR161 merged as cb5331dc6a5efbf0ed55d94490bcdad64031ecaa after all18 checks and
all10 triggered workflows passed. That prerequisite does not approve this draft or the full broker.

## Recommendation for review

Use the existing standard library HTTP client with a private Unix transport and exact API **1.45**; implement only
the four typed Engine methods plus fixed construction/resource disposal. The [closed contract](../../specs/011-container-actions/engine-adapter.md)
owns exact routes, bounds, selected fields, finite errors and fixture rules. Public construction fixes
`/engine/docker.sock` and requires an explicit approved rootful socket witness. Socket metadata is drift evidence,
not an automatically minted EngineGeneration or authorization. No inherited context/environment or arbitrary HTTP
client/path/options, retries, daemon error bodies, new dependency, installer or observation privilege expansion.

Validate advertised minimum/maximum on bounded /version, then require their intersection with the singleton1.45
profile. Using this older fixed version intentionally differs from Docker's recommendation to use its latest SDK/API:
the required four operations have a frozen1.45 contract, both observed daemon ranges include it, and a tiny closed
surface is easier to audit. Revisit if the daemon drops1.45, a security advisory changes these semantics, or a separately
approved operation needs a newer API. Refuse unsupported versions instead of silently following latest or downgrading.
Do not claim support for all intervening advertised versions or other OS/rootless profiles.

## Alternatives and consequences

| Option | Fit and cost |
| --- | --- |
| Keep all actions disabled | Safe status quo; remains required until the complete broker gates pass |
| Dedicated stdlib HTTP adapter | Reuses existing dependency/runtime, makes four routes and retry/body limits explicit; owns a small exact decoder/version check; proposed choice |
| Maintained official moby/moby/client SDK | Viable typed/versioned client, especially for a larger API; still needs explicit non-environment host/version, transport/error/privacy/retry wrappers here and adds dependency surface without a needed operation |

The SDK is not intrinsically unsafe. Its general-purpose options are unnecessary for this bounded port; neither
option makes Docker socket access daemon-enforced least privilege. Root/same-privilege malicious replacement remains
outside the endpoint witness guarantee. Raw inspect is unavoidable transient input, not permission to retain it.
Cancellation/timeout can leave a completed daemon operation unknown; only the durable journal prevents replay.

## Primary evidence checked 2026-11-01

- [Docker API1.45 schema, Moby v26.0.2](https://raw.githubusercontent.com/moby/moby/v26.0.2/docs/api/v1.45.yaml):
  exact inspect/start/stop/restart routes,204 acknowledgement,304 already-state and integer stop/restart timeout;
  responses have an open schema. Unknown properties must not break compatible selected-field decoding.
- [Docker API/version guidance](https://docs.docker.com/reference/api/engine/) and
  [official SDK guidance](https://docs.docker.com/reference/api/engine/sdk/): check daemon minimum/maximum, explicitly
  version HTTP requests; current official Go SDK is github.com/moby/moby/client. Environment API override disables
  negotiation and is deliberately excluded here.
- [Protect the daemon socket](https://docs.docker.com/engine/security/protect-access/): Unix socket is the default;
  network/TLS/SSH endpoints are separate security configurations, not automatic fallbacks.
- [Go1.27.3 net/http](https://pkg.go.dev/net/http@go1.27.3) and its pinned
  [transport source](https://raw.githubusercontent.com/golang/go/go1.27.3/src/net/http/transport.go): a reused connection
  can retry after zero bytes written even for a no-body POST. Fresh HTTP/1 connections plus disabled keep-alives,
  no idempotency headers and no outer retry avoid that branch; actual Unix HTTP fault tests must verify one POST.

## Proof, operation and remaining decisions

First freeze this decision/contract, then TDD fake Unix transport/decoder/witness boundaries and Worker+real-journal
integration. Only separately reviewed native tests may create one exact owned source-built fixture; verify image,
full ID, labels and caps before mutation/cleanup. Record the actual API1.45 daemon tuple without claiming broad parity.
No existing workload is a fixture. Safe finite codes, no raw logs/inspect/error export, and unchanged observation
packages are acceptance requirements. No public metrics/listener/audit API is added.

There is no data migration or installed rollout. Removal leaves durable uncertainty intact; never retry to compensate.
Standalone/remote authentication, local grants/allowlist provenance, EngineGeneration approval, authenticated owner
reconciliation and separate worker OS isolation remain explicit future decisions. Accepting this unused adapter cannot
approve those boundaries, expose buttons or authorize existing-container operations.
