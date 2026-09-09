# Native log observations without acquiring message bodies

Research date: 2026-09-09. Decision preparation for the log-observation slice, after Spec 008.
Versions: Go 1.27/CGO-disabled native builds, Windows Event Log WEVTAPI, systemd journalctl 242+; macOS OSLogStore.

## Question and current implementation

How can the dashboard correlate errors with resource pressure without becoming an unrestricted log reader?
ADR 003 makes log bodies disabled by default and retention bounded. The current snapshot contract already has a logs
section, but `internal/projection` emits an unavailable empty section and there is no native reader. Metric history
is a seven-day/250 MiB SQLite store. The current API must remain read-only: requests read a cached view, not launch
native collection. Spec 008's own diagnostic log is a separate code-owned product health channel.

## Alternatives

| Option | Benefit | Cost / privacy concern | Fit |
| --- | --- | --- | --- |
| Leave logs unsupported | No new local authority or runtime complexity | Cannot explain error bursts | Safe fallback, insufficient as the next rich-data slice |
| Fixed-source metadata readers | Severity/time/event codes without message text | Native adapters, incomplete OS parity | Recommended first slice |
| Native CLI full messages then redact | Easy prototype with familiar tools | Acquires sensitive bodies and depends on perfect redaction | Reject for this slice |
| Full log stack / remote search | Rich query and aggregation ecosystem | Extra services, unrestricted ingestion, tenancy/retention cost | Separate product, not a single-machine preview default |

## Platform evidence and recommended adapters

