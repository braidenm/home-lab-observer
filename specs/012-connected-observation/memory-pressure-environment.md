# Fixed service memory-pressure environment

Status: accepted narrow profile correction; packaged VM rerun remains required.

All owned collector, uploader and enrollment/validation profiles explicitly set
`MemoryPressureWatch=skip`. The worker environment allowlist remains closed and
does not accept `MEMORY_PRESSURE_WATCH` or `MEMORY_PRESSURE_WRITE`.

The [systemd v255 resource-control contract](https://raw.githubusercontent.com/systemd/systemd/v255/man/systemd.resource-control.xml)
distinguishes `skip`, which does not set either variable, from `off`, which still
sets `MEMORY_PRESSURE_WATCH=/dev/null`. Defaults such as `auto` depend on manager
memory accounting and are not a stable input contract. An initial disposable VM
A/B diagnostic observed an early active state with `skip`, but did not establish
a sustained blocked-input state. A subsequent rebuilt packaged VM run and
empty-input diagnostic still exited before input. Therefore `skip` corrects the
documented environment contract but is not established as the sole cause or a
complete fix for the startup failure. Further isolated startup diagnosis remains
required.

Rendered profiles and loaded owned-service checks must require the exact value
`skip`, rejecting missing, duplicate, unknown, `off`, `on` or `auto` values.
Ancestor slice IP checks remain unchanged; they do not impose service environment
policy on unrelated slices. Existing `MemoryMax`, task, time, syscall, credential
and network limits remain unchanged. No production activation is authorized by
this correction, and the diagnostic is not full packaged acceptance.

Regression checks cover both steady workers, all three enrollment modes, online
effective policy, offline policy and steady-worker manager drift. Existing closed
environment tests must continue rejecting injected memory-pressure variables.
