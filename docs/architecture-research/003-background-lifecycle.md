# Background lifecycle: authority, shutdown and bounded diagnostics

Research date: 2026-09-09. Scope: Spec 008's local, unsigned native preview; Go 1.27, Task Scheduler 2.0,
systemd user managers and macOS LaunchAgents. This is not the isolated remote collector/uploader profile.

## Current evidence and decision

Spec 007 delivers six immutable native archives with a stable per-user launcher and explicit upgrade/rollback.
`cmd/observer/serve.go` already drains HTTP and the scheduler on cancellation. Its stderr logger uses code-owned
messages, but has no retained, bounded background sink. Installers deliberately do not register startup jobs.

Keep foreground as the default. An explicit `background enable` registers only the current user's session manager
and starts the observer. Use typed settings and fixed identities, not arbitrary command strings. The owner can
inspect, stop, restart and disable the registration. Disable preserves observation state.

| Option | Fit | Decision |
| --- | --- | --- |
| Foreground only | Portable and no startup mutation; terminal must remain open | Retain as default/fallback |
| User-session manager | No saved password or elevation; visible login/logout limitations | Preview profile |
| Machine-wide service | Boot-before-login, but requires privileged installation and account/credential design | Separate future spec |

## Primary evidence

- [Microsoft task security](https://learn.microsoft.com/en-us/windows/win32/taskschd/security-contexts-for-running-tasks)
  documents current-user interactive-token tasks and least-privilege registration. We use neither password nor S4U.
- [Microsoft task settings](https://learn.microsoft.com/en-us/windows/win32/taskschd/tasks) documents the default
  72-hour execution limit. A long-running observer needs an explicit unlimited execution time, bounded restart policy
  and single-instance behavior. Interactive-token operation does not promise service-before-login coverage.
- [Apple launch jobs](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html)
  distinguishes user agents from system daemons. Use a ProgramArguments array, throttled failure restart and a user
  registration; do not request sudo, install a LaunchDaemon or bypass Background Items approval.
- [systemd login control](https://www.freedesktop.org/software/systemd/man/252/loginctl.html) documents lingering as
  a separate user-manager lifetime choice. Do not enable it automatically. Missing user manager/session bus is an
  actionable unavailable state. The primary page was independently checked during design; a later root fetch failed.
- [systemd termination policy](https://github.com/systemd/systemd/blob/main/man/systemd.kill.xml) and
  [service semantics](https://github.com/systemd/systemd/blob/main/man/systemd.service.xml), independently checked from
  upstream source on the same date, show why a stop timeout alone is insufficient: automatic final killing must be
  disabled for the normal stop path. Explicit force must not accidentally restart the service through failure policy.
  Generated arguments also need systemd's own quoting/expansion rules, not merely shell quoting.

Sources were checked against the existing foreground implementation and independently reviewed across runtime and
packaging responsibilities. Native tests, not documentation alone, must establish executable behavior.

## Failure-mode FAQ

**Does closing a terminal stop it?** Not after explicit enable. Logout/reboot coverage depends on the user session;
the preview makes no before-login guarantee. The owner was offered a separate Windows Service design and the
no-elevation default remains in effect unless changed.

**Can scheduled-task stop lose history?** Its termination is not a graceful signal. First send an owner-only,
instance-nonce-bound fixed stop request and wait for listener/history shutdown. Never kill by PID alone or silently
force after a timeout. An explicit force operation has a documented data-loss risk.

**Can logs fill a disk?** Product-owned diagnostics have five fixed files, each at most 2 MiB, seven-day age and 8 KiB
record limits. Managers discard stdout/stderr. No raw observation fields, paths, errors, credentials or request data
are diagnostic inputs. A failed diagnostic write increments a counter without blocking collection or logging itself.

**What if an existing task has the same name?** Validate ownership, exact configuration and marker. Refuse unknown or
mismatched registrations. Serialize lifecycle changes and test interrupted registration/removal recovery.

**How do upgrade and rollback work?** They only change the selected installed version. Explicit restart adopts that
version; neither operation restarts a running observer. Uninstall requires background disable first and preserves data.

### Native verification finding: Task Scheduler defaults

The disposable Windows registration test showed that persisted task XML omits several explicitly supplied
default-valued elements even though an in-memory template round trip retains them. Microsoft's
[Task Scheduler schema](https://learn.microsoft.com/en-us/windows/win32/taskschd/task-scheduler-schema)
documents optional settings and trigger defaults. Ownership comparison must account for those semantics, not just
compare serialized tokens. Only documented defaults at exact known paths qualify; changed values, duplicate elements,
unexpected attributes, additional actions and different principals must still fail closed. Verify the effective
least-privilege principal separately from assuming that an absent element proves the requested privilege level.
This source and the hosted evidence were checked on 2026-09-09. The actual saved-task lifecycle remains the release
gate; passing only an in-memory template test is insufficient.

The following hosted run isolated a second serialization difference: expected versus saved XML differed at the
second direct child of `Task`, while COM and the command-line query agreed. Microsoft's
[taskType definition](https://learn.microsoft.com/en-us/windows/win32/taskschd/taskschedulerschema-tasktype-complextype)
declares direct children with `xs:all`, so their order is not significant. Normalize only this documented top-level
order, preserving namespace, cardinality, attributes, values and nested action/trigger ordering. This finding does
not justify accepting an unknown task or skipping saved-registration verification. Root independently verified the
schema on 2026-09-09 against the sanitized diagnostic from hosted run `34408746316`.

Run `34410120460` then exposed the same order-only difference inside `Settings`. A complete review of Microsoft's
schema confirms `xs:all` for the emitted profile's `Task`, `RegistrationInfo`, `Settings`, `IdleSettings`,
`RestartOnFailure` and `Principal` nodes. Those exact namespace-qualified paths can be compared without child-order
significance; attributes, values, allowed children and multiplicity remain exact. Trigger, action and executable
sequences are not included. This extends the initial top-level finding rather than making every XML subtree unordered.

## Acceptance and rollback

Fake adapters verify exact arguments and manager states; native CI verifies syntax and isolated registrations when a
real user manager exists. Do not register startup on the developer's machine merely to run tests. Verify stale nonce,
timeout, malicious paths/registrations, retention, secret canaries and shutdown/store reuse. API/UI distinguish absent
diagnostics, healthy zero counts and failures. Disable returns to foreground operation without schema/data migration.

See [Spec 008](../../specs/008-background-lifecycle/spec.md) and [ADR 008](../adr/008-local-background-convenience-profile.md).
