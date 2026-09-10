import type {
  CollectionState,
  Freshness,
  LogCounts,
  LogCoverageState,
  LogSeverityCounts,
  LogSource,
  LogSourceSummary,
  LogSummary,
  LogSummaryBucket,
  SectionQuality,
  SupportState,
  TrendRange
} from "../types";

const MAX_SAFE_COUNT = Number.MAX_SAFE_INTEGER;
const ranges: Record<TrendRange, { durationSeconds: number; intervalSeconds: 60 | 300 | 900 | 3600; bucketCount: 60 | 72 | 96 | 168 }> = {
  "1h": { durationSeconds: 3600, intervalSeconds: 60, bucketCount: 60 },
  "6h": { durationSeconds: 21600, intervalSeconds: 300, bucketCount: 72 },
  "24h": { durationSeconds: 86400, intervalSeconds: 900, bucketCount: 96 },
  "7d": { durationSeconds: 604800, intervalSeconds: 3600, bucketCount: 168 }
};
const severityKeys = ["trace", "debug", "info", "warn", "error", "critical", "unknown"] as const;
const historicalReasons = ["CHECKPOINT_RESET", "PERMISSION_DENIED", "DEADLINE_EXCEEDED", "INVALID_RESPONSE", "RESPONSE_TOO_LARGE", "READER_FAILED", "BACKLOG_DEFERRED", "MISSED_COLLECTION"] as const;
const sourceReasons = new Set([
  ...historicalReasons, "NO_VISIBLE_JOURNAL", "LOG_HELPER_UNAVAILABLE", "LOG_HELPER_MISMATCH",
  "LOG_STORAGE_UNAVAILABLE", "PLATFORM_UNSUPPORTED"
]);

/**
 * Projects only known v1 fields. Producers remain closed, while additive
 * response metadata is intentionally ignored and never retained or rendered.
 */
export function mapLogSummary(value: unknown): LogSummary {
  const root = object(value);
  const range = logRange(root.range);
  const rangeConfig = ranges[range];
  const generatedAt = timestamp(root.generated_at, "generated_at");
  const generatedNanos = timestampNanos(generatedAt);
  const windowStart = wholeSecondTimestamp(root.window_start, "window_start");
  const windowEnd = wholeSecondTimestamp(root.window_end, "window_end");
  const startMillis = Date.parse(windowStart);
  const endMillis = Date.parse(windowEnd);
  const endNanos = timestampNanos(windowEnd);
  const intervalNanos = BigInt(rangeConfig.intervalSeconds) * 1_000_000_000n;
  if (endMillis - startMillis !== rangeConfig.durationSeconds * 1000 ||
      endNanos > generatedNanos || endNanos !== floorDiv(generatedNanos, intervalNanos) * intervalNanos) {
    throw new Error("log summary window does not match its range and generated time");
  }

  const sources = boundedArray(root.sources, 0, 2).map((entry) => mapSource(entry, startMillis, rangeConfig.intervalSeconds, rangeConfig.bucketCount, generatedNanos));
  let previousSourceRank = -1;
  for (const source of sources) {
    const sourceRank = source.source === "system" ? 0 : 1;
    if (sourceRank <= previousSourceRank) throw new Error("log sources are duplicated or outside fixed order");
    previousSourceRank = sourceRank;
  }
  const quality = mapQuality(root);
  const aggregate = aggregateQuality(sources);
  if (!sameQuality(quality, aggregate)) throw new Error("aggregate log quality is inconsistent");

  const counts = nullableCounts(root.counts);
  const expectedCounts = sumNullableCounts(sources.map((source) => source.counts));
  if (!sameCounts(counts, expectedCounts)) throw new Error("aggregate log counts are inconsistent");
  const coverageState = coverage(root.coverage_state);
  if (coverageState !== aggregateCoverage(sources.map((source) => source.coverageState), sources.reduce((total, source) => safeAdd(total, source.coveredSeconds), 0))) {
    throw new Error("aggregate log coverage is inconsistent");
  }

  const limits = object(root.limits);
  const privacy = object(root.privacy);
  return {
    schemaVersion: literal(root.schema_version, "observer-log-summary/v1"),
    generatedAt,
    range,
    windowStart,
    windowEnd,
    bucketIntervalSeconds: literal(root.bucket_interval_seconds, rangeConfig.intervalSeconds),
    expectedBucketCount: literal(root.expected_bucket_count, rangeConfig.bucketCount),
    ...quality,
    coverageState,
    counts,
    limits: {
      maxSources: literal(limits.max_sources, 2),
      maxBucketsPerSource: literal(limits.max_buckets_per_source, 168),
      maxResponseBytes: literal(limits.max_response_bytes, 262144)
    },
    privacy: {
      dataClassification: literal(privacy.data_classification, "LOCAL_SENSITIVE"),
      containsLogBodies: literal(privacy.contains_log_bodies, false),
      containsEventCodes: literal(privacy.contains_event_codes, false),
      containsIdentityFields: literal(privacy.contains_identity_fields, false),
      remoteUploadEligible: literal(privacy.remote_upload_eligible, false)
    },
    sources
  };
}

