import { describe, expect, it, vi } from "vitest";
import linuxCapabilities from "../../../../schemas/v1/fixtures/valid/capabilities-linux.json";
import windowsCapabilities from "../../../../schemas/v1/fixtures/valid/capabilities-windows.json";
import linuxSnapshot from "../../../../schemas/v1/fixtures/valid/current-snapshot-linux.json";
import logOptInSnapshot from "../../../../schemas/v1/fixtures/valid/current-snapshot-log-opt-in.json";
import nativeFailedSnapshot from "../../../../schemas/v1/fixtures/valid/current-snapshot-native-failed.json";
import nativePreviewSnapshot from "../../../../schemas/v1/fixtures/valid/current-snapshot-native-preview.json";
import metricSeries from "../../../../schemas/v1/fixtures/valid/metric-series-6h.json";
import unsupportedMetricSeries from "../../../../schemas/v1/fixtures/valid/metric-series-unsupported.json";
import arbitraryMetricSeries from "../../../../schemas/v1/fixtures/invalid/metric-series-arbitrary-query.json";
import invalidQueryProblem from "../../../../schemas/v1/fixtures/valid/problem-invalid-query.json";
import diagnosticsHealth from "../../../../schemas/v1/fixtures/valid/diagnostics-health-available.json";
import logSummaryFixture from "../../../../schemas/v1/fixtures/valid/log-summary-1h.json";
import { LocalHttpObserverDataSource, ObserverTransportError, mapCapabilities, mapContainerInventory, mapCurrentSnapshot, mapDiagnosticsHealth, mapLogSummary, mapMetricSeries } from "./local-http";

