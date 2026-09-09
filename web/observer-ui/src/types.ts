export type SupportState = "SUPPORTED" | "DISABLED" | "UNAVAILABLE" | "PERMISSION_DENIED" | "UNSUPPORTED" | "UNKNOWN";
export type CollectionState = "OK" | "PARTIAL" | "FAILED" | "NOT_RUN" | "UNKNOWN";
export type Freshness = "CURRENT" | "STALE" | "UNKNOWN";
export type Severity = "TRACE" | "DEBUG" | "INFO" | "WARN" | "ERROR" | "CRITICAL" | "UNKNOWN";
export type SectionName = "overview" | "filesystems" | "processes" | "services" | "containers" | "logs" | "observer";
export type TrendRange = "1h" | "6h" | "24h" | "7d";

export interface CollectorCapability {
  name: SectionName;
  supportState: SupportState;
  reasonCode: string | null;
  dataClassification: "PUBLIC_METADATA" | "LOCAL_SENSITIVE" | "UNKNOWN";
  uploadEligible: boolean;
}

export interface ObserverCapabilities {
  schemaVersion: "observer-capabilities/v1";
  apiVersion: "v1";
  generatedAt: string;
  observer: { version: string; mode: "HEADLESS" | "LOCAL_DASHBOARD" | "UNKNOWN" };
  platform: { os: "linux" | "windows" | "darwin" | "other"; architecture: string };
  policy: {
    readOnly: true;
    defaultBind: "127.0.0.1";
    corsEnabled: false;
    retentionDays: number;
    retentionBytes: number;
    logBodies: { defaultState: "OMITTED"; maximumBodyBytes: 2048; uploadEligible: false };
    responseLimitsBytes: { capabilities: 131072; currentSnapshot: 1048576 };
  };
  collectors: CollectorCapability[];
}

export interface SectionQuality {
  supportState: SupportState;
  collectionState: CollectionState;
  freshness: Freshness;
  observedAt: string | null;
  reasonCode: string | null;
}

export interface ListSection<T> extends SectionQuality {
  totalCount: number;
  returnedCount: number;
  truncated: boolean;
  items: T[];
}

export interface OverviewObservation {
  hostAlias: string;
  os: "linux" | "windows" | "darwin" | "other";
  architecture: string;
  uptimeSeconds: number;
  cpuLogicalCount: number;
  memoryTotalBytes: number;
  memoryUsedBytes: number;
}

export interface OverviewSection extends SectionQuality {
  data: OverviewObservation | null;
}

export interface FilesystemObservation {
  mountAlias: string;
  filesystemType: string;
  totalBytes: number;
  usedBytes: number;
  availableBytes: number;
}

export interface ProcessObservation {
  pid: number;
  name: string;
  state: string;
  cpuPercent: number;
  memoryBytes: number;
}

export interface ServiceObservation { name: string; state: string; startMode: string }

export interface ContainerObservation {
  idAlias: string;
  name: string;
  image: string;
  state: string;
  cpuPercent: number;
  memoryBytes: number;
}

export interface LogMetadata {
  observedAt: string;
  source: string;
  severity: Severity;
  eventCode: string;
}

export type LogBody =
  | { state: "OMITTED" }
  | { state: "REDACTED_LOCAL_ONLY"; redactedText: string; redactionCount: number; uploadEligible: false };

/** Exact v1 projection: no arbitrary structured fields, summary, or raw-body property. */
export interface LogRecord { metadata: LogMetadata; body: LogBody }

export interface ObserverSignal {
  name: string;
  state: "OK" | "WARN" | "ERROR";
  value: number;
  unit: "count" | "bytes" | "milliseconds" | "percent";
}

export interface CurrentSnapshot {
  schemaVersion: "observer-current-snapshot/v1";
  snapshotId: string;
  sequence: number;
  observedAt: string;
  durationMilliseconds: number;
  collectionState: CollectionState;
  privacy: {
    profile: "SAFE_DEFAULT" | "LOCAL_LOG_BODIES";
    remoteProjection: "home-lab-server-snapshot/v1";
    messageBodiesUploadEligible: false;
    redactionCount: number;
    droppedCount: number;
    excludedFields: Array<"process_arguments" | "environment_variables" | "container_commands" | "container_mounts" | "container_labels" | "raw_log_bodies" | "credentials" | "tokens">;
  };
  sections: {
    overview: OverviewSection;
    filesystems: ListSection<FilesystemObservation>;
    processes: ListSection<ProcessObservation>;
    services: ListSection<ServiceObservation>;
    containers: ListSection<ContainerObservation>;
    logs: ListSection<LogRecord>;
    observer: ListSection<ObserverSignal>;
  };
}

export interface TrendPoint { at: string; value: number | null }
export interface TrendSeries { id: string; label: string; unit: "%" | "bytes"; color: "mint" | "cyan" | "amber" | "violet"; points: TrendPoint[] }
export interface TrendSnapshot { range: TrendRange; sampledEverySeconds: number; series: TrendSeries[] }

export interface ObserverDataSource {
  getCapabilities(signal?: AbortSignal): Promise<ObserverCapabilities>;
  getCurrentSnapshot(signal?: AbortSignal): Promise<CurrentSnapshot>;
  /** Optional because the accepted v1 local API has no historical-series endpoint. */
  getTrends?(range: TrendRange, signal?: AbortSignal): Promise<TrendSnapshot>;
}

export interface ObserverProblemDetails {
  type: string;
  title: string;
  status: number;
  detail?: string;
  instance?: string;
  code: string;
  requestId: string;
  fields?: Array<{ field: string; message: string; code: string }>;
}
