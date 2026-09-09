# Spec 009: Opt-in native log metadata and meaningful local summaries

Status: Prepared for implementation after Spec 008; no native source is enabled by this document.

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
- L3: Native work is scheduler-owned, single-flight and bounded: two sources maximum, four-second overall deadline,
  two seconds per source, 512 accepted events plus one lookahead, two MiB output per source, four KiB journal lines,
  16 KiB checkpoints and 200 recent in-memory records. Windows reads run in a fixed hidden helper process using the
  same verified executable, with bounded stdin/stdout and parent-enforced kill/wait on timeout; query handles stay on
  their creating OS thread. Linux passes checkpoints through a private per-attempt cursor file, never process argv.
  Cancellation closes/reaps all native resources. Intersecting byte/row limits can stop work before 512 events.
  Initial reads capture at most the last five minutes. A stale/invalid cursor reset uses that window only to establish
  a tail cursor, contributes no duplicate counts, and discloses an unknown-size gap.
- L4: Normalize UTC time, fixed source alias, seven code-owned severities and a validated bounded event code.
  Bodies are always `{state:"OMITTED"}`, including when a caller requests `include_log_bodies=true`. Never retain raw
  payloads, identities, paths, process IDs, unit/executable names, message hashes or arbitrary attributes. Event codes
  stay in the bounded current ring, not a high-cardinality persisted dimension.
- L5: Persist source/severity rollups, coverage and the next private cursor atomically in the existing bounded SQLite
  store using a checkpoint-revision compare-and-swap. A failed write does not advance the cursor; an ambiguous commit
  rereads the revision before retry. Resume after restart without double-counting a committed batch.
  Preserve seven-day/250 MiB shared retention, WAL/checkpoint limits and schema migration/rollback guidance. Defaults
  must not create another unbounded history store or write observed metadata into self-diagnostic logs.
- L6: Expose an additive authenticated read-only `GET/HEAD /api/v1/logs/summary?range=1h|6h|24h|7d`, at most one MiB,
  with closed `observer-log-summary/v1` schema. Fixed source/severity dimensions, captured/discarded counts, nullable
  coverage buckets and latest source status. No free-text search, arbitrary facet, source parameter or file/log body
  download. HTTP requests read cached/store views and never invoke native readers.
- L7: Summary latest support/collection/freshness/reason is separate from historical points. An empty successful read
  is a captured zero; permission failure, unsupported source, missed coverage or failed persistence is a gap. Last
  useful recent records can remain explicitly stale after failure. Counts describe captured observations, not all
  machine events. Only successful caught-up reads advance covered-through time to the query start time. Backlogs and
  missed polls remain partial/gaps. A lookahead deferred to the next cursor is not falsely counted as a dropped event;
  ring eviction and retention expiry are not source discards. Unknown loss never becomes an invented numeric estimate.
- L8: Populate the existing current log contract, newest first, and enforce `log_limit` 0..200 with truthful total,
  returned and truncated counts. Preserve current/capabilities contracts and privacy policy. Local log observations
  remain ineligible for remote upload; legacy Platform Demo snapshots are unchanged.
- L9: The optional transport-neutral summary source drives a responsive severity histogram, captured/discarded cards,
  source status and gaps, alongside the existing recent source/severity/event-code filters. Summary failure does not
  blank current observations. No body drawer, raw JSON, live tail or misleading cross-platform parity. Verify keyboard,
  contrast and layout at 390/768/1440 widths.
- L10: Deterministic native fakes and synthetic secret canaries prove acquisition boundaries, disabled-no-call behavior,
  source validation, bounds, cancellation, native handle/process cleanup, malformed/oversized records, permission and
  stale checkpoint behavior, atomic retries, retention, strict producer schemas and additive client compatibility.
  Native smoke checks report only bounded pass/capability state; never dump the developer's real host logs.

## Initial platform gaps

macOS native log metadata, non-systemd Linux/custom journalctl layouts, Windows Security channel, bodies, live tail,
service-specific readers and remote rollups require later specs. Existing host/process/container observations continue
to work where supported. Missing MESSAGE_ID uses a fixed severity-derived fallback code rather than reading a message.

See [the source research](../../docs/architecture-research/004-safe-native-log-observations.md). This is a read-only,
metadata-only feature, not a SIEM or a new command-execution capability.

The [independent-review decisions](review-decisions.md) define the batch, coverage and failure-state checkpoint to
freeze with executable contracts before implementation.
