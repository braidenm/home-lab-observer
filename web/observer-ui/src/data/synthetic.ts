import type { CurrentSnapshot, DiagnosticsHealth, ObserverCapabilities, ObserverDataSource, TrendRange, TrendSnapshot } from "../types";

const observedAt = "2026-09-09T18:42:00.000Z";
const before = (minutes: number) => new Date(Date.parse(observedAt) - minutes * 60_000).toISOString();
const quality = { supportState: "SUPPORTED", collectionState: "OK", freshness: "CURRENT", observedAt, reasonCode: null } as const;

export const syntheticCapabilities: ObserverCapabilities = {
  schemaVersion: "observer-capabilities/v1",
  apiVersion: "v1",
  generatedAt: observedAt,
  observer: { version: "0.2.0-preview.3", mode: "LOCAL_DASHBOARD" },
  platform: { os: "linux", architecture: "amd64" },
  policy: {
    readOnly: true,
    defaultBind: "127.0.0.1",
    corsEnabled: false,
    retentionDays: 1,
    retentionBytes: 262_144_000,
    logBodies: { defaultState: "OMITTED", maximumBodyBytes: 2048, uploadEligible: false },
    responseLimitsBytes: { capabilities: 131_072, currentSnapshot: 1_048_576 }
  },
  collectors: [
    { name: "overview", supportState: "SUPPORTED", reasonCode: null, dataClassification: "PUBLIC_METADATA", uploadEligible: true },
    { name: "filesystems", supportState: "SUPPORTED", reasonCode: null, dataClassification: "PUBLIC_METADATA", uploadEligible: true },
    { name: "processes", supportState: "SUPPORTED", reasonCode: null, dataClassification: "LOCAL_SENSITIVE", uploadEligible: false },
    { name: "services", supportState: "UNAVAILABLE", reasonCode: "SERVICE_MANAGER_NOT_FOUND", dataClassification: "PUBLIC_METADATA", uploadEligible: true },
    { name: "containers", supportState: "SUPPORTED", reasonCode: null, dataClassification: "PUBLIC_METADATA", uploadEligible: true },
    { name: "logs", supportState: "SUPPORTED", reasonCode: null, dataClassification: "LOCAL_SENSITIVE", uploadEligible: false },
    { name: "observer", supportState: "SUPPORTED", reasonCode: null, dataClassification: "PUBLIC_METADATA", uploadEligible: true }
  ]
};

export const syntheticSnapshot: CurrentSnapshot = {
  schemaVersion: "observer-current-snapshot/v1",
  snapshotId: "synthetic_demo_001",
  sequence: 42,
  observedAt,
  durationMilliseconds: 184,
  collectionState: "PARTIAL",
  privacy: {
    profile: "SAFE_DEFAULT",
    remoteProjection: "home-lab-server-snapshot/v1",
    messageBodiesUploadEligible: false,
    redactionCount: 38,
    droppedCount: 4,
    excludedFields: ["process_arguments", "environment_variables", "container_commands", "container_mounts", "container_labels", "raw_log_bodies", "credentials", "tokens"]
  },
  sections: {
    overview: {
      ...quality,
      data: { hostAlias: "studio-node", os: "linux", architecture: "amd64", uptimeSeconds: 1_147_860, cpuLogicalCount: 8, memoryTotalBytes: 17_179_869_184, memoryUsedBytes: 11_716_624_384 }
    },
    filesystems: {
      ...quality, totalCount: 2, returnedCount: 2, truncated: false,
      items: [
        { mountAlias: "system", filesystemType: "ext4", totalBytes: 536_870_912_000, usedBytes: 400_505_700_352, availableBytes: 136_365_211_648 },
        { mountAlias: "archive", filesystemType: "xfs", totalBytes: 2_199_023_255_552, usedBytes: 1_099_511_627_776, availableBytes: 1_099_511_627_776 }
      ]
    },
    processes: {
      ...quality, totalCount: 231, returnedCount: 4, truncated: true,
      items: [
        { pid: 1842, name: "postgres", state: "running", cpuPercent: 12.8, memoryBytes: 1_442_840_576 },
        { pid: 2941, name: "platform-demo", state: "running", cpuPercent: 8.1, memoryBytes: 986_710_016 },
        { pid: 702, name: "dockerd", state: "running", cpuPercent: 3.6, memoryBytes: 422_576_128 },
        { pid: 3110, name: "node", state: "sleeping", cpuPercent: 1.2, memoryBytes: 271_581_184 }
      ]
    },
    services: {
      supportState: "UNAVAILABLE", collectionState: "NOT_RUN", freshness: "UNKNOWN", observedAt: null,
      reasonCode: "SERVICE_MANAGER_NOT_FOUND", totalCount: 0, returnedCount: 0, truncated: false, items: []
    },
    containers: {
      ...quality, totalCount: 3, returnedCount: 3, truncated: false,
      items: [
        { idAlias: "container-001", name: "platform-demo", image: "ghcr.io/example/platform-demo@sha256:8c1…9a7", state: "running", cpuPercent: 7.8, memoryBytes: 943_718_400 },
        { idAlias: "container-002", name: "postgres", image: "postgres@sha256:1ae…3f2", state: "running", cpuPercent: 12.5, memoryBytes: 1_396_703_232 },
        { idAlias: "container-003", name: "media-worker", image: "ghcr.io/example/media-worker@sha256:7b4…110", state: "running", cpuPercent: 4.9, memoryBytes: 335_544_320 }
      ]
    },
    logs: {
      ...quality, totalCount: 5, returnedCount: 5, truncated: false,
      items: [
        { metadata: { observedAt: before(3), source: "observer", severity: "INFO", eventCode: "COLLECTION_COMPLETE" }, body: { state: "OMITTED" } },
        { metadata: { observedAt: before(9), source: "docker", severity: "WARN", eventCode: "CONTAINER_RESTART" }, body: { state: "OMITTED" } },
        { metadata: { observedAt: before(16), source: "systemd", severity: "INFO", eventCode: "UNIT_ACTIVE" }, body: { state: "OMITTED" } },
        { metadata: { observedAt: before(21), source: "observer", severity: "WARN", eventCode: "UTILIZATION_THRESHOLD" }, body: { state: "OMITTED" } },
        { metadata: { observedAt: before(47), source: "systemd", severity: "ERROR", eventCode: "UNIT_FAILED" }, body: { state: "OMITTED" } }
      ]
    },
    observer: {
      ...quality, totalCount: 3, returnedCount: 3, truncated: false,
      items: [
        { name: "collection_duration", state: "OK", value: 184, unit: "milliseconds" },
        { name: "redacted_fields", state: "WARN", value: 38, unit: "count" },
        { name: "dropped_records", state: "WARN", value: 4, unit: "count" }
      ]
    }
  }
};