function mapSource(value: unknown, windowStartMillis: number, intervalSeconds: number, bucketCount: number, generatedNanos: bigint): LogSourceSummary {
  const item = object(value);
  const source = logSource(item.source);
  const statusItem = object(item.status);
  const status = {
    ...mapQuality(statusItem),
    attemptedAt: nullableTimestamp(statusItem.attempted_at, "attempted_at"),
    coverageThrough: nullableTimestamp(statusItem.coverage_through, "coverage_through")
  };
  validateSourceStatus(status, generatedNanos);
  const buckets = boundedArray(item.buckets, bucketCount, bucketCount).map((entry, index) =>
    mapBucket(entry, windowStartMillis + index * intervalSeconds * 1000, intervalSeconds)
  );
  const coveredSeconds = safeInteger(item.covered_seconds);
  const expectedCovered = buckets.reduce((total, bucket) => safeAdd(total, bucket.coveredSeconds), 0);
  const counts = nullableCounts(item.counts);
  const expectedCounts = sumNullableCounts(buckets.map((bucket) => bucket.counts));
  const coverageState = coverage(item.coverage_state);
  if (coveredSeconds !== expectedCovered || !sameCounts(counts, expectedCounts) ||
      coverageState !== aggregateCoverage(buckets.map((bucket) => bucket.coverageState), coveredSeconds)) {
    throw new Error("log source aggregates are inconsistent");
  }
  return { source, status, coverageState, coveredSeconds, counts, buckets };
}

function mapBucket(value: unknown, expectedAtMillis: number, intervalSeconds: number): LogSummaryBucket {
  const item = object(value);
  const at = wholeSecondTimestamp(item.at, "bucket.at");
  if (Date.parse(at) !== expectedAtMillis) throw new Error("log bucket is outside the fixed ascending grid");
  const coverageState = coverage(item.coverage_state);
  const coveredSeconds = safeInteger(item.covered_seconds);
  const reasonCode = nullableReason(item.reason_code);
  const counts = item.counts === null ? null : bucketCounts(item.counts);
  const positive = counts !== null && (counts.captured > 0 || counts.discarded > 0);
  switch (coverageState) {
    case "FULL":
      if (coveredSeconds !== intervalSeconds || reasonCode !== null || counts === null) throw new Error("full log bucket is inconsistent");
      break;
    case "PARTIAL":
      if (coveredSeconds < 1 || coveredSeconds >= intervalSeconds || !isHistoricalReason(reasonCode, true) || counts === null) throw new Error("partial log bucket is inconsistent");
      break;
    case "GAP":
      if (coveredSeconds !== 0 || !isHistoricalReason(reasonCode, false) || (counts !== null && !positive)) throw new Error("gap log bucket is inconsistent");
      break;
    case "UNKNOWN":
      if (coveredSeconds !== 0 || reasonCode !== "NOT_YET_OBSERVED" || (counts !== null && !positive)) throw new Error("unknown log bucket is inconsistent");
      break;
  }
  return { at, coverageState, coveredSeconds, reasonCode, counts };
}

