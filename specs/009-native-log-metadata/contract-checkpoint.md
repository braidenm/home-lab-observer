# Spec 009 contract checkpoint: native log summary

Status: Accepted design for executable contract validation. This document does not enable a source.

## Local endpoint

`GET|HEAD /api/v1/logs/summary?range=1h|6h|24h|7d` is authenticated, read-only, and capped at 262144 bytes. `range` is
required exactly once; every other query key, duplicate value, body, or unsupported method is rejected. `HEAD` has
the same status and headers as `GET` and no body. A request reads stored/cache views only and never invokes a native
reader. Standard Problem Details responses and local bearer handling remain unchanged.

Ranges use half-open `[window_start, window_end)` UTC windows. `window_end` is the greatest UTC epoch bucket boundary
not after `generated_at`; `window_start` is exactly one requested range earlier. Storage rollups remain one minute;
the response aggregates them into this fixed presentation grid:

| range | interval | expected buckets per configured source |
| --- | ---: | ---: |
| `1h` | 60 seconds | 60 |
| `6h` | 300 seconds | 72 |
| `24h` | 900 seconds | 96 |
| `7d` | 3600 seconds | 168 |

## Closed v1 wire shape

All timestamps are UTC RFC 3339 values ending in `Z`. All counts are non-negative integers no greater than
`Number.MAX_SAFE_INTEGER` (9007199254740991); acquisition and persistence reject an overflowing increment before
commit rather than wrap or project an imprecise number. Reasons are code-owned `^[A-Z0-9_]{1,64}$` values; clients
format them as labels and never treat them as log text.
The following structural illustration shortens one 72-bucket array to two entries; executable fixtures must contain
the exact bucket count required below.

```json
{
  "schema_version": "observer-log-summary/v1",
  "generated_at": "2026-09-09T20:05:17Z",
  "range": "6h",
  "window_start": "2026-09-09T14:05:00Z",
  "window_end": "2026-09-09T20:05:00Z",
  "bucket_interval_seconds": 300,
  "expected_bucket_count": 72,
  "support_state": "SUPPORTED",
  "collection_state": "PARTIAL",
  "freshness": "CURRENT",
  "observed_at": "2026-09-09T20:04:02Z",
  "reason_code": "SOURCE_PARTIAL",
  "coverage_state": "PARTIAL",
  "counts": { "captured": 17, "discarded": 1 },
  "limits": { "max_sources": 2, "max_buckets_per_source": 168, "max_response_bytes": 262144 },
  "privacy": {
    "data_classification": "LOCAL_SENSITIVE",
    "contains_log_bodies": false,
    "contains_event_codes": false,
    "contains_identity_fields": false,
    "remote_upload_eligible": false
  },
  "sources": [
    {
      "source": "system",
      "status": {
        "support_state": "SUPPORTED",
        "collection_state": "PARTIAL",
        "freshness": "CURRENT",
        "observed_at": "2026-09-09T20:04:02Z",
        "attempted_at": "2026-09-09T20:04:02Z",
        "coverage_through": "2026-09-09T20:03:00Z",
        "reason_code": "BACKLOG_DEFERRED"
      },
      "coverage_state": "PARTIAL",
      "covered_seconds": 21000,
      "counts": { "captured": 17, "discarded": 1 },
      "buckets": [
        {
          "at": "2026-09-09T14:05:00Z",
          "coverage_state": "FULL",
          "covered_seconds": 300,
          "reason_code": null,
          "counts": {
            "captured": 2,
            "discarded": 0,
            "severity": { "trace": 0, "debug": 0, "info": 1, "warn": 1, "error": 0, "critical": 0, "unknown": 0 }
          }
        },
        {
          "at": "2026-09-09T14:10:00Z",
          "coverage_state": "GAP",
          "covered_seconds": 0,
          "reason_code": "CHECKPOINT_RESET",
          "counts": {
            "captured": 1,
            "discarded": 0,
            "severity": { "trace": 0, "debug": 0, "info": 0, "warn": 1, "error": 0, "critical": 0, "unknown": 0 }
          }
        }
      ]
    }
  ]
}
```

