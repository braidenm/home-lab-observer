import type { LocalHttpObserverDataSourceOptions } from "./local-http.types";
import type {
  CollectionState,
  CurrentSnapshot,
  Freshness,
  ListSection,
  LogBody,
  LogRecord,
  MetricId,
  ObserverCapabilities,
  ObserverDataSource,
  ObserverProblemDetails,
  SectionName,
  SectionQuality,
  Severity,
  SupportState,
  TrendRange,
  TrendSeries,
  TrendSnapshot
} from "../types";

const DEFAULT_BASE_URL = "http://127.0.0.1:9847";
const LOOPBACK_HOSTS = new Set(["127.0.0.1", "localhost", "[::1]", "::1"]);
const CAPABILITIES_MAX_BYTES = 131_072;
const SNAPSHOT_MAX_BYTES = 1_048_576;
const SERIES_MAX_BYTES = 1_048_576;
const SECTION_NAMES: SectionName[] = ["overview", "filesystems", "processes", "services", "containers", "logs", "observer"];
const METRIC_IDS: MetricId[] = [
  "cpu.utilization.percent",
  "memory.utilization.percent",
  "filesystem.aggregate.utilization.percent",
  "network.receive.bytes_per_second",
  "network.transmit.bytes_per_second",
  "process.count"
];
const METRIC_PRESENTATION: Record<MetricId, { label: string; color: TrendSeries["color"]; unit: TrendSeries["unit"] }> = {
  "cpu.utilization.percent": { label: "CPU utilization", color: "mint", unit: "percent" },
  "memory.utilization.percent": { label: "Memory utilization", color: "cyan", unit: "percent" },
  "filesystem.aggregate.utilization.percent": { label: "Aggregate filesystem utilization", color: "amber", unit: "percent" },
  "network.receive.bytes_per_second": { label: "Network receive", color: "cyan", unit: "bytes_per_second" },
  "network.transmit.bytes_per_second": { label: "Network transmit", color: "violet", unit: "bytes_per_second" },
  "process.count": { label: "Process count", color: "mint", unit: "count" }
};

export class ObserverTransportError extends Error {
  constructor(
    message: string,
    readonly status?: number,
    readonly problem?: ObserverProblemDetails
  ) {
    super(message);
    this.name = "ObserverTransportError";
  }
}

/**
 * Reads only the accepted local v1 endpoints. Payloads are projected into the
 * closed UI model; no log summary, query expression, or arbitrary labels are derived.
 */
export class LocalHttpObserverDataSource implements ObserverDataSource {
  private readonly baseUrl: string;
  private readonly fetcher: typeof fetch;
  private readonly bearerToken?: string;

  constructor(options: LocalHttpObserverDataSourceOptions = {}) {
    this.baseUrl = normalizeBaseUrl(options.baseUrl ?? DEFAULT_BASE_URL);
    this.fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis);
    this.bearerToken = options.bearerToken;
  }

  getCapabilities(signal?: AbortSignal): Promise<ObserverCapabilities> {
    return this.request("/api/v1/capabilities", CAPABILITIES_MAX_BYTES, mapCapabilities, signal);
  }

  getCurrentSnapshot(signal?: AbortSignal): Promise<CurrentSnapshot> {
    return this.request("/api/v1/snapshots/current", SNAPSHOT_MAX_BYTES, mapCurrentSnapshot, signal);
  }

  getTrends(range: TrendRange, signal?: AbortSignal): Promise<TrendSnapshot> {
    if (!isTrendRange(range)) throw new TypeError("Metric series range is not allowlisted");
    const query = new URLSearchParams({ range });
    for (const metric of METRIC_IDS) query.append("metric", metric);
    return this.request(`/api/v1/metrics/series?${query.toString()}`, SERIES_MAX_BYTES, mapMetricSeries, signal);
  }

  private async request<T>(path: string, maximumBytes: number, map: (value: unknown) => T, signal?: AbortSignal): Promise<T> {
    const headers = new Headers({ Accept: "application/json" });
    if (this.bearerToken) headers.set("Authorization", `Bearer ${this.bearerToken}`);
    const response = await this.fetcher(`${this.baseUrl}${path}`, {
      method: "GET",
      headers,
      credentials: "omit",
      cache: "no-store",
      redirect: "error",
      signal
    });
    const contentType = (response.headers.get("content-type") ?? "").toLowerCase();
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.byteLength > maximumBytes) {
      throw new ObserverTransportError("Observer API response exceeded its documented size limit", response.status);
    }
    if (!response.ok) {
      const problem = contentType.includes("application/problem+json") ? parseProblem(bytes, response.status) : undefined;
      const message = problem ? `${problem.title}${problem.detail ? `: ${problem.detail}` : ""}` : `Observer API request failed with ${response.status}`;
      throw new ObserverTransportError(message, response.status, problem);
    }
    if (!contentType.includes("application/json")) {
      throw new ObserverTransportError("Observer API returned a non-JSON response", response.status);
    }
    try {
      return map(JSON.parse(new TextDecoder().decode(bytes)) as unknown);
    } catch (error) {
      if (error instanceof ObserverTransportError) throw error;
      throw new ObserverTransportError("Observer API returned an invalid contract payload", response.status);
    }
  }
}

