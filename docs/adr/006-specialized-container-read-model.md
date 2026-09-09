# ADR 006: Specialized read-only container inventory

**Status:** Accepted

The current v1 snapshot requires numeric container CPU/memory fields. A running container can deny stats while inventory
remains readable, and stopped containers have no current sampling interval. Changing those fields to nullable would
break the published contract. Omitting such containers or inventing zeros would hide information.

Add a separately versioned container inventory endpoint and optional reusable UI data-source method. The legacy snapshot
is unchanged and remains explicit about unsupported container projection. Each read model expresses its own quality.
This follows the existing split between current host reads and historical metric series.

Docker access is opt-in over local OS transports only. The observer uses a fixed GET-only request set; socket possession
still carries daemon authority, so a compromised observer process remains inside that trust boundary. The application
never exposes raw socket access. Future containerized packaging requires a constrained proxy. No daemon is enabled,
reconfigured, installed or restarted by this feature.

Use a four-second collection budget and bounded fan-out. Inventory and live metrics stay in memory, are local-sensitive,
and never enter the aggregate metric database. Separate service/log and remote-control specifications must revisit
permissions and threat models rather than extending this adapter into arbitrary execution.

References: [Docker Engine API](https://docs.docker.com/reference/api/engine/),
[Docker daemon security](https://docs.docker.com/engine/security/),
[Podman service boundary](https://docs.podman.io/en/latest/markdown/podman-system-service.1.html).
