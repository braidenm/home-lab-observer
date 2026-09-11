# E2: numeric-only collection library

Status: implementation slice; no worker, listener, installation or remote activation.

Extract CPU, memory/swap, filesystem and uptime normalization from the rich collector into
`internal/numerichost`. The rich collector delegates those sections in its existing order,
with identical local quality semantics. Numeric collection accepts a provider that has no
network, process, log or container operations. It must never call broad collection then redact.

The numeric profile defaults to unproven filesystem coverage. Its filesystem section is
degraded unless trusted installation evidence establishes namespace coverage. This is not a
user checkbox or a claim that accessible mounts necessarily represent all host storage.
Installation must independently prove the namespace profile before setting that evidence.
The existing rich collector continues to describe its existing local view, unchanged.

The narrow native provider initially supports Linux only. The gopsutil host package imports
process adapters on Windows/macOS, so those numeric provider targets explicitly return
unsupported without importing those adapters. Their remote implementation remains deferred.

The explicit remoteprojection boundary remains mandatory before handoff. It emits unavailable
overview data for missing, degraded, failed or truncated numeric inputs. No credential, HTTP,
storage, scheduler, command or installed principal configuration is introduced here.

Acceptance: numeric-only fake call sequence; rich normalization parity; permission and error
states; default unproven scope and bounded/truncated mounts; no sensitive source strings in
projection; Linux production dependency closure excludes process/network/log/container
collectors; focused and full tests, vet, cross-builds. Kernel calls may still block despite
context deadlines: this library does not claim to forcibly interrupt device/filesystem I/O.

## Evidence

- Full Windows Go suite and vet pass; existing rich collector tests retain the same results.
- Focused Linux numeric tests pass ten repetitions under Ubuntu WSL with synthetic providers.
- Production Linux dependency graph excludes gopsutil process/network and application
  broad collection, log, container, upload and HTTP packages. Common gopsutil utility code
  still imports os/exec; this slice does not claim the binary is unable to execute programs.
  Installed systemd syscall/capability/namespace enforcement remains a separate gate.
- Unsupported native Windows/macOS provider is explicit, not a substitute for remote support.
- No native sampling of user machine data, privileged fixture or installed worker was run.
