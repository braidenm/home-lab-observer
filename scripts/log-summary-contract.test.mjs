import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { aggregateLatestQuality, logRanges, maxSafeInteger, validateLogSummaryFixture } from "./log-summary-contract.mjs";

const readJson = (relative) => JSON.parse(readFileSync(new URL(`../${relative}`, import.meta.url), "utf8"));
const schema = readJson("schemas/v1/log-summary-v1.schema.json");
const ajv = new Ajv2020({ allErrors: true, strict: false });
addFormats(ajv);
const validateSchema = ajv.compile(schema);
const rich = readJson("schemas/v1/fixtures/valid/log-summary-1h.json");
const unavailable = readJson("schemas/v1/fixtures/valid/log-summary-storage-unavailable.json");

function mutated(source, mutate) {
  const value = structuredClone(source);
  mutate(value);
  return value;
}

function assertSchemaInvalid(name, source, mutate) {
  const value = mutated(source, mutate);
  assert.equal(validateSchema(value), false, `${name}: ${JSON.stringify(validateSchema.errors)}`);
}

function assertSemanticInvalid(name, mutate) {
  const value = mutated(rich, mutate);
  assert.equal(validateSchema(value), true, `${name}: ${JSON.stringify(validateSchema.errors)}`);
  assert.throws(() => validateLogSummaryFixture(value, name));
}

function assertSemanticValid(name, source, mutate) {
  const value = mutated(source, mutate);
  assert.equal(validateSchema(value), true, `${name}: ${JSON.stringify(validateSchema.errors)}`);
  assert.doesNotThrow(() => validateLogSummaryFixture(value, name));
}

test("closed schema rejects unsafe states and unbounded values", () => {
  assertSchemaInvalid("wrong fixed grid length", rich, (value) => value.sources[0].buckets.pop());
  assertSchemaInvalid("GAP cannot use unknown reason", rich, (value) => { value.sources[0].buckets[0].reason_code = "NOT_YET_OBSERVED"; });
  assertSchemaInvalid("GAP known counts cannot be zero", rich, (value) => {
    value.sources[0].buckets[0].counts.captured = 0;
    value.sources[0].buckets[0].counts.severity.warn = 0;
  });
  assertSchemaInvalid("unsupported storage failure", unavailable, (value) => { value.sources[0].status.support_state = "UNSUPPORTED"; });
  assertSchemaInvalid("configured source disabled", unavailable, (value) => { value.sources[0].status.support_state = "DISABLED"; });
  assertSchemaInvalid("supported source cannot be not-run", unavailable, (value) => {
    value.sources[0].status.support_state = "SUPPORTED";
    value.sources[0].status.collection_state = "NOT_RUN";
    value.sources[0].status.reason_code = "NO_VISIBLE_JOURNAL";
  });
  assertSchemaInvalid("unknown status reason", unavailable, (value) => { value.sources[0].status.reason_code = "NATIVE_ERROR_TEXT"; });
  assertSchemaInvalid("permission state cannot hide its reason", unavailable, (value) => {
    value.sources[0].status.support_state = "PERMISSION_DENIED";
    value.sources[0].status.collection_state = "NOT_RUN";
  });
  assertSchemaInvalid("safe integer overflow", rich, (value) => { value.counts.captured = maxSafeInteger + 1; });
  assertSchemaInvalid("year zero", rich, (value) => { value.generated_at = "0000-01-01T00:00:00Z"; });
  assertSchemaInvalid("year above 9999", rich, (value) => { value.generated_at = "+010000-01-01T00:00:00Z"; });
  assertSchemaInvalid("excess timestamp precision", rich, (value) => { value.generated_at = "2026-09-09T13:00:17.1234567890Z"; });
});

