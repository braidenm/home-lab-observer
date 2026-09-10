export const maxSafeInteger = 9007199254740991;

export const logRanges = {
  "1h": { durationSeconds: 3600, intervalSeconds: 60, bucketCount: 60 },
  "6h": { durationSeconds: 21600, intervalSeconds: 300, bucketCount: 72 },
  "24h": { durationSeconds: 86400, intervalSeconds: 900, bucketCount: 96 },
  "7d": { durationSeconds: 604800, intervalSeconds: 3600, bucketCount: 168 }
};

const severityKeys = ["trace", "debug", "info", "warn", "error", "critical", "unknown"];
const prohibitedKeys = new Set([
  "authorization", "token", "secret", "message", "body", "log_body", "raw_log_body", "event_code",
  "provider", "provider_name", "identity", "path", "pid", "unit", "executable", "hash", "cursor",
  "checkpoint", "native_payload", "raw_payload"
]);

function fail(message) {
  throw new Error(message);
}

function safeAdd(left, right, context) {
  const value = left + right;
  if (!Number.isSafeInteger(value) || value > maxSafeInteger) fail(`${context} exceeds the JSON safe-integer bound`);
  return value;
}

function sumNullableCounts(values, context) {
  const present = values.filter((value) => value !== null);
  if (present.length === 0) return null;
  return present.reduce((total, value) => ({
    captured: safeAdd(total.captured, value.captured, `${context}.captured`),
    discarded: safeAdd(total.discarded, value.discarded, `${context}.discarded`)
  }), { captured: 0, discarded: 0 });
}

function sameCounts(actual, expected) {
  return actual === null
    ? expected === null
    : expected !== null && actual.captured === expected.captured && actual.discarded === expected.discarded;
}

function aggregateCoverage(states, coveredSeconds) {
  if (states.length === 0 || states.every((state) => state === "UNKNOWN")) return "UNKNOWN";
  if (states.every((state) => state === "FULL")) return "FULL";
  if (coveredSeconds === 0 && states.some((state) => state === "GAP")) return "GAP";
  return "PARTIAL";
}

function wholeSecondUTC(value) {
  return /^(?!0000)\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.0{1,9})?Z$/.test(value);
}

function parseTimestamp(value, context) {
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) fail(`${context} is not a finite UTC timestamp`);
  return parsed;
}

