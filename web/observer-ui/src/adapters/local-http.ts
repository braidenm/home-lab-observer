import type { LocalHttpObserverDataSourceOptions } from "./local-http.types";
import type {
  CollectionState,
  ContainerInventory,
  ContainerInventoryItem,
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
const CONTAINER_INVENTORY_MAX_BYTES = 1_048_576;
const SECTION_NAMES: SectionName[] = ["overview", "filesystems", "processes", "services", "containers", "logs", "observer"];
const METRIC_IDS: MetricId[] = [
  "cpu.utilization.percent",
  "memory.utilization.percent",
  "filesystem.aggregate.utilization.percent",
  "network.receive.bytes_per_second",
  "network.transmit.bytes_per_second",
  "process.count"
];
const EXCLUDED_FIELDS: CurrentSnapshot["privacy"]["excludedFields"] = [
  "process_arguments",
  "environment_variables",
  "container_commands",
  "container_mounts",
  "container_labels",
  "raw_log_bodies",
  "credentials",
  "tokens"
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

  getContainerInventory(signal?: AbortSignal): Promise<ContainerInventory> {
    return this.request("/api/v1/containers?limit=500", CONTAINER_INVENTORY_MAX_BYTES, mapContainerInventory, signal);
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
    const bytes = await readBoundedResponse(response, maximumBytes);
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

async function readBoundedResponse(response: Response, maximumBytes: number): Promise<Uint8Array> {
  const tooLarge = () => new ObserverTransportError("Observer API response exceeded its documented size limit", response.status);
  const declared = response.headers.get("content-length");
  if (declared !== null && Number(declared) > maximumBytes) {
    await response.body?.cancel();
    throw tooLarge();
  }
  if (!response.body) return new Uint8Array();
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;
  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > maximumBytes) {
        await reader.cancel();
        throw tooLarge();
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  const bytes = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
  return bytes;
}

export function mapCapabilities(value: unknown): ObserverCapabilities {
  const root = object(value);
  exactKeys(root, ["schema_version", "api_version", "generated_at", "observer", "platform", "policy", "collectors"]);
  const observer = object(root.observer);
  const platform = object(root.platform);
  const policy = object(root.policy);
  const bodies = object(policy.log_bodies);
  const limits = object(policy.response_limits_bytes);
  exactKeys(observer, ["version", "mode"]);
  exactKeys(platform, ["os", "architecture"]);
  exactKeys(policy, ["read_only", "default_bind", "cors_enabled", "retention_days", "retention_bytes", "log_bodies", "response_limits_bytes"]);
  exactKeys(bodies, ["default_state", "maximum_body_bytes", "upload_eligible"]);
  exactKeys(limits, ["capabilities", "current_snapshot"]);
  const seen = new Set<SectionName>();
  const collectors = boundedArray(root.collectors, 7, 7).map((entry) => {
    const item = object(entry);
    exactKeys(item, ["name", "support_state", "reason_code", "data_classification", "upload_eligible"]);
    const name = sectionName(item.name);
    if (seen.has(name)) throw new Error("duplicate collector");
    seen.add(name);
    return {
      name,
      supportState: support(item.support_state),
      reasonCode: reason(item.reason_code),
      dataClassification: classification(item.data_classification),
      uploadEligible: boolean(item.upload_eligible)
    };
  });
  if (SECTION_NAMES.some((name) => !seen.has(name))) throw new Error("missing collector");
  return {
    schemaVersion: literal(root.schema_version, "observer-capabilities/v1"),
    apiVersion: literal(root.api_version, "v1"),
    generatedAt: rfc3339DateTime(root.generated_at),
    observer: {
      version: boundedText(observer.version, 1, 64),
      mode: observerMode(observer.mode)
    },
    platform: {
      os: operatingSystem(platform.os),
      architecture: boundedText(platform.architecture, 1, 32)
    },
    policy: {
      readOnly: literal(policy.read_only, true),
      defaultBind: literal(policy.default_bind, "127.0.0.1"),
      corsEnabled: literal(policy.cors_enabled, false),
      retentionDays: integer(policy.retention_days, 0, 7),
      retentionBytes: integer(policy.retention_bytes, 0, 262_144_000),
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
  exactKeys(root, ["schema_version", "snapshot_id", "sequence", "observed_at", "duration_ms", "collection_state", "privacy", "sections"]);
  const privacy = object(root.privacy);
  const sections = object(root.sections);
  exactKeys(privacy, ["profile", "remote_projection", "message_bodies_upload_eligible", "redaction_count", "dropped_count", "excluded_fields"]);
  exactKeys(sections, SECTION_NAMES);
  const excludedFields = boundedArray(privacy.excluded_fields, 0, 32).map(excludedField);
  if (new Set(excludedFields).size !== excludedFields.length) throw new Error("duplicate excluded field");
  return {
    schemaVersion: literal(root.schema_version, "observer-current-snapshot/v1"),
    snapshotId: patternText(root.snapshot_id, /^[A-Za-z0-9_-]{8,64}$/, "invalid snapshot identifier"),
    sequence: nonNegativeInteger(root.sequence),
    observedAt: rfc3339DateTime(root.observed_at),
    durationMilliseconds: integer(root.duration_ms, 0, 60_000),
    collectionState: collection(root.collection_state),
    privacy: {
      profile: privacy.profile === "LOCAL_LOG_BODIES" ? "LOCAL_LOG_BODIES" : literal(privacy.profile, "SAFE_DEFAULT"),
      remoteProjection: literal(privacy.remote_projection, "home-lab-server-snapshot/v1"),
      messageBodiesUploadEligible: literal(privacy.message_bodies_upload_eligible, false),
      redactionCount: nonNegativeInteger(privacy.redaction_count),
      droppedCount: nonNegativeInteger(privacy.dropped_count),
      excludedFields
    },
    sections: {
      overview: mapOverviewSection(sections.overview),
      filesystems: mapListSection(sections.filesystems, 16, (entry) => {
        const item = object(entry);
        exactKeys(item, ["mount_alias", "filesystem_type", "total_bytes", "used_bytes", "available_bytes"]);
        return { mountAlias: boundedText(item.mount_alias, 1, 64), filesystemType: boundedText(item.filesystem_type, 1, 32), totalBytes: nonNegativeInteger(item.total_bytes), usedBytes: nonNegativeInteger(item.used_bytes), availableBytes: nonNegativeInteger(item.available_bytes) };
      }),
      processes: mapListSection(sections.processes, 200, (entry) => {
        const item = object(entry);
        exactKeys(item, ["pid", "name", "state", "cpu_percent", "memory_bytes"]);
        return { pid: nonNegativeInteger(item.pid), name: boundedText(item.name, 1, 128), state: boundedText(item.state, 1, 32), cpuPercent: boundedNumber(item.cpu_percent, 0, 100), memoryBytes: nonNegativeInteger(item.memory_bytes) };
      }),
      services: mapListSection(sections.services, 500, (entry) => {
        const item = object(entry);
        exactKeys(item, ["name", "state", "start_mode"]);
        return { name: boundedText(item.name, 1, 128), state: boundedText(item.state, 1, 32), startMode: boundedText(item.start_mode, 1, 32) };
      }),
      containers: mapListSection(sections.containers, 500, (entry) => {
        const item = object(entry);
        exactKeys(item, ["id_alias", "name", "image", "state", "cpu_percent", "memory_bytes"]);
        return { idAlias: boundedText(item.id_alias, 1, 64), name: boundedText(item.name, 1, 128), image: boundedText(item.image, 1, 256), state: boundedText(item.state, 1, 32), cpuPercent: boundedNumber(item.cpu_percent, 0, 100), memoryBytes: nonNegativeInteger(item.memory_bytes) };
      }),
      logs: mapListSection(sections.logs, 200, mapLogRecord),
      observer: mapListSection(sections.observer, 32, (entry) => {
        const item = object(entry);
        exactKeys(item, ["name", "state", "value", "unit"]);
        if (item.state !== "OK" && item.state !== "WARN" && item.state !== "ERROR") throw new Error("invalid observer state");
        if (item.unit !== "count" && item.unit !== "bytes" && item.unit !== "milliseconds" && item.unit !== "percent") throw new Error("invalid observer unit");
        return { name: boundedText(item.name, 1, 64), state: item.state, value: number(item.value), unit: item.unit };
      })
    }
  };
}

export function mapContainerInventory(value: unknown): ContainerInventory {
  const root = object(value);
  exactKeys(root, ["schema_version", "observed_at", "support_state", "collection_state", "freshness", "reason_code", "total_count", "returned_count", "truncated", "items", "policy"]);
  const quality = mapQuality(root);
  const policy = object(root.policy);
  exactKeys(policy, ["read_only", "data_classification", "remote_upload_eligible"]);
  const totalCount = nonNegativeInteger(root.total_count);
  const returnedCount = integer(root.returned_count, 0, 500);
  const truncated = boolean(root.truncated);
  const items = boundedArray(root.items, 0, 500).map(mapContainerInventoryItem);
  if (returnedCount !== items.length || returnedCount > totalCount || (!truncated && totalCount > returnedCount)) {
    throw new Error("container inventory counts are inconsistent");
  }
  assertNonSupportedSectionIsEmpty(quality, totalCount === 0 && returnedCount === 0 && !truncated && items.length === 0);
  return {
    schemaVersion: literal(root.schema_version, "observer-container-inventory/v1"),
    ...quality,
    totalCount,
    returnedCount,
    truncated,
    items,
    policy: {
      readOnly: literal(policy.read_only, true),
      dataClassification: literal(policy.data_classification, "LOCAL_SENSITIVE"),
      remoteUploadEligible: literal(policy.remote_upload_eligible, false)
    }
  };
}

function mapContainerInventoryItem(value: unknown): ContainerInventoryItem {
  const item = object(value);
  exactKeys(item, ["id_alias", "name", "image", "state", "cpu_percent", "memory_bytes", "metrics_state", "reason_code"]);
  const state = containerState(item.state);
  const cpuPercent = item.cpu_percent === null ? null : boundedNumber(item.cpu_percent, 0, 100);
  const memoryBytes = item.memory_bytes === null ? null : nonNegativeInteger(item.memory_bytes);
  const metricsState = containerMetricsState(item.metrics_state);
  const reasonCode = reason(item.reason_code);
  if (metricsState === "AVAILABLE" && (cpuPercent === null || memoryBytes === null || reasonCode !== null)) {
    throw new Error("available container metrics are inconsistent");
  }
  if (metricsState === "PARTIAL" && ((cpuPercent === null) === (memoryBytes === null) || reasonCode === null)) {
    throw new Error("partial container metrics are inconsistent");
  }
  if ((metricsState === "UNAVAILABLE" || metricsState === "NOT_RUNNING") && (cpuPercent !== null || memoryBytes !== null || reasonCode === null)) {
    throw new Error("unavailable container metrics are inconsistent");
  }
  if ((state === "running") === (metricsState === "NOT_RUNNING")) {
    throw new Error("container state and metrics state are inconsistent");
  }
  return {
    idAlias: patternText(item.id_alias, /^ctr_[a-f0-9]{16}$/, "invalid container alias"),
    name: boundedText(item.name, 1, 128),
    image: boundedText(item.image, 1, 256),
    state,
    cpuPercent,
    memoryBytes,
    metricsState,
    reasonCode
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
  exactKeys(item, ["support_state", "collection_state", "freshness", "observed_at", "reason_code", "data"]);
  const quality = mapQuality(item);
  const data = item.data === null ? null : object(item.data);
  if (data !== null) exactKeys(data, ["host_alias", "os", "architecture", "uptime_seconds", "cpu_logical_count", "memory_total_bytes", "memory_used_bytes"]);
  assertNonSupportedSectionIsEmpty(quality, data === null);
  return {
    ...quality,
    data: data === null ? null : {
      hostAlias: boundedText(data.host_alias, 1, 64), os: operatingSystem(data.os),
      architecture: boundedText(data.architecture, 1, 32), uptimeSeconds: nonNegativeInteger(data.uptime_seconds), cpuLogicalCount: integer(data.cpu_logical_count, 1, 4096),
      memoryTotalBytes: nonNegativeInteger(data.memory_total_bytes), memoryUsedBytes: nonNegativeInteger(data.memory_used_bytes)
    }
  };
}

function mapListSection<T>(value: unknown, maximumItems: number, mapItem: (value: unknown) => T): ListSection<T> {
  const item = object(value);
  exactKeys(item, ["support_state", "collection_state", "freshness", "observed_at", "reason_code", "total_count", "returned_count", "truncated", "items"]);
  const quality = mapQuality(item);
  const totalCount = nonNegativeInteger(item.total_count);
  const returnedCount = nonNegativeInteger(item.returned_count);
  const truncated = boolean(item.truncated);
  const items = boundedArray(item.items, 0, maximumItems).map(mapItem);
  if (returnedCount !== items.length || returnedCount > totalCount || (!truncated && totalCount > returnedCount)) throw new Error("list section counts are inconsistent");
  assertNonSupportedSectionIsEmpty(quality, totalCount === 0 && returnedCount === 0 && !truncated && items.length === 0);
  return { ...quality, totalCount, returnedCount, truncated, items };
}

function mapQuality(item: Record<string, unknown>): SectionQuality {
  return { supportState: support(item.support_state), collectionState: collection(item.collection_state), freshness: freshness(item.freshness), observedAt: nullableDateTime(item.observed_at), reasonCode: reason(item.reason_code) };
}

function assertNonSupportedSectionIsEmpty(quality: SectionQuality, empty: boolean): void {
  if (quality.supportState === "SUPPORTED") return;
  if (quality.collectionState !== "NOT_RUN" || quality.freshness !== "UNKNOWN" || quality.observedAt !== null || quality.reasonCode === null || !empty) {
    throw new Error("non-supported section must not imply collected data");
  }
}

function mapLogRecord(value: unknown): LogRecord {
  const item = object(value);
  exactKeys(item, ["metadata", "body"]);
  const metadata = object(item.metadata);
  const body = object(item.body);
  exactKeys(metadata, ["observed_at", "source", "severity", "event_code"]);
  const state = text(body.state);
  let mappedBody: LogBody;
  if (state === "OMITTED") {
    exactKeys(body, ["state"]);
    mappedBody = { state };
  } else if (state === "REDACTED_LOCAL_ONLY") {
    exactKeys(body, ["state", "redacted_text", "redaction_count", "upload_eligible"]);
    mappedBody = { state, redactedText: boundedText(body.redacted_text, 0, 2048), redactionCount: nonNegativeInteger(body.redaction_count), uploadEligible: literal(body.upload_eligible, false) };
  } else {
    throw new Error("invalid log body state");
  }
  return {
    metadata: { observedAt: rfc3339DateTime(metadata.observed_at), source: boundedText(metadata.source, 1, 96), severity: severity(metadata.severity), eventCode: boundedText(metadata.event_code, 1, 64) },
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
// The accepted v1 response schemas are closed. Unknown wire fields are rejected
// here until an accepted schema revision makes them part of this transport.
function exactKeys(value: Record<string, unknown>, allowed: readonly string[]): void { const expected = new Set(allowed); if (Object.keys(value).some((key) => !expected.has(key)) || allowed.some((key) => !Object.hasOwn(value, key))) throw new Error("unexpected or missing contract field"); }
function array(value: unknown): unknown[] { if (!Array.isArray(value)) throw new Error("expected array"); return value; }
function boundedArray(value: unknown, minimum: number, maximum: number): unknown[] { const items = array(value); if (items.length < minimum || items.length > maximum) throw new Error("array exceeds contract bounds"); return items; }
function text(value: unknown): string { if (typeof value !== "string") throw new Error("expected string"); return value; }
function nullableText(value: unknown): string | null { return value === null ? null : text(value); }
function number(value: unknown): number { if (typeof value !== "number" || !Number.isFinite(value)) throw new Error("expected number"); return value; }
function boundedNumber(value: unknown, minimum: number, maximum: number): number { const result = number(value); if (result < minimum || result > maximum) throw new Error("number exceeds contract bounds"); return result; }
function nullableNonNegativeNumber(value: unknown): number | null { if (value === null) return null; const result = number(value); if (result < 0) throw new Error("expected non-negative number"); return result; }
function integer(value: unknown, minimum: number, maximum: number): number { const result = number(value); if (!Number.isInteger(result) || result < minimum || result > maximum) throw new Error("integer exceeds contract bounds"); return result; }
function nonNegativeInteger(value: unknown): number { const result = number(value); if (!Number.isInteger(result) || result < 0) throw new Error("expected non-negative integer"); return result; }
function boolean(value: unknown): boolean { if (typeof value !== "boolean") throw new Error("expected boolean"); return value; }
function literal<T extends string | number | boolean>(value: unknown, expected: T): T { if (value !== expected) throw new Error("unexpected literal"); return expected; }
function isSectionName(value: string): value is SectionName { return (SECTION_NAMES as string[]).includes(value); }
function sectionName(value: unknown): SectionName { const result = text(value); if (!isSectionName(result)) throw new Error("invalid section name"); return result; }
function isTrendRange(value: unknown): value is TrendRange { return value === "1h" || value === "6h" || value === "24h" || value === "7d"; }
function trendRange(value: unknown): TrendRange { if (!isTrendRange(value)) throw new Error("invalid trend range"); return value; }
function metricId(value: unknown): MetricId { if (typeof value !== "string" || !(METRIC_IDS as string[]).includes(value)) throw new Error("invalid metric identifier"); return value as MetricId; }
function containerState(value: unknown): ContainerInventoryItem["state"] { if (value !== "created" && value !== "running" && value !== "paused" && value !== "restarting" && value !== "removing" && value !== "exited" && value !== "dead" && value !== "unknown") throw new Error("invalid container state"); return value; }
function containerMetricsState(value: unknown): ContainerInventoryItem["metricsState"] { if (value !== "AVAILABLE" && value !== "PARTIAL" && value !== "UNAVAILABLE" && value !== "NOT_RUNNING") throw new Error("invalid container metrics state"); return value; }
function trendUnit(value: unknown): TrendSeries["unit"] { if (value !== "percent" && value !== "bytes_per_second" && value !== "count") throw new Error("invalid metric unit"); return value; }
function seriesSupport(value: unknown): TrendSeries["supportState"] { if (value !== "SUPPORTED" && value !== "DISABLED" && value !== "UNAVAILABLE" && value !== "PERMISSION_DENIED" && value !== "UNSUPPORTED") throw new Error("invalid metric support state"); return value; }
function seriesCollection(value: unknown): TrendSeries["collectionState"] { if (value !== "OK" && value !== "PARTIAL" && value !== "FAILED" && value !== "NOT_RUN") throw new Error("invalid metric collection state"); return value; }
function seriesFreshnessValue(value: unknown): Freshness { if (value !== "CURRENT" && value !== "STALE" && value !== "UNKNOWN") throw new Error("invalid metric freshness"); return value; }
function reason(value: unknown): string | null { const result = nullableText(value); if (result !== null && !/^[A-Z0-9_]{1,64}$/.test(result)) throw new Error("invalid reason code"); return result; }
function dateTime(value: unknown): string { const result = rfc3339DateTime(value); if (!result.endsWith("Z")) throw new Error("invalid UTC date-time"); return result; }
function nullableDateTime(value: unknown): string | null { return value === null ? null : rfc3339DateTime(value); }
function rfc3339DateTime(value: unknown): string {
  const result = text(value);
  const match = /^(\d{4})-(\d{2})-(\d{2})[Tt](\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:[Zz]|([+-])(\d{2}):(\d{2}))$/.exec(result);
  if (!match) throw new Error("invalid date-time");
  const [, year, month, day, hour, minute, second, , offsetHour, offsetMinute] = match;
  const yearValue = Number(year);
  const monthValue = Number(month);
  const maximumDay = monthValue >= 1 && monthValue <= 12 ? new Date(Date.UTC(yearValue, monthValue, 0)).getUTCDate() : 0;
  if (Number(day) < 1 || Number(day) > maximumDay || Number(hour) > 23 || Number(minute) > 59 || Number(second) > 59 || (offsetHour !== undefined && (Number(offsetHour) > 23 || Number(offsetMinute) > 59)) || !Number.isFinite(Date.parse(result))) {
    throw new Error("invalid date-time");
  }
  return result;
}
function patternText(value: unknown, pattern: RegExp, message: string): string { const result = text(value); if (!pattern.test(result)) throw new Error(message); return result; }
function boundedText(value: unknown, minimum: number, maximum: number): string { const result = text(value); const length = Array.from(result).length; if (length < minimum || length > maximum) throw new Error("string exceeds contract bounds"); return result; }
function support(value: unknown): SupportState { if (value !== "SUPPORTED" && value !== "DISABLED" && value !== "UNAVAILABLE" && value !== "PERMISSION_DENIED" && value !== "UNSUPPORTED") throw new Error("invalid support state"); return value; }
function collection(value: unknown): CollectionState { if (value !== "OK" && value !== "PARTIAL" && value !== "FAILED" && value !== "NOT_RUN") throw new Error("invalid collection state"); return value; }
function freshness(value: unknown): Freshness { if (value !== "CURRENT" && value !== "STALE" && value !== "UNKNOWN") throw new Error("invalid freshness"); return value; }
function severity(value: unknown): Severity { if (value !== "TRACE" && value !== "DEBUG" && value !== "INFO" && value !== "WARN" && value !== "ERROR" && value !== "CRITICAL" && value !== "UNKNOWN") throw new Error("invalid severity"); return value; }
function classification(value: unknown): ObserverCapabilities["collectors"][number]["dataClassification"] { if (value !== "PUBLIC_METADATA" && value !== "LOCAL_SENSITIVE") throw new Error("invalid data classification"); return value; }
function observerMode(value: unknown): ObserverCapabilities["observer"]["mode"] { if (value !== "HEADLESS" && value !== "LOCAL_DASHBOARD") throw new Error("invalid observer mode"); return value; }
function operatingSystem(value: unknown): ObserverCapabilities["platform"]["os"] { if (value !== "linux" && value !== "windows" && value !== "darwin" && value !== "other") throw new Error("invalid operating system"); return value; }
function excludedField(value: unknown): CurrentSnapshot["privacy"]["excludedFields"][number] { const result = text(value); if (!(EXCLUDED_FIELDS as string[]).includes(result)) throw new Error("invalid excluded field"); return result as CurrentSnapshot["privacy"]["excludedFields"][number]; }
