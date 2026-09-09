import { describe, expect, it, vi } from "vitest";
import linuxCapabilities from "../../../../schemas/v1/fixtures/valid/capabilities-linux.json";
import windowsCapabilities from "../../../../schemas/v1/fixtures/valid/capabilities-windows.json";
import linuxSnapshot from "../../../../schemas/v1/fixtures/valid/current-snapshot-linux.json";
import logOptInSnapshot from "../../../../schemas/v1/fixtures/valid/current-snapshot-log-opt-in.json";
import metricSeries from "../../../../schemas/v1/fixtures/valid/metric-series-6h.json";
import unsupportedMetricSeries from "../../../../schemas/v1/fixtures/valid/metric-series-unsupported.json";
import arbitraryMetricSeries from "../../../../schemas/v1/fixtures/invalid/metric-series-arbitrary-query.json";
import invalidQueryProblem from "../../../../schemas/v1/fixtures/valid/problem-invalid-query.json";
import { LocalHttpObserverDataSource, ObserverTransportError, mapCapabilities, mapCurrentSnapshot, mapMetricSeries } from "./local-http";

const jsonResponse = (body: unknown, status = 200, contentType = "application/json; charset=utf-8") =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": contentType } });

describe("LocalHttpObserverDataSource", () => {
  it("cancels an oversized stream before consuming an unbounded response", async () => {
    let pulled = 0;
    let canceled = false;
    const stream = new ReadableStream<Uint8Array>({
      pull(controller) { pulled += 1; controller.enqueue(new Uint8Array(65536)); },
      cancel() { canceled = true; }
    });
    const source = new LocalHttpObserverDataSource({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(new Response(stream, { headers: { 'content-type': 'application/json' } })) });
    await expect(source.getCapabilities()).rejects.toThrow('size limit');
    expect(canceled).toBe(true);
    expect(pulled).toBeLessThanOrEqual(4);
  });

  it("uses only canonical endpoints on the documented default port", async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/capabilities")) return jsonResponse(linuxCapabilities);
      if (url.includes("/metrics/series")) return jsonResponse(metricSeries);
      return jsonResponse(linuxSnapshot);
    });
    const source = new LocalHttpObserverDataSource({ bearerToken: "local-test-token", fetcher });

    const capabilities = await source.getCapabilities();
    const snapshot = await source.getCurrentSnapshot();
    const trends = await source.getTrends("6h");

    expect(fetcher.mock.calls.slice(0, 2).map(([url]) => url)).toEqual([
      "http://127.0.0.1:9847/api/v1/capabilities",
      "http://127.0.0.1:9847/api/v1/snapshots/current"
    ]);
    const seriesUrl = new URL(String(fetcher.mock.calls[2][0]));
    expect(seriesUrl.pathname).toBe("/api/v1/metrics/series");
    expect(seriesUrl.searchParams.get("range")).toBe("6h");
    expect(seriesUrl.searchParams.getAll("metric")).toEqual([
      "cpu.utilization.percent", "memory.utilization.percent", "filesystem.aggregate.utilization.percent",
      "network.receive.bytes_per_second", "network.transmit.bytes_per_second", "process.count"
    ]);
    for (const [url, init] of fetcher.mock.calls) {
      expect(String(url)).not.toContain("local-test-token");
      expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer local-test-token");
      expect(init).toMatchObject({ method: "GET", credentials: "omit", cache: "no-store", redirect: "error" });
    }
    expect(capabilities.collectors.find((item) => item.name === "logs")?.supportState).toBe("DISABLED");
    expect(snapshot.sections.processes).toMatchObject({ totalCount: 231, returnedCount: 1, truncated: true });
    expect(snapshot.sections.services).toMatchObject({ supportState: "UNAVAILABLE", collectionState: "NOT_RUN", freshness: "UNKNOWN", observedAt: null });
    expect(snapshot.sections.logs.items[0]).toEqual({ metadata: { observedAt: "2026-09-09T11:59:58Z", source: "sample-api", severity: "INFO", eventCode: "READY" }, body: { state: "OMITTED" } });
    expect(trends).toMatchObject({ schemaVersion: "observer-metric-series/v1", range: "6h", sampleIntervalSeconds: 900, privacy: { containsProcessIdentity: false, containsLogBodies: false, remoteUploadEligible: false } });
  });

  it("maps real series fixtures without inventing labels or filling collection gaps", () => {
    const trends = mapMetricSeries(metricSeries);
    const cpu = trends.series.find((item) => item.id === "cpu.utilization.percent");
    expect(cpu).toMatchObject({ label: "CPU utilization", unit: "percent", supportState: "SUPPORTED", collectionState: "PARTIAL", pointCount: 3, gapCount: 1 });
    expect(cpu?.points.map((point) => point.value)).toEqual([21.4, null, 18.9]);
    expect(trends.sampleIntervalSeconds).toBe(900);
    expect(mapMetricSeries(unsupportedMetricSeries).series[0]).toMatchObject({ supportState: "UNSUPPORTED", collectionState: "NOT_RUN", freshness: "UNKNOWN", points: [] });
    expect(() => mapMetricSeries(arbitraryMetricSeries)).toThrow(/contract field|metric identifier/);
  });

  it("maps the real Windows capability and local-redacted-body fixtures", () => {
    const capabilities = mapCapabilities(windowsCapabilities);
    const snapshot = mapCurrentSnapshot(logOptInSnapshot);
    expect(capabilities.collectors.find((item) => item.name === "processes")?.supportState).toBe("PERMISSION_DENIED");
    expect(capabilities.collectors.find((item) => item.name === "containers")?.reasonCode).toBe("ENGINE_NOT_RUNNING");
    expect(snapshot.sections.services.supportState).toBe("UNSUPPORTED");
    expect(snapshot.sections.logs.items[0].body).toEqual({ state: "REDACTED_LOCAL_ONLY", redactedText: "retry for user [REDACTED]", redactionCount: 2, uploadEligible: false });
    expect("summary" in snapshot.sections.logs.items[0]).toBe(false);
    expect("structuredFields" in snapshot.sections.logs.items[0]).toBe(false);
  });

  it("parses bounded Problem Details into a typed safe error", async () => {
    const source = new LocalHttpObserverDataSource({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(invalidQueryProblem, 400, "application/problem+json")) });
    const rejection = source.getCurrentSnapshot();
    await expect(rejection).rejects.toBeInstanceOf(ObserverTransportError);
    await expect(rejection).rejects.toMatchObject({ status: 400, problem: { code: "INVALID_QUERY", requestId: "request_sample_001", fields: [{ field: "process_limit", code: "OUT_OF_RANGE" }] } });
  });

  it("rejects non-loopback URLs, URL credentials, bad media types, and oversized payloads", async () => {
    expect(() => new LocalHttpObserverDataSource({ baseUrl: "https://observer.example.test" })).toThrow(/loopback/);
    expect(() => new LocalHttpObserverDataSource({ baseUrl: "http://name:secret@127.0.0.1:9847" })).toThrow(/credentials/);
    const html = new LocalHttpObserverDataSource({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(new Response("no", { headers: { "content-type": "text/plain" } })) });
    await expect(html.getCapabilities()).rejects.toThrow(/non-JSON/);
    const oversized = new LocalHttpObserverDataSource({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ padding: "x".repeat(132_000) })) });
    await expect(oversized.getCapabilities()).rejects.toThrow(/size limit/);
    const oversizedSeries = new LocalHttpObserverDataSource({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ padding: "x".repeat(1_048_600) })) });
    await expect(oversizedSeries.getTrends("1h")).rejects.toThrow(/size limit/);
  });
});
