# Native log runtime integration

Status: implementation contract; native activation and packaged proof pending.

- `serve` and `background enable` accept repeated `--log-source system` / `--log-source application`.
  No values means disabled. Reject duplicate, unknown, comma-separated or whitespace-padded values, and more than
  two values before opening state or registering a background profile. Canonical order is system, application.
  Linux rejects application; macOS accepts the fixed choices but reports unsupported without launching a helper.
- Background settings persist only explicit source choices. Empty choices omit the additive field, preserving
  existing v1 profiles. A log-enabled profile is not readable by an older binary's closed parser: disable background
  mode before rolling back to a version without native logs, then enable the old profile without log options.
  Reconfiguration uses existing disable/enable semantics; no silent expansion of permissions or source discovery.
- One log collector borrows the existing Store and supplies both cached current metadata and summary history.
  It has an independent 60-second schedule. No HTTP request triggers collection. No sources means no helper
  resolution, native query, or optional log schema initialization.
- Shutdown drains HTTP first, cancels and joins log collection, then stops the host scheduler. Both borrow Store;
  the composition root closes it only after both join. If either cannot join within its fixed timeout, report an
  unsuccessful shutdown and leave final resource cleanup to process exit, never close storage under live work.
- Linux main remains static and free of native bindings. Its fixed adjacent helper hardens itself before reading
  stdin or loading libsystemd. Windows has only a fixed private helper mode. Neither is an arbitrary command API.

Verification includes strict option parsing, legacy/opt-in background persistence, no-source inactivity,
independent scheduling/cache-only API behavior, shutdown order, and actual packaged helper identity/process tests.
Windows continuation policy is accepted in ADR 011; actual owned native fixture acceptance remains required.

## Implementation evidence (not a release-completion claim)

Independent review caught unconditional container-client teardown after an unjoined host scheduler. Owned container
resources now close only before collection starts or after both collectors have joined. Failure-path tests ensure
the shared close callback is withheld; a real local HTTP test verifies disabled summary wiring and reusable history
after shutdown. Current and summary ports receive the same collector.

The fixed Linux helper entrypoint has independent review and passed its synthetic entrypoint tests under Ubuntu WSL.
Those tests use injected native-open/hardening seams and read no host logs. Linux test compilation/vet and the Linux
main dependency inspection pass; the main imports neither native journal bindings, journal hardening nor purego.
Full combined Windows Go tests/vet pass. Packaged end-to-end native activation remains a separate gate, so no new
preview is published from this integration branch yet.
