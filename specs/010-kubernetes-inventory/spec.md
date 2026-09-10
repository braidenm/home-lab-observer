# Spec 010: Read-only Kubernetes workload inventory

**Status:** Proposed; pending owner acceptance of bounded full-object ingress with immediate projection/discard and the
initial native-versus-in-cluster deployment profile. This document does not enable a cluster connection.

## Outcome

An owner can explicitly connect Home Lab Observer to one HTTPS Kubernetes API server and inspect Pods plus the
`apps/v1` Deployment, StatefulSet, DaemonSet and ReplicaSet controllers in at most eight configured namespaces. The
observer remains headless-first, list-only and useful without Platform Demo. A local dashboard can add an optional
Kubernetes view without changing the existing host, process or Docker contracts.

No Kubernetes write, watch, logs, events, exec, attach, port-forward, proxy, secret/config-map read, node inventory,
custom resource, Helm operation or discovery-driven request belongs to this slice. Spec 011 owns any future action.

## Requirements

- **K1 Explicit single-cluster configuration:** Kubernetes collection is disabled by default. Enabling it requires one
  absolute `https://` API-server origin, one protected bearer-token file, a system-trust decision or one bounded CA
  bundle file, and an allowlist of one to eight exact namespace names. Reject URL user information, non-HTTPS schemes,
  query, fragment, non-root path, whitespace and duplicate namespaces. Do not read kubeconfig, contexts, environment
  variables or default in-cluster paths implicitly.
- **K2 Fixed transport:** Use a dedicated HTTP client with system proxy lookup disabled, redirects refused, TLS server
  name and certificate validation enabled, bounded connect/TLS/header/body timeouts, and `Accept-Encoding: identity`.
  Send the token only as an `Authorization: Bearer` header to the configured origin. Never log or return the origin,
  token, CA path, authorization header, continuation token or raw response. Do not support insecure TLS or a local
  `kubectl proxy`.
- **K3 Protected rotating credential:** Read the token afresh for each collection attempt so an operator can rotate it.
  Bound it to 16 KiB and reject empty, multiline or surrounding-whitespace values. The native profile requires a
  private, regular, single-link token file and validated non-link ancestors. An in-cluster profile would instead need a
  separately tested, fixed projected-volume boundary because kubelet rotates projected service-account tokens. The two
  profiles are not treated as the same filesystem trust model.
- **K4 Namespace-scoped list-only RBAC:** The installation guidance grants only the Kubernetes `list` verb for core
  `pods` and apps `deployments`, `statefulsets`, `daemonsets` and `replicasets`, through a RoleBinding in each allowlisted
  namespace. It grants no `get`, `watch`, wildcard, cluster-wide binding, subresource, non-resource URL or other kind.
  The collector never lists namespaces; configuration is its only namespace source.
- **K5 Fixed API surface:** Issue only JSON `GET` collection requests to
  `/api/v1/namespaces/{namespace}/pods` and
  `/apis/apps/v1/namespaces/{namespace}/{deployments|statefulsets|daemonsets|replicasets}`. Query keys are only the
  code-owned `limit=100` and an opaque server-returned `continue` value reused for the same namespace, resource kind and
  collection attempt. No caller supplies a resource path, selector or continuation token.
- **K6 Bounded fair collection:** One non-overlapping collection has a 10-second total deadline, three-second request
  deadline, four-request concurrency, 80-request maximum, 1 MiB decoded-response maximum, 256 KiB encoded-object
  maximum, 8 MiB decoded-attempt maximum and 1,000 examined-object maximum. Return at most 500 Pods and 500 controllers.
  Fetch one page for each namespace/kind before any second page; rotate the first namespace/kind on later attempts so a
  repeatedly exhausted global budget does not permanently favor one stream. Each stream has at most two pages. These
  are proposed first-slice defaults and hard maxima, not sizing claims about Kubernetes generally.
- **K7 Pagination integrity:** A nonempty `continue` value means that stream is incomplete. Preserve the collection
  `resourceVersion` across its pages and reject a changed value. If a continuation expires with `410 Gone`, restart that
  stream once from page one only if its remaining budgets permit; never combine an inconsistent replacement token with
  earlier pages. If the stream still cannot complete, expose bounded partial status rather than a total count.
- **K8 Minimal projection:** Decode only enough of each full JSON object to project bounded records, then release the raw
  object. Pod output is limited to an installation-keyed opaque alias, namespace, bounded name, phase, deletion state,
  start time, Ready condition, bounded container-ready/known counts, summed restart count, and an optional managing
  controller kind/alias derived from a controller owner reference. Controller output is limited to kind, opaque alias,
  namespace, bounded name, deletion state, the selected desired replica value from `.spec.replicas` where that kind
  defines it, and allowlisted current/ready/available/updated status counters. DaemonSet desired count comes from its
  status. No other spec field survives projection.
  Missing fields remain null/unknown, never zero by invention.
- **K9 Prohibited data:** Do not retain, persist, upload or return raw Pod/controller specs, labels, annotations, selectors,
  container images, commands, arguments, environment variables, volume/source details, service-account names, node names,
  IPs, condition messages/reasons, managed fields, raw UIDs or raw object JSON. No collected Kubernetes inventory enters
  host metric history. Names and aliases are `LOCAL_SENSITIVE`; remote upload eligibility is false.
- **K10 Explicit ingress decision:** Kubernetes list authorization returns full object content, and status needed for this
  view is not available from `PartialObjectMetadata`. Even a minimal projection therefore receives full Pod and
  controller objects before discarding prohibited fields. Implementation is blocked until the owner accepts this
  bounded in-process ingress, or chooses a separately deployed filtering proxy. No document may describe list-only RBAC
  as field-level redaction.
