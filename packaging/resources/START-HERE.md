# Home Lab Observer native preview

This archive contains a self-contained Home Lab Observer binary. It does not need Go, Node.js, Docker, or a GitHub token.

## Run in the foreground

- Linux: run `./run-observer.sh` from a terminal.
- macOS: double-click `Run-Observer.command`, or run `./Run-Observer.command` from Terminal.
- Windows: run `Run-Observer.cmd` from Command Prompt or PowerShell.

The observer stays in the foreground and prints its local access information. Stop it with Ctrl+C. It does not install a service, change PATH, open a browser, enable Docker, or contact Platform Demo.

Pass normal command-line options after the helper name. For example, use the platform helper followed by `version --json` to verify the build without starting the server.

Container observations remain disabled unless you explicitly provide a supported local Docker endpoint to the observer. See the repository documentation for the security implications before enabling one.

## Preview trust notice

This preview is checksummed and accompanied by GitHub build provenance, but its Windows and macOS binaries are not yet publisher-signed or notarized. A checksum verifies downloaded bytes against the release; it does not establish publisher identity. Review the matching GitHub release and provenance before running it. Follow the normal per-file prompts from your operating system; do not disable system-wide security protections.

Project documentation: https://github.com/braidenm/home-lab-observer
