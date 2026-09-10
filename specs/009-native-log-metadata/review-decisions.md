# Rationale from independent contract review

Proposed on 2026-09-09 after a read-only review by `runtime_architecture`. These conservative defaults refine the
metadata-only slice; they do not enable sources or expand authority. Incorporate them into the accepted ADR and
executable contract before implementation begins, after Spec 008 merges. The authoritative exact checkpoint is
[the wire contract](contract-checkpoint.md) and [the internal ports](internal-contracts.md); this document is rationale.

Owner accepted the separate bundled Linux helper on 2026-09-10. The journalctl/cursor-file proposals in items 2 and 4
below are retained as review history and superseded by [ADR 010](../../docs/adr/010-optional-linux-journal-helper.md):
native exact-cursor testing, private pipes, no staging files, and caller-accessible journal-view semantics. The current
spec/ports are authoritative. Native field caps replace journal-line caps; byte, row and deadline bounds still apply.

1. **Windows hard deadlines:** use one hidden fixed helper process per enabled source (maximum two), executing the
   same verified observer binary in a code-owned internal mode. Pass the private checkpoint through bounded stdin,
   return a closed metadata batch through bounded stdout, discard stderr. The parent enforces two seconds and
   kills/waits on timeout; incomplete responses never commit. The child pins the query to its creating OS thread and
   orders cancellation, native-call completion and handle closure correctly. It has no SQLite write role or generic
   command/argument surface. This avoids pretending an in-process context instantly cancels WEVTAPI.
2. **Linux baseline:** require journalctl 242+ and a fixed private per-attempt cursor file, not the opaque cursor value
   in process arguments. The fixed `--cursor-file=<code-owned-path>` option necessarily exposes that non-observed
   temporary path in argv. Never use that temporary file as durable progress: ignore its final cursor, remove it after
   the attempt and atomically persist only the last accepted/fully examined record cursor in our database.
3. **Atomic progress:** the batch carries expected checkpoint revision, start/finish times, normalized events, next
   checkpoint, examined/discarded counts, deferred/caught-up flags and stable state/reason, not coverage intervals.
   Store derives coverage and commits it with event-time rollups, checkpoint and incremented revision in one
   compare-and-swap transaction. An ambiguous outcome
   requires rereading the revision; no blind repeat increment.
4. **Recoverable reset without counts:** an initial empty checkpoint may capture the last five minutes and may remain
   cursorless when it returns no rows. After an invalid/stale checkpoint, use a fixed metadata-only tail probe outside
   that time window. Windows queries the fixed channel with `EvtQueryReverseDirection`, calls `EvtNext` for at most one
   event, renders only the selected properties and creates its bookmark. Linux uses fixed `journalctl -n 1` selected
   JSON fields and accepts the automatic `__CURSOR`; it never requests MESSAGE. The probe obeys the ordinary two-second,
   byte, checkpoint and field bounds, contributes no captured or discarded counts or covered-through advancement, and
   CAS-commits the new cursor with `CHECKPOINT_RESET` and a bounded attempt-window gap, without estimating lost events.
   Any cursorless checkpoint without reset-pending remains a normal five-minute read regardless of revision.
   An empty source clears the stale cursor
   and remains `CHECKPOINT_RESET_PENDING`/cursorless; later cycles repeat the bounded probe until one tail record proves a cursor,
   and the cycle after that resumes normal after-cursor reads. Rebuilding skipped historical counts needs a later
   deduplication design.
5. **Coverage is not a count:** retain the latest committed `PreviousAttemptAt` and covered-through watermark, not one
   row per attempt. Only a successful caught-up
   read advances coverage through its query start time. Histogram events use event timestamps. Backlog, timeout,
   committed permission failure and absent polls remain partial/gaps; a successful caught-up empty read can be zero.
   First success proves five minutes, later success at most the final 60 seconds since the prior attempt; older missed
   time is GAP. Failed/reset attempts prove no coverage. Clamp starts to seven days. Explicit gaps are sticky, while
   unknown cells may resolve with proof. Storage failure overlays volatile status only and cannot invent durable gaps.
