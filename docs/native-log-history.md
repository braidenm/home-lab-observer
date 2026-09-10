# Native log metadata and history

**Release status:** these options are available in published
[`v0.1.0-preview.3`](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-preview.3). Preview 2 and earlier do
not understand the native-log settings. Preview 3 remains unsigned on Windows and not notarized on macOS; verify the
download, checksums and provenance as described in the [install guide](install.md).

## What it observes

Native logs are optional and disabled by default. The observer captures bounded event timestamps, severity and
validated event codes, not message bodies. Recent events live in a 200-record session cache; compact source/severity
history shares the existing seven-day, 250-MiB database budget. Restarting clears recent events but preserves committed
history. The observer's own rotating diagnostics are a different feature. Collection runs as the current user, stays
headless when the dashboard is closed, and does not upload native log metadata.

| Platform | Native sources | Limitations |
| --- | --- | --- |
| Windows | System, Application | Only events accessible to the current account; Security is not collected |
| Linux | Local system journal | Requires the matching bundled helper and compatible host libsystemd; access permissions still apply |
| macOS | None in this specification | Explicit unsupported state; ordinary host observations remain available |

## Explicit foreground opt-in

After installing Preview 3, run from its verified archive directory (or use the installed `bin/observer` launcher):

Linux:

```sh
./observer serve --log-source system
```

Windows PowerShell:

```powershell
.\observer.exe serve --log-source system --log-source application
```

Omit a source to leave it disabled. Stop another observer using the same port or data directory first. No dashboard
button enables collection, no environment variable silently opts in, and no GitHub token is required.

For managed background operation, pass the same repeated `--log-source` options to `background enable`, alongside
the installation root described in [background operation](background-operation.md). This creates only the documented
per-user systemd service, LaunchAgent or signed-in-user Windows task; it is not a machine-wide service and cannot see
events unavailable to that user. To change an existing source selection, explicitly disable and re-enable the owned
registration; upgrades do not broaden it. Disable background operation with Preview 3 before rolling back to a version
that does not understand log-source settings, then re-enable without those options. Stop/restart/disable do not accept
source-selection options.

## Read the dashboard honestly

The Logs view separates recent session events from persisted summaries. Choose one hour, six hours, one day or seven
days. Counts represent captured metadata, not a complete inventory of machine events. A known healthy zero differs
from an unknown count. Gaps, missed polls, permissions, backlog and unsupported sources are shown explicitly; current
collection failures do not erase earlier history.

On Windows, clearing a log and recreating an event with identical selected identity fields can evade reset detection.
This is operational history, not a forensic or tamper-evident audit. [ADR 011](adr/011-windows-log-continuation-proof.md)
records that deliberate limitation.

If Linux reports a missing or mismatched helper, reinstall the complete verified Preview 3 package from the same release.
Do not copy a helper from another version or weaken directory permissions. A missing runtime library or denied
source does not authorize elevation or an automatic package installation; host/process observations remain independent.

## Local API and privacy

Authenticated `GET /api/v1/logs/summary?range=1h` (also `6h`, `24h`, `7d`) returns fixed-grid summaries without native
reads in the HTTP request. Recent events use the existing current-snapshot interface. Log bodies remain omitted,
including when explicitly requested. Preview 3 has no remote uploader, and native log metadata is not eligible for
remote upload by this contract.

See [Spec 009](../specs/009-native-log-metadata/spec.md) and its
[acceptance ledger](../specs/009-native-log-metadata/traceability.md) for implementation and release gates.