const trendPoints = (values: Array<number | null>) => values.map((value, index) => ({ at: before((values.length - index - 1) * 30), value }));
export const syntheticTrends: TrendSnapshot = {
  schemaVersion: "observer-metric-series/v1",
  range: "6h",
  generatedAt: observedAt,
  windowStart: before(360),
  windowEnd: observedAt,
  sampleIntervalSeconds: 1_800,
  limits: { maxMetrics: 6, maxPointsPerSeries: 2048, maxResponseBytes: 1048576 },
  privacy: { dataClassification: "PUBLIC_METADATA", containsProcessIdentity: false, containsLogBodies: false, remoteUploadEligible: false },
  requestedMetrics: ["memory.utilization.percent", "filesystem.aggregate.utilization.percent"],
  series: [
    { id: "memory.utilization.percent", label: "Memory utilization", unit: "percent", color: "cyan", supportState: "SUPPORTED", collectionState: "PARTIAL", freshness: "CURRENT", reasonCode: "SAMPLE_GAP", pointCount: 12, gapCount: 1, truncated: false, points: trendPoints([59, 60, 61, null, 62, 63, 65, 65, 66, 67, 68, 68]) },
    { id: "filesystem.aggregate.utilization.percent", label: "Aggregate filesystem utilization", unit: "percent", color: "amber", supportState: "SUPPORTED", collectionState: "OK", freshness: "CURRENT", reasonCode: null, pointCount: 12, gapCount: 0, truncated: false, points: trendPoints([70, 70.4, 70.9, 71.2, 71.7, 72.1, 72.8, 73, 73.5, 74, 74.3, 74.6]) }
  ]
};

export const syntheticDiagnosticsHealth: DiagnosticsHealth = {
  schemaVersion: "observer-diagnostics-health/v1",
  generatedAt: observedAt,
  enabled: true,
  available: true,
  state: "AVAILABLE",
  reasonCode: null,
  limits: { maxFiles: 5, maxFileBytes: 2_097_152, maxTotalBytes: 10_485_760, maxRecordBytes: 8_192, maxAgeSeconds: 604_800 },
  usage: { totalBytes: 32_768, fileCount: 1 },
  counters: { droppedRecords: 2, writeFailures: 0 },
  policy: { dataClassification: "PUBLIC_METADATA", containsLogContents: false, containsPaths: false, remoteUploadEligible: false }
};

export class SyntheticObserverDataSource implements ObserverDataSource {
  async getCapabilities(): Promise<ObserverCapabilities> { return structuredClone(syntheticCapabilities); }
  async getCurrentSnapshot(): Promise<CurrentSnapshot> { return structuredClone(syntheticSnapshot); }
  async getTrends(range: TrendRange): Promise<TrendSnapshot> {
    const seconds = { "1h": 3600, "6h": 21600, "24h": 86400, "7d": 604800 }[range];
    const windowStart = new Date(Date.parse(observedAt) - seconds * 1000).toISOString();
    const series = structuredClone(syntheticTrends.series).map((item) => {
      const points = item.points.filter((point) => Date.parse(point.at) >= Date.parse(windowStart));
      return { ...item, points, pointCount: points.length, gapCount: points.filter((point) => point.value === null).length };
    });
    return { ...structuredClone(syntheticTrends), range, windowStart, series };
  }
  async getDiagnosticsHealth(): Promise<DiagnosticsHealth> { return structuredClone(syntheticDiagnosticsHealth); }
}