The only source aliases are `system` and `application`; sources appear in that order with no duplicates. `sources`
contains only explicitly configured presets (maximum two). `application` is Windows-only and rejected in Linux
configuration. macOS performs no native call and reports a configured source as `UNSUPPORTED/NOT_RUN`.
Linux reports only the caller-accessible local system-journal view. A missing helper/runtime/library or no visible
initial journal evidence is unavailable, not a healthy empty source; inaccessible files cannot be claimed covered.

Top-level support, collection, freshness, observed time, and reason use the existing generic enums. With no configured
source they are `DISABLED/NOT_RUN/UNKNOWN/null/LOG_SOURCES_DISABLED`, `sources` is empty, `coverage_state` is `UNKNOWN`,
and `counts` is null. Mixed source outcomes aggregate as `SUPPORTED/PARTIAL`; retained useful data after a later failure
is `STALE` and keeps its last-success `observed_at`. `attempted_at` describes the latest attempt independently. The
`coverage_through` value is the query-start watermark from the latest successful caught-up read, not a claim that all
earlier time is contiguous; bucket coverage remains authoritative for historical gaps. Either timestamp is null when
absent.

`observed_at` is the query start of the latest committed attempt that captured at least one event or explicitly proved
caught-up, including a caught-up empty read; it is not an event timestamp. A definite persistence-write failure cannot
durably advance attempt or coverage state. When stored history remains readable, `Collector.Summary` overlays only the
matching source's process-current latest status with collection `FAILED`, reason `LOG_STORAGE_UNAVAILABLE`, and
`attempted_at` equal to that failed query start. Its support candidate is the validated attempted batch's proven
support, otherwise prior cached support, otherwise `UNAVAILABLE`; it is never promoted to `SUPPORTED`. If that candidate
is `SUPPORTED`, the overlay is `SUPPORTED/FAILED`. If it is `DISABLED`, `UNAVAILABLE`, `PERMISSION_DENIED` or
`UNSUPPORTED`, the overlay is `UNAVAILABLE/FAILED`: the existing current-snapshot and metric-series schemas reserve all
non-`SUPPORTED` source states for `NOT_RUN/UNKNOWN`, so the new summary schema uses this explicit summary-only storage
failure combination rather than weakening those contracts or emitting invalid `UNSUPPORTED/FAILED`. The current
snapshot retains its existing state rules.

The overlay retains prior `observed_at`/`coverage_through`, uses `STALE` when a prior success exists and `UNKNOWN`
otherwise, and recomputes top-level latest quality from those overlaid statuses. It never changes stored buckets,
counts, coverage state/seconds, or creates a gap/previous-attempt value. It is explicitly volatile and may disappear
after restart; the next successful commit derives any retained-window gap from the last durable attempt. If stored
history itself is unreadable, the endpoint returns its fixed unavailable Problem rather than synthesizing a summary.

Coverage states have exact meanings:

- `FULL`: every second of the interval is covered; `covered_seconds` equals the interval, counts are present (including
  a proven zero when empty), and reason is null.
- `PARTIAL`: some but not all seconds are covered; `covered_seconds` is between 1 and interval minus 1, counts are
  present (including a proven zero for the covered portion), do not claim completeness for the rest, and reason is
  non-null.
- `GAP`: a code-owned known gap exists (for example timeout, missed poll, or checkpoint reset); covered seconds are 0,
  reason is non-null, and independently known captured/discarded counts may still be present.
- `UNKNOWN`: no trustworthy coverage evidence exists; covered seconds are 0, reason is non-null, and independently
  known captured/discarded counts may still be present.

Historical reason selection is a closed deterministic precedence, highest first:

1. `CHECKPOINT_RESET`
2. `PERMISSION_DENIED`
3. `DEADLINE_EXCEEDED`
4. `INVALID_RESPONSE`
5. `RESPONSE_TOO_LARGE`
6. `READER_FAILED`
7. `BACKLOG_DEFERRED`
8. `MISSED_COLLECTION`
9. `NOT_YET_OBSERVED`