export function mapCapabilities(value: unknown): ObserverCapabilities {
  const root = object(value);
  const observer = object(root.observer);
  const platform = object(root.platform);
  const policy = object(root.policy);
  const bodies = object(policy.log_bodies);
  const limits = object(policy.response_limits_bytes);
  const seen = new Set<SectionName>();
  const collectors = array(root.collectors).flatMap((entry) => {
    const item = object(entry);
    const name = text(item.name);
    if (!isSectionName(name)) return [];
    if (seen.has(name)) throw new Error("duplicate collector");
    seen.add(name);
    return [{
      name,
      supportState: support(item.support_state),
      reasonCode: nullableText(item.reason_code),
      dataClassification: classification(item.data_classification),
      uploadEligible: boolean(item.upload_eligible)
    }];
  });
  if (SECTION_NAMES.some((name) => !seen.has(name))) throw new Error("missing collector");
  return {
    schemaVersion: literal(root.schema_version, "observer-capabilities/v1"),
    apiVersion: literal(root.api_version, "v1"),
    generatedAt: text(root.generated_at),
    observer: {
      version: text(observer.version),
      mode: observer.mode === "HEADLESS" || observer.mode === "LOCAL_DASHBOARD" ? observer.mode : "UNKNOWN"
    },
    platform: {
      os: platform.os === "linux" || platform.os === "windows" || platform.os === "darwin" || platform.os === "other" ? platform.os : "other",
      architecture: text(platform.architecture)
    },
    policy: {
      readOnly: literal(policy.read_only, true),
      defaultBind: literal(policy.default_bind, "127.0.0.1"),
      corsEnabled: literal(policy.cors_enabled, false),
      retentionDays: number(policy.retention_days),
      retentionBytes: number(policy.retention_bytes),
      logBodies: {
        defaultState: literal(bodies.default_state, "OMITTED"),
        maximumBodyBytes: literal(bodies.maximum_body_bytes, 2048),
        uploadEligible: literal(bodies.upload_eligible, false)
      },
      responseLimitsBytes: {
        capabilities: literal(limits.capabilities, 131072),
        currentSnapshot: literal(limits.current_snapshot, 1048576)
      }
    },
    collectors
  };
}

export function mapCurrentSnapshot(value: unknown): CurrentSnapshot {
  const root = object(value);
  const privacy = object(root.privacy);
  const sections = object(root.sections);
  return {
    schemaVersion: literal(root.schema_version, "observer-current-snapshot/v1"),
    snapshotId: text(root.snapshot_id),
    sequence: number(root.sequence),
    observedAt: text(root.observed_at),
    durationMilliseconds: number(root.duration_ms),
    collectionState: collection(root.collection_state),
    privacy: {
      profile: privacy.profile === "LOCAL_LOG_BODIES" ? "LOCAL_LOG_BODIES" : literal(privacy.profile, "SAFE_DEFAULT"),
      remoteProjection: literal(privacy.remote_projection, "home-lab-server-snapshot/v1"),
      messageBodiesUploadEligible: literal(privacy.message_bodies_upload_eligible, false),
      redactionCount: number(privacy.redaction_count),
      droppedCount: number(privacy.dropped_count),
      excludedFields: array(privacy.excluded_fields).map((field) => text(field)) as CurrentSnapshot["privacy"]["excludedFields"]
    },
    sections: {
      overview: mapOverviewSection(sections.overview),
      filesystems: mapListSection(sections.filesystems, (entry) => {
        const item = object(entry);
        return { mountAlias: text(item.mount_alias), filesystemType: text(item.filesystem_type), totalBytes: number(item.total_bytes), usedBytes: number(item.used_bytes), availableBytes: number(item.available_bytes) };
      }),
      processes: mapListSection(sections.processes, (entry) => {
        const item = object(entry);
        return { pid: number(item.pid), name: text(item.name), state: text(item.state), cpuPercent: number(item.cpu_percent), memoryBytes: number(item.memory_bytes) };
      }),
      services: mapListSection(sections.services, (entry) => {
        const item = object(entry);
        return { name: text(item.name), state: text(item.state), startMode: text(item.start_mode) };
      }),
      containers: mapListSection(sections.containers, (entry) => {
        const item = object(entry);
        return { idAlias: text(item.id_alias), name: text(item.name), image: text(item.image), state: text(item.state), cpuPercent: number(item.cpu_percent), memoryBytes: number(item.memory_bytes) };
      }),
      logs: mapListSection(sections.logs, mapLogRecord),
      observer: mapListSection(sections.observer, (entry) => {
        const item = object(entry);
        if (item.state !== "OK" && item.state !== "WARN" && item.state !== "ERROR") throw new Error("invalid observer state");
        if (item.unit !== "count" && item.unit !== "bytes" && item.unit !== "milliseconds" && item.unit !== "percent") throw new Error("invalid observer unit");
        return { name: text(item.name), state: item.state, value: number(item.value), unit: item.unit };
      })
    }
  };
}

