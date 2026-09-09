# Keep the observer running in the background

Background operation is optional. Downloading, unpacking or installing the observer never enables it.
This guide describes Spec 008's native preview profile; the initial `0.1.0-preview.1` release is foreground-only.
Use a preview whose release notes include background lifecycle support.

## What to expect

| Machine | Manager | Important limitation |
| --- | --- | --- |
| Linux | Your systemd user service | Requires a working user manager/session bus; no lingering is enabled |
| macOS | Your LaunchAgent | Requires your login session; macOS may request Background Items approval |
| Windows | Your least-privilege scheduled task | Runs while you are signed in, not a Windows Service before login |

No administrator password is saved. No firewall, system-wide security, PATH or Docker settings are changed.
The local dashboard still only listens on `127.0.0.1`. Background mode does not connect to Platform Demo or permit
remote commands. If the manager is unavailable, you can always run the foreground observer in a terminal.

## 1. Install and enable explicitly

Use the [verified per-user installer](install.md) first. An unpacked archive by itself is not a managed installation.
Keep the program directory printed by the installer. The examples below use the default directory; substitute your
explicit installation directory if you chose a different one.

Linux:

```sh
observer_root="${XDG_DATA_HOME:-$HOME/.local/share}/home-lab-observer"
"$observer_root/bin/observer" background enable --install-root "$observer_root"
```

macOS:

```sh
observer_root="$HOME/Applications/Home Lab Observer"
"$observer_root/bin/observer" background enable --install-root "$observer_root"
```

Windows PowerShell:

```powershell
$observerRoot = Join-Path $env:LOCALAPPDATA 'Programs\Home Lab Observer'
& "$observerRoot\bin\observer.cmd" background enable --install-root $observerRoot
```

Enable registers future-login startup and starts the observer now. It does not open a browser. Optional enable settings
are `--state-dir` for a dedicated local data directory, `--listen` for an explicit IPv4 loopback address/port, and
`--docker-endpoint` for a [validated local Docker socket/pipe](container-observations.md). Docker remains disabled
unless you choose it. Stop a foreground instance using that port/state before starting background operation.

## 2. Check it and open the dashboard

Use the same launcher and program root for every lifecycle command:

```sh
"$observer_root/bin/observer" background status --install-root "$observer_root"
"$observer_root/bin/observer" background status --install-root "$observer_root" --json
```

On Windows:

```powershell
& "$observerRoot\bin\observer.cmd" background status --install-root $observerRoot
& "$observerRoot\bin\observer.cmd" background status --install-root $observerRoot --json
```

Status distinguishes missing manager, unregistered, stopped, running and an unreachable/not-ready local service.
Running is not proof that every collector is healthy. Open `http://127.0.0.1:9847` (or your selected local port), unlock
using the local token file, and inspect collection freshness and **Observer & privacy**. The token is generated locally;
it is not a GitHub token. Status JSON never contains it.

## 3. Stop, restart, disable or upgrade

Replace `status` above with the operation you want:

| Operation | Effect |
| --- | --- |
| `start` | Start an existing registration |
| `stop` | Request graceful shutdown; keep the future-login registration |
| `restart` | Gracefully stop, then start the currently selected installed version |
| `disable` | Gracefully stop and remove the owned startup registration; preserve data |

Normal shutdown waits up to 35 seconds for the instance, HTTP requests and history to close. A timeout is reported;
it does not silently force termination. `--force` on stop/restart/disable explicitly permits manager termination when
needed and may interrupt a write. Use it only after inspecting the reported state; never delete live database files.

Installing an upgrade or selecting a rollback version does not restart a running observer. Use an explicit restart
after checking release compatibility notes. Disable background operation before program uninstall; the installer
refuses to remove programs while a managed registration remains. History, local token and diagnostics stay separate
from the program files and are preserved.

## Bounded diagnostics, not unrestricted logs

Background mode writes only product-owned diagnostic events in the state directory's `diagnostics` folder:
`observer.jsonl` and four numbered rotations. Each file is capped at 2 MiB, all five at 10 MiB, each record at 8 KiB,
with a seven-day age limit. These are not copies of your OS, application or container logs.

Records contain only fixed event/result codes, version, UTC time, counts and durations. They exclude tokens, requests,
headers, raw errors, filesystem paths and process/container identities. The OS-manager profile discards ordinary output
so it cannot create a second unbounded output file. **Observer & privacy** reports enabled/available state, retained
bytes, limits, dropped records and write failures, without exposing file contents through the API.

A diagnostic write failure does not stop machine observation. Unsafe files, links, an unknown registration or a
mismatched ownership marker are refused, not overwritten. If startup is unavailable, use foreground mode and include
only the stable error code, version and OS in a support request—not your token or private logs.

## Architecture and scope

```text
Explicit local CLI -> validated user-manager registration -> selected observer version
                                                       -> local collection/history/API/UI
                                                       -> bounded self-diagnostics
Local stop request -> private fresh-instance nonce -> graceful shutdown
```

The read-only HTTP API cannot enable startup or stop the host. This is a local convenience profile at your account's
existing authority, not a privilege-isolated collector/uploader installation. Machine-wide boot-before-login service,
remote sync, native log observations and operational actions have separate specifications.

See [ADR 008](adr/008-local-background-convenience-profile.md), [the decision research](architecture-research/003-background-lifecycle.md)
and [Spec 008](../specs/008-background-lifecycle/spec.md).