6. **Intersecting limits:** four KiB is a per-line maximum, not a promise that 513 maximum-sized lines fit into two MiB.
   A stream byte cap may end sooner; only fully examined complete rows can contribute to a trustworthy completed batch.
   Preserve the last complete cursor, disclose partial/deferred status and reap the reader. Accepted plus discarded
   records total at most 512; a 513th examined record is only a deferred sentinel and must be read again, not discarded.
7. **Counter meanings:** captured means normalized events committed; discarded means known examined rows intentionally
   skipped, such as invalid or outside retention. Deferred lookahead is not loss; ring eviction and retention expiry
   are not source discards. Unknown loss is a gap, never an invented numeric estimate.
8. **State matrix:** disabled = DISABLED/NOT_RUN; unsupported = UNSUPPORTED/NOT_RUN; initial known permission denial
   follows the existing generic PERMISSION_DENIED/NOT_RUN contract. Mixed sources aggregate SUPPORTED/PARTIAL. Later
   failure with a retained ring keeps last-success observed time and explicit stale quality; summary latest attempt
   remains separate from historical data. Do not force a current generic status to encode every summary fact.
9. **Preset validation:** application is Windows-only and rejected on Linux. macOS never runs a log reader; any displayed
   source request is explicitly unsupported. No automatic aliasing, source substitution or environment enablement.
10. **Independent cadence and compact persistence:** collect immediately and every 60 seconds in a separate single-flight
    acquisition lane outside the 15-second host snapshot. Brief bounded shared-writer contention is expected. Persist
    minute source/severity rollups, coalesced coverage and latest attempt/checkpoint state, not one row per poll. All use
    the existing writer, retention and WAL budget. Join log work before the host scheduler closes the borrowed store.
11. **Current versus history:** one 200-record ring is recent process-memory/session state. Current `total_count` is
    exactly the ring length, `returned_count=min(log_limit,total_count)`, and truncation means the limit is below that
    length. It is valid for a restarted process to show persisted summary history and an empty current ring.
12. **Wire truthfulness:** grids are UTC-epoch aligned and include every configured source. FULL/PARTIAL buckets with
    positive coverage always have numeric counts, including proven zero. GAP/UNKNOWN counts are positive-known or null,
    never invented zero; null requires no known records and no coverage evidence. Attribute invalid-timestamp discards
    to attempt time. Latest failure cannot erase history; a mixed-source aggregate with any known coverage is PARTIAL;
    `coverage_through` is only a caught-up watermark. Reject values above 9,007,199,254,740,991 atomically.
13. **Additive storage and rollback:** keep SQLite `user_version=2`, add backward-compatible log tables, and record a
    dedicated log-metadata schema version in `store_metadata`. Before rolling back, disable sources with the new binary,
    stop it, and never let old/new processes share the state directory. The previous preview ignores additive tables;
    it does not prune them, so rollback is a temporary compatibility path rather than an indefinite operating mode.
14. **Closed native vocabulary:** source is exactly `system` or Windows-only `application`; normalized event code is
    exactly `WIN_<uint32>`, `WIN_<32-lowercase-hex-provider-guid>_<uint32>`,
    `SYSTEMD_<32-lowercase-hex-message-id>`, or `SYSTEMD_PRIORITY_<0..7>`, at most 64 bytes. Provider names and all
    unrecognized fields are rejected rather than projected.

These decisions intentionally prefer disclosed gaps over duplicate/invented counts. Tests must cover actual child
timeout/reaping and protocol limits, journal cursor-file cleanup/argv privacy, source/native field allowlists, stale to
tail-proof to normal-read recovery, repeated empty reset-pending probes, zero-count reset commits, storage CAS/retries,
and both zero and unknown coverage. No real host log bodies are needed for verification.
