# Native observer preview: optional background operation

Download the archive matching your operating system and architecture. No GitHub token, Go, Node or Docker runtime is
needed. Verify SHA256SUMS and GitHub provenance before running. Windows/macOS binaries are not publisher-signed or
notarized; no production signing certificate is used in this preview.

This release adds explicit per-user background enable/start/status/stop/restart/disable operations, graceful private
stop requests, bounded product diagnostics and an authenticated diagnostic-health view. Installing does not enable
startup automatically. Background operation requires a managed per-user install and your login session; it is not a
machine-wide boot-before-login service.

- [Download, verify, install and remove](https://github.com/braidenm/home-lab-observer/blob/main/docs/install.md)
- [Enable background operation, find your local token and troubleshoot](https://github.com/braidenm/home-lab-observer/blob/main/docs/background-operation.md)
- [Optional local Docker observations](https://github.com/braidenm/home-lab-observer/blob/main/docs/container-observations.md)

Local host history, process summaries and the embedded dashboard continue to work without background mode. Product
diagnostics are limited to five 2 MiB files and seven days while running; they contain fixed result metadata, not copies
of operating-system/application logs. Machine observations remain local. Remote enrollment/upload and operational
commands are not enabled by this release.

Upgrades select a new program version but do not restart a running observer; restart explicitly after reading these
notes. Disable background operation before uninstalling or rolling back to a foreground-only version. Program removal
preserves the separate data directory and token. Previous immutable preview downloads remain available for rollback.
