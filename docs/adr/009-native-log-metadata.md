# ADR 009: Fixed-source native event metadata, not unrestricted log ingestion

Status: Proposed; implementation starts only after Spec 008 completes.

Date: 2026-09-09

## Context

Owners want to relate error bursts to host and workload pressure. The existing current-snapshot log section and
reusable dashboard provide a contract boundary, but no native source is implemented. Unrestricted message acquisition
would introduce secrets and identities that cannot be made safe merely by hiding a field in the UI.

## Decision

Add explicit, disabled-by-default fixed source presets. Windows System/Application use selected-property WEVTAPI;
Linux system logs use journalctl 242+ selected-field output. macOS remains explicitly unsupported for native logs.
Bodies are never acquired or exposed by this slice. Native source authority is unchanged: no elevation, new group
membership, user-selected path/channel/query, shell, remote host or arbitrary command API.

Windows reads use a bounded hidden helper invocation of the same verified binary. The child pins its query to its
creating thread, returns a closed metadata batch and cannot write history. The parent enforces the deadline and reaps
the child. Linux checkpoints travel through private per-attempt cursor files, not visible process arguments. Neither
helper output nor journalctl's temporary cursor file is durable progress.

The store atomically commits fixed source/severity rollups, attempt coverage and a checkpoint revision. Revision
compare-and-swap prevents replayed batches from incrementing counts twice; an uncertain commit is resolved by rereading
the revision. Stale checkpoint reset establishes a new tail without recounting its reset window. Coverage uses caught-up
query-start time and remains separate from event-time histogram counts. Unknown loss is a gap, never an invented number.

Recent event codes live only in a bounded memory ring. Persist no raw messages, provider names, paths, identities,
message hashes or unbounded facets. Retain rollups within the existing seven-day/250 MiB store budget. API requests read
cached/store views; they never trigger native acquisition. An optional summary source lets other clients reuse the UI.
The current log contract remains compatible, with bodies OMITTED and remote upload ineligible.

## Alternatives and consequences

- Keeping logs unsupported has the smallest authority footprint, but cannot meet the requested error-trend experience.
- Reading complete messages then redacting is simpler to prototype, but acquires unnecessary sensitive data first.
- Embedding a full log platform adds useful search, but expands services, retention, tenancy and operational scope.
- In-process Windows cancellation alone cannot enforce a hard deadline for a blocked native operation; the fixed child
  boundary adds code, but permits bounded process lifetime without generic execution authority.

This produces useful captured-event trends, not a complete audit log or SIEM. Source permission failures, backlog,
rotation and reset gaps remain visible. macOS parity, raw bodies, more sources, live tail and remote rollups require
separate specifications. The privacy defaults do not depend on the future uploader choice.

## Verification and rollback

Synthetic native fixtures and secret canaries verify selected-field acquisition, bounded input/output, child reaping,
cursor-file cleanup and no disabled calls. Tests cover atomic retries, reset gaps, retention, contract compatibility,
current limits and responsive zero/unavailable presentation. Native CI must not print machine log contents.

Disable sources to stop new acquisition. Ordinary retention removes old rollups. Document database migration and
rollback compatibility before publication; never run two versions against the same live state directory.

See [Spec 009](../../specs/009-native-log-metadata/spec.md), its
[contract checkpoint](../../specs/009-native-log-metadata/review-decisions.md), and the
[dated primary-source research](../architecture-research/004-safe-native-log-observations.md).