function bucketCounts(value: unknown): LogCounts & { severity: LogSeverityCounts } {
  const item = object(value);
  const severityItem = object(item.severity);
  const severity = Object.fromEntries(severityKeys.map((key) => [key, safeInteger(severityItem[key])])) as unknown as LogSeverityCounts;
  const captured = safeInteger(item.captured);
  const discarded = safeInteger(item.discarded);
  const severityTotal = severityKeys.reduce((total, key) => safeAdd(total, severity[key]), 0);
  if (captured !== severityTotal) throw new Error("captured count differs from severity total");
  return { captured, discarded, severity };
}

function mapQuality(value: Record<string, unknown>): SectionQuality {
  return {
    supportState: support(value.support_state),
    collectionState: collection(value.collection_state),
    freshness: freshness(value.freshness),
    observedAt: nullableTimestamp(value.observed_at, "observed_at"),
    reasonCode: nullableReason(value.reason_code)
  };
}

function validateSourceStatus(status: LogSourceSummary["status"], generatedNanos: bigint): void {
  const observed = optionalNanos(status.observedAt);
  const attempted = optionalNanos(status.attemptedAt);
  const covered = optionalNanos(status.coverageThrough);
  if ([observed, attempted, covered].some((instant) => instant !== null && instant > generatedNanos)) throw new Error("log source status is newer than its response");
  if (observed !== null && attempted === null) throw new Error("observed log status requires an attempt");
  if (covered !== null && observed === null) throw new Error("log coverage watermark requires an observation");
  if (observed !== null && attempted !== null && observed > attempted) throw new Error("log observation is after its attempt");
  if (covered !== null && observed !== null && covered > observed) throw new Error("log coverage watermark is after its observation");
  if ((status.freshness === "CURRENT" || status.freshness === "STALE") !== (status.observedAt !== null)) throw new Error("log freshness and observation disagree");

  const reason = status.reasonCode;
  const valid =
    (status.supportState === "SUPPORTED" && status.collectionState === "OK" && status.freshness !== "UNKNOWN" && reason === null) ||
    (status.supportState === "SUPPORTED" && status.collectionState === "PARTIAL" && reason !== null && ["CHECKPOINT_RESET", "INVALID_RESPONSE", "DEADLINE_EXCEEDED", "RESPONSE_TOO_LARGE", "BACKLOG_DEFERRED"].includes(reason)) ||
    (status.supportState === "SUPPORTED" && status.collectionState === "FAILED" && status.freshness !== "CURRENT" && reason !== null && ["DEADLINE_EXCEEDED", "INVALID_RESPONSE", "RESPONSE_TOO_LARGE", "READER_FAILED", "LOG_STORAGE_UNAVAILABLE"].includes(reason)) ||
    (status.supportState === "UNAVAILABLE" && status.collectionState === "NOT_RUN" && status.freshness === "UNKNOWN" && status.observedAt === null && status.coverageThrough === null && reason !== null && ["NO_VISIBLE_JOURNAL", "LOG_HELPER_UNAVAILABLE", "LOG_HELPER_MISMATCH", "READER_FAILED"].includes(reason)) ||
    (status.supportState === "UNAVAILABLE" && status.collectionState === "FAILED" && status.freshness !== "CURRENT" && reason !== null && ["READER_FAILED", "LOG_STORAGE_UNAVAILABLE"].includes(reason)) ||
    (status.supportState === "PERMISSION_DENIED" && status.collectionState === "NOT_RUN" && status.freshness === "UNKNOWN" && status.observedAt === null && status.coverageThrough === null && reason === "PERMISSION_DENIED") ||
    (status.supportState === "UNSUPPORTED" && status.collectionState === "NOT_RUN" && status.freshness === "UNKNOWN" && status.observedAt === null && status.coverageThrough === null && reason === "PLATFORM_UNSUPPORTED");
  if (!valid || (reason !== null && !sourceReasons.has(reason))) throw new Error("log source status is inconsistent");
}

