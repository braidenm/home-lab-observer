# Docker container observations

Container collection is optional and read-only. The observer does not install Docker, start its daemon, change group
membership, or restart workloads. It can list all containers visible to the selected engine, including stopped ones,
subject to the displayed 500-record limit. Available CPU/memory readings are shown alongside honest missing-data states.

## Enable on the observed machine

Build the native binary using [the local service instructions](local-service.md). Stop an already-running observer,
then restart with the endpoint of the Docker engine you intend to observe.

Linux system engine:

```sh
./observer serve --docker-endpoint unix:///var/run/docker.sock
```

For a rootless Linux engine or Docker Desktop on macOS, pass that engine's actual local socket path. You can inspect
your selected Docker context's endpoint with `docker context inspect`; the observer deliberately does not inherit
Docker contexts or `DOCKER_HOST`. Do not paste a TCP or SSH endpoint; remote engines are not supported by this preview.

Windows Docker Desktop (Linux engine, when that local pipe is enabled):

```powershell
.\observer.exe serve --docker-endpoint npipe:////./pipe/dockerDesktopLinuxEngine
```

Some installations expose `npipe:////./pipe/docker_engine` instead. Use the pipe reported by your local Docker setup;
these names are examples, not an automatic detection promise. Windows-container resource-stat formats and Podman are
not certified in this slice. Inventory can still be listed for another engine OS, but running-container stats remain
unavailable with `ENGINE_OS_UNSUPPORTED` unless the engine reports Linux. Missing engine-OS metadata is treated the same
way. The observer never starts Docker Desktop for you.

Unlock the local dashboard, choose **Workloads**, then **Containers**. To disable collection, stop the observer and
restart without `--docker-endpoint`. There is no stored engine credential or remote configuration to remove.

## Readings and limits

- Container name/image are local-sensitive display metadata. Full IDs become stable hashed aliases.
- CPU is a percentage of the selected engine host's capacity, calculated from valid counter deltas. It differs from
  Docker's per-core percentage convention. A new/reset/unsupported counter has no reading until a valid delta exists.
- Memory is the engine's reported usage in bytes, not an inference from a missing field. Stopped containers have no
  current stats and display unavailable values; they are not described as measured zero.
- Collection shares the native scheduler but has a four-second budget and at most four concurrent stats reads.
  Inventory is bounded to 500 records and two MiB; each stats response is bounded to 256 KiB. Large/busy engines can
  show partial results. The UI/API reports limits and freshness rather than hiding omitted observations.
- Names, images and per-container metrics stay in memory. They are not retained in SQLite or uploaded anywhere.
  Environment, commands, labels, mounts, ports and log bodies are not projected or exposed.

## Security and troubleshooting

Socket/pipe permission denied: grant only the access you deliberately intend, using Docker's supported OS permissions.
Do not make the socket world-writable or enable an unauthenticated TCP daemon. A Docker socket can grant host-level
authority even though this observer sends only a fixed set of read requests. Use a separate constrained proxy in future
containerized deployment, as described in [ADR 006](adr/006-specialized-container-read-model.md).

Engine unavailable: confirm the engine is running and the endpoint belongs to this machine. Host metrics still work.
Unsupported engine version: the reader supports negotiated Engine API 1.41 through 1.45; an engine whose minimum API
exceeds that range is reported unsupported. We do not broaden versions silently.

The protected `/api/v1/containers` read model accepts only a bounded `limit` query. The older snapshot's container section
continues to report its unsupported legacy projection. Existing consumers are unchanged; the reusable dashboard opts
into the new read model through an optional data-source method.
