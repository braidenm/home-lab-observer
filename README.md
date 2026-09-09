# Home Lab Observer

Home Lab Observer is a cross-platform, headless-first observability service for a single machine. The current source preview collects host and process signals, optionally observes local Docker containers, keeps bounded numeric host history, and serves an authenticated local dashboard. Service observations, opt-in logs, downloadable installers, and secure upload to a management application such as Platform Demo are planned extensions.

The project is intentionally public and self-contained. It does not contain, build from, or grant access to the private home-lab infrastructure repository.

## Project status

The source preview includes the [Spec 005](specs/005-local-data-plane/spec.md) loopback service, bounded history,
real trends, and embedded local dashboard, plus [Spec 006](specs/006-container-observations/spec.md)
opt-in container inventory and resource readings. See [build and run instructions](docs/local-service.md)
and [connect a local Docker engine](docs/container-observations.md).
The project remains pre-release. [Native delivery instructions](docs/install.md) describe the verified preview archives
and per-user helpers; [GitHub Releases](https://github.com/braidenm/home-lab-observer/releases) lists published versions.
Windows/macOS previews are explicitly not publisher-signed/notarized. Background services and remote sync remain separate milestones.

## Target product shape

- One native binary for Windows, macOS, and Linux, with no runtime language installation.
- Headless collection by default; an optional responsive dashboard is served on loopback only.
- Host health, CPU, memory, disks, network, sensors when available, processes, services, Docker workloads, events, and bounded trends.
- Logs from explicit sources such as journald, Windows Event Log, macOS unified logging, and Docker, with conservative privacy defaults.
- Versioned JSON and OpenMetrics-compatible endpoints for local and remote clients.
- Optional signed Linux container for container-oriented deployments; native installs remain the authoritative source for full host metrics on Windows and macOS.
- Read-only in the initial release. Permissioned operational actions are a separately specified capability.

## Architecture at a glance

```text
Native host/process adapters -> current reads + bounded numeric host history --+
Optional local Docker adapter -> current in-memory container inventory --------+-> authenticated local API + embedded UI

Future: an explicitly enrolled outbound uploader -> Platform Demo or another compatible client
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
- [Run and manage the local preview](docs/local-service.md)
- [Enable and understand Docker observations](docs/container-observations.md)
- [Download, verify, install and manage native previews](docs/install.md)
- [How native releases are built and verified](docs/releasing.md)

## Supported delivery targets

| Target | Native binary | Background service | Local UI | Docker observations |
| --- | --- | --- | --- | --- |
| Linux amd64/arm64 | Source preview | Planned: systemd | Source preview | Opt-in local Unix socket |
| macOS Intel/Apple silicon | Source preview | Planned: launchd | Source preview | Opt-in local Unix socket; Desktop live validation pending |
| Windows amd64/arm64 | Source preview | Planned: Windows Service | Source preview | Opt-in local named pipe; Desktop live validation pending |
| Linux container amd64/arm64 | Planned | Container restart policy | Planned | Planned through a constrained proxy |

## License

[MIT](LICENSE)
