# Spec 008: Explicit background lifecycle and bounded self-diagnostics

**Status:** Accepted preview defaults under the owner's continuing delivery instruction; Windows boot-before-login
support remains a separately surfaced owner choice, not an implied capability.

## Outcome

An owner can explicitly keep the installed local observer running in the background, see whether it is healthy, stop
it gracefully and remove its background registration. Its own diagnostics cannot grow without a bound. The verified
native installer remains side-effect-free: downloading or installing never enables background operation.

## Requirements

- B1: Keep no-argument/`serve` foreground behavior. Add an explicit local `background` lifecycle interface for managed
  native installations: enable, start, status, stop, restart, disable. Enable explains and performs start-now plus
  future-login registration; nothing elevates, changes PATH, opens a browser, enables Docker or enrolls a remote host.
- B2: The preview is per-user and session-scoped: Linux systemd user service without enabling lingering, macOS user
  LaunchAgent, and Windows least-privilege current-user Task Scheduler without a password. Explain logout, reboot and
  missing-manager behavior. Windows Service/boot-before-login and machine-wide profiles require a later explicit design.
- B3: Use the stable managed launcher/current version, fixed manager identity and typed validated configuration only.
  Support dedicated install/state directories, explicit IPv4 loopback listen address and optional validated local Docker
  endpoint. No shell command, extra arbitrary arguments, remote host or user-supplied service template is accepted.
- B4: Registration and lifecycle operations are idempotent, serialized and bounded. Inspect and validate an existing
  registration/managed marker before changing or deleting it; refuse unknown or mismatched registrations, links/reparse
  paths and broad roots. Do not modify other services, tasks, launch agents or user state.
- B5: Status distinguishes manager unavailable, not registered, registered/stopped, running, and unreachable/not-ready
  local service. Report the effective session limitation and diagnostic availability; never treat missing evidence as
  healthy zero values. Stable status/error codes support scripts without parsing localized manager prose.
- B6: Normal stop/restart/disable requests a graceful stop and waits up to 35 seconds for listener/history shutdown.
  Windows must not rely on Task Scheduler's immediate termination for normal stop. Use an owner-only fixed local stop
  request containing only a fresh instance nonce; reject stale/wrong-instance/oversized/malformed requests. No HTTP
  mutation route, arbitrary command execution or process-ID-only kill channel is introduced. Forced manager termination
  requires an explicit force option and is not the fallback for a timeout.
- B7: Upgrade and rollback do not restart a background process implicitly. The next explicit restart uses the newly
  selected installed version. Program uninstall refuses while a managed background registration exists and tells the
  owner to disable it first. Disable preserves history, local token and bounded diagnostics.
- B8: Background self-diagnostics use a fixed owner-only `diagnostics` subdirectory of the state directory, not an
  arbitrary log sink. Cap the active JSONL file and four rotations at 2 MiB each (10 MiB total), individual records at
  8 KiB and age at seven days. Rotate before a write would exceed a limit; reject unsafe existing entries. Only
  code-owned event/code/version/counter/duration fields are permitted. Never persist tokens, headers, request/query
  text, raw exceptions/paths, process/container identities or observed log bodies. No recursive self-ingestion.
- B9: Diagnostics failures must not stop collection or recursively log their own failures. Provide authenticated,
  bounded read-only diagnostic health (enabled/available, byte/retention limits, dropped/write-failure counters) and a
  small corresponding Observer & privacy panel. Do not expose a general file-read/log-download API. Foreground stderr
  remains useful; background managers discard ordinary stdout/stderr rather than create their own unbounded files.
- B10: Test lifecycle templates/arguments, manager absence, concurrent operations, unknown registrations, path attacks,
  interrupted enable/disable, nonce replay/timeout, graceful restart/store reuse, version selection, program uninstall
  refusal, and diagnostics rotation/age/permissions/failure/secret canaries. Native CI validates platform syntax and
  uses isolated test registrations only where a real user manager is available. Never infer login/reboot behavior from
  a unit test or modify the developer's real startup configuration while testing.

## Boundaries

This is an explicitly labeled local convenience profile running at the owner's existing authority. It is not the
separately isolated collector/uploader remote profile described by ADR 001. It cannot remotely manage the computer.
Native application/OS log observations, container packaging and outbound enrollment/upload remain separate specs.

The asynchronous Windows question may promote boot-before-login support into a later Windows Service specification;
until answered, the no-elevation preview default is used and its limitation is visible before enablement.