export function mapMetricSeries(value: unknown): TrendSnapshot {
  const root = object(value);
  exactKeys(root, ["schema_version", "range", "generated_at", "window_start", "window_end", "sample_interval_seconds", "limits", "privacy", "requested_metrics", "series"]);
  const limits = object(root.limits);
  const privacy = object(root.privacy);
  exactKeys(limits, ["max_metrics", "max_points_per_series", "max_response_bytes"]);
  exactKeys(privacy, ["data_classification", "contains_process_identity", "contains_log_bodies", "remote_upload_eligible"]);
  const range = trendRange(root.range);
  const windowStart = dateTime(root.window_start);
  const windowEnd = dateTime(root.window_end);
  const windowStartMs = Date.parse(windowStart);
  const windowEndMs = Date.parse(windowEnd);
  if (windowStartMs >= windowEndMs) throw new Error("invalid series window");

  const requestedMetrics = boundedArray(root.requested_metrics, 1, 6).map(metricId);
  if (new Set(requestedMetrics).size !== requestedMetrics.length) throw new Error("duplicate requested metric");
  const returned = new Set<MetricId>();
  const mappedSeries = boundedArray(root.series, 1, 6).map((entry): TrendSeries => {
    const item = object(entry);
    exactKeys(item, ["metric_id", "unit", "support_state", "collection_state", "freshness", "reason_code", "point_count", "gap_count", "truncated", "points"]);
    const id = metricId(item.metric_id);
    if (returned.has(id)) throw new Error("duplicate metric series");
    returned.add(id);
    const presentation = METRIC_PRESENTATION[id];
    const unit = trendUnit(item.unit);
    if (unit !== presentation.unit) throw new Error("metric unit does not match identifier");
    const supportState = seriesSupport(item.support_state);
    const collectionState = seriesCollection(item.collection_state);
    const seriesFreshness = seriesFreshnessValue(item.freshness);
    const reasonCode = reason(item.reason_code);
    const points = boundedArray(item.points, 0, 2048).map((entry) => {
      const point = object(entry);
      exactKeys(point, ["at", "value"]);
      const at = dateTime(point.at);
      const instant = Date.parse(at);
      if (instant < windowStartMs || instant > windowEndMs) throw new Error("metric point falls outside window");
      const pointValue = nullableNonNegativeNumber(point.value);
      if (pointValue !== null && unit === "percent" && pointValue > 100) throw new Error("percent exceeds 100");
      if (pointValue !== null && unit === "count" && !Number.isInteger(pointValue)) throw new Error("count must be an integer");
      return { at, value: pointValue };
    });
    for (let index = 1; index < points.length; index += 1) {
      if (Date.parse(points[index].at) <= Date.parse(points[index - 1].at)) throw new Error("metric points are not strictly ordered");
    }
    const pointCount = integer(item.point_count, 0, 2048);
    const gapCount = integer(item.gap_count, 0, 2048);
    const truncated = boolean(item.truncated);
    if (pointCount !== points.length || gapCount !== points.filter((point) => point.value === null).length) throw new Error("metric point metadata is inconsistent");
    if (supportState !== "SUPPORTED" && (collectionState !== "NOT_RUN" || seriesFreshness !== "UNKNOWN" || reasonCode === null || pointCount !== 0 || gapCount !== 0 || truncated || points.length !== 0)) {
      throw new Error("unsupported metric must not imply collection");
    }
    return { id, label: presentation.label, unit, color: presentation.color, supportState, collectionState, freshness: seriesFreshness, reasonCode, pointCount, gapCount, truncated, points };
  });
  if (mappedSeries.some((item) => !requestedMetrics.includes(item.id)) || requestedMetrics.some((id) => !returned.has(id))) throw new Error("returned series do not match requested metrics");

  return {
    schemaVersion: literal(root.schema_version, "observer-metric-series/v1"),
    range,
    generatedAt: dateTime(root.generated_at),
    windowStart,
    windowEnd,
    sampleIntervalSeconds: integer(root.sample_interval_seconds, 1, 86400),
    limits: {
      maxMetrics: literal(limits.max_metrics, 6),
      maxPointsPerSeries: literal(limits.max_points_per_series, 2048),
      maxResponseBytes: literal(limits.max_response_bytes, 1048576)
    },
    privacy: {
      dataClassification: literal(privacy.data_classification, "PUBLIC_METADATA"),
      containsProcessIdentity: literal(privacy.contains_process_identity, false),
      containsLogBodies: literal(privacy.contains_log_bodies, false),
      remoteUploadEligible: literal(privacy.remote_upload_eligible, false)
    },
    requestedMetrics,
    series: mappedSeries
  };
}

