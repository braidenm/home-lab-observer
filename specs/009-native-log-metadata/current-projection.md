# Spec 009 current-log projection

Status: Frozen implementation contract for the current-snapshot and capabilities slice.

## Boundary

`internal/localapi` may receive an optional cache-only log source:

```go
type LogSource interface {
	Current() logobs.Snapshot
}
```

The current and capabilities handlers may call only `Current`. They never call a native reader or `logobs.Store`, and
the current handler does not call the source when `logs` was not selected. The caller supplies the same collector used
by the log-summary endpoint. Projection never mutates the host snapshot or the returned log snapshot.

## Current-snapshot projection

`projection.WithLogs` maps a validated `logobs.Snapshot` to the existing `observer-current-snapshot/v1` `logs`
section. The list is newest first and remains bounded by the current-process ring:

- `total_count` is the validated ring length, never a persisted historical count;
- `returned_count` is `min(log_limit, total_count)`, where `log_limit` is `0..200` and defaults to `50`;
- `truncated` is exactly `returned_count < total_count`;
- metadata is limited to `observed_at`, the closed `system|application` source, closed severity, and normalized
  `event_code`; and
- every body is exactly `{ "state": "OMITTED" }`, including when `include_log_bodies=true`.

The query flag remains additive compatibility syntax but grants no collection or disclosure authority in this slice.
No checkpoint, message, provider, identity, path, PID, unit, native payload, arbitrary attribute, or error text reaches
the projection. An invalid cache snapshot is replaced by an empty `UNAVAILABLE/NOT_RUN/UNKNOWN/INVALID_RESPONSE`
section.

With no configured `LogSource`, the existing empty
`UNSUPPORTED/NOT_RUN/UNKNOWN/COLLECTOR_NOT_IMPLEMENTED` section remains. A configured collector projects its exact
code-owned status, including `DISABLED/LOG_SOURCES_DISABLED`, `UNSUPPORTED/PLATFORM_UNSUPPORTED`, permission, helper,
reader, reset, partial, and storage outcomes. Capabilities use the same cache status and never hard-code support when a
source exists.

## Freshness and retained records

Log collection runs every 60 seconds. The current endpoint independently marks log evidence stale only after 120
seconds (two full collection intervals) from the log status observation time. Host and observer sections retain their
existing 45-second rule. Aging a successful log status changes only `freshness` to `STALE`; it does not invent a new
reason code, and a nil success reason remains nil.

A later failed or permission-denied observation may keep the last validated ring. When records are retained, the log
section is explicitly `STALE`, and `observed_at` is the newest retained record's time when the latest source status has
no observation time. Support, collection and reason continue to describe the latest attempt; they are never promoted
to imply success. A first `UNAVAILABLE/FAILED/UNKNOWN` attempt is valid and empty. An `UNAVAILABLE/FAILED/STALE`
status may also retain a prior successful observation time while the ring is empty; this is truthful evidence of a
previous caught-up empty cycle, not an invented record count.

This is a narrow additive exception to the v1 generic list rule. The current-snapshot schema and UI parser permit
retained non-supported data only for the `logs` section when freshness is `STALE`, `observed_at` is non-null,
`total_count` proves a non-empty ring, and a code-owned reason is present. `log_limit=0` may intentionally return no
items while retaining that bounded count. The same exception permits an empty `UNAVAILABLE/FAILED/STALE` section
whose non-null observation time came from a prior healthy empty cycle, and an empty first
`UNAVAILABLE/FAILED/UNKNOWN` attempt. Filesystem, process, service, container and observer sections remain unchanged. Producer,
direct-schema and client-parser tests must cover both the allowed retained case and rejected UNKNOWN/non-supported,
extra-field and inconsistent-count cases before merge.

The top-level `snapshot_id`, `sequence`, `observed_at` and `duration_ms` continue to describe the host snapshot. Log
projection does not rewrite them. After section selection and freshness aging, the top-level collection state is
recomputed from the emitted section statuses.