function timestampNanos(value, context) {
  const match = /^(?!0000)(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?Z$/.exec(value);
  if (!match) fail(`${context} is not a supported UTC timestamp`);
  const wholeMilliseconds = Date.parse(`${match[1]}Z`);
  if (!Number.isFinite(wholeMilliseconds)) fail(`${context} is not a finite UTC timestamp`);
  const fractionalNanos = BigInt((match[2] ?? "").padEnd(9, "0") || "0");
  return BigInt(wholeMilliseconds) * 1000000n + fractionalNanos;
}

function validateStatusTimes(status, generatedNanos, fixturePath, source) {
  const values = [status.observed_at, status.attempted_at, status.coverage_through].filter((value) => value !== null);
  if (values.some((value) => timestampNanos(value, `${fixturePath} source ${source} status`) > generatedNanos)) {
    fail(`${fixturePath} source ${source} status timestamps must be UTC and no later than generated_at`);
  }
  if (status.observed_at !== null && status.attempted_at === null) fail(`${fixturePath} observed_at requires attempted_at`);
  if (status.coverage_through !== null && status.observed_at === null) fail(`${fixturePath} coverage_through requires observed_at`);
  if (status.attempted_at !== null) {
    const attempted = timestampNanos(status.attempted_at, `${fixturePath} attempted_at`);
    if (status.observed_at !== null && timestampNanos(status.observed_at, `${fixturePath} observed_at`) > attempted) fail(`${fixturePath} observed_at is after attempted_at`);
  }
  if (status.coverage_through !== null && timestampNanos(status.coverage_through, `${fixturePath} coverage_through`) > timestampNanos(status.observed_at, `${fixturePath} observed_at`)) {
    fail(`${fixturePath} coverage_through is after observed_at`);
  }
}

export function aggregateLatestQuality(sources) {
  if (sources.length === 0) {
    return { support_state: "DISABLED", collection_state: "NOT_RUN", freshness: "UNKNOWN", observed_at: null, reason_code: "LOG_SOURCES_DISABLED" };
  }
  if (sources.length === 1) {
    const status = sources[0].status;
    return {
      support_state: status.support_state,
      collection_state: status.collection_state,
      freshness: status.freshness,
      observed_at: status.observed_at,
      reason_code: status.collection_state === "PARTIAL" ? "SOURCE_PARTIAL" : status.reason_code
    };
  }

  const statuses = sources.map((source) => source.status);
  const supportedCount = statuses.filter((status) => status.support_state === "SUPPORTED").length;
  const supportState = supportedCount > 0
    ? "SUPPORTED"
    : statuses.every((status) => status.support_state === statuses[0].support_state)
      ? statuses[0].support_state
      : "UNAVAILABLE";
  let collectionState;
  if (supportedCount > 0) {
    collectionState = supportedCount !== statuses.length || !statuses.every((status) => status.collection_state === statuses[0].collection_state)
      ? "PARTIAL"
      : statuses[0].collection_state;
  } else {
    collectionState = statuses.some((status) => status.collection_state === "FAILED") ? "FAILED" : "NOT_RUN";
  }
  const observed = statuses.filter((status) => status.observed_at !== null);
  const observedAt = observed.length === 0
    ? null
    : observed.reduce((latest, status) => timestampNanos(status.observed_at, "source observed_at") > timestampNanos(latest, "source observed_at") ? status.observed_at : latest, observed[0].observed_at);
  const freshness = observed.length === 0
    ? "UNKNOWN"
    : observed.some((status) => status.freshness === "STALE") ? "STALE" : "CURRENT";
  const sameOutcome = statuses.every((status) => status.support_state === statuses[0].support_state
    && status.collection_state === statuses[0].collection_state
    && status.reason_code === statuses[0].reason_code);
  const reasonCode = collectionState === "OK"
    ? null
    : collectionState === "PARTIAL" || !sameOutcome ? "SOURCE_PARTIAL" : statuses[0].reason_code;
  return { support_state: supportState, collection_state: collectionState, freshness, observed_at: observedAt, reason_code: reasonCode };
}

// Call only after the closed JSON Schema succeeds; this enforces invariants that span fields and array entries.
export function validateLogSummaryFixture(fixture, fixturePath = "log summary") {
  const range = logRanges[fixture.range] ?? fail(`${fixturePath} has an unknown range`);
  const generated = parseTimestamp(fixture.generated_at, `${fixturePath} generated_at`);
  const generatedNanos = timestampNanos(fixture.generated_at, `${fixturePath} generated_at`);
  const start = parseTimestamp(fixture.window_start, `${fixturePath} window_start`);
  const end = parseTimestamp(fixture.window_end, `${fixturePath} window_end`);
  if (![fixture.generated_at, fixture.window_start, fixture.window_end].every((value) => value.endsWith("Z"))) {
    fail(`${fixturePath} timestamps must be UTC`);
  }
  if (!wholeSecondUTC(fixture.window_start) || !wholeSecondUTC(fixture.window_end)) {
    fail(`${fixturePath} window boundaries must be exact whole UTC seconds`);
  }
  if (end - start !== range.durationSeconds * 1000 || fixture.bucket_interval_seconds !== range.intervalSeconds || fixture.expected_bucket_count !== range.bucketCount) {
    fail(`${fixturePath} range, window, interval, and bucket count disagree`);
  }
  if (end > generated || end !== Math.floor(generated / (range.intervalSeconds * 1000)) * range.intervalSeconds * 1000) {
    fail(`${fixturePath} window_end is not the greatest epoch-aligned bucket boundary at generated_at`);
  }

  const sourceRank = new Map([["system", 0], ["application", 1]]);
  let priorSourceRank = -1;
  for (const source of fixture.sources) {
    const rank = sourceRank.get(source.source);
    if (rank === undefined || rank <= priorSourceRank) fail(`${fixturePath} sources must be unique and ordered system, application`);
    priorSourceRank = rank;
    if (source.buckets.length !== range.bucketCount) fail(`${fixturePath} source ${source.source} has the wrong fixed grid length`);
    validateStatusTimes(source.status, generatedNanos, fixturePath, source.source);

    let coveredSeconds = 0;
    let expectedAt = start;
    for (const [index, bucket] of source.buckets.entries()) {
      const at = Date.parse(bucket.at);
      if (!wholeSecondUTC(bucket.at) || at !== expectedAt) fail(`${fixturePath} source ${source.source} bucket ${index} is not the exact ascending grid`);
      expectedAt += range.intervalSeconds * 1000;
      if (bucket.coverage_state === "FULL" && bucket.covered_seconds !== range.intervalSeconds) fail(`${fixturePath} FULL bucket ${index} must cover the complete interval`);
      if (bucket.coverage_state === "PARTIAL" && !(bucket.covered_seconds > 0 && bucket.covered_seconds < range.intervalSeconds)) fail(`${fixturePath} PARTIAL bucket ${index} must cover only part of the interval`);
      if ((bucket.coverage_state === "GAP" || bucket.coverage_state === "UNKNOWN") && bucket.covered_seconds !== 0) fail(`${fixturePath} uncovered bucket ${index} has covered seconds`);
      coveredSeconds = safeAdd(coveredSeconds, bucket.covered_seconds, `${fixturePath} covered_seconds`);
      if (bucket.counts !== null) {
        const severityTotal = severityKeys.reduce((total, key) => safeAdd(total, bucket.counts.severity[key], `${fixturePath} severity`), 0);
        if (severityTotal !== bucket.counts.captured) fail(`${fixturePath} bucket ${index} captured count differs from severity total`);
        if ((bucket.coverage_state === "GAP" || bucket.coverage_state === "UNKNOWN") && bucket.counts.captured === 0 && bucket.counts.discarded === 0) {
          fail(`${fixturePath} uncovered bucket ${index} cannot invent known zero counts`);
        }
      }
    }
    if (source.covered_seconds !== coveredSeconds) fail(`${fixturePath} source ${source.source} covered_seconds is not the bucket sum`);
    const sourceCounts = sumNullableCounts(source.buckets.map((bucket) => bucket.counts), `${fixturePath}.${source.source}`);
    if (!sameCounts(source.counts, sourceCounts)) fail(`${fixturePath} source ${source.source} counts are not the bucket sum`);
    const coverage = aggregateCoverage(source.buckets.map((bucket) => bucket.coverage_state), coveredSeconds);
    if (source.coverage_state !== coverage) fail(`${fixturePath} source ${source.source} coverage aggregate is ${coverage}`);
  }

  const topCounts = sumNullableCounts(fixture.sources.map((source) => source.counts), `${fixturePath}.top`);
  if (!sameCounts(fixture.counts, topCounts)) fail(`${fixturePath} top-level counts are not the source sum`);
  const topCovered = fixture.sources.reduce((total, source) => safeAdd(total, source.covered_seconds, `${fixturePath}.top.covered`), 0);
  const topCoverage = aggregateCoverage(fixture.sources.map((source) => source.coverage_state), topCovered);
  if (fixture.coverage_state !== topCoverage) fail(`${fixturePath} top-level coverage aggregate is ${topCoverage}`);

  const latestQuality = aggregateLatestQuality(fixture.sources);
  for (const field of ["support_state", "collection_state", "freshness", "observed_at", "reason_code"]) {
    if (fixture[field] !== latestQuality[field]) fail(`${fixturePath} top-level ${field} must be ${latestQuality[field]}`);
  }
  if (fixture.observed_at !== null && timestampNanos(fixture.observed_at, `${fixturePath} observed_at`) > generatedNanos) {
    fail(`${fixturePath} top-level observed_at is after generated_at`);
  }

  const inspect = (value, location = "$") => {
    if (Array.isArray(value)) return value.forEach((item, index) => inspect(item, `${location}[${index}]`));
    if (value && typeof value === "object") for (const [key, item] of Object.entries(value)) {
      if (prohibitedKeys.has(key.toLowerCase())) fail(`Forbidden log summary field ${key} in ${fixturePath} at ${location}`);
      inspect(item, `${location}.${key}`);
    }
  };
  inspect(fixture);
}