function aggregateQuality(sources: LogSourceSummary[]): SectionQuality {
  if (sources.length === 0) return { supportState: "DISABLED", collectionState: "NOT_RUN", freshness: "UNKNOWN", observedAt: null, reasonCode: "LOG_SOURCES_DISABLED" };
  if (sources.length === 1) {
    const status = sources[0].status;
    return { supportState: status.supportState, collectionState: status.collectionState, freshness: status.freshness, observedAt: status.observedAt, reasonCode: status.collectionState === "PARTIAL" ? "SOURCE_PARTIAL" : status.reasonCode };
  }
  const statuses = sources.map((source) => source.status);
  const supported = statuses.filter((status) => status.supportState === "SUPPORTED").length;
  const supportState = supported > 0 ? "SUPPORTED" : statuses.every((status) => status.supportState === statuses[0].supportState) ? statuses[0].supportState : "UNAVAILABLE";
  const collectionState = supported > 0
    ? supported !== statuses.length || !statuses.every((status) => status.collectionState === statuses[0].collectionState) ? "PARTIAL" : statuses[0].collectionState
    : statuses.some((status) => status.collectionState === "FAILED") ? "FAILED" : "NOT_RUN";
  const observed = statuses.filter((status) => status.observedAt !== null);
  const observedAt = observed.reduce<string | null>((latest, status) => latest === null || timestampNanos(status.observedAt!) > timestampNanos(latest) ? status.observedAt : latest, null);
  const aggregateFreshness = observed.length === 0 ? "UNKNOWN" : observed.some((status) => status.freshness === "STALE") ? "STALE" : "CURRENT";
  const sameOutcome = statuses.every((status) => status.supportState === statuses[0].supportState && status.collectionState === statuses[0].collectionState && status.reasonCode === statuses[0].reasonCode);
  const reasonCode = collectionState === "OK" ? null : collectionState === "PARTIAL" || !sameOutcome ? "SOURCE_PARTIAL" : statuses[0].reasonCode;
  return { supportState, collectionState, freshness: aggregateFreshness, observedAt, reasonCode };
}

function aggregateCoverage(states: LogCoverageState[], coveredSeconds: number): LogCoverageState {
  if (states.length === 0 || states.every((state) => state === "UNKNOWN")) return "UNKNOWN";
  if (states.every((state) => state === "FULL")) return "FULL";
  if (coveredSeconds === 0 && states.some((state) => state === "GAP")) return "GAP";
  return "PARTIAL";
}

function sumNullableCounts(values: Array<LogCounts | null>): LogCounts | null {
  const known = values.filter((value): value is LogCounts => value !== null);
  if (known.length === 0) return null;
  return known.reduce((total, value) => ({ captured: safeAdd(total.captured, value.captured), discarded: safeAdd(total.discarded, value.discarded) }), { captured: 0, discarded: 0 });
}

function sameCounts(left: LogCounts | null, right: LogCounts | null): boolean {
  return left === null ? right === null : right !== null && left.captured === right.captured && left.discarded === right.discarded;
}

function sameQuality(left: SectionQuality, right: SectionQuality): boolean {
  return left.supportState === right.supportState && left.collectionState === right.collectionState && left.freshness === right.freshness && left.observedAt === right.observedAt && left.reasonCode === right.reasonCode;
}

function isHistoricalReason(value: string | null, includeUnknown: boolean): boolean {
  return value !== null && (historicalReasons.includes(value as typeof historicalReasons[number]) || includeUnknown && value === "NOT_YET_OBSERVED");
}

