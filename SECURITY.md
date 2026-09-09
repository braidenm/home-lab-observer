# Security policy

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub's private vulnerability reporting for this repository. Include the affected version, platform, configuration, reproduction steps, and expected impact without including real credentials or private host data.

## Security boundary

Home Lab Observer reads sensitive machine state. Its local API is loopback-only by default; remote observation is outbound and explicitly configured. The first stable release will not execute arbitrary commands or expose raw Docker or operating-system control APIs.

The project does not guarantee that every possible log message is free of secrets. Log bodies are disabled by default, sources are allowlisted, common secret patterns are redacted, retention is bounded, and operators remain responsible for source selection.

## Supported versions

Until the first stable release, only the most recent tagged prerelease receives security fixes. After 1.0, the support window will be documented here.
