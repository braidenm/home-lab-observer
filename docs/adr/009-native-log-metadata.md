# ADR 009: Fixed-source native event metadata, not unrestricted log ingestion

Status: Accepted; executable contracts precede feature implementation. Spec 008 is merged and preview 2 is published.

Date: 2026-09-09

## Context

Owners want to relate error bursts to host and workload pressure. The existing current-snapshot log section and
reusable dashboard provide a contract boundary, but no native source is implemented. Unrestricted message acquisition
would introduce secrets and identities that cannot be made safe merely by hiding a field in the UI.

## Decision

Add explicit, disabled-by-default fixed source presets. Windows System/Application use selected-property WEVTAPI;
Linux system logs use the optional bundled libsystemd helper in [ADR 010](010-optional-linux-journal-helper.md), not
journalctl continuation. macOS remains explicitly unsupported for native logs.
Bodies are never acquired or exposed by this slice. Native source authority is unchanged: no elevation, new group
membership, user-selected path/channel/query, shell, remote host or arbitrary command API.

Log work starts immediately and then every 60 seconds in a separate scheduler-owned single-flight acquisition lane.
It does not run in or reschedule the 15-second host cycle; brief bounded shared-database serialization is expected.
API requests consume cache/store views only. Shutdown joins log work before the host scheduler closes the borrowed store.

Windows reads use a bounded hidden helper invocation of the same verified binary. The child pins its query to its
creating thread, returns a closed metadata batch and cannot write history. The parent enforces the deadline and reaps
the child. Linux checkpoint values also travel only through bounded pipes; no cursor staging file is created.
Neither helper output nor a native cursor is durable progress before the store's atomic commit.

The store atomically commits compact minute source/severity rollups, coalesced attempt coverage, latest attempt state
and a checkpoint revision. It does not persist one row per poll. Revision
compare-and-swap prevents replayed batches from incrementing counts twice; an uncertain commit is resolved by rereading
the revision. A stale checkpoint triggers one bounded metadata-only latest-record probe: Windows uses a reverse fixed-
channel query and bookmark; Linux uses native seek-tail/previous/get-cursor after exact continuation validation. The probe
never requests bodies, adds no counts or covered-through time, and CAS-commits only the proved cursor,
`CHECKPOINT_RESET` and a bounded attempt-window gap, without estimating lost events.
An empty source stays reset-pending and cursorless until a later probe proves a tail; normal collection resumes on the
following cycle. Coverage uses caught-up query-start time and remains separate from event-time histogram counts.
Unknown loss is a gap, never an invented number.

A nil cursor without reset-pending state is a normal five-minute read even when its revision is nonzero. Store derives
coverage rather than accepting native-reader intervals: first caught-up success proves five minutes; later success
proves at most the final 60 seconds since the prior committed attempt. Older missed time is a gap. Failed/reset
attempts prove no coverage. Every committed attempt updates `PreviousAttemptAt`; the latest caught-up watermark is
status only. Clamp interval starts to seven-day retention. Explicit gaps remain sticky; unknown cells are merely absent
evidence and can be resolved. Persistence failure can overlay volatile current status but cannot invent durable history.

Recent event codes live only in one 200-record process-memory/session ring. Current counts describe that ring, not
persisted history; a restart can therefore retain summary history while the current list is empty. Persist no raw
messages, provider names, paths, identities,
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
private pipe boundaries and no disabled calls. Tests cover atomic retries, reset gaps, retention, contract compatibility,
current limits and responsive zero/unavailable presentation. Native CI must not print machine log contents.

Keep the core database at SQLite `user_version=2`; add backward-compatible log tables and store a dedicated log metadata
schema version in `store_metadata`. This lets the previous preview ignore additional tables instead of quarantining the
database, but the old preview will not prune log rows. To roll back, disable native sources with the new binary, stop it,
then start the old binary; never run both versions against the same live state directory. Ordinary shared retention
removes old rollups while the new version is active.

See [Spec 009](../../specs/009-native-log-metadata/spec.md), its
[wire checkpoint](../../specs/009-native-log-metadata/contract-checkpoint.md),
[internal ports](../../specs/009-native-log-metadata/internal-contracts.md), and the
[dated primary-source research](../architecture-research/004-safe-native-log-observations.md).