function timestamp(value: unknown, field: string): string {
  const result = text(value);
  const match = /^(?!0001-01-01T00:00:00(?:\.0{1,9})?Z$)([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(?:\.([0-9]{1,9}))?Z$/.exec(result);
  if (!match) throw new Error(`${field} is not a bounded UTC timestamp`);
  const [, year, month, day, hour, minute, second] = match;
  const yearNumber = Number(year);
  const monthNumber = Number(month);
  const days = monthNumber >= 1 && monthNumber <= 12 ? [31, leap(yearNumber) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][monthNumber - 1] : 0;
  if (yearNumber < 1 || Number(day) < 1 || Number(day) > days || Number(hour) > 23 || Number(minute) > 59 || Number(second) > 59 || !Number.isFinite(Date.parse(result))) {
    throw new Error(`${field} is not a valid UTC timestamp`);
  }
  return result;
}

function wholeSecondTimestamp(value: unknown, field: string): string {
  const result = timestamp(value, field);
  if (!/T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.0{1,9})?Z$/.test(result)) throw new Error(`${field} is not on a whole-second boundary`);
  return result;
}

function timestampNanos(value: string): bigint {
  const match = /^(.*T[0-9]{2}:[0-9]{2}:[0-9]{2})(?:\.([0-9]{1,9}))?Z$/.exec(value);
  if (!match) throw new Error("invalid UTC timestamp");
  return BigInt(Date.parse(`${match[1]}Z`)) * 1_000_000n + BigInt((match[2] ?? "").padEnd(9, "0") || "0");
}

function floorDiv(value: bigint, divisor: bigint): bigint {
  const quotient = value / divisor;
  return value < 0n && value % divisor !== 0n ? quotient - 1n : quotient;
}

function optionalNanos(value: string | null): bigint | null { return value === null ? null : timestampNanos(value); }
function nullableTimestamp(value: unknown, field: string): string | null { return value === null ? null : timestamp(value, field); }
function leap(year: number): boolean { return year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0); }
function object(value: unknown): Record<string, unknown> { if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("expected log summary object"); return value as Record<string, unknown>; }
function boundedArray(value: unknown, minimum: number, maximum: number): unknown[] { if (!Array.isArray(value) || value.length < minimum || value.length > maximum) throw new Error("log summary array exceeds bounds"); return value; }
function text(value: unknown): string { if (typeof value !== "string") throw new Error("expected log summary string"); return value; }
function literal<T extends string | number | boolean>(value: unknown, expected: T): T { if (value !== expected) throw new Error("unexpected log summary literal"); return expected; }
function safeInteger(value: unknown): number { if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0 || value > MAX_SAFE_COUNT) throw new Error("log summary integer exceeds safe bounds"); return value; }
function safeAdd(left: number, right: number): number { const result = left + right; if (!Number.isSafeInteger(result) || result > MAX_SAFE_COUNT) throw new Error("log summary aggregate exceeds safe bounds"); return result; }
function nullableCounts(value: unknown): LogCounts | null { if (value === null) return null; const item = object(value); return { captured: safeInteger(item.captured), discarded: safeInteger(item.discarded) }; }
function nullableReason(value: unknown): string | null { if (value === null) return null; const result = text(value); if (!/^[A-Z0-9_]{1,64}$/.test(result)) throw new Error("invalid log reason code"); return result; }
function logRange(value: unknown): TrendRange { if (value !== "1h" && value !== "6h" && value !== "24h" && value !== "7d") throw new Error("invalid log summary range"); return value; }
function logSource(value: unknown): LogSource { if (value !== "system" && value !== "application") throw new Error("invalid log source"); return value; }
function coverage(value: unknown): LogCoverageState { if (value !== "FULL" && value !== "PARTIAL" && value !== "GAP" && value !== "UNKNOWN") throw new Error("invalid log coverage state"); return value; }
function support(value: unknown): SupportState { if (value !== "SUPPORTED" && value !== "DISABLED" && value !== "UNAVAILABLE" && value !== "PERMISSION_DENIED" && value !== "UNSUPPORTED") throw new Error("invalid log support state"); return value; }
function collection(value: unknown): CollectionState { if (value !== "OK" && value !== "PARTIAL" && value !== "FAILED" && value !== "NOT_RUN") throw new Error("invalid log collection state"); return value; }
function freshness(value: unknown): Freshness { if (value !== "CURRENT" && value !== "STALE" && value !== "UNKNOWN") throw new Error("invalid log freshness"); return value; }