const jsonResponse = (body: unknown, status = 200, contentType = "application/json; charset=utf-8") =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": contentType } });
const cloneFixture = <T>(value: T): any => JSON.parse(JSON.stringify(value));
const containerInventory = {
  schema_version: "observer-container-inventory/v1",
  observed_at: "2026-09-09T12:00:00Z",
  support_state: "SUPPORTED",
  collection_state: "PARTIAL",
  freshness: "CURRENT",
  reason_code: "STATS_PARTIAL",
  total_count: 2,
  returned_count: 2,
  truncated: false,
  items: [
    { id_alias: "ctr_0000000000000001", name: "observer-smoke-running", image: "example.invalid/observer:1.0", state: "running", cpu_percent: 0, memory_bytes: 134217728, metrics_state: "AVAILABLE", reason_code: null },
    { id_alias: "ctr_0000000000000002", name: "observer-smoke-stopped", image: "example.invalid/worker:1.0", state: "exited", cpu_percent: null, memory_bytes: null, metrics_state: "NOT_RUNNING", reason_code: "NOT_RUNNING" }
  ],
  policy: { read_only: true, data_classification: "LOCAL_SENSITIVE", remote_upload_eligible: false }
};

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

  it("loads the bounded dedicated container endpoint with existing transport protections", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(containerInventory));
    const source = new LocalHttpObserverDataSource({ bearerToken: "local-test-token", fetcher });
    const inventory = await source.getContainerInventory();
    expect(String(fetcher.mock.calls[0][0])).toBe("http://127.0.0.1:9847/api/v1/containers?limit=500");
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get("Authorization")).toBe("Bearer local-test-token");
    expect(fetcher.mock.calls[0][1]).toMatchObject({ method: "GET", credentials: "omit", cache: "no-store", redirect: "error" });
    expect(inventory).toMatchObject({ schemaVersion: "observer-container-inventory/v1", totalCount: 2, returnedCount: 2, policy: { readOnly: true, dataClassification: "LOCAL_SENSITIVE", remoteUploadEligible: false } });
    expect(inventory.items[0]).toMatchObject({ name: "observer-smoke-running", cpuPercent: 0, metricsState: "AVAILABLE" });
    expect(inventory.items[1]).toMatchObject({ state: "exited", cpuPercent: null, memoryBytes: null, metricsState: "NOT_RUNNING" });
  });

  it("loads bounded diagnostics health and discards additive fields without reflecting them", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(diagnosticsHealth));
    const source = new LocalHttpObserverDataSource({ bearerToken: "local-test-token", fetcher });
    const health = await source.getDiagnosticsHealth();
    expect(String(fetcher.mock.calls[0][0])).toBe("http://127.0.0.1:9847/api/v1/diagnostics/health");
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get("Authorization")).toBe("Bearer local-test-token");
    expect(health).toMatchObject({ state: "AVAILABLE", usage: { totalBytes: 4096, fileCount: 1 }, counters: { droppedRecords: 0, writeFailures: 0 }, policy: { containsLogContents: false, containsPaths: false, remoteUploadEligible: false } });

    const unsafe = cloneFixture(diagnosticsHealth);
    unsafe.log_contents = ["synthetic-secret"];
    unsafe.limits.future_metadata = "synthetic-secret";
    const projected = mapDiagnosticsHealth(unsafe);
    expect(JSON.stringify(projected)).not.toContain("synthetic-secret");
    const inconsistent = cloneFixture(diagnosticsHealth);
    inconsistent.available = false;
    expect(() => mapDiagnosticsHealth(inconsistent)).toThrow(/inconsistent/);
  });

  it("loads the exact bounded log-summary endpoint and projects only known fields", async () => {
    const payload = cloneFixture(logSummaryFixture);
    payload.future_metadata = { body: "synthetic-secret" };
    payload.sources[0].status.future_path = "C:\\private\\journal";
    payload.sources[0].buckets[0].counts.severity.future_identity = "synthetic-user";
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(payload));
    const source = new LocalHttpObserverDataSource({ bearerToken: "local-test-token", fetcher });

    const summary = await source.getLogSummary("1h");

    expect(String(fetcher.mock.calls[0][0])).toBe("http://127.0.0.1:9847/api/v1/logs/summary?range=1h");
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get("Authorization")).toBe("Bearer local-test-token");
    expect(fetcher.mock.calls[0][1]).toMatchObject({ method: "GET", credentials: "omit", cache: "no-store", redirect: "error" });
    expect(summary).toMatchObject({ range: "1h", coverageState: "PARTIAL", counts: { captured: 2, discarded: 1 }, privacy: { containsLogBodies: false, containsEventCodes: false, containsIdentityFields: false, remoteUploadEligible: false } });
    expect(summary.sources[0].buckets.slice(0, 3).map((bucket) => [bucket.coverageState, bucket.counts?.captured ?? null])).toEqual([["GAP", 1], ["UNKNOWN", 1], ["PARTIAL", 0]]);
    expect(JSON.stringify(summary)).not.toMatch(/synthetic-secret|private|synthetic-user/);
  });

  it("rejects malformed known log-summary semantics without filling unavailable counts", () => {
    const unsafeCount = cloneFixture(logSummaryFixture);
    unsafeCount.sources[0].buckets[0].counts.captured = Number.MAX_SAFE_INTEGER + 1;
    expect(() => mapLogSummary(unsafeCount)).toThrow(/safe bounds/);

    const fabricatedUnknownZero = cloneFixture(logSummaryFixture);
    fabricatedUnknownZero.sources[0].buckets[1].counts.captured = 0;
    fabricatedUnknownZero.sources[0].buckets[1].counts.severity.error = 0;
    expect(() => mapLogSummary(fabricatedUnknownZero)).toThrow(/unknown log bucket/);

    const badGrid = cloneFixture(logSummaryFixture);
    badGrid.sources[0].buckets[1].at = badGrid.sources[0].buckets[0].at;
    expect(() => mapLogSummary(badGrid)).toThrow(/fixed ascending grid/);

    const badSourceReason = cloneFixture(logSummaryFixture);
    badSourceReason.sources[0].status.reason_code = "LOG_SOURCES_DISABLED";
    expect(() => mapLogSummary(badSourceReason)).toThrow(/source status/);

    const futureAttempt = cloneFixture(logSummaryFixture);
    futureAttempt.sources[0].status.attempted_at = "2026-09-09T13:00:17.000000001Z";
    expect(() => mapLogSummary(futureAttempt)).toThrow(/newer than its response/);

    const applicationOnly = cloneFixture(logSummaryFixture);
    applicationOnly.sources[0].source = "application";
    expect(mapLogSummary(applicationOnly).sources[0].source).toBe("application");

    const twoSources = cloneFixture(logSummaryFixture);
    const application = cloneFixture(twoSources.sources[0]);
    application.source = "application";
    twoSources.sources.push(application);
    twoSources.counts = { captured: 4, discarded: 2 };
    expect(mapLogSummary(twoSources).sources.map((item) => item.source)).toEqual(["system", "application"]);

    const reversed = cloneFixture(twoSources);
    reversed.sources.reverse();
    expect(() => mapLogSummary(reversed)).toThrow(/fixed order/);
  });

  it("strictly rejects invalid or secret-bearing container inventory fields", () => {
    const knownTruncation = cloneFixture(containerInventory);
    knownTruncation.truncated = true;
    expect(mapContainerInventory(knownTruncation).truncated).toBe(true);

    const secret = cloneFixture(containerInventory);
    secret.items[0].environment_variables = ["TOKEN=synthetic-secret"];
    expect(() => mapContainerInventory(secret)).toThrow(/contract field/);

    const hiddenRawID = cloneFixture(containerInventory);
    hiddenRawID.items[0].id = "raw-container-id";
    expect(() => mapContainerInventory(hiddenRawID)).toThrow(/contract field/);

    const fabricatedStoppedZero = cloneFixture(containerInventory);
    fabricatedStoppedZero.items[1].cpu_percent = 0;
    expect(() => mapContainerInventory(fabricatedStoppedZero)).toThrow(/unavailable container metrics/);

    const badPolicy = cloneFixture(containerInventory);
    badPolicy.policy.remote_upload_eligible = true;
    expect(() => mapContainerInventory(badPolicy)).toThrow(/literal/);
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

  it("accepts every current-snapshot fixture covered by the closed v1 schema", () => {
    for (const fixture of [linuxSnapshot, logOptInSnapshot, nativeFailedSnapshot, nativePreviewSnapshot]) {
      expect(() => mapCurrentSnapshot(fixture)).not.toThrow();
    }
  });

  it("rejects capability enum, integer, and nested closure violations", () => {
    const badState = cloneFixture(linuxCapabilities);
    badState.collectors[0].support_state = "MYSTERY";
    expect(() => mapCapabilities(badState)).toThrow(/support state/);

    const fractionalRetention = cloneFixture(linuxCapabilities);
    fractionalRetention.policy.retention_days = 1.5;
    expect(() => mapCapabilities(fractionalRetention)).toThrow(/integer/);

    const unsafeExtra = cloneFixture(linuxCapabilities);
    unsafeExtra.collectors[0].environment_variables = ["TOKEN=secret"];
    expect(() => mapCapabilities(unsafeExtra)).toThrow(/contract field/);
  });

  it("rejects invalid current states, inconsistent counts, and data on non-supported sections", () => {
    const knownTruncation = cloneFixture(linuxSnapshot);
    knownTruncation.sections.processes.total_count = knownTruncation.sections.processes.items.length;
    knownTruncation.sections.processes.returned_count = knownTruncation.sections.processes.items.length;
    knownTruncation.sections.processes.truncated = true;
    expect(mapCurrentSnapshot(knownTruncation).sections.processes.truncated).toBe(true);

    const badState = cloneFixture(linuxSnapshot);
    badState.sections.processes.collection_state = "HEALTHY";
    expect(() => mapCurrentSnapshot(badState)).toThrow(/collection state/);

    const badCounts = cloneFixture(linuxSnapshot);
    badCounts.sections.processes.returned_count = 2;
    expect(() => mapCurrentSnapshot(badCounts)).toThrow(/counts/);

    const oversizedFilesystems = cloneFixture(linuxSnapshot);
    oversizedFilesystems.sections.filesystems.items = Array.from({ length: 17 }, () => cloneFixture(linuxSnapshot.sections.filesystems.items[0]));
    oversizedFilesystems.sections.filesystems.total_count = 17;
    oversizedFilesystems.sections.filesystems.returned_count = 17;
    oversizedFilesystems.sections.filesystems.truncated = false;
    expect(() => mapCurrentSnapshot(oversizedFilesystems)).toThrow(/array/);

    const invalidTimestamp = cloneFixture(linuxSnapshot);
    invalidTimestamp.observed_at = "2026-02-30T12:00:00Z";
    expect(() => mapCurrentSnapshot(invalidTimestamp)).toThrow(/date-time/);

    const fractionalPID = cloneFixture(linuxSnapshot);
    fractionalPID.sections.processes.items[0].pid = 1.5;
    expect(() => mapCurrentSnapshot(fractionalPID)).toThrow(/integer/);

    const unavailableWithData = cloneFixture(linuxSnapshot);
    unavailableWithData.sections.services.total_count = 1;
    unavailableWithData.sections.services.returned_count = 1;
    unavailableWithData.sections.services.items = [{ name: "hidden-service", state: "running", start_mode: "auto" }];
    expect(() => mapCurrentSnapshot(unavailableWithData)).toThrow(/non-supported section/);

    const processWithArguments = cloneFixture(linuxSnapshot);
    processWithArguments.sections.processes.items[0].arguments = "--token=secret";
    expect(() => mapCurrentSnapshot(processWithArguments)).toThrow(/contract field/);
  });

  it("enforces the closed log metadata and message-body union", () => {
    const omittedWithBody = cloneFixture(linuxSnapshot);
    omittedWithBody.sections.logs.items[0].body.redacted_text = "must not be accepted";
    expect(() => mapCurrentSnapshot(omittedWithBody)).toThrow(/contract field/);

    const metadataWithRawMessage = cloneFixture(linuxSnapshot);
    metadataWithRawMessage.sections.logs.items[0].metadata.raw_message = "secret";
    expect(() => mapCurrentSnapshot(metadataWithRawMessage)).toThrow(/contract field/);

    const invalidRedactionCount = cloneFixture(logOptInSnapshot);
    invalidRedactionCount.sections.logs.items[0].body.redaction_count = -1;
    expect(() => mapCurrentSnapshot(invalidRedactionCount)).toThrow(/integer/);

    const retainedAfterPermissionFailure = cloneFixture(linuxSnapshot);
    retainedAfterPermissionFailure.sections.logs.support_state = "PERMISSION_DENIED";
    retainedAfterPermissionFailure.sections.logs.collection_state = "NOT_RUN";
    retainedAfterPermissionFailure.sections.logs.freshness = "STALE";
    retainedAfterPermissionFailure.sections.logs.reason_code = "PERMISSION_DENIED";
    expect(mapCurrentSnapshot(retainedAfterPermissionFailure).sections.logs).toMatchObject({ supportState: "PERMISSION_DENIED", freshness: "STALE", returnedCount: 1 });

    const unknownWithRetainedData = cloneFixture(retainedAfterPermissionFailure);
    unknownWithRetainedData.sections.logs.freshness = "UNKNOWN";
    unknownWithRetainedData.sections.logs.observed_at = null;
    expect(() => mapCurrentSnapshot(unknownWithRetainedData)).toThrow(/non-supported log section/);

    const falselyHealthyRetainedData = cloneFixture(retainedAfterPermissionFailure);
    falselyHealthyRetainedData.sections.logs.collection_state = "OK";
    expect(() => mapCurrentSnapshot(falselyHealthyRetainedData)).toThrow(/non-supported log section/);

    const retainedWithUnsafeField = cloneFixture(retainedAfterPermissionFailure);
    retainedWithUnsafeField.sections.logs.items[0].checkpoint = "opaque-secret";
    expect(() => mapCurrentSnapshot(retainedWithUnsafeField)).toThrow(/contract field/);
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
    const oversizedContainers = new LocalHttpObserverDataSource({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ padding: "x".repeat(1_048_600) })) });
    await expect(oversizedContainers.getContainerInventory()).rejects.toThrow(/size limit/);
    const oversizedDiagnostics = new LocalHttpObserverDataSource({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ padding: "x".repeat(32_800) })) });
    await expect(oversizedDiagnostics.getDiagnosticsHealth()).rejects.toThrow(/size limit/);
    const oversizedLogs = new LocalHttpObserverDataSource({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ padding: "x".repeat(262_200) })) });
    await expect(oversizedLogs.getLogSummary("7d")).rejects.toThrow(/size limit/);
  });
});
