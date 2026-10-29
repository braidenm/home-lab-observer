# Spec 013: Rich connected system inventory

Status: owner-authorized product scope, 2026-10-29; pure workload profile implemented and independently reviewed locally.
Exact-head CI, receiver, installed runtime and release acceptance remain pending.

## Outcome

An owner can use the hosted dashboard to understand a machine and its Docker workloads, rather than only seeing
CPU/memory counters. The same collection adapters remain useful in the standalone local application. Linux is the
first connected target; do not imply connected Windows/macOS parity. Kubernetes remains deferred.

The owner authorized expanding collection and separately permissioned management. That does not authorize silently
expanding every existing installation's egress, granting delegates control, or modifying unrelated workloads.

## Stories and acceptance

1. **Understand coverage.** An owner/viewer can distinguish not configured, unsupported, permission denied,
   temporarily unavailable, partially collected, stale and genuinely empty inventory. Never label uncollected rows
   as zero running containers/processes. Show collection time separately from receipt time and bound loaded counts.
2. **Inspect processes.** An explicitly enabled process group exposes at most 200 current rows, using the existing
   bounded process collector. Allow only PID, process basename, closed state, CPU measurement when actually known,
   resident bytes and elapsed time. No account names/IDs, arguments, executable paths, environment, working directory,
   open files, connections or raw failure strings. A PID is display data, never management authority.
3. **Inspect Docker.** An explicitly enabled Docker group includes running and stopped containers, at most 500 rows.
   Allow bounded display name, image display reference, closed state, health/restart count only when supplied, and
   nullable CPU/memory/network/block-I/O measurements. Preserve unsupported/denied/stopped metric states; never invent
   zero measurements to fit the old server snapshot. Do not export environment, labels, mounts, commands, ports,
   raw inspect/error bodies, engine connection details or daemon socket. Display aliases never authorize actions.
4. **Understand the host.** Retain quality-aware numeric host data. Add network and disk-I/O counters/rates only with
   proven host-namespace visibility. Counter reset, reboot and unknown sampling interval create a gap, not a spike.
   Storage labels come from an explicit local owner mapping to bounded display names, not automatic upload of paths.
   Existing ordinal filesystem aliases remain honest, non-stable display aliases until that mapping is implemented.
5. **Understand services.** Add bounded service name/state through a separate read-only platform adapter, initially
   systemd. No service descriptions, ExecStart/environment, unit bodies or arbitrary D-Bus forwarding. If absent or
   unsupported, report that explicitly. Service controls require another action specification; Docker grants do not
   authorize system services.
6. **See trends.** Keep workload rows latest-only in the initial release. Add bounded aggregate numeric history as a
   separately measurable slice with disclosed retention and gap semantics. No raw inventory/event body is smuggled into
   a metric label. No raw log bodies in this spec; safe log metadata needs a separate opt-in remote contract.
7. **Retain control of data.** New process, container, service and storage-label groups default off and require an
   explicit host-local policy. Enrollment alone does not consent to them. Policy advertises effective collection and
   egress separately; remote UI cannot widen local policy. Names/image references are sensitive metadata, not anonymous
   because they passed a sanitizer. Every hosted read retains owner/user-ID-bound authorization and revocation.
8. **Manage safely later.** Docker start, stop and restart remain Spec 011, each independently granted for an approved
   immutable target and separately enabled local broker. No shell, exec, arbitrary command, image/config/volume change,
   container deletion, host reboot, automatic remediation or Kubernetes action is added here.

## Contract and operational requirements

- Use new explicitly versioned workload contracts; preserve every byte-level numeric-host/v1 producer/receiver fixture.
  Receiver acceptance precedes producer activation. Consumers must retain null metrics and section-specific quality.
- Construct egress DTOs by explicit assignments from normalized adapters; never serialize a local snapshot and redact
  afterward. Validate schema, source binding, duplicate identities, closed states, finite/exact numeric bounds, row and
  UTF-8 text limits, payload byte budget and deadlines before any persistence or upload.
- Define exact per-profile size budgets in executable contracts before increasing handoff, ledger or transport limits.
  A larger parser cap alone is not complete pipeline support. Preserve sequence, pending request bytes and credentials
  across compatible code changes; never reset a used ledger to recover an upgrade.
- Current numeric workers keep their accepted installed sandbox. No automatic removal of ProtectProc/PrivateNetwork,
  added Docker group, new socket access, unit drop-in or silent service restart. Rich observation requires an explicit
  reviewed installed profile and exact packaged isolation acceptance; do not claim a compatible-code-only update can
  change that profile. See ADR 023 for the staged decision and unresolved rollout gate.
- A Docker-capable helper is authority-bearing even when its API issues only GETs. Use separate bounded local mediation
  and keep uploader credentials unreachable; do not call a read-only socket mount an Engine authorization policy.
- Keep collection fan-out, process scans, outstanding requests, memory, disk, retry and retention bounded. Prove normal
  and disconnected behavior without harming other workloads. Routine PR tests target <15 minutes; packaged VM and
  disposable Docker fault journeys remain explicit manual release gates, not silently skipped requirements.
- Existing connector/release stays usable until the replacement profile is accepted. Never reinstall over installed
  state, copy a credential into a different server identity, silently inherit grants, delete private state, or claim
  an upgrade/rollback works without its actual fixture. A profile-changing migration needs an explicit reviewed plan.

## Completion evidence

Schema/producer/receiver negatives; real backend owner/delegate denial and revocation; desktop/phone UI review; isolated
Ubuntu and disposable Docker tests; exact immutable public artifacts and provenance; supported install/manage/rollback
instructions; measured owner-host resource use; and actual hosted fresh process/container rows. A merged pure projector
or a zero/unavailable panel is not delivery. Control has its own independent Spec 011 acceptance.

## Decision status

No further product decision is needed for the read-only scope. Exact rich-profile sandbox, profile-changing migration,
local consent persistence and bounded workload identity policy require independent architecture/security review before
activation. Choosing those mechanisms is delegated engineering work, not permission to weaken accepted boundaries.

See [plan](plan.md), [tasks](tasks.md), [research](../../docs/architecture-research/008-connected-inventory.md),
[ADR 023](../../docs/adr/023-opt-in-connected-inventory.md), and [the existing numeric contract](../012-connected-observation/spec.md).