function mapOverviewSection(value: unknown): CurrentSnapshot["sections"]["overview"] {
  const item = object(value);
  const data = item.data === null ? null : object(item.data);
  return {
    ...mapQuality(item),
    data: data === null ? null : {
      hostAlias: text(data.host_alias), os: data.os === "linux" || data.os === "windows" || data.os === "darwin" || data.os === "other" ? data.os : "other",
      architecture: text(data.architecture), uptimeSeconds: number(data.uptime_seconds), cpuLogicalCount: number(data.cpu_logical_count),
      memoryTotalBytes: number(data.memory_total_bytes), memoryUsedBytes: number(data.memory_used_bytes)
    }
  };
}

function mapListSection<T>(value: unknown, mapItem: (value: unknown) => T): ListSection<T> {
  const item = object(value);
  return {
    ...mapQuality(item), totalCount: number(item.total_count), returnedCount: number(item.returned_count),
    truncated: boolean(item.truncated), items: array(item.items).map(mapItem)
  };
}

function mapQuality(item: Record<string, unknown>): SectionQuality {
  return { supportState: support(item.support_state), collectionState: collection(item.collection_state), freshness: freshness(item.freshness), observedAt: nullableText(item.observed_at), reasonCode: nullableText(item.reason_code) };
}

function mapLogRecord(value: unknown): LogRecord {
  const item = object(value);
  const metadata = object(item.metadata);
  const body = object(item.body);
  const state = text(body.state);
  let mappedBody: LogBody;
  if (state === "OMITTED") {
    mappedBody = { state };
  } else if (state === "REDACTED_LOCAL_ONLY") {
    mappedBody = { state, redactedText: text(body.redacted_text), redactionCount: number(body.redaction_count), uploadEligible: literal(body.upload_eligible, false) };
  } else {
    throw new Error("invalid log body state");
  }
  return {
    metadata: { observedAt: text(metadata.observed_at), source: text(metadata.source), severity: severity(metadata.severity), eventCode: text(metadata.event_code) },
    body: mappedBody
  };
}

function parseProblem(bytes: Uint8Array, responseStatus: number): ObserverProblemDetails | undefined {
  try {
    const item = object(JSON.parse(new TextDecoder().decode(bytes)) as unknown);
    const status = number(item.status);
    if (status !== responseStatus) return undefined;
    const fields = item.fields === undefined ? undefined : array(item.fields).map((entry) => {
      const field = object(entry);
      return { field: text(field.field), message: text(field.message), code: text(field.code) };
    });
    return { type: text(item.type), title: text(item.title), status, ...(item.detail === undefined ? {} : { detail: text(item.detail) }), ...(item.instance === undefined ? {} : { instance: text(item.instance) }), code: text(item.code), requestId: text(item.request_id), ...(fields ? { fields } : {}) };
  } catch {
    return undefined;
  }
}

function normalizeBaseUrl(baseUrl: string): string {
  if (baseUrl === "" || baseUrl === "/") return "";
  const url = new URL(baseUrl);
  if (url.protocol !== "http:" && url.protocol !== "https:") throw new TypeError("Local observer URL must use HTTP or HTTPS");
  if (url.username || url.password) throw new TypeError("Local observer URL must not contain credentials");
  if (!LOOPBACK_HOSTS.has(url.hostname)) throw new TypeError("Local observer URL must use a loopback host");
  if (url.pathname !== "/" || url.search || url.hash) throw new TypeError("Local observer URL must not contain a path, query, or fragment");
  return url.origin;
}

