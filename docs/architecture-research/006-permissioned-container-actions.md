# Permissioned container actions: bounded proposal

Research for proposed [Spec 011](../../specs/011-container-actions/spec.md). No actions are implemented or enabled.

## API behavior and product policy

Docker exposes start, stop and restart as separate Engine operations. Start operates on existing stopped containers;
this proposal excludes checkpoint restore, attachment and interactive input. This is not container creation.
[Docker start](https://docs.docker.com/reference/cli/docker/container/start/),
[Engine API](https://docs.docker.com/reference/api/engine/).

Stop uses the container's configured stop signal and can force termination after its grace period. Docker accepts an
infinite timeout, which this proposal deliberately rejects. Our proposed 30-second default, 1..120-second range and
confirmation are product choices, not universal daemon defaults or a promise of graceful application shutdown.
[Docker stop](https://docs.docker.com/reference/cli/docker/container/stop/).

Restart is a stop/start operation with configurable grace. Our running-only precheck does not create an atomic daemon
precondition: concurrent external state changes can occur. Give restart its own grant and explain that it can start the
target as part of the cycle. Do not expose caller-selected signals or convert a denied restart into stop plus start.
[Docker restart](https://docs.docker.com/reference/cli/docker/container/restart/).

## Authority boundary

Docker's default authorization is all-or-nothing for daemon callers. Moving a socket into a dedicated broker protects
other application roles, but does not make a compromised broker least-privileged inside Docker. Authorization plugins
can apply finer HTTP policy; they require explicit daemon configuration, and streaming/upgraded protocols have special
limitations. A strict broker must reject those protocols rather than rely on generic request forwarding. An optional
plugin/proxy is a separate installation decision, not a silently installed security guarantee.
[Docker authorization](https://docs.docker.com/engine/extend/plugins_authorization/).

The proposed authorization tuple includes engine connection generation and full container ID. Names and display aliases
are presentation, not stable authorization. An owner replacing a deployment/engine must grant again. Request deduplication
and a durable dispatch record reduce duplicate effects but cannot make an external daemon operation transactional with
our ledger. A timeout is ambiguous; observing a running container afterward does not prove a particular restart happened.
These are design conclusions, not an Engine exactly-once guarantee.

## Why Kubernetes remains separate

Upstream kubectl's workload restart implementation modifies a Pod-template annotation; it is not a direct Docker
container restart. A future Deployment-only action needs resource UID/resourceVersion checks and separate rollout state.
[Upstream restart implementation](https://raw.githubusercontent.com/kubernetes/kubectl/master/pkg/polymorphichelpers/objectrestarter.go).

Namespaced `get`/`patch` rights on specifically named Deployments can constrain resources, but RBAC does not restrict
the patch to one annotation. Fixed broker operations and, where required, admission enforcement must address that wider
credential authority. Direct pod deletion/eviction is a distinct disruption operation, not a fallback for failed rollout.
[Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/),
[Eviction API](https://kubernetes.io/docs/concepts/scheduling-eviction/api-eviction/).

## Before implementation

Review broker isolation and accepted residual daemon authority; choose protected standalone-owner authentication and
remote revocation freshness; approve storage durability, proposed bounds and explicit ambiguous-outcome recovery.
Freeze exact Engine API negotiation/response mappings with executable fixtures. No new library, credential, container,
daemon setting or private endpoint is introduced by this research.