The first eight retain their established code meanings. `NOT_YET_OBSERVED` is the new exact fallback meaning only that
the fixed historical cell has neither positive coverage nor persisted gap evidence. For a `GAP` or `PARTIAL` bucket
with multiple persisted gap reasons, select the first present code in this list. A full bucket has null reason. An
otherwise unknown bucket uses `NOT_YET_OBSERVED`. There is no lexical or arbitrary-string fallback, and a later
positive interval never displaces an overlapping persisted gap reason. This ordering applies to historical bucket
reduction only; latest source/top-level status reasons continue to follow the fixed state aggregation matrix.
`LOG_STORAGE_UNAVAILABLE` is the new status-only code for the volatile overlay described above; it is never persisted
as a historical gap reason.

Every configured source always returns exactly `expected_bucket_count` ascending epoch-aligned buckets, including an
unsupported or permission-denied latest state. A source with no historical evidence returns all `UNKNOWN` buckets with
null counts; the UI may hide its empty histogram but must show its status. A later failure never removes retained
historical bucket counts or coverage. Source coverage is `FULL` only when every bucket is full, `UNKNOWN` when all are
unknown, `GAP` when no seconds are covered and at least one bucket is a gap, and `PARTIAL` otherwise. Thus a mixture of
full/partial/gap/unknown buckets aggregates to `PARTIAL` whenever it has some but not full coverage. Top-level coverage
uses the same aggregation across configured sources.

Counts and coverage are orthogonal. A bucket count is present whenever at least one committed captured/discarded record
is known, regardless of `FULL/PARTIAL/GAP/UNKNOWN`. It is also present as all zeroes when positive coverage proves an
empty observed portion. It is null only when the bucket has no known record and zero coverage. Therefore numeric zero
never substitutes for unknown, while a gap does not hide a captured backlog event.

For every non-null bucket count, `captured` equals the sum of the seven severity values. Source counts equal the sums
of non-null buckets in the requested window and are null only when every bucket count is null; top-level counts use the
same rule across sources. Captured observations are bucketed by event time. A discarded row with a valid event time is
bucketed by event time; one whose timestamp is missing or invalid is bucketed by its attempt query-start time. Discarded
source/window totals therefore describe known rows attributed to the requested window, not exclusively event-time
records. Captured means normalized committed observations. Discarded means known examined rows intentionally skipped
in a committed batch. Deferred lookahead, retention expiry, ring eviction, failed/uncommitted attempts, and unknown
loss are not discarded.

## Transport-neutral UI DTO

```ts
type LogSource = "system" | "application";
type LogCoverageState = "FULL" | "PARTIAL" | "GAP" | "UNKNOWN";
type LogCounts = { captured: number; discarded: number } | null;
type LogSeverityCounts = {
  trace: number; debug: number; info: number; warn: number;
  error: number; critical: number; unknown: number;
};
type LogSummaryBucket = {
  at: string; coverageState: LogCoverageState; coveredSeconds: number; reasonCode: string | null;
  counts: ({ captured: number; discarded: number; severity: LogSeverityCounts }) | null;
};
type LogSourceSummary = {
  source: LogSource;
  status: SectionQuality & { attemptedAt: string | null; coverageThrough: string | null };
  coverageState: LogCoverageState;
  coveredSeconds: number;
  counts: LogCounts;
  buckets: LogSummaryBucket[];
};
interface LogSummary extends SectionQuality {
  schemaVersion: "observer-log-summary/v1";
  generatedAt: string;
  range: TrendRange;
  windowStart: string;
  windowEnd: string;
  bucketIntervalSeconds: 60 | 300 | 900 | 3600;
  expectedBucketCount: 60 | 72 | 96 | 168;
  coverageState: LogCoverageState;
  counts: LogCounts;
  limits: { maxSources: 2; maxBucketsPerSource: 168; maxResponseBytes: 262144 };
  privacy: {
    dataClassification: "LOCAL_SENSITIVE";
    containsLogBodies: false;
    containsEventCodes: false;
    containsIdentityFields: false;
    remoteUploadEligible: false;
  };
  sources: LogSourceSummary[];
}
```