function object(value: unknown): Record<string, unknown> { if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("expected object"); return value as Record<string, unknown>; }
function exactKeys(value: Record<string, unknown>, allowed: string[]): void { const expected = new Set(allowed); if (Object.keys(value).some((key) => !expected.has(key)) || allowed.some((key) => !Object.hasOwn(value, key))) throw new Error("unexpected or missing contract field"); }
function array(value: unknown): unknown[] { if (!Array.isArray(value)) throw new Error("expected array"); return value; }
function boundedArray(value: unknown, minimum: number, maximum: number): unknown[] { const items = array(value); if (items.length < minimum || items.length > maximum) throw new Error("array exceeds contract bounds"); return items; }
function text(value: unknown): string { if (typeof value !== "string") throw new Error("expected string"); return value; }
function nullableText(value: unknown): string | null { return value === null ? null : text(value); }
function number(value: unknown): number { if (typeof value !== "number" || !Number.isFinite(value)) throw new Error("expected number"); return value; }
function nullableNonNegativeNumber(value: unknown): number | null { if (value === null) return null; const result = number(value); if (result < 0) throw new Error("expected non-negative number"); return result; }
function integer(value: unknown, minimum: number, maximum: number): number { const result = number(value); if (!Number.isInteger(result) || result < minimum || result > maximum) throw new Error("integer exceeds contract bounds"); return result; }
function boolean(value: unknown): boolean { if (typeof value !== "boolean") throw new Error("expected boolean"); return value; }
function literal<T extends string | number | boolean>(value: unknown, expected: T): T { if (value !== expected) throw new Error("unexpected literal"); return expected; }
function isSectionName(value: string): value is SectionName { return (SECTION_NAMES as string[]).includes(value); }
function isTrendRange(value: unknown): value is TrendRange { return value === "1h" || value === "6h" || value === "24h" || value === "7d"; }
function trendRange(value: unknown): TrendRange { if (!isTrendRange(value)) throw new Error("invalid trend range"); return value; }
function metricId(value: unknown): MetricId { if (typeof value !== "string" || !(METRIC_IDS as string[]).includes(value)) throw new Error("invalid metric identifier"); return value as MetricId; }
function trendUnit(value: unknown): TrendSeries["unit"] { if (value !== "percent" && value !== "bytes_per_second" && value !== "count") throw new Error("invalid metric unit"); return value; }
function seriesSupport(value: unknown): TrendSeries["supportState"] { if (value !== "SUPPORTED" && value !== "DISABLED" && value !== "UNAVAILABLE" && value !== "PERMISSION_DENIED" && value !== "UNSUPPORTED") throw new Error("invalid metric support state"); return value; }
function seriesCollection(value: unknown): TrendSeries["collectionState"] { if (value !== "OK" && value !== "PARTIAL" && value !== "FAILED" && value !== "NOT_RUN") throw new Error("invalid metric collection state"); return value; }
function seriesFreshnessValue(value: unknown): Freshness { if (value !== "CURRENT" && value !== "STALE" && value !== "UNKNOWN") throw new Error("invalid metric freshness"); return value; }
function reason(value: unknown): string | null { const result = nullableText(value); if (result !== null && !/^[A-Z0-9_]{1,64}$/.test(result)) throw new Error("invalid reason code"); return result; }
function dateTime(value: unknown): string { const result = text(value); if (!result.endsWith("Z") || !Number.isFinite(Date.parse(result))) throw new Error("invalid UTC date-time"); return result; }
function support(value: unknown): SupportState { return value === "SUPPORTED" || value === "DISABLED" || value === "UNAVAILABLE" || value === "PERMISSION_DENIED" || value === "UNSUPPORTED" ? value : "UNKNOWN"; }
function collection(value: unknown): CollectionState { return value === "OK" || value === "PARTIAL" || value === "FAILED" || value === "NOT_RUN" ? value : "UNKNOWN"; }
function freshness(value: unknown): Freshness { return value === "CURRENT" || value === "STALE" ? value : "UNKNOWN"; }
function severity(value: unknown): Severity { return value === "TRACE" || value === "DEBUG" || value === "INFO" || value === "WARN" || value === "ERROR" || value === "CRITICAL" ? value : "UNKNOWN"; }
function classification(value: unknown): ObserverCapabilities["collectors"][number]["dataClassification"] { return value === "PUBLIC_METADATA" || value === "LOCAL_SENSITIVE" ? value : "UNKNOWN"; }
