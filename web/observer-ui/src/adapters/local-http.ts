import type {
  LocalHttpObserverDataSourceOptions,
  ObserverTransportRequest
} from "./local-http.types";
import type {
  LogQuery,
  LogSnapshot,
  ObserverDataSource,
  ObserverHealthSnapshot,
  OverviewSnapshot,
  TrendRange,
  TrendSnapshot,
  WorkloadQuery,
  WorkloadSnapshot
} from "../types";

const LOOPBACK_HOSTS = new Set(["127.0.0.1", "localhost", "[::1]", "::1"]);

export class ObserverTransportError extends Error {
  constructor(
    message: string,
    readonly status?: number
  ) {
    super(message);
    this.name = "ObserverTransportError";
  }
}

export class LocalHttpObserverDataSource implements ObserverDataSource {
  private readonly baseUrl: string;
  private readonly fetcher: typeof fetch;
  private readonly bearerToken?: string;

  constructor(options: LocalHttpObserverDataSourceOptions = {}) {
    this.baseUrl = normalizeBaseUrl(options.baseUrl ?? "");
    this.fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis);
    this.bearerToken = options.bearerToken;
  }

  getOverview(signal?: AbortSignal): Promise<OverviewSnapshot> {
    return this.request("/api/v1/overview", {}, signal);
  }

  getTrends(range: TrendRange, signal?: AbortSignal): Promise<TrendSnapshot> {
    return this.request("/api/v1/trends", { range }, signal);
  }

  getWorkloads(query: WorkloadQuery = {}, signal?: AbortSignal): Promise<WorkloadSnapshot> {
    return this.request("/api/v1/workloads", { kind: query.kind, search: query.search }, signal);
  }

  getLogs(query: LogQuery = {}, signal?: AbortSignal): Promise<LogSnapshot> {
    return this.request("/api/v1/logs", {
      ...query,
      severities: query.severities?.join(",")
    }, signal);
  }

  getObserverHealth(signal?: AbortSignal): Promise<ObserverHealthSnapshot> {
    return this.request("/api/v1/observer-health", {}, signal);
  }

  private async request<T>(
    path: string,
    query: ObserverTransportRequest,
    signal?: AbortSignal
  ): Promise<T> {
    const search = new URLSearchParams();
    Object.entries(query).forEach(([key, value]) => {
      if (value !== undefined && value !== "") {
        search.set(key, String(value));
      }
    });
    const suffix = search.size > 0 ? `?${search.toString()}` : "";
    const headers = new Headers({ Accept: "application/json" });
    if (this.bearerToken) {
      headers.set("Authorization", `Bearer ${this.bearerToken}`);
    }

    const response = await this.fetcher(`${this.baseUrl}${path}${suffix}`, {
      method: "GET",
      headers,
      credentials: "omit",
      cache: "no-store",
      redirect: "error",
      signal
    });
    if (!response.ok) {
      throw new ObserverTransportError(`Observer API request failed with ${response.status}`, response.status);
    }
    const contentType = response.headers.get("content-type") ?? "";
    if (!contentType.toLowerCase().includes("application/json")) {
      throw new ObserverTransportError("Observer API returned a non-JSON response", response.status);
    }
    return (await response.json()) as T;
  }
}

function normalizeBaseUrl(baseUrl: string): string {
  if (baseUrl === "" || baseUrl === "/") {
    return "";
  }
  const url = new URL(baseUrl);
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new TypeError("Local observer URL must use HTTP or HTTPS");
  }
  if (url.username || url.password) {
    throw new TypeError("Local observer URL must not contain credentials");
  }
  if (!LOOPBACK_HOSTS.has(url.hostname)) {
    throw new TypeError("Local observer URL must use a loopback host");
  }
  if (url.pathname !== "/" || url.search || url.hash) {
    throw new TypeError("Local observer URL must not contain a path, query, or fragment");
  }
  return url.origin;
}