**Windows:** use WEVTAPI directly through the existing Windows Go support. Microsoft's
[rendering guidance](https://learn.microsoft.com/en-us/windows/win32/wes/rendering-events) supports selecting individual
properties with a render context rather than formatting messages or full XML. Request only time, severity, event ID,
record cursor and optional provider GUID. Fixed System/Application channel presets; no Security channel, arbitrary
XPath or remote computer. [Bookmarks](https://learn.microsoft.com/en-us/windows/win32/wes/bookmarking-events) support
continuation. The adapter must cancel, wait and close handles safely. Do not call EvtFormatMessage, render EventData,
or copy Microsoft's unrestricted display example verbatim. Explicit unknown/permission states replace guessed zeros.
The [EvtQuery reference](https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtquery) requires query
handle use on the creating thread, so a Go adapter must pin its native operation to one OS thread.
[EvtCancel](https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtcancel) permits cancellation from
another thread but requires waiting before handle closure and does not promise instantaneous cancellation. Do not
claim a hard native deadline merely because a Go context expires. The reviewed design uses a fixed same-executable
helper process with bounded private stdin/metadata stdout; the parent kills and waits at the deadline. The helper has
no storage write role or arbitrary command surface.

**Linux:** execute a fixed approved absolute `journalctl` path directly, without a shell or user-selected command.
The [journalctl v255 manual](https://www.freedesktop.org/software/systemd/man/255/journalctl.html) describes bounded
JSON output and `--output-fields` (available since 236). Request severity and message ID only; cursor/time are
addressing fields and the normalized model drops unrelated automatic fields. Never request MESSAGE, environment,
unit/executable names or user/host identity. A controlled environment, output byte/line limits and a total deadline
are required even with bounded row count. Reject binary/array/malformed fields. Missing manager, version, approved
binary or access is explicit unavailable/unsupported; non-systemd and custom layouts are an initial gap. Require 242+
for `--cursor-file`: pass the opaque cursor value through a private per-attempt file rather than placing the value in
command arguments. The fixed option still exposes the code-owned temporary path in argv; that path contains no observed
data and grants no arbitrary-file selection.
That file is temporary, not the durable checkpoint; only our atomic database transaction advances durable progress.
The [journal export format](https://systemd.io/JOURNAL_EXPORT_FORMATS/) explains why fields are not necessarily simple
UTF-8 strings. One research fetch of the v255 page succeeded during independent review; a later root fetch failed.
Root subsequently verified the [v255 upstream manual source](https://github.com/systemd/systemd/blob/v255/man/journalctl.xml),
including the explicit version-242 introduction and continuation semantics for `--cursor-file`.

**macOS:** keep native logs explicitly unsupported in this slice. Apple's
[OSLogStore local access](https://developer.apple.com/documentation/oslog/oslogstore/local()) has entitlement and
privilege requirements that need a separately packaged native design. The pure-Go preview must not quietly request
sudo or spawn unrestricted `log show`. Host/process/container metrics and the local UI still work on macOS.

All sources above were consulted on 2026-09-09. Focused review independently checked Microsoft's selected-property
rendering and bookmark behavior. Apple documentation is JavaScript-heavy, so do not infer an unentitled cross-platform
implementation from a search snippet. Native test evidence is required before claiming supported log collection.

## Meaningful presentation and data policy

Use explicit source presets, default none. Run their immediate/60-second schedule in a single-flight lane independent
of the 15-second host snapshot. Current records contain UTC time, exact source alias (`system` or Windows-only
`application`), normalized severity,
event code restricted to `WIN_<uint32>`, `WIN_<32-lowercase-hex-provider-guid>_<uint32>`,
`SYSTEMD_<32-lowercase-hex-message-id>` or `SYSTEMD_PRIORITY_<0..7>`, and an OMITTED body. Store only low-cardinality
source/severity counts, collection coverage
and a private continuation checkpoint; recent event codes remain one bounded 200-record process-memory/session ring.
Persist compact minute rollups, coalesced coverage and latest attempt state rather than every poll. Do not persist
identities, raw event XML/JSON, message hashes, bodies or high-cardinality provider/event-code dimensions.

The useful pattern from [Netdata's logs view](https://github.com/netdata/netdata/blob/master/integrations/logs/metadata.yaml)
is a time histogram with source/severity facets. [Loki cardinality guidance](https://grafana.com/docs/loki/latest/get-started/labels/cardinality/)
reinforces keeping stored dimensions small. Adopt those interpretation patterns, not unrestricted raw-log search.

Show captured counts, disclosed drops, source status and gaps. Coverage and counts are independent: backlog events
remain attributed to event time even when covered seconds are zero, and known counts survive GAP/UNKNOWN coverage.
FULL/PARTIAL with positive coverage always has numeric counts, including proven zero. GAP/UNKNOWN counts are positive-
known or null, never invented zero; null requires no known records and no coverage evidence. Invalid-timestamp discards
belong to the attempt-time bucket. A successful empty poll is a real captured zero;
unsupported, permission denied, storage failure or missed coverage is not zero. A latest failure must not erase useful
historical counts or pretend the retained current ring is fresh. Each UI resource fails independently.

## Failure-mode FAQ and operational proof

- **Can a UI parameter enable logs?** No. Only explicit local startup configuration enables fixed source presets;
  requests never add sources, execute queries, select files or acquire bodies.
- **Can retries double-count?** Commit rollup and checkpoint atomically. Storage failure does not advance the cursor;
  restart resumes the last committed cursor. An invalid/stale checkpoint discloses a gap and stays unchanged unless a
  native, documented metadata-only mechanism can prove a new tail cursor; a bounded time window is not such proof.
- **What if the OS produces too much?** Cap source work, accepted rows, bytes, time and in-memory records; expose drops
  and partial coverage, not a claim of total machine events. Native cancellation must reap processes/close handles.
- **What is retained?** Seven days within the shared store budget. Bodies never enter this slice. Apply the same
  privacy tests to SQLite/WAL, API, UI, diagnostics and any future uploader.
- **How is this tested safely?** Synthetic native adapters with secret canaries prove only allowlisted fields can cross
  the boundary. Do not dump the developer's real host logs. Native CI checks supported/permission/unavailable outcomes
  without printing raw content. Test malformed rows, timeouts, overflow, checkpoint replay, transactional failure,
  deep-cloned cache, fixed coverage states, nullable count objects and responsive histogram/filter behavior.
- **How do I roll back?** Keep core SQLite `user_version=2` and version additive log tables through `store_metadata`.
  Disable sources with the new binary and stop it before starting the previous preview. The previous preview ignores
  additive tables but does not prune them, so do not use it indefinitely or open the live file concurrently.

## Owner decisions and later scope

The safe preview can proceed without deciding a raw-body policy: bodies are omitted and remote upload is false.
An entitled macOS reader, extra channels, redacted bodies, live tail, service-specific queries and remote rollups
require new explicit source/privacy/capability specifications. None should be silently bundled into this metadata slice.
