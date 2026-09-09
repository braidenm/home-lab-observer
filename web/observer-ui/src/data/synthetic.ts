import type {
  LogQuery,
  LogSnapshot,
  ObserverDataSource,
  ObserverHealthSnapshot,
  OverviewSnapshot,
  Severity,
  TrendRange,
  TrendSnapshot,
  WorkloadQuery,
  WorkloadSnapshot
} from "../types";

const now = new Date("2026-09-09T18:42:00.000Z");
const isoBefore = (minutes: number) => new Date(now.getTime() - minutes * 60_000).toISOString();

export const syntheticOverview: OverviewSnapshot = {
  schemaVersion: "observer-overview/v1",
  observedAt: now.toISOString(),
  freshnessSeconds: 4,
  status: "attention",
  host: {
    displayName: "studio-node",
    operatingSystem: "Ubuntu 24.04.3 LTS",
    architecture: "amd64",
    kernel: "6.8.0-79-generic",
    uptimeSeconds: 1_147_860
  },
  metrics: [
    {
      id: "cpu",
      label: "CPU",
      measurement: { value: 37.4, unit: "%", availability: "supported" },
      detail: "8 logical cores · load 2.18",
      trend: "steady"
    },
    {
      id: "memory",
      label: "Memory",
      measurement: { value: 68.2, unit: "%", availability: "supported" },
      detail: "10.9 GB of 16 GB",
      trend: "rising"
    },
    {
      id: "storage",
      label: "Primary storage",
      measurement: { value: 74.6, unit: "%", availability: "supported" },
      detail: "119 GB free · /data",
      trend: "rising"
    },
    {
      id: "temperature",
      label: "Package temp",
      measurement: { value: 53, unit: "celsius", availability: "supported" },
      detail: "Peak 61 °C in 24 hours",
      trend: "falling"
    }
  ],
  notices: [
    {
      id: "notice-storage",
      state: "attention",
      title: "Storage pressure is building",
      detail: "/data grew 8.2 GB over the last 24 hours.",
      occurredAt: isoBefore(21)
    },
    {
      id: "notice-restart",
      state: "healthy",
      title: "Media worker recovered",
      detail: "Container restarted once and has remained healthy for 3 hours.",
      occurredAt: isoBefore(182)
    }
  ]
};

const trendPoints = (values: number[]) =>
  values.map((value, index) => ({ at: isoBefore((values.length - index - 1) * 30), value }));

export const syntheticTrends: TrendSnapshot = {
  schemaVersion: "observer-trends/v1",
  range: "6h",
  sampledEverySeconds: 1_800,
  series: [
    {
      id: "cpu",
      label: "CPU utilization",
      unit: "%",
      availability: "supported",
      color: "mint",
      points: trendPoints([22, 26, 31, 28, 34, 48, 43, 39, 55, 46, 41, 37])
    },
    {
      id: "memory",
      label: "Memory utilization",
      unit: "%",
      availability: "supported",
      color: "cyan",
      points: trendPoints([59, 60, 61, 61, 62, 63, 65, 65, 66, 67, 68, 68])
    },
    {
      id: "storage",
      label: "Storage utilization",
      unit: "%",
      availability: "supported",
      color: "amber",
      points: trendPoints([70, 70.4, 70.9, 71.2, 71.7, 72.1, 72.8, 73, 73.5, 74, 74.3, 74.6])
    },
    {
      id: "temperature",
      label: "Package temperature",
      unit: "celsius",
      availability: "supported",
      color: "violet",
      points: trendPoints([44, 46, 49, 48, 54, 61, 58, 55, 57, 56, 54, 53])
    }
  ],
  annotations: [
    { id: "deploy", at: isoBefore(151), severity: "info", label: "media-api deployed", source: "systemd" },
    { id: "restart", at: isoBefore(107), severity: "warning", label: "media-worker restarted", source: "docker" },
    { id: "disk", at: isoBefore(21), severity: "warning", label: "/data crossed 74%", source: "observer" }
  ]
};

