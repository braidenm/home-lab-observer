# Home Lab Observer

Home Lab Observer is a cross-platform, headless-first observability service for a single machine. It collects host, process, service, container, and opt-in log signals; exposes them through a versioned local API; can show them in an optional local dashboard; and can securely upload normalized snapshots to a separate management application such as Platform Demo.

The project is intentionally public and self-contained. It does not contain, build from, or grant access to the private home-lab infrastructure repository.

## Project status

The foundation, local API contract, native snapshot preview, and reusable dashboard are complete. Work is now tracked
in [Spec 005](specs/005-local-data-plane/spec.md): a secure loopback service, bounded history, real trends, and an
embedded local dashboard. The project remains pre-release and does not yet publish an installable agent.

## Product shape

- One native binary for Windows, macOS, and Linux, with no runtime language installation.
- Headless collection by default; an optional responsive dashboard is served on loopback only.
- Host health, CPU, memory, disks, network, sensors when available, processes, services, Docker workloads, events, and bounded trends.
- Logs from explicit sources such as journald, Windows Event Log, macOS unified logging, and Docker, with conservative privacy defaults.
- Versioned JSON and OpenMetrics-compatible endpoints for local and remote clients.
- Optional signed Linux container for container-oriented deployments; native installs remain the authoritative source for full host metrics on Windows and macOS.
- Read-only in the initial release. Permissioned operational actions are a separately specified capability.

## Architecture at a glance

```text
OS and workload adapters
  -> normalized observations
  -> bounded local history
  -> versioned local API + optional embedded dashboard
  -> optional authenticated snapshot uploader
  -> Platform Demo or another compatible client
```

Platform Demo is a separate consumer of the observer contract. It does not iframe the local dashboard or require this repository to know about Platform Demo's UI.

## Native snapshot preview

With Go 1.27 installed, `go run ./cmd/observer --help` prints the command help and
`go run ./cmd/observer collect-once` writes one `observer-current-snapshot/v1` JSON document to standard output.
Help exits 0, invalid usage exits 2, and an `OK` or `PARTIAL` snapshot exits 0. If all implemented visible sections
fail, the command still emits a schema-valid `FAILED` snapshot for diagnostics and exits 1. An encoding failure also
exits 1 but cannot guarantee a complete JSON document.

## Documentation

- [Constitution](.specify/memory/constitution.md)
- [Repository standards](docs/coding-standards.md)
- [Architecture research](docs/architecture-research/001-cross-platform-observer.md)
- [Dashboard and signal research](docs/architecture-research/002-meaningful-machine-observability.md)
- [Architecture decisions](docs/adr/README.md)
- [Specifications](specs/README.md)
- [Security model](SECURITY.md)
- [Threat model](docs/security/threat-model.md)
- [Data policy](docs/privacy/data-policy.md)
- [Reusable observer dashboard](web/observer-ui/README.md)

## Supported delivery targets

| Target | Native binary | Background service | Local UI | Docker observations |
| --- | --- | --- | --- | --- |
| Linux amd64/arm64 | Planned | systemd | Planned | Planned |
| macOS Intel/Apple silicon | Planned | launchd | Planned | Docker Desktop, when available |
| Windows amd64/arm64 | Planned | Windows Service | Planned | Docker Desktop, when available |
| Linux container amd64/arm64 | Planned | Container restart policy | Planned | Planned through a constrained proxy |

## License

[MIT](LICENSE)
