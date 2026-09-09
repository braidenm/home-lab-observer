# Contract checkpoint decisions from independent review

Prepared on 2026-09-09 after a read-only review by `runtime_architecture`. These conservative defaults refine the
metadata-only slice; they do not enable sources or expand authority. Incorporate them into the accepted ADR and
executable contract before implementation begins, after Spec 008 merges.

1. **Windows hard deadlines:** use one hidden fixed helper process per enabled source (maximum two), executing the
   same verified observer binary in a code-owned internal mode. Pass the private checkpoint through bounded stdin,
   return a closed metadata batch through bounded stdout, discard stderr. The parent enforces two seconds and
   kills/waits on timeout; incomplete responses never commit. The child pins the query to its creating OS thread and
   orders cancellation, native-call completion and handle closure correctly. It has no SQLite write role or generic
   command/argument surface. This avoids pretending an in-process context instantly cancels WEVTAPI.
2. **Linux baseline:** require journalctl 242+ and a fixed private per-attempt cursor file, not an opaque cursor in
   process arguments. Never use that temporary file as durable progress: ignore its final cursor, remove it after the
   attempt and atomically persist only the last accepted/fully examined record cursor in our database.
3. **Atomic progress:** the batch carries expected checkpoint revision, start/finish times, normalized events, next
   checkpoint, examined/discarded counts, deferred/caught-up flags and stable state/reason. Commit event-time rollups,
   attempt coverage, checkpoint and incremented revision in one compare-and-swap transaction. An ambiguous outcome
   requires rereading the revision; no blind repeat increment.
4. **Reset without duplicate counts:** an initial empty checkpoint may capture the last five minutes. An invalid/stale
   checkpoint seeks a bounded last-five-minute window only to establish a new tail cursor, does not count reset-window
   events, and records `CHECKPOINT_RESET` plus an unknown-size coverage gap. Reconstructing lost counts needs a later
   deduplication/rebuild design. This is intentionally conservative and visible in the UI.
5. **Coverage is not a count:** store each attempt and covered-through time separately. Only a successful caught-up
   read advances coverage through its query start time. Histogram events use event timestamps. Backlog, timeout,
   permission/storage failure and absent polls remain partial/gaps; a successful caught-up empty read can be zero.
6. **Intersecting limits:** four KiB is a per-line maximum, not a promise that 513 maximum-sized lines fit into two MiB.
   A stream byte cap may end sooner; only fully examined complete rows can contribute to a trustworthy completed batch.
   Preserve the last complete cursor, disclose partial/deferred status and reap the reader. At most the first 512 of
   513 examined lookahead rows commit; the sentinel must be read again, not counted as discarded.
7. **Counter meanings:** captured means normalized events committed; discarded means known examined rows intentionally
   skipped, such as invalid or outside retention. Deferred lookahead is not loss; ring eviction and retention expiry
   are not source discards. Unknown loss is a gap, never an invented numeric estimate.
8. **State matrix:** disabled = DISABLED/NOT_RUN; unsupported = UNSUPPORTED/NOT_RUN; initial known permission denial
   follows the existing generic PERMISSION_DENIED/NOT_RUN contract. Mixed sources aggregate SUPPORTED/PARTIAL. Later
   failure with a retained ring keeps last-success observed time and explicit stale quality; summary latest attempt
   remains separate from historical data. Do not force a current generic status to encode every summary fact.
9. **Preset validation:** application is Windows-only and rejected on Linux. macOS never runs a log reader; any displayed
   source request is explicitly unsupported. No automatic aliasing, source substitution or environment enablement.

These decisions intentionally prefer disclosed gaps over duplicate/invented counts. Tests must cover actual child
timeout/reaping and protocol limits, journal cursor-file cleanup/argv privacy, source/native field allowlists, storage
CAS/retry/reset behavior, and both zero and unknown coverage. No real host log bodies are needed for verification.