export const syntheticWorkloads: WorkloadSnapshot = {
  schemaVersion: "observer-workloads/v1",
  observedAt: now.toISOString(),
  limits: { processes: 200, services: 250, containers: 500 },
  items: [
    { kind: "process", id: "p-1842", processId: 1842, name: "postgres", account: "postgres", state: "running", cpuPercent: 12.8, memoryBytes: 1_442_840_576, startedAt: isoBefore(16_420) },
    { kind: "process", id: "p-2941", processId: 2941, name: "platform-demo", account: "app", state: "running", cpuPercent: 8.1, memoryBytes: 986_710_016, startedAt: isoBefore(3_220) },
    { kind: "process", id: "p-702", processId: 702, name: "dockerd", account: "root", state: "running", cpuPercent: 3.6, memoryBytes: 422_576_128, startedAt: isoBefore(16_430) },
    { kind: "process", id: "p-3110", processId: 3110, name: "node", account: "portfolio", state: "sleeping", cpuPercent: 1.2, memoryBytes: 271_581_184, startedAt: isoBefore(970) },
    { kind: "service", id: "s-observer", name: "home-lab-observer", manager: "systemd", state: "active", startup: "enabled", cpuPercent: 0.4, memoryBytes: 41_943_040, startedAt: isoBefore(9_580), restartCount: 0 },
    { kind: "service", id: "s-docker", name: "docker", manager: "systemd", state: "active", startup: "enabled", cpuPercent: 3.6, memoryBytes: 422_576_128, startedAt: isoBefore(16_430), restartCount: 0 },
    { kind: "service", id: "s-backup", name: "homelab-backup", manager: "systemd", state: "inactive", startup: "timer", cpuPercent: null, memoryBytes: null, startedAt: null, restartCount: 0 },
    { kind: "service", id: "s-smart", name: "smart-monitor", manager: "systemd", state: "denied", startup: "enabled", cpuPercent: null, memoryBytes: null, startedAt: null, restartCount: null },
    { kind: "container", id: "c-platform", name: "platform-demo", image: "ghcr.io/example/platform-demo@sha256:8c1…9a7", state: "running", health: "healthy", cpuPercent: 7.8, memoryBytes: 943_718_400, startedAt: isoBefore(3_220), restartCount: 0 },
    { kind: "container", id: "c-postgres", name: "postgres", image: "postgres@sha256:1ae…3f2", state: "running", health: "healthy", cpuPercent: 12.5, memoryBytes: 1_396_703_232, startedAt: isoBefore(16_420), restartCount: 0 },
    { kind: "container", id: "c-media", name: "media-worker", image: "ghcr.io/example/media-worker@sha256:7b4…110", state: "running", health: "healthy", cpuPercent: 4.9, memoryBytes: 335_544_320, startedAt: isoBefore(182), restartCount: 1 },
    { kind: "container", id: "c-cache", name: "redis-cache", image: "redis@sha256:992…48a", state: "running", health: null, cpuPercent: 0.7, memoryBytes: 96_468_992, startedAt: isoBefore(8_310), restartCount: 0 },
    { kind: "container", id: "c-migrate", name: "schema-migration", image: "ghcr.io/example/schema@sha256:21c…dd8", state: "exited", health: null, cpuPercent: 0, memoryBytes: 0, startedAt: isoBefore(3_228), restartCount: 0 }
  ]
};

export const syntheticLogs: LogSnapshot = {
  schemaVersion: "observer-logs/v1",
  bodyStorageDefault: "off",
  sources: [
    { id: "observer", label: "Observer", availability: "supported", bodyStorage: "off" },
    { id: "systemd", label: "systemd", availability: "supported", bodyStorage: "off" },
    { id: "docker", label: "Docker", availability: "supported", bodyStorage: "off" },
    { id: "kernel", label: "Kernel", availability: "permission-denied", bodyStorage: "off", reason: "Additional permission is required" }
  ],
  events: [
    { id: "l1", at: isoBefore(3), severity: "info", sourceId: "observer", sourceLabel: "Observer", unit: "collector", code: "COLLECTION_COMPLETE", summary: "Collection cycle completed", structuredFields: { duration_ms: 184, collectors: 8 } },
    { id: "l2", at: isoBefore(9), severity: "warning", sourceId: "docker", sourceLabel: "Docker", unit: "media-worker", code: "CONTAINER_RESTART", summary: "Container restart count changed", structuredFields: { previous: 0, current: 1 } },
    { id: "l3", at: isoBefore(16), severity: "info", sourceId: "systemd", sourceLabel: "systemd", unit: "platform-demo.service", code: "UNIT_ACTIVE", summary: "Service entered active state", structuredFields: { result: "success" } },
    { id: "l4", at: isoBefore(21), severity: "warning", sourceId: "observer", sourceLabel: "Observer", unit: "filesystem", code: "UTILIZATION_THRESHOLD", summary: "Filesystem utilization crossed a configured threshold", structuredFields: { mount: "/data", utilization_percent: 74.1 } },
    { id: "l5", at: isoBefore(47), severity: "error", sourceId: "systemd", sourceLabel: "systemd", unit: "backup.service", code: "UNIT_FAILED", summary: "Service exited unsuccessfully", structuredFields: { result: "exit-code", status: 1 } },
    { id: "l6", at: isoBefore(53), severity: "info", sourceId: "observer", sourceLabel: "Observer", unit: "redactor", code: "FIELDS_REDACTED", summary: "Sensitive structured fields were removed", structuredFields: { fields: 3, policy: "default" } }
  ],
  redactedFieldCount: 38,
  droppedEventCount: 4
};

