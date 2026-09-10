# ADR 008: Explicit user-session background operation with bounded diagnostics

**Status:** Accepted  
**Date:** 2026-09-09

## Context

Verified native downloads already run without development tools, but foreground operation ends with the terminal.
Owners need a simple optional background mode. System-wide services require additional privileges, credentials and
platform-specific signing/account decisions. Unbounded manager-captured output would contradict the privacy/storage
constitution.

## Decision

Keep foreground operation as the default. Add an explicit per-user local convenience profile managed by systemd user
services, a macOS LaunchAgent or a least-privilege Windows interactive-token scheduled task. Do not automatically enable
Linux lingering, install a Windows SCM service, save a password or modify system-wide startup/security settings.
State the login-session limitation before enabling it. This does not claim the collector/uploader privilege separation
required of the future remote installed profile in ADR 001.

Use typed local lifecycle operations, fixed registrations and the stable managed program selection. Normal shutdown
uses the runtime's graceful path, including an owner-only instance-bound stop request where manager stop is abrupt.
Forced termination stays explicit. Existing or unknown registrations are not overwritten based on a matching name alone.

Own diagnostic retention inside the application: code-owned structured events, five files totaling at most 10 MiB,
seven-day age limit and per-record bounds. Manager output is discarded in background mode. Collector data, raw errors,
secrets and native log bodies are not diagnostic fields. Report logging failures without stopping observation.

## Alternatives and consequences

- Foreground only remains the most portable and least invasive fallback but does not meet terminal-independent use.
- Machine-wide systemd/LaunchDaemon/Windows SCM profiles can run before login, but require an explicit elevated account,
  credential and service-control design. They remain a separately testable later profile.
- OS-manager logging alone is inconsistent: journald retention is administrator policy; launchd output files do not
  provide this product's fixed rotation policy; scheduled tasks do not provide a uniform stdout store.
- Native container packaging remains optional and is not an equivalent Windows/macOS full-host implementation.

The preview must not advertise uninterrupted boot-to-logout coverage, remote commands or a new isolation boundary.
See [Spec 008](../../specs/008-background-lifecycle/spec.md) and its operator documentation for acceptance and rollback.
