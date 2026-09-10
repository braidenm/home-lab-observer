# Plan

1. Keep background management outside collection, HTTP handlers and presentation. Use a small manager interface with
   OS-specific adapters and a fake command runner. Typed settings are validated with existing bind/Docker policies.
2. Preserve Spec 007's fixed archives. The installed observer executable implements lifecycle commands; any generated
   manager registration/helper lives in a dedicated managed background area with an ownership marker and exact allowlist.
   Installers recognize the marker for upgrades and refuse program uninstall while it exists.
3. Give the runtime a narrow local lifecycle package for fresh nonce publication and fixed graceful-stop requests.
   Treat private filesystem access as the local owner boundary; do not expose control over HTTP or accept commands/PIDs.
4. Add a bounded diagnostics writer independent of collectors and OS managers. It accepts only normalized code-owned
   structured records and exports a thread-safe health snapshot. Integrate it into background `serve` startup/shutdown.
5. Add a closed authenticated diagnostics-health read contract and optional transport-neutral dashboard source/panel.
   Missing support remains explicit; no new remote projection is enabled.
6. Verify fake-manager and native syntax/lifecycle tests, real local graceful shutdown/history restart, Windows and Unix
   path/permission boundaries, deterministic retention, UI mobile layout and API schema/security tests.
7. Perform independent cross-slice review, merge through required checks, then publish a new immutable preview only
   after the first Spec 007 release has been verified. Keep each slice's accepted evidence separate.

## Runtime defaults

- Local listen: `127.0.0.1:9847`; existing dedicated per-user state default.
- Docker: disabled unless an explicit validated local socket/pipe is configured.
- User manager only; Linux lingering and machine-wide installation are never enabled automatically.
- Graceful lifecycle wait: 35 seconds; forced stop must be explicitly requested.
- Diagnostic storage: five known files, 2 MiB each, seven days, 8 KiB per record. Unknown files are preserved/refused.
- Secrets and local API tokens are never included in manager arguments or diagnostic health.

## Delivery ownership

- Runtime/lifecycle package: bounded diagnostics and nonce stop request, deterministic/security tests.
- Packaging/manager package: OS adapters, known registration state, stable installed launchers and lifecycle tests.
- CLI/API/UI integration: entry-point orchestration, diagnostics-health contract and responsive status presentation.
- Root: specification/research/ADR/operator documentation, integration and independent acceptance/CI/release.