export const syntheticHealth: ObserverHealthSnapshot = {
  schemaVersion: "observer-health/v1",
  status: "healthy",
  version: "0.2.0-preview.3",
  buildCommit: "66f31cc",
  startedAt: isoBefore(9_580),
  collectors: [
    { id: "host", label: "Host", availability: "supported", lastSuccessAt: isoBefore(1), durationMilliseconds: 18 },
    { id: "processes", label: "Processes", availability: "supported", lastSuccessAt: isoBefore(1), durationMilliseconds: 31 },
    { id: "services", label: "Services", availability: "supported", lastSuccessAt: isoBefore(1), durationMilliseconds: 47 },
    { id: "containers", label: "Containers", availability: "supported", lastSuccessAt: isoBefore(1), durationMilliseconds: 72 },
    { id: "sensors", label: "Hardware sensors", availability: "unavailable", lastSuccessAt: null, durationMilliseconds: null, reason: "No compatible sensor adapter detected" },
    { id: "kernel-logs", label: "Kernel logs", availability: "permission-denied", lastSuccessAt: null, durationMilliseconds: null, reason: "Message bodies remain disabled" }
  ],
  storage: {
    usedBytes: 46_137_344,
    limitBytes: 268_435_456,
    oldestRecordAt: isoBefore(1_440),
    retentionHours: 24
  },
  counters: { collectionFailures: 2, redactedFields: 38, droppedRecords: 4, queuedUploads: 0 },
  remoteUpload: {
    enabled: false,
    state: "disabled",
    policySummary: "Local only. No observations leave this machine."
  },
  privacy: {
    localOnly: true,
    processArgumentsCollected: false,
    environmentCollected: false,
    logBodiesEnabledSources: [],
    publicListener: false
  }
};

export class SyntheticObserverDataSource implements ObserverDataSource {
  async getOverview(): Promise<OverviewSnapshot> {
    return structuredClone(syntheticOverview);
  }

  async getTrends(range: TrendRange): Promise<TrendSnapshot> {
    return { ...structuredClone(syntheticTrends), range };
  }

  async getWorkloads(query: WorkloadQuery = {}): Promise<WorkloadSnapshot> {
    const search = query.search?.trim().toLowerCase();
    const items = syntheticWorkloads.items.filter(
      (item) =>
        (!query.kind || item.kind === query.kind) &&
        (!search || item.name.toLowerCase().includes(search) || item.state.toLowerCase().includes(search))
    );
    return { ...structuredClone(syntheticWorkloads), items };
  }

  async getLogs(query: LogQuery = {}): Promise<LogSnapshot> {
    const severities = new Set<Severity>(query.severities ?? []);
    const search = query.search?.trim().toLowerCase();
    const events = syntheticLogs.events
      .filter(
        (event) =>
          (!query.source || event.sourceId === query.source) &&
          (severities.size === 0 || severities.has(event.severity)) &&
          (!search || `${event.summary} ${event.code} ${event.unit}`.toLowerCase().includes(search)) &&
          (!query.before || event.at < query.before)
      )
      .slice(0, query.limit ?? 100);
    return { ...structuredClone(syntheticLogs), events };
  }

  async getObserverHealth(): Promise<ObserverHealthSnapshot> {
    return structuredClone(syntheticHealth);
  }
}
