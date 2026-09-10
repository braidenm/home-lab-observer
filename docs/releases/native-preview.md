# Proposed preview 3: optional native log metadata and history

**Draft for review, not a publication announcement.** Preview 3 remains pending final native acceptance (including
Windows), review and owner-controlled publication. Remove this draft status only when those gates are satisfied.
The owner must record a successful independent Native reproducibility check for the exact authorized commit before
dispatch; publication reuses that evidence instead of duplicating sixteen compilation operations. The publication
workflow still verifies all six native target artifacts, checks vulnerabilities and attests final bytes before release.

## Planned additions

Opt-in Linux system-journal and Windows System/Application metadata, a recent-session event list, and persisted
one-hour/six-hour/day/week summaries. Collection keeps bounded timestamps, severity and validated event codes, not
message bodies. Counts and coverage distinguish known zero from missing observations, permissions, backlog and gaps.
The 200-record recent list clears on restart; compact committed history survives graceful or forced restart and shares
the existing seven-day, 250-MiB database budget. This is local operational history, not a complete forensic audit.

- Linux archives include a version-matched, digest-bound `observer-journal-helper`. Host libsystemd and a compatible
  dynamic runtime are required only for native logs; they are not bundled or automatically installed. Missing runtime
  support reports unavailable without preventing ordinary host observations or the local API from starting.
- Windows uses the same executable for its isolated helper. Only current-account-accessible System/Application events
  are in scope; Security is excluded. Clearing a log and recreating an event with identical selected identity fields
  can evade reset detection. Do not treat the history as tamper-evident.
- macOS native logs remain unsupported. Existing host observations and the dashboard remain available.

Nothing enables native logs on installation or upgrade. After confirming this feature is available in the published
release, use `observer serve --log-source system` on Linux, or
`observer.exe serve --log-source system --log-source application` on Windows. Managed background registration accepts
the same explicit source options on `background enable`.
Follow the [native log guide](https://github.com/braidenm/home-lab-observer/blob/main/docs/native-log-history.md).

## Installation and rollback

Download the archive for your OS/architecture; verify SHA256SUMS and provenance before running. No GitHub token, Go,
Node or Docker runtime is needed for the native program. Windows/macOS binaries remain unsigned/not notarized previews.
Use the installer supplied by the chosen release. V2 offline installs require its checksum-covered manifest and checksum
file as well as the archive; Linux needs the complete pair, never a helper copied from another release.

Upgrades select a program version but do not restart a running observer. Restart explicitly. Before rollback to a version
without native source settings, disable the background registration with the new binary and re-enable using the older
version without log-source options. Older previews do not prune the new log rows: program rollback is not database
rollback. Removal preserves the separate data directory and token; existing immutable previews remain rollback options.

- [Install, verify and remove](https://github.com/braidenm/home-lab-observer/blob/main/docs/install.md)
- [Background operation](https://github.com/braidenm/home-lab-observer/blob/main/docs/background-operation.md)
- [Read-only Docker observations](https://github.com/braidenm/home-lab-observer/blob/main/docs/container-observations.md)

Existing per-user background lifecycle and bounded product diagnostics remain available. These do not confer machine-wide
boot-before-login service support. Native metadata is not remote-upload eligible. Remote enrollment, Kubernetes collection
and start/stop/restart management of containers are separate future work, not capabilities announced by these draft notes.
