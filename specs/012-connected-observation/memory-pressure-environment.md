# Fixed service memory-pressure environment

Status: accepted narrow profile correction; packaged VM rerun remains required.

All owned collector, uploader and enrollment/validation profiles explicitly set
`MemoryPressureWatch=skip`. The worker environment allowlist remains closed and
does not accept `MEMORY_PRESSURE_WATCH` or `MEMORY_PRESSURE_WRITE`.

The [systemd v255 resource-control contract](https://raw.githubusercontent.com/systemd/systemd/v255/man/systemd.resource-control.xml)
distinguishes `skip`, which does not set either variable, from `off`, which still
sets `MEMORY_PRESSURE_WATCH=/dev/null`. Defaults such as `auto` depend on manager
memory accounting and are not a stable input contract. The disposable VM A/B
diagnostic reported baseline uploader refusal before input, while `skip` reached
the blocked-input state and passed the loaded offline policy check.

Rendered profiles and loaded owned-service checks must require the exact value
`skip`, rejecting missing, duplicate, unknown, `off`, `on` or `auto` values.
Ancestor slice IP checks remain unchanged; they do not impose service environment
policy on unrelated slices. Existing `MemoryMax`, task, time, syscall, credential
and network limits remain unchanged. No production activation is authorized by
this correction, and the diagnostic is not full packaged acceptance.

Regression checks cover both steady workers, all three enrollment modes, online
effective policy, offline policy and steady-worker manager drift. Existing closed
environment tests must continue rejecting injected memory-pressure variables.
