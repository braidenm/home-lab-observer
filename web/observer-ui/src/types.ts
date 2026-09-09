export type Availability = "supported" | "disabled" | "unavailable" | "permission-denied";
export type HealthState = "healthy" | "attention" | "degraded" | "offline";
export type Severity = "debug" | "info" | "warning" | "error" | "critical";
export type WorkloadKind = "process" | "service" | "container";
export type TrendRange = "1h" | "6h" | "24h" | "7d";

export interface Measurement {
  value: number | null;
  unit: "%" | "bytes" | "celsius" | "count" | "bytes-per-second";
  availability: Availability;
  reason?: string;
}

export interface HostIdentity {
  displayName: string;
  operatingSystem: string;
  architecture: string;
  kernel: string;
  uptimeSeconds: number;
}

export interface OverviewMetric {
  id: string;
  label: string;
  measurement: Measurement;
  detail: string;
  trend: "rising" | "steady" | "falling";
}

export interface StatusNotice {
  id: string;
  state: HealthState;
  title: string;
  detail: string;
  occurredAt: string;
}

export interface OverviewSnapshot {
  schemaVersion: "observer-overview/v1";
  observedAt: string;
  freshnessSeconds: number;
  status: HealthState;
  host: HostIdentity;
  metrics: OverviewMetric[];
  notices: StatusNotice[];
}

export interface TrendPoint {
  at: string;
  value: number | null;
}

export interface TrendAnnotation {
  id: string;
  at: string;
  severity: Severity;
  label: string;
  source: string;
}

export interface TrendSeries {
  id: string;
  label: string;
  unit: Measurement["unit"];
  availability: Availability;
  color: "mint" | "cyan" | "amber" | "violet";
  points: TrendPoint[];
}

export interface TrendSnapshot {
  schemaVersion: "observer-trends/v1";
  range: TrendRange;
  sampledEverySeconds: number;
  series: TrendSeries[];
  annotations: TrendAnnotation[];
}

interface WorkloadBase {
  id: string;
  name: string;
  state: string;
  cpuPercent: number | null;
  memoryBytes: number | null;
  startedAt: string | null;
}

export interface ProcessWorkload extends WorkloadBase {
  kind: "process";
  processId: number;
  account: string;
}

export interface ServiceWorkload extends WorkloadBase {
  kind: "service";
  manager: "systemd" | "launchd" | "windows-service";
  startup: string;
  restartCount: number | null;
}

export interface ContainerWorkload extends WorkloadBase {
  kind: "container";
  image: string;
  health: string | null;
  restartCount: number;
}

export type Workload = ProcessWorkload | ServiceWorkload | ContainerWorkload;

export interface WorkloadSnapshot {
  schemaVersion: "observer-workloads/v1";
  observedAt: string;
  limits: {
    processes: number;
    services: number;
    containers: number;
  };
  items: Workload[];
}

export interface LogSourcePolicy {
  id: string;
  label: string;
  availability: Availability;
  bodyStorage: "off" | "redacted";
  reason?: string;
}

export interface LogEvent {
  id: string;
  at: string;
  severity: Severity;
  sourceId: string;
  sourceLabel: string;
  unit: string;
  code: string;
  summary: string;
  structuredFields: Record<string, string | number | boolean | null>;
  body?: string;
}

export interface LogSnapshot {
  schemaVersion: "observer-logs/v1";
  bodyStorageDefault: "off";
  sources: LogSourcePolicy[];
  events: LogEvent[];
  redactedFieldCount: number;
  droppedEventCount: number;
}

export interface CollectorHealth {
  id: string;
  label: string;
  availability: Availability;
  lastSuccessAt: string | null;
  durationMilliseconds: number | null;
  reason?: string;
}

export interface ObserverHealthSnapshot {
  schemaVersion: "observer-health/v1";
  status: HealthState;
  version: string;
  buildCommit: string;
  startedAt: string;
  collectors: CollectorHealth[];
  storage: {
    usedBytes: number;
    limitBytes: number;
    oldestRecordAt: string;
    retentionHours: number;
  };
  counters: {
    collectionFailures: number;
    redactedFields: number;
    droppedRecords: number;
    queuedUploads: number;
  };
  remoteUpload: {
    enabled: boolean;
    state: "disabled" | "connected" | "pending" | "rejected";
    destination?: string;
    policySummary: string;
  };
  privacy: {
    localOnly: boolean;
    processArgumentsCollected: false;
    environmentCollected: false;
    logBodiesEnabledSources: string[];
    publicListener: false;
  };
}

export interface WorkloadQuery {
  kind?: WorkloadKind;
  search?: string;
}

export interface LogQuery {
  source?: string;
  severities?: Severity[];
  search?: string;
  before?: string;
  limit?: number;
}

export interface ObserverDataSource {
  getOverview(signal?: AbortSignal): Promise<OverviewSnapshot>;
  getTrends(range: TrendRange, signal?: AbortSignal): Promise<TrendSnapshot>;
  getWorkloads(query?: WorkloadQuery, signal?: AbortSignal): Promise<WorkloadSnapshot>;
  getLogs(query?: LogQuery, signal?: AbortSignal): Promise<LogSnapshot>;
  getObserverHealth(signal?: AbortSignal): Promise<ObserverHealthSnapshot>;
}

export interface DashboardData {
  overview: OverviewSnapshot;
  trends: TrendSnapshot;
  workloads: WorkloadSnapshot;
  logs: LogSnapshot;
  health: ObserverHealthSnapshot;
}
