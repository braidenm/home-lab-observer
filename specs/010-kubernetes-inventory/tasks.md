# Tasks

**Status:** Proposed; unchecked work is not an implemented or supported capability.

- [x] Draft the bounded single-cluster, namespaced, list-only scope from official Kubernetes API and RBAC behavior.
- [x] Record that list authorization permits full Pod/controller object ingress and does not provide field-level redaction.
- [ ] Obtain owner acceptance of bounded full-object ingress or choose a separate filtering proxy.
- [ ] Choose and accept the first deployment profile: protected native token file or in-cluster projected token/CA.
- [ ] Approve the proposed cadence, concurrency, pagination, object, byte, request, returned-record and timeout limits.
- [ ] Record the accepted credential/TLS/deployment boundary and residual risks in a new ADR.
- [ ] Freeze normalized domain types, capability/current JSON schemas, OpenAPI endpoint and privacy/count invariants.
- [ ] Implement/test configuration, protected credential reload and fixed TLS transport without kubeconfig/proxy/redirect.
- [ ] Implement/test consistent pagination, `410` restart, first-page fairness, rotating start order and global budgets.
- [ ] Implement/test Pod/apps-controller projection with raw-spec/prohibited-field canaries and installation-local aliases.
- [ ] Add the optional local API/UI data-source method and responsive Pods/Controllers views without changing legacy APIs.
- [ ] Add exact namespace Role/RoleBinding examples and operator enable/rotate/disable/remove guidance.
- [ ] Run synthetic chosen-profile smoke, contract/privacy/security suites and independent architecture/API/UX reviews.
- [ ] Complete traceability and release evidence without contacting or mutating a developer/production cluster.

Kubernetes writes and actions remain prohibited. Spec 011 is separate and cannot inherit this observation credential.