test("semantic validator rejects invalid grids, aggregates, and timestamps", () => {
  assertSemanticInvalid("subsecond grid timestamp", (value) => { value.sources[0].buckets[10].at = "2026-09-09T12:10:00.000000001Z"; });
  assertSemanticInvalid("window end is not current boundary", (value) => { value.generated_at = "2026-09-09T13:01:17Z"; });
  assertSemanticInvalid("severity sum", (value) => { value.sources[0].buckets[0].counts.severity.warn = 0; });
  assertSemanticInvalid("source count sum", (value) => { value.sources[0].counts.captured = 3; });
  assertSemanticInvalid("top count sum", (value) => { value.counts.discarded = 2; });
  assertSemanticInvalid("source covered seconds", (value) => { value.sources[0].covered_seconds += 1; });
  assertSemanticInvalid("top coverage aggregation", (value) => { value.coverage_state = "FULL"; });
  assertSemanticInvalid("status time after generation", (value) => { value.sources[0].status.attempted_at = "2026-09-09T13:01:00Z"; });
  assertSemanticInvalid("submillisecond observed after attempt", (value) => {
    value.generated_at = "2026-09-09T13:00:17.1Z";
    value.sources[0].status.observed_at = "2026-09-09T13:00:00.000000002Z";
    value.sources[0].status.attempted_at = "2026-09-09T13:00:00.000000001Z";
    value.observed_at = value.sources[0].status.observed_at;
  });
  assertSemanticInvalid("coverage after observation", (value) => { value.sources[0].status.coverage_through = "2026-09-09T13:00:01Z"; });
  assertSemanticInvalid("submillisecond coverage after observation", (value) => {
    value.sources[0].status.observed_at = "2026-09-09T12:59:00.000000001Z";
    value.sources[0].status.coverage_through = "2026-09-09T12:59:00.000000002Z";
    value.observed_at = value.sources[0].status.observed_at;
  });
  assertSemanticInvalid("source ordering", (value) => { value.sources = [structuredClone(unavailable.sources[0]), value.sources[0]]; });
  assertSemanticInvalid("singleton top-level status", (value) => { value.freshness = "STALE"; });
});

test("semantic validator rejects aggregate overflow within and across sources", () => {
  assertSemanticInvalid("within-source counter overflow", (value) => {
    for (const index of [3, 4]) {
      value.sources[0].buckets[index].counts.captured = maxSafeInteger;
      value.sources[0].buckets[index].counts.severity.critical = maxSafeInteger;
    }
  });
  assertSemanticInvalid("cross-source counter overflow", (value) => {
    for (const bucket of value.sources[0].buckets) {
      if (bucket.counts !== null && (bucket.coverage_state === "FULL" || bucket.coverage_state === "PARTIAL")) {
        bucket.counts.captured = 0;
        for (const key of Object.keys(bucket.counts.severity)) bucket.counts.severity[key] = 0;
      }
    }
    value.sources[0].buckets[3].counts.captured = maxSafeInteger - 2;
    value.sources[0].buckets[3].counts.severity.info = maxSafeInteger - 2;
    value.sources[0].counts.captured = maxSafeInteger;
    const application = structuredClone(value.sources[0]);
    application.source = "application";
    value.sources.push(application);
  });
});

test("accepted latest and historical state combinations remain distinct", () => {
  assertSemanticValid("PARTIAL bucket with unknown remainder", rich, (value) => { value.sources[0].buckets[2].reason_code = "NOT_YET_OBSERVED"; });
  assertSemanticValid("PARTIAL latest before first observed success", rich, (value) => {
    value.freshness = "UNKNOWN";
    value.observed_at = null;
    value.sources[0].status.freshness = "UNKNOWN";
    value.sources[0].status.observed_at = null;
    value.sources[0].status.coverage_through = null;
  });
  assertSemanticValid("aged OK status", rich, (value) => {
    value.collection_state = "OK";
    value.freshness = "STALE";
    value.reason_code = null;
    value.sources[0].status.collection_state = "OK";
    value.sources[0].status.freshness = "STALE";
    value.sources[0].status.reason_code = null;
  });
  assertSemanticValid("singleton application source", unavailable, () => {});
  assertSemanticValid("no visible journal", unavailable, (value) => {
    value.collection_state = "NOT_RUN";
    value.reason_code = "NO_VISIBLE_JOURNAL";
    value.sources[0].status.collection_state = "NOT_RUN";
    value.sources[0].status.reason_code = "NO_VISIBLE_JOURNAL";
  });
  assertSemanticValid("helper mismatch", unavailable, (value) => {
    value.collection_state = "NOT_RUN";
    value.reason_code = "LOG_HELPER_MISMATCH";
    value.sources[0].status.collection_state = "NOT_RUN";
    value.sources[0].status.reason_code = "LOG_HELPER_MISMATCH";
  });
});

