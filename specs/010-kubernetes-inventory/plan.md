# Proposed implementation plan

**Status:** Proposed; documentation only. No cluster connection is enabled by this plan.

## Decision boundary

Keep Kubernetes collection separate from `internal/containerobs`, host snapshots, processes, native logs and history.
The collector produces a new normalized inventory contract; storage, local API and reusable UI depend only on that
contract. The implementation starts only after the two owner decisions in the spec are recorded in an ADR.

The first adapter has one fixed HTTPS origin and five namespaced list streams per configured namespace. It does not use
client-go configuration loaders, kubeconfig, discovery, watch or generic REST paths. Small interfaces isolate credential
loading, TLS transport, page decoding, collection scheduling and UI projection so security tests can inject each failure.

## Delivery order

1. Resolve full-object ingress versus filtering proxy and native versus in-cluster deployment. Record the credential
   filesystem trust model, CA model and accepted residual exposure in a new ADR.
2. Freeze a versioned normalized domain model plus closed JSON Schema/OpenAPI contracts for capabilities and current
   inventory. Include nullable totals, per-stream quality and explicit local-sensitive/no-upload policy.
3. Implement configuration validation and a transport-neutral pager with synthetic HTTP/TLS fixtures. Prove exact fixed
   paths, list-only query grammar, token reload, no proxy/redirect, resourceVersion/continue handling and byte limits.
4. Implement the fair bounded collector and safe projection. Decode one bounded object at a time, discard the raw value,
   derive installation-local UID aliases and retain only the selected fields. Add canaries for every prohibited field.
5. Add capability/current local API projection and optional reusable dashboard data source. Preserve older data-source
   behavior and keep Kubernetes loading independent of host, Docker and Logs views.
6. Add only the chosen deployment profile, least-privilege RBAC examples, protected credential rotation and disable/remove
   instructions. Run its synthetic native or in-cluster smoke without contacting an existing cluster.
7. Complete independent architecture, security, privacy, API and responsive UX review before any release claim.

## Collection schedule and fairness

Create a stream for each `(namespace, kind)` pair. At each cadence, rotate the starting stream, issue one page from every
stream before scheduling a continuation, and cap concurrency at four. The collection owns one global deadline and byte,
object and request counters; each response and object also has an independent cap. A stream whose first page cannot be
attempted is `NOT_RUN/BUDGET_EXHAUSTED`, not empty. A partially read stream preserves bounded projected records and marks
`complete=false,truncated=true` unless pagination consistency was lost, in which case its inconsistent records are
discarded and its failure is explicit.

No HTTP request starts collection. The cache stores normalized records only. Collection cancellation joins all requests
before replacing the cache or shutting down. Last-good data and latest attempt status are separate so failure does not
erase useful records or disguise their age.

## RBAC shape

Provide an operator-reviewed ClusterRole containing only these rules, then bind it with a RoleBinding in each explicitly
configured namespace (an equivalent namespace-local Role is acceptable):

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["list"]
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets", "replicasets"]
    verbs: ["list"]
```

Do not create a ClusterRoleBinding. No wildcard, Secret, ConfigMap, Namespace, Node, Event, status subresource, log,
exec, attach, port-forward, proxy, impersonation, TokenRequest or SubjectAccessReview permission is required at runtime.
RBAC installation is explicit operator work; the observer installer does not apply manifests.

## Verification

- Domain/contract: exact fields and enums, aliases, null semantics, per-stream count invariants, response cap, unknown
  additive client fields and malformed producer output.
- Configuration/credentials: disabled default, one origin, namespace grammar/deduplication, token/CA file ownership and
  links, rotation, secret canaries and the selected deployment profile's distinct mount behavior.
- HTTP/pagination: synthetic TLS server, hostname/CA failures, redirect/proxy refusal, fixed requests, 401/403/404/410,
  malformed/oversized pages and objects, continue/resourceVersion mismatch and one bounded restart.
- Scheduling: non-overlap, four-request concurrency, breadth-first first pages, rotating start, timeout/cancel join,
  independent stream status, global budgets and last-good freshness.
- Projection/privacy: full-object fixtures containing tokens, environment, labels, annotations, images, volumes, node/IP
  data and condition text; none survives normalization, caching, API encoding, logs or metric labels.
- UI: legacy source without the optional method, independently aborted/raced loads, namespace/kind filters, honest
  partial/empty states, keyboard access and responsive contained data at 390, 768 and 1440 CSS pixels.

## Rollout and rollback

Ship contracts and disabled code first. Validate against a synthetic disposable cluster with the exact RBAC before an
operator writes any real configuration. Enabling requires an explicit cluster profile and namespace allowlist. Rollback
disables the Kubernetes source and removes only observer-owned configuration; it does not delete the ServiceAccount,
RoleBindings or cluster workloads automatically. The operator revokes cluster credentials separately and can continue
using every existing observer feature.

## Deferred

Cluster enrollment, multiple clusters, cluster-wide namespace discovery, Nodes, Services, Jobs/CronJobs, CRDs, metrics,
events, logs, watches, history/upload, alerts, actions, Helm and Kubernetes dashboard embedding are separate work.