- **K11 Honest quality and counts:** Overall, namespace and resource-kind status use code-owned support, collection and
  freshness states that distinguish disabled, unsupported, unavailable, permission denied, partial, failed, stale and
  current. Record `examined_count`, `returned_count`, `discarded_count`, `pages_fetched`, `complete` and `truncated` for
  each stream. A total count is nullable and may be set only after an unchanged-resourceVersion pagination sequence ends
  with an empty continuation token. Budget exhaustion, oversized objects/responses, malformed selected fields and
  per-stream authorization failures never become healthy zeroes and do not suppress successful streams.
- **K12 Cache and isolation:** Collection runs on its own proposed 30-second cadence and never from an HTTP request.
  Retain at most one bounded in-memory last-good projection per stream. A later failure can present those records only as
  stale with the latest failure code and original observation time. Kubernetes failure never changes host, process,
  Docker, logs, diagnostics or local API readiness.
- **K13 Versioned local contract and UI:** Add a closed producer schema and a separate authenticated read-only local API
  contract with a 1 MiB response cap. The reusable dashboard data source gains an optional Kubernetes-inventory method.
  Older data sources keep working and show a clear unavailable/disabled state. The UI provides independent Pods and
  Controllers views, namespace/kind filters, freshness and partial/truncation notices, accessible tables/cards and
  contained 390-pixel layouts. It exposes no action or configuration controls.
- **K14 Operability and portability:** Document enable, rotate, disable and removal without printing token contents.
  Installers do not create RBAC, copy an administrator kubeconfig or enable Kubernetes automatically. Native and
  in-cluster deployment require separate fixtures and credential-path tests; no supported-platform claim is made until
  the owner chooses the initial profile and its native smoke passes.

## Privacy-safe record semantics

Aliases are deterministic only within one observer installation and object UID. Recreated Kubernetes objects receive a
new alias even when their names match. Namespace and workload names are shown because the owner explicitly allowlists
the namespaces, but they remain local-sensitive. Controller relationships use `metadata.ownerReferences` only when one
entry has `controller=true` and its kind is in the fixed apps-controller set; labels are never used for correlation.

Pod readiness comes only from the selected Ready condition and container status counters. Controller availability uses
the fixed status counters defined for that kind; arbitrary condition text is not normalized. Kubernetes status is an
observation that may lag actual cluster state, so the UI labels collection time and never presents it as a health probe.

## Proposed configuration boundary

Names are illustrative until the deployment profile is accepted:

```text
observer serve \
  --kubernetes-server https://cluster.example.invalid:6443 \
  --kubernetes-token-file /dedicated/observer/kubernetes.token \
  --kubernetes-ca-file /dedicated/observer/cluster-ca.pem \
  --kubernetes-namespace media \
  --kubernetes-namespace monitoring
```

Token contents never appear in arguments. System roots may be used instead of `--kubernetes-ca-file`, but insecure TLS
is never accepted. A background profile persists only validated endpoint and file paths plus the namespace allowlist in
owner-only settings; it never copies the token into settings.

## Owner decisions still required

1. Accept bounded full-object ingress and immediate discard, or require a separate field-filtering proxy.
2. Choose the first supported deployment: native observer with an operator-rotated protected token file, or an
   in-cluster observer with an explicitly projected short-lived service-account token and read-only CA volume.
3. Approve the proposed cadence, pagination, request/object/byte/time and returned-record limits. These are reasonable
   home-lab defaults but not inferred commitments.

## Acceptance evidence

- Contract fixtures cover valid, disabled, partial, permission-denied, stale, truncated and empty states plus secret
  canaries and every numeric/string/count cross-field bound.
- HTTP adapter tests prove exact method/path/query/header sets, no redirects/proxy, certificate and hostname validation,
  token reload, request/response limits, continuation scoping, resourceVersion consistency and `410` restart behavior.
- RBAC fixtures contain exactly `list` on the five namespaced resources and RoleBindings only for configured namespaces.
- Collector tests prove first-page fairness, rotating start order, no overlap, cancellation, independent failures,
  last-good staleness and global/per-stream budgets. Raw specs and every prohibited field use canary values and are absent
  from normalized values, cache, JSON, logs and metrics.
- UI tests cover optional-source compatibility, disabled/loading/empty/error/stale/partial/truncated states, null versus
  zero, keyboard behavior and 390/768/1440-pixel layouts. Native tests use synthetic owned cluster fixtures only.
- Required pull-request checks remain parallel and below 15 minutes. No test contacts a developer or production cluster.

## Primary-source basis

Kubernetes maps HTTP GET on a collection to the RBAC `list` verb and notes that list includes full object content:
[authorization](https://kubernetes.io/docs/reference/access-authn-authz/authorization/). Namespaced Roles and
RoleBindings can constrain the five list permissions without cluster-wide access:
[RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/). Paginated lists use `limit` and `continue`, keep a
consistent resource version within a continuation sequence, and can return `410 Gone` after token expiry:
[API concepts](https://kubernetes.io/docs/reference/using-api/api-concepts/#retrieving-large-results-sets-in-chunks).
Pod responses contain both desired spec and observed status, while owner references identify a managing controller:
[Pod API](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/),
[OwnerReference](https://kubernetes.io/docs/reference/kubernetes-api/definitions/owner-reference-v1-meta/).
Projected service-account tokens are short-lived and kubelet-rotated, whereas long-lived token Secrets are discouraged:
[service accounts](https://kubernetes.io/docs/concepts/security/service-accounts/),
[token projection](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#serviceaccount-token-volume-projection).
In-cluster clients use a bearer token and CA certificate to verify the HTTPS API server:
[access from a Pod](https://kubernetes.io/docs/tasks/run-application/access-api-from-pod/).