test("mixed source latest quality has one deterministic reduction", () => {
  const value = structuredClone(rich);
  value.sources.push(structuredClone(unavailable.sources[0]));
  const expected = aggregateLatestQuality(value.sources);
  Object.assign(value, expected);
  assert.equal(validateSchema(value), true, JSON.stringify(validateSchema.errors));
  assert.doesNotThrow(() => validateLogSummaryFixture(value, "mixed sources"));
  value.support_state = "DISABLED";
  assert.throws(() => validateLogSummaryFixture(value, "invalid mixed source top quality"));
});

test("latest observed time compares fractional UTC instants instead of strings", () => {
  const system = structuredClone(rich.sources[0]);
  system.status.observed_at = "2026-09-09T13:00:00.1Z";
  system.status.attempted_at = "2026-09-09T13:00:00.2Z";
  system.status.coverage_through = null;
  const application = structuredClone(system);
  application.source = "application";
  application.status.observed_at = "2026-09-09T13:00:00.01Z";
  const quality = aggregateLatestQuality([system, application]);
  assert.equal(quality.observed_at, "2026-09-09T13:00:00.1Z");
});

test("maximum two-source seven-day grid stays below the response cap", () => {
  const range = logRanges["7d"];
  const end = Date.parse("2026-09-09T13:00:00Z");
  const start = end - range.durationSeconds * 1000;
  const severityCount = Math.floor(maxSafeInteger / (2 * range.bucketCount * 7));
  const capturedPerBucket = severityCount * 7;
  const discardedPerBucket = Math.floor(maxSafeInteger / (2 * range.bucketCount));
  const capturedPerSource = capturedPerBucket * range.bucketCount;
  const discardedPerSource = discardedPerBucket * range.bucketCount;
  const buckets = Array.from({ length: range.bucketCount }, (_, index) => ({
    at: new Date(start + index * range.intervalSeconds * 1000).toISOString().replace(".000Z", ".000000000Z"),
    coverage_state: "FULL",
    covered_seconds: range.intervalSeconds,
    reason_code: null,
    counts: {
      captured: capturedPerBucket, discarded: discardedPerBucket,
      severity: Object.fromEntries(["trace", "debug", "info", "warn", "error", "critical", "unknown"].map((key) => [key, severityCount]))
    }
  }));
  const status = {
    support_state: "SUPPORTED", collection_state: "OK", freshness: "CURRENT",
    observed_at: "2026-09-09T13:00:00Z", attempted_at: "2026-09-09T13:00:00Z",
    coverage_through: "2026-09-09T13:00:00Z", reason_code: null
  };
  const source = (name) => ({
    source: name, status: structuredClone(status), coverage_state: "FULL", covered_seconds: range.durationSeconds,
    counts: { captured: capturedPerSource, discarded: discardedPerSource }, buckets: structuredClone(buckets)
  });
  const value = {
    ...structuredClone(rich), range: "7d", window_start: new Date(start).toISOString(),
    bucket_interval_seconds: range.intervalSeconds, expected_bucket_count: range.bucketCount,
    support_state: "SUPPORTED", collection_state: "OK", freshness: "CURRENT",
    observed_at: "2026-09-09T13:00:00Z", reason_code: null, coverage_state: "FULL",
    counts: { captured: capturedPerSource * 2, discarded: discardedPerSource * 2 }, sources: [source("system"), source("application")]
  };
  assert.equal(validateSchema(value), true, JSON.stringify(validateSchema.errors));
  assert.doesNotThrow(() => validateLogSummaryFixture(value, "maximum grid"));
  assert.ok(Buffer.byteLength(JSON.stringify(value), "utf8") <= 262144);

  // Reasons take more bytes than null; partial history remains independent of the latest successful status.
  value.coverage_state = "PARTIAL";
  for (const item of value.sources) {
    item.coverage_state = "PARTIAL";
    item.covered_seconds = (range.intervalSeconds - 1) * range.bucketCount;
    for (const bucket of item.buckets) {
      bucket.coverage_state = "PARTIAL";
      bucket.covered_seconds = range.intervalSeconds - 1;
      bucket.reason_code = "RESPONSE_TOO_LARGE";
    }
  }
  assert.equal(validateSchema(value), true, JSON.stringify(validateSchema.errors));
  assert.doesNotThrow(() => validateLogSummaryFixture(value, "maximum partial grid"));
  assert.ok(Buffer.byteLength(JSON.stringify(value), "utf8") <= 262144);
});
