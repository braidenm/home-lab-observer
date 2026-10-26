# Linux connected start, stop and status

Implements the current invocation gate in ADR 021 for a completed first install.
Scope: Ubuntu 24.04, systemd 255, Linux amd64. An explicit root command starts
the exact installed collector/uploader pair after the installation lease,
artifact/configuration/unit/principal checks and same-process request/probe/commit
transaction. It never enrolls again, resets a ledger or adopts partial setup.

## Acceptance

- Invoke the installer from the selected immutable installed release. Its inode
  must match the selected bundle's verified installer. The config must have exact
  canonical installed and preparing records, known root/state members, exact
  units, fixed resolver/hostname data and trusted root-owned CA bytes.
- Both workers must be stopped. A failed invocation with PID zero can be started
  explicitly; active or transitional invocations refuse. Services remain disabled
  and `Restart=no`; a later boot supervisor must repeat the complete transaction.
- Start the uploader behind its absent-request barrier, audit that invocation's
  descriptors and loaded policy, publish the fresh request, verify bound probes
  and response, start the collector, then publish the matching commit. Bound the
  transaction to 30 seconds. Do not label process startup as remote acknowledgement.
- On failure, cancel and join only the captured invocations with an independent
  bounded cleanup context. A replacement/unknown invocation requires recovery.
  Preserve credential/ledger bytes, including ambiguous post-commit delivery.
- `stop` validates the installed identity and exact loaded units before joining
  both services. `status` reads only bounded private records, checks invocation
  and manager liveness before and after reads, downgrades stale acknowledgement,
  and reports a failed worker separately from a stopped one.
- Unknown commands, arbitrary units, alternate network targets, executable paths
  and service enable/management operations are not accepted inputs.

## Verification and release

Focused transaction tests inject each failed phase, cancellation, stale response,
unexpected probe traffic, short entropy, replaced process and uncertain commit.
The existing dependency-closure, exact-unit and stopped-install checks continue.
The exact packaged binary must then pass disposable-VM upload, denied-network,
mount/identity, restart, response-loss, revocation and retained-ledger acceptance
before an owner canary. Unit tests alone do not establish installed enforcement.

Rollback for this slice is explicit stop with retained delivery state. The prior
read-only connector remains independently operational. Compatible code selection,
endpoint refresh and unattended boot/recovery remain distinct deliverables, not
claims made by this manual start command.
