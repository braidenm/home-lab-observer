# Spec 009: Opt-in native log metadata and meaningful local summaries

Status: Proposed; implementation starts only after Spec 008 is merged. No native source is enabled by this document.

## Outcome

An owner can correlate captured native error/warning bursts with resource trends, see which sources are available,
and inspect recent bounded event metadata without collecting message bodies. The local API and reusable UI remain
useful without Platform Demo, with honest cross-platform gaps.

## Requirements

- L1: Native logs are disabled by default. Only repeated local startup presets `--log-source system` and (Windows)
  `--log-source application` enable them. No environment auto-enable, arbitrary files/channels/units/provider names,
  XPath/predicates, executable paths, remote machines or API-driven source changes. Persist approved source enums in
  explicitly enabled background configuration; never broaden an existing configuration automatically.
- L2: Windows uses selected-property WEVTAPI reads of fixed System/Application channels. Linux uses a fixed approved
  absolute journalctl 242+ path, JSON output with only severity/message-ID fields plus cursor/time addressing fields,
  a controlled environment and no shell. Do not render full event XML, format Windows messages, request journald
  MESSAGE or acquire bodies before trying to redact. macOS returns explicit unsupported without spawning `log show`.
- L3: Native work runs in a scheduler-owned lane that is independent of the 15-second host snapshot cycle. It starts
  once immediately and then at fixed 60-second intervals, is single-flight, and never delays or changes host snapshot
  publication. It is bounded to two sources maximum, a four-second overall deadline,
  two seconds per source, 512 accepted events plus one lookahead, two MiB output per source, four KiB journal lines,
  16 KiB checkpoints and 200 recent in-memory records. Windows reads run in a fixed hidden helper process using the
  same verified executable, with bounded stdin/stdout and parent-enforced kill/wait on timeout; query handles stay on
  their creating OS thread. Linux passes the opaque checkpoint value through a private per-attempt cursor file; the
  fixed `--cursor-file=<code-owned-path>` option necessarily exposes only that non-observed temporary path in argv.
  Cancellation closes/reaps all native resources. Intersecting byte/row limits can stop work before 512 events.
  Initial reads capture at most the last five minutes. On a stale/invalid checkpoint, run one metadata-only tail probe:
  Windows queries the fixed channel in reverse order and reads at most one event to produce a bookmark; Linux runs the
  fixed selected-field journal query with `-n 1`, outside the five-minute filter, and accepts only its automatic cursor.
  The probe uses the same source deadline, byte/checkpoint limits and field allowlist, never requests a message/body,
  and contributes neither captured nor discarded counts or covered-through advancement. Atomically persist the proved
  cursor with `CHECKPOINT_RESET` and an unknown-size gap. If the source is empty, atomically clear the stale cursor and
  remain `CHECKPOINT_RESET_PENDING`/cursorless; repeat the same bounded probe until a tail is proved, then resume
  normal after-cursor collection on the following cycle.
- L4: Normalize only UTC time, exact lowercase source alias (`system` or Windows-only `application`), seven code-owned
  severities and a validated bounded event code. Map journald priority 0..2 to CRITICAL, 3 to ERROR, 4 to WARN, 5..6
  to INFO and 7 to DEBUG. Map Windows Level 1 to CRITICAL, 2 to ERROR, 3 to WARN, 4 to INFO, 5 to TRACE, and 0,
  missing or unknown values to UNKNOWN.
  Bodies are always `{state:"OMITTED"}`, including when a caller requests `include_log_bodies=true`. Never retain raw
  payloads, identities, paths, process IDs, unit/executable names, message hashes or arbitrary attributes. Event code
  is exactly `WIN_<uint32>`, `WIN_<32-lowercase-hex-provider-guid>_<uint32>`,
  `SYSTEMD_<32-lowercase-hex-message-id>`, or `SYSTEMD_PRIORITY_<0..7>`, always at most 64 bytes. Use one
  process-memory/session ring of at most 200 records across all sources; event codes stay there and never become a
  high-cardinality persisted dimension.
- L5: Persist compact minute source/severity rollups, coalesced coverage intervals, latest attempt metadata and the next
  private cursor atomically in the existing bounded SQLite
  store using a checkpoint-revision compare-and-swap. A failed write does not advance the cursor; an ambiguous commit
  rereads the revision before retry. Resume after restart without double-counting a committed batch.
  Preserve seven-day/250 MiB shared retention, WAL/checkpoint limits and the existing single writer/checkpoint owner.
  Keep `PRAGMA user_version=2`, add only backward-compatible tables, and version this feature through a dedicated
  `log_metadata_schema_version` value in existing `store_metadata`; the previous preview must ignore the additive
  tables instead of quarantining the database. Do not persist one row for every 60-second attempt. Defaults
  must not create another unbounded history store or write observed metadata into self-diagnostic logs.
