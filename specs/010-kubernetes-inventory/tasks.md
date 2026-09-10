# Tasks

**Status:** Proposed; unchecked work is not an implemented or supported capability.

- [x] Draft the bounded single-cluster, namespaced, list-only scope from official Kubernetes API and RBAC behavior.
- [x] Record that list authorization permits full Pod/controller object ingress and does not provide field-level redaction.
- [ ] Obtain owner acceptance of bounded full-object ingress or choose a separate filtering proxy.
- [ ] Choose and accept the first deployment profile: protected native token file or in-cluster projected token/CA.
- [ ] Review and tune the proposed engineering limits during contract freeze without adding an owner consent gate unless
      a change materially expands the accepted risk or scope.
- [ ] Record the accepted credential/TLS/deployment boundary and residual risks in a new ADR.
- [ ] Freeze normalized domain types, capability/current JSON schemas, OpenAPI endpoint and privacy/count invariants.
- [ ] Implement/test configuration, protected credential reload and fixed TLS transport without kubeconfig/proxy/redirect.
- [ ] Implement/test consistent pagination, `410` restart, first-page fairness, rotating start order and global budgets.
- [ ] Implement/test Pod, bounded per-container-status and apps-controller projection with raw-spec/prohibited-field
      canaries, installation-local aliases, deterministic child fairness and honest omitted/discarded counts.
- [ ] Add the optional local API/UI data-source method and responsive Pods/Containers/Controllers views without changing
      legacy APIs or describing bounded results as complete inventory.
- [ ] Add exact namespace Role/RoleBinding examples and operator enable/rotate/disable/remove guidance.
- [ ] Run synthetic chosen-profile smoke, contract/privacy/security suites and independent architecture/API/UX reviews.
- [ ] Complete traceability and release evidence without contacting or mutating a developer/production cluster.

Kubernetes writes and actions remain prohibited. Spec 011 is Docker-only; any future Kubernetes action requires its own
specification and separately reviewed credential rather than inheriting this observation credential.
