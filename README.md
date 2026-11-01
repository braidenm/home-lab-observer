# Home Lab Observer

Home Lab Observer is a cross-platform, headless-first observability service for a single machine. Its rich local preview collects host/process signals, optional Docker readings and Windows/Linux log metadata, keeps bounded local history, and serves an authenticated local dashboard. Separate Ubuntu connected packages support numeric host readings or explicitly approved process/Docker inventory for an owner-authorized hosted dashboard such as Platform Demo; each requires its own site rollout.

The project is intentionally public and self-contained. It does not contain, build from, or grant access to the private home-lab infrastructure repository.

## Project status

The preview includes the [Spec 005](specs/005-local-data-plane/spec.md) loopback service, bounded history,
real trends and embedded local dashboard; [Spec 006](specs/006-container-observations/spec.md) opt-in container
inventory and resource readings; and [Spec 009](specs/009-native-log-metadata/spec.md) opt-in Windows/Linux native log
metadata and bounded severity history. See [build and run instructions](docs/local-service.md),
[connect a local Docker engine](docs/container-observations.md), and [native log guidance](docs/native-log-history.md).
The project remains pre-release. [Native delivery instructions](docs/install.md) describe the verified preview archives
and per-user helpers; [GitHub Releases](https://github.com/braidenm/home-lab-observer/releases) lists published versions.
Windows/macOS previews are explicitly not publisher-signed/notarized.
[Preview 3](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-preview.3) adds opt-in native Windows/Linux
log metadata, bounded local severity summaries and a digest-bound Linux journal helper. Background operation remains
per-user and headless; it does not upload observations.

[Connected Linux beta 0.1.0-canary.1](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-canary.1)
is published for Ubuntu 24.04/systemd 255/amd64, with immutable assets, provenance/SBOM verification and
[exact-artifact bootstrap and activation VM evidence](docs/acceptance/connected-linux/0.1.0-canary.1.json).
It is an unsigned, explicit-start beta, not GA or an unattended service. Application catalog rollout and real
owner install/view/share acceptance remain separate work; publication does not prove either is enabled on a given site.

[Connected process/Docker beta 0.1.0-canary.3](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-canary.3)
is also published for that exact Ubuntu target. Independent builders produced identical public files, and both
installation/refusal cases passed first boot and reboot; the [canonical acceptance record](docs/acceptance/connected-linux-docker/0.1.0-canary.3.json)
binds those results to the released bytes. This is not an automatic numeric upgrade: hosted enrollment/admission,
explicit consent, owner-host installation and richer sharing acceptance are still separate rollout gates.

## Try the local dashboard without development tools

1. [Choose the verified local Preview 3](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-preview.3)
   for your Windows, Mac or Linux machine. No GitHub account/token, Docker, Go or Node is needed.
2. Follow the [short OS-specific install guide](docs/install.md) to verify the archive and open its launch helper.
3. Open `http://127.0.0.1:9847`, then unlock it with the local token file shown in the console.

This local preview stays on your machine. Opening the dashboard is optional; it collects while running headless too.
Docker readings require an explicit local socket/pipe option. Installation does not start a service or upload anything.

For a hosted dashboard on a supported Ubuntu server, follow the [connected Linux guide](docs/connected-linux.md):
create an enrollment in **My servers**, run the verified public command, paste the separate one-use token at its
hidden prompt, explicitly start, then check fresh readings. Downloads need no GitHub account/PAT or Docker.
When the site's sharing capability is enabled, the owner can grant another existing user read-only access by user ID.
The legacy Platform Demo Compose connector is a separate path; the native beta does not replace or upgrade it.

## Target product shape

- One native download for Windows, macOS, and Linux, with no runtime language installation. Linux native log
  observations use a digest-bound bundled helper so the main observer retains its portability when logs are disabled.
- Headless collection by default; an optional responsive dashboard is served on loopback only.
- Host health, CPU, memory, disks, network, sensors when available, processes, services, Docker workloads, events, and bounded trends.
- Opt-in metadata from the local system journal on Linux and the System/Application event logs on Windows, with no
  message bodies, remote upload or elevated installation. macOS native logs and Docker log bodies are not supported.
- Versioned JSON and OpenMetrics-compatible endpoints for local and remote clients.
- Optional signed Linux container for container-oriented deployments; native installs remain the authoritative source for full host metrics on Windows and macOS.
- Read-only in the initial release. Permissioned operational actions are a separately specified capability.

## Architecture at a glance

```text
Native host/process adapters -> current reads + bounded numeric host history --+
Optional local Docker adapter -> current in-memory container inventory --------+-> authenticated local API + embedded UI
Optional Windows/Linux log adapters -> safe current metadata + bounded rollups-+

Connected Ubuntu beta: numeric-host collector -> private handoff -> scoped outbound uploader -> authorized hosted views
Opt-in Ubuntu process/Docker package: isolated Docker reader -> policy-bound collector -> durable uploader -> gated hosted views
```

Platform Demo is a separate consumer of the observer contract. It does not iframe the local dashboard or require this repository to know about Platform Demo's UI.

### Will this support restarting containers or services later?

Yes, the platform-adapter and owner-bound capability boundaries are intended to support that extension. The current
observer cannot accept remote restart requests. A future management worker will need separate, explicitly approved
permissions for named resources and fixed actions, plus authorization and an audit record for each request. Reading
telemetry must not automatically grant control. Docker, operating-system services and Kubernetes workloads require
their own adapters and permission models; none will be exposed through a generic shell-command endpoint.
See the [threat model](docs/security/threat-model.md) for the separation between observation and future control.

An [unused typed coordinator](specs/011-container-actions/worker-core.md) now tests operation permissions,
deadlines and intent-before-dispatch ordering with synthetic adapters. The separately merged
[unused durable journal](specs/011-container-actions/action-journal.md) adds persistence, but there is no installed
transport/store composition, listener or action role. This is not a management release and grants no additional
access to an installed agent.

The [fixed Linux Engine adapter contract](specs/011-container-actions/engine-adapter.md) is accepted for unused Linux
implementation only after the separately reviewed dormant journal: exact typed operations and disposable proof,
not an installed broker,
permission grant, action release or approval to change an existing container. Current monitoring remains read-only.

The [delivery roadmap](docs/delivery-plan.md) prioritizes completing real hosted connection/sharing acceptance, then separately permissioned Docker
start, stop and restart. Kubernetes is deferred. Our [open-source comparison](docs/architecture-research/007-open-source-alignment.md)
explains the established patterns informing installation, connectivity, monitoring and management boundaries.
Those management capabilities are not available in either package. Monitoring access will not implicitly
grant management access.

[Connected observation](specs/012-connected-observation/spec.md) uses a separate numeric-host egress projection,
not rich local snapshots. The published Linux beta composes isolated enrollment, collection and upload workers;
the [operator guide](docs/connected-linux.md) covers its narrow host profile and manual lifecycle.
Local process, container and log details do not become upload-eligible through enrollment.

The [rich connected inventory slice](specs/013-connected-inventory/spec.md) expands hosted process/container and
system views through explicit local opt-in and a separately reviewed installed profile. Its accepted package reuses the local collectors,
does not silently widen the numeric beta, and keeps Docker actions separately permissioned under Spec 011.
Its isolated Docker reader retains a minimal filesystem and private networking; the
[systemd 255 isolation investigation](docs/architecture-research/2026-10-30-reader-private-network-sys.md)
records the startup correction, while the [packaged proof](specs/013-connected-inventory/docker-vm-proof.md)
and versioned acceptance record describe the executed release gate. Docker socket access is root-equivalent authority;
GET-only application code is not daemon-enforced least privilege. No raw logs or container controls are included.
The pure [combined-envelope contract](specs/013-connected-inventory/delivery-envelope.md) binds host metrics and
workloads to one upload cycle; the separately scoped package does not by itself enable a site's receiver admission.
The [process-only package proof](specs/013-connected-inventory/installable-process-proof.md) connects that contract to
separate collector/uploader identities, policy-bound durable retries and an explicit installer. It is a disposable-VM
acceptance slice, not a published upgrade or permission to collect processes on an existing numeric installation.

### Native log metadata and history

[Preview 3](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-preview.3) implements
[Spec 009](specs/009-native-log-metadata/spec.md): opt-in Windows/Linux event metadata and meaningful severity trends
without reading message bodies. The
[Linux helper decision](docs/adr/010-optional-linux-journal-helper.md) keeps native-library failures separate from
host/Docker monitoring. Charts distinguish captured counts, proven empty periods and missing coverage; Linux results
describe only logs accessible to the running account. macOS native logs, raw bodies and remote log upload remain later
work. Local-preview installation neither grants elevated log permissions nor enables log collection automatically.
The [operator guide](docs/native-log-history.md) explains source selection, history, rollback and
the [accepted Windows reset-detection limitation](docs/adr/011-windows-log-continuation-proof.md).

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
- [Optional background operation, token location and troubleshooting](docs/background-operation.md)
- [Enable and understand Docker observations](docs/container-observations.md)
- [Download, verify, install and manage native previews](docs/install.md)
- [Connect, check and share the Ubuntu monitoring beta](docs/connected-linux.md)
- [How native releases are built and verified](docs/releasing.md)

## Rich local preview delivery targets

| Target | Native binary | Per-user background mode | Local UI | Docker observations | Native log metadata |
| --- | --- | --- | --- | --- | --- |
| Linux amd64/arm64 | Downloadable preview | systemd user service | Embedded preview | Opt-in local Unix socket | Opt-in local system journal; bundled helper and compatible libsystemd required |
| macOS Intel/Apple silicon | Downloadable preview | User LaunchAgent | Embedded preview | Opt-in local Unix socket; Desktop live validation pending | Unsupported in Preview 3 |
| Windows amd64/arm64 | Downloadable preview | Signed-in user task; machine-wide service later | Embedded preview | Opt-in local named pipe; Desktop live validation pending | Opt-in System and Application event metadata |
| Linux container amd64/arm64 | Planned | Container restart policy | Planned | Planned through a constrained proxy | Not supported |

The separate connected packages support only Ubuntu 24.04/systemd 255/amd64. The numeric beta remains independent;
the process/Docker package needs its own hosted rollout and consent. Connected Windows/macOS, remote logs,
unattended lifecycle and management controls are not provided by those local-preview downloads.

## License

[MIT](LICENSE)