- L6: Expose an additive authenticated read-only `GET/HEAD /api/v1/logs/summary?range=1h|6h|24h|7d`, at most 256 KiB,
  with closed `observer-log-summary/v1` schema. Fixed source/severity dimensions, nullable captured/discarded counts,
  non-null coverage buckets and latest source status. Every configured source always receives the fixed grid, including
  an unsupported source whose buckets are UNKNOWN with null counts. Bucket coverage is exactly FULL, PARTIAL, GAP or
  UNKNOWN; fixed grids are UTC-epoch aligned: 60 one-minute, 72 five-minute, 96 fifteen-minute or 168 hourly buckets.
  No free-text search, arbitrary facet, source parameter or file/log body download. HTTP requests read cached/store
  views and never invoke native readers.
- L7: Summary latest support/collection/freshness/reason is separate from historical points. An empty successful read
  is a captured zero; permission failure, unsupported source, missed coverage or failed persistence is a gap. Last
  useful recent records can remain explicitly stale after failure. Counts describe captured observations, not all
  machine events. Only successful caught-up reads advance covered-through time to the query start time. Backlogs and
  missed polls remain partial/gaps. A lookahead deferred to the next cursor is not falsely counted as a dropped event;
  ring eviction and retention expiry are not source discards. Invalid-timestamp discards are assigned to the attempt-
  time bucket; event-time buckets remain captured-event counts. A latest permission or collection failure never erases
  historical buckets. `coverage_through` is the latest caught-up watermark, not proof that earlier time is contiguous.
  Captured counts remain visible through GAP or UNKNOWN coverage. FULL/PARTIAL buckets with positive covered seconds
  always have numeric counts, including a proven zero. GAP/UNKNOWN counts are positive-known or null, never invented
  zero; null requires no known records and no coverage evidence. Unknown loss never becomes an invented numeric estimate.
  An aggregate with mixed source outcomes and any known coverage is PARTIAL rather than AVAILABLE or UNAVAILABLE.
  FULL covers the complete bucket and has no reason; PARTIAL covers 1..interval-1 seconds and has a reason; GAP and
  UNKNOWN cover zero seconds and have a reason. Source/window coverage is FULL only when all buckets are full, UNKNOWN
  when all are unknown, GAP when no seconds are covered and any bucket is a gap, and PARTIAL otherwise.
- L8: Populate the existing current log contract, newest first, and enforce `log_limit` 0..200 with truthful total,
  returned and truncated counts. This is explicitly a recent process-memory/session cache: `total_count` is the current
  bounded ring length, `returned_count=min(log_limit,total_count)`, and `truncated=returned_count<total_count`. Summary
  historical counts are separate; after restart the summary can contain history while the current ring is empty.
  Preserve current/capabilities contracts and privacy policy. Local log observations
  remain ineligible for remote upload; legacy Platform Demo snapshots are unchanged.
- L9: The optional transport-neutral summary source drives a responsive severity histogram, captured/discarded cards,
  source status and gaps, alongside the existing recent source/severity/event-code filters. Summary failure does not
  blank current observations. No body drawer, raw JSON, live tail or misleading cross-platform parity. Verify keyboard,
  contrast and layout at 390/768/1440 widths.
- L10: Deterministic native fakes and synthetic secret canaries prove acquisition boundaries, disabled-no-call behavior,
  source validation, bounds, cancellation, native handle/process cleanup, malformed/oversized records, permission and
  stale checkpoint behavior, atomic retries, retention, strict producer schemas and additive client compatibility.
  Native smoke checks report only bounded pass/capability state; never dump the developer's real host logs.

All counters are non-negative JSON-safe integers. Any increment, persisted value or aggregate greater than
9,007,199,254,740,991 aborts the atomic batch without advancing its checkpoint; values never wrap or silently round.
The executable contract checkpoint must encode these exact patterns rather than accept provider strings. This
specification remains proposed until Spec 008 is merged.

## Initial platform gaps

macOS native log metadata, non-systemd Linux/custom journalctl layouts, Windows Security channel, bodies, live tail,
service-specific readers and remote rollups require later specs. Existing host/process/container observations continue
to work where supported. Missing MESSAGE_ID uses a fixed severity-derived fallback code rather than reading a message.

See [the source research](../../docs/architecture-research/004-safe-native-log-observations.md). This is a read-only,
metadata-only feature, not a SIEM or a new command-execution capability.

The [independent-review decisions](review-decisions.md) define the batch, coverage and failure-state checkpoint to
freeze with executable contracts before implementation.