Add optional `getLogSummary?(range: TrendRange, signal?: AbortSignal): Promise<LogSummary>` to `ObserverDataSource`.
The producer schema remains closed for privacy. The client strictly validates known fields and invariants but ignores
and drops additive unknown response fields, matching the existing compatibility policy. A data source without this
method retains the current Logs view without making a summary request.

## Safe current-log projection

The existing current-snapshot schema does not change. Native records use source `system` or `application`, newest
first, and always use `{ "state": "OMITTED" }`, even when `include_log_bodies=true`. Severities are mapped exactly:

- systemd priorities 0-2 `CRITICAL`, 3 `ERROR`, 4 `WARN`, 5-6 `INFO`, 7 `DEBUG`;
- Windows levels 1 `CRITICAL`, 2 `ERROR`, 3 `WARN`, 4 `INFO`, 5 `TRACE`, 0/unknown `UNKNOWN`.

Event codes are only `WIN_<uint32>`, `WIN_<32-lowercase-hex-provider-guid>_<uint32>`,
`SYSTEMD_<32-lowercase-hex-message-id>`, or the fixed fallback `SYSTEMD_PRIORITY_<0..7>`; all are at most 64 bytes.
No provider name, message, identity, path, PID, unit, executable, hash, arbitrary attribute, or native payload crosses
the projection.

`logs.total_count` is the bounded current-process/session memory-ring size (0..200), not a seven-day summary count.
`returned_count` is `min(log_limit,total_count)`, items are newest first, and `truncated` is exactly
`returned_count < total_count`; `log_limit=0` is valid. Later source failure may retain explicitly stale ring records.
The UI labels this section “Recent memory/session events” and labels the existing count strip “Current returned
snapshot” so it cannot be confused with summary-window totals. Legacy Platform Demo snapshots and generic log records
remain valid; native production is narrower than the existing bounded generic schema.

## UI presentation and acceptance boundary

Load summary independently from current observations. Render aggregate captured/discarded cards (use “Unavailable”,
not zero, for null), then at most two source panels with status, last attempt/coverage-through, and a keyboard-readable
stacked severity histogram. Gap/unknown buckets remain visually distinct and expose time, coverage, reason and safe
known counts in accessible text; a positive bar may coexist with a gap overlay because counts do not prove coverage.
Hide an unsupported source's all-unknown/null chart while retaining its status. Source panels stack at 390 px and may
form two columns at 768/1440 px; legends wrap.

Summary loading, disabled, unsupported, permission-denied, empty-covered, partial, gap, stale,
and request-error states must not blank or modify the existing source/severity/event-code filters and context list.
No body drawer, raw JSON, live tail, arbitrary query/facet, install action, or cross-platform parity claim is added.
Fixtures and tests include additive unknown metadata, malformed known fields, safe-integer overflow, inconsistent
count/coverage invariants, all-null gaps, positive-count gaps, zero-with-full-coverage, latest-failure history retention,
pairwise historical-reason precedence, volatile storage-failure status without history mutation, caught-up empty
success, ambiguous same-batch retry without a second read, secret/body/path canaries, optional-method fallback,
keyboard access, and 390/768/1440 layouts.
An explicit regression proves an unsupported attempted batch followed by store failure is never rendered or projected
as `SUPPORTED`; summary uses `UNAVAILABLE/FAILED/LOG_STORAGE_UNAVAILABLE`, while legacy current-snapshot state remains
schema-compatible.

Collection remains scheduler-owned at an independent 60-second cadence with an immediate first attempt; those native,
checkpoint-CAS, coalesced-coverage, fixed-minute-rollup, retention, and process-reaping contracts stay outside this DTO.
The producer marks an attempt caught-up only after explicit exhausted/current-source proof with no deferred lookahead,
byte/line/row truncation, deadline, cancellation, or malformed protocol. Known discarded rows do not prevent caught-up
when the complete selected source was examined.
After an ambiguous store result, an unchanged checkpoint revision permits one resubmission of the exact immutable
validated batch—same kind, timestamps, cursor and counts. It never invokes the native reader again or substitutes a new
batch; an incremented revision means the original applied, and any other revision is a conflict.
