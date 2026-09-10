import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { ObserverDashboard } from "./ObserverDashboard";
import { SyntheticObserverDataSource, syntheticCapabilities, syntheticSnapshot } from "./data/synthetic";
import type { ContainerInventory, ObserverDataSource } from "./types";

afterEach(cleanup);

describe("ObserverDashboard", () => {
  it("renders only contract-derived overview facts and explicit data quality", async () => {
    render(<ObserverDashboard dataSource={new SyntheticObserverDataSource()} mode="demo" />);
    expect(await screen.findByRole("heading", { name: "studio-node" })).toBeTruthy();
    expect(screen.getByText("Synthetic demo")).toBeTruthy();
    expect(screen.getAllByText("Snapshot PARTIAL").length).toBeGreaterThan(0);
    expect(screen.getByText("4 of 231 returned")).toBeTruthy();
    expect(screen.queryByText(/remain on this machine|one item needs attention/i)).toBeNull();
  });

  it("uses a complete ARIA tab relationship and retains section counts", async () => {
    const user = userEvent.setup();
    render(<ObserverDashboard dataSource={new SyntheticObserverDataSource()} />);
    await screen.findByRole("heading", { name: "studio-node" });
    await user.click(screen.getByRole("button", { name: "Workloads" }));
    const tab = screen.getByRole("tab", { name: "Processes" });
    const panel = screen.getByRole("tabpanel");
    expect(tab.getAttribute("aria-controls")).toBe(panel.id);
    expect(within(panel).getByText("4 of 231 · truncated")).toBeTruthy();
    await user.type(screen.getByPlaceholderText("Find process…"), "platform");
    expect(within(panel).queryByText("postgres")).toBeNull();
    expect(screen.queryByText(/account|owner|--password|authorization=/i)).toBeNull();
  });

  it("clears a workload filter when keyboard navigation changes tabs", async () => {
    const user = userEvent.setup();
    render(<ObserverDashboard dataSource={new SyntheticObserverDataSource()} />);
    await screen.findByRole("heading", { name: "studio-node" });
    await user.click(screen.getByRole("button", { name: "Workloads" }));
    const filter = screen.getByPlaceholderText("Find process…") as HTMLInputElement;
    await user.type(filter, "visual studio code");
    screen.getByRole("tab", { name: "Processes" }).focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Services" }).getAttribute("aria-selected")).toBe("true");
    expect((screen.getByPlaceholderText("Find service…") as HTMLInputElement).value).toBe("");
  });

  it("shows only exact log metadata and body state", async () => {
    const user = userEvent.setup();
    render(<ObserverDashboard dataSource={new SyntheticObserverDataSource()} />);
    await screen.findByRole("heading", { name: "studio-node" });
    await user.click(screen.getByRole("button", { name: "Logs" }));
    expect(screen.getByText("Default body state: OMITTED")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /Collection Complete/i }));
    expect(screen.getByRole("heading", { name: "Event context" })).toBeTruthy();
    expect(screen.getByText(/exact metadata and body-state information only/i)).toBeTruthy();
    expect(screen.queryByText(/retry for user/i)).toBeNull();
  });

  it("isolates endpoint failure and does not make local claims in embedded mode", async () => {
    const source: ObserverDataSource = {
      getCapabilities: async () => { throw new Error("Capability read failed"); },
      getCurrentSnapshot: async () => structuredClone(syntheticSnapshot)
    };
    const user = userEvent.setup();
    render(<ObserverDashboard dataSource={source} mode="embedded" />);
    expect(await screen.findByRole("heading", { name: "studio-node" })).toBeTruthy();
    expect(screen.getByRole("alert").textContent).toContain("Capabilities unavailable");
    expect(screen.getByText(/transport and egress are controlled by the embedding application/i)).toBeTruthy();
    expect(screen.queryByText(/default bind/i)).toBeNull();
    await user.click(screen.getByRole("button", { name: "Observer & privacy" }));
    expect(screen.getByText("Transport supplied by the embedding application")).toBeTruthy();
    expect(screen.getAllByText("Snapshot PARTIAL").length).toBeGreaterThan(0);
  });

  it("marks trends unsupported when a data source implements only Spec 002", async () => {
    const source: ObserverDataSource = {
      getCapabilities: async () => structuredClone(syntheticCapabilities),
      getCurrentSnapshot: async () => structuredClone(syntheticSnapshot)
    };
    const user = userEvent.setup();
    render(<ObserverDashboard dataSource={source} />);
    await screen.findByRole("heading", { name: "studio-node" });
    await user.click(screen.getByRole("button", { name: "Trends" }));
    expect(screen.getByRole("heading", { name: "Trends" })).toBeTruthy();
    expect(screen.getByText("No series endpoint")).toBeTruthy();
    expect(screen.getByText(/no chart is inferred from the current snapshot/i)).toBeTruthy();
  });

  it("keeps the legacy snapshot container view for older data sources", async () => {
    const user = userEvent.setup();
    render(<ObserverDashboard dataSource={{
      getCapabilities: async () => structuredClone(syntheticCapabilities),
      getCurrentSnapshot: async () => structuredClone(syntheticSnapshot)
    }} />);
    await screen.findByRole("heading", { name: "studio-node" });
    await user.click(screen.getByRole("button", { name: "Workloads" }));
    await user.click(screen.getByRole("tab", { name: "Containers" }));
    expect(screen.getByText("platform-demo")).toBeTruthy();
    expect(screen.queryByText(/dedicated container inventory/i)).toBeNull();
  });

  it("explains explicit local endpoint options when container observations are disabled", async () => {
    const user = userEvent.setup();
    const source = sourceWithContainerInventory({
      ...containerInventory,
      observedAt: null,
      supportState: "DISABLED",
      collectionState: "NOT_RUN",
      freshness: "UNKNOWN",
      reasonCode: "DOCKER_NOT_CONFIGURED",
      totalCount: 0,
      returnedCount: 0,
      items: []
    });
    render(<ObserverDashboard dataSource={source} />);
    await screen.findByRole("heading", { name: "studio-node" });
    await user.click(screen.getByRole("button", { name: "Workloads" }));
    await user.click(screen.getByRole("tab", { name: "Containers" }));
    expect(await screen.findByRole("heading", { name: "Container observations are disabled" })).toBeTruthy();
    expect(screen.getByText("observer serve --docker-endpoint unix:///var/run/docker.sock")).toBeTruthy();
    expect(screen.getByText("observer serve --docker-endpoint npipe:////./pipe/docker_engine")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /install|enable docker/i })).toBeNull();
  });

  it("shows running zero measurements and stopped null metrics without fabricating parity", async () => {
    const user = userEvent.setup();
    const source = sourceWithContainerInventory(containerInventory, async () => { throw new Error("Snapshot failed independently"); });
    render(<ObserverDashboard dataSource={source} />);
    await screen.findByRole("alert");
    await user.click(screen.getByRole("button", { name: "Workloads" }));
    await user.click(screen.getByRole("tab", { name: "Containers" }));
    expect(await screen.findByText("observer-smoke-running")).toBeTruthy();
    expect(screen.getByText("observer-smoke-stopped")).toBeTruthy();
    expect(screen.getByText("0.0%")).toBeTruthy();
    expect(screen.getAllByText("Unavailable")).toHaveLength(2);
    expect(screen.getByText("Not Running")).toBeTruthy();
  });

  it("isolates a dedicated container inventory error inside the Containers tab", async () => {
    const user = userEvent.setup();
    const source: ObserverDataSource = {
      getCapabilities: async () => structuredClone(syntheticCapabilities),
      getCurrentSnapshot: async () => structuredClone(syntheticSnapshot),
      getContainerInventory: async () => { throw new Error("Container inventory request failed"); }
    };
    render(<ObserverDashboard dataSource={source} />);
    await screen.findByRole("heading", { name: "studio-node" });
    await user.click(screen.getByRole("button", { name: "Workloads" }));
    await user.click(screen.getByRole("tab", { name: "Containers" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Container inventory unavailable");
    expect(alert.textContent).toContain("Container inventory request failed");
  });

  it("makes empty, truncated, failed, and stale inventory states explicit", async () => {
    const user = userEvent.setup();
    const staleInventory: ContainerInventory = { ...containerInventory, collectionState: "FAILED", freshness: "STALE", reasonCode: "ENGINE_UNAVAILABLE", totalCount: 4, truncated: true };
    const { rerender } = render(<ObserverDashboard dataSource={sourceWithContainerInventory(staleInventory)} />);
    await screen.findByRole("heading", { name: "studio-node" });
    await user.click(screen.getByRole("button", { name: "Workloads" }));
    await user.click(screen.getByRole("tab", { name: "Containers" }));
    expect(screen.getByText("Failed")).toBeTruthy();
    expect(screen.getByText("Stale")).toBeTruthy();
    expect(screen.getByText("2 of 4 · truncated")).toBeTruthy();
    expect(screen.getByText(/Showing 2 of 4/)).toBeTruthy();

    const emptyInventory: ContainerInventory = { ...containerInventory, collectionState: "OK", reasonCode: null, totalCount: 0, returnedCount: 0, truncated: false, items: [] };
    rerender(<ObserverDashboard dataSource={sourceWithContainerInventory(emptyInventory)} />);
    expect(await screen.findByText("Supported collection completed with zero records.")).toBeTruthy();
  });

  it("clarifies that legacy container capability does not describe dedicated inventory", async () => {
    const user = userEvent.setup();
    render(<ObserverDashboard dataSource={sourceWithContainerInventory(containerInventory)} />);
    await screen.findByRole("heading", { name: "studio-node" });
    await user.click(screen.getByRole("button", { name: "Observer & privacy" }));
    expect(screen.getByText(/legacy snapshot container capability.*does not describe the dedicated container inventory/i)).toBeTruthy();
    expect(screen.getByText(/Workloads › Containers/)).toBeTruthy();
  });

  it("shows bounded diagnostics health and keeps missing support explicit", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<ObserverDashboard dataSource={new SyntheticObserverDataSource()} mode="demo" />);
    await screen.findByRole("heading", { name: "studio-node" });
    await user.click(screen.getByRole("button", { name: "Observer & privacy" }));
    expect(screen.getByRole("heading", { name: "Self-diagnostics" })).toBeTruthy();
    expect(screen.getByText("32 KB")).toBeTruthy();
    expect(screen.getByText(/Diagnostic records, paths, request data.*never returned/i)).toBeTruthy();
    expect(screen.getByText(/remote upload not eligible/i)).toBeTruthy();

    rerender(<ObserverDashboard dataSource={{
      getCapabilities: async () => structuredClone(syntheticCapabilities),
      getCurrentSnapshot: async () => structuredClone(syntheticSnapshot)
    }} initialView="health" />);
    expect(await screen.findByRole("heading", { name: "Self-diagnostics unavailable" })).toBeTruthy();
    expect(screen.getByText(/No zero values are inferred/i)).toBeTruthy();
  });

  it("isolates diagnostics failure and renders disabled status truthfully", async () => {
    const base = new SyntheticObserverDataSource();
    const failure: ObserverDataSource = { ...base, getCapabilities: base.getCapabilities.bind(base), getCurrentSnapshot: base.getCurrentSnapshot.bind(base), getDiagnosticsHealth: async () => { throw new Error("Diagnostics health request failed"); } };
    const { rerender } = render(<ObserverDashboard dataSource={failure} initialView="health" />);
    expect((await screen.findByRole("alert")).textContent).toMatch(/Self-diagnostics health unavailable.*continues independently/i);

    const disabled: ObserverDataSource = {
      getCapabilities: base.getCapabilities.bind(base),
      getCurrentSnapshot: base.getCurrentSnapshot.bind(base),
      getDiagnosticsHealth: async () => ({
        schemaVersion: "observer-diagnostics-health/v1", generatedAt: "2026-09-09T19:00:00Z", enabled: false, available: false, state: "DISABLED", reasonCode: "DIAGNOSTICS_DISABLED",
        limits: { maxFiles: 5, maxFileBytes: 2_097_152, maxTotalBytes: 10_485_760, maxRecordBytes: 8_192, maxAgeSeconds: 604_800 }, usage: { totalBytes: 0, fileCount: 0 }, counters: { droppedRecords: 0, writeFailures: 0 },
        policy: { dataClassification: "PUBLIC_METADATA", containsLogContents: false, containsPaths: false, remoteUploadEligible: false }
      })
    };
    rerender(<ObserverDashboard dataSource={disabled} initialView="health" />);
    expect((await screen.findAllByText("Disabled")).length).toBeGreaterThan(0);
    expect(screen.getByText(/Retained diagnostics are not enabled/i)).toBeTruthy();
  });

  it("does not render unavailable diagnostics storage as a healthy zero", async () => {
    const base = new SyntheticObserverDataSource();
    render(<ObserverDashboard dataSource={{
      getCapabilities: base.getCapabilities.bind(base),
      getCurrentSnapshot: base.getCurrentSnapshot.bind(base),
      getDiagnosticsHealth: async () => ({
        schemaVersion: "observer-diagnostics-health/v1", generatedAt: "2026-09-09T19:00:00Z", enabled: true, available: false, state: "UNAVAILABLE", reasonCode: "DIAGNOSTICS_UNAVAILABLE",
        limits: { maxFiles: 5, maxFileBytes: 2_097_152, maxTotalBytes: 10_485_760, maxRecordBytes: 8_192, maxAgeSeconds: 604_800 }, usage: { totalBytes: 0, fileCount: 0 }, counters: { droppedRecords: 0, writeFailures: 1 },
        policy: { dataClassification: "PUBLIC_METADATA", containsLogContents: false, containsPaths: false, remoteUploadEligible: false }
      })
    }} initialView="health" />);
    expect((await screen.findAllByText("Unavailable")).length).toBeGreaterThan(0);
    expect(screen.getByText(/zero is not inferred/i)).toBeTruthy();
    expect(screen.queryByText("0 B")).toBeNull();
  });
});

const containerInventory: ContainerInventory = {
  schemaVersion: "observer-container-inventory/v1",
  observedAt: "2026-09-09T12:00:00Z",
  supportState: "SUPPORTED",
  collectionState: "PARTIAL",
  freshness: "CURRENT",
  reasonCode: "STATS_PARTIAL",
  totalCount: 2,
  returnedCount: 2,
  truncated: false,
  items: [
    { idAlias: "ctr_0000000000000001", name: "observer-smoke-running", image: "example.invalid/observer:1.0", state: "running", cpuPercent: 0, memoryBytes: 134217728, metricsState: "AVAILABLE", reasonCode: null },
    { idAlias: "ctr_0000000000000002", name: "observer-smoke-stopped", image: "example.invalid/worker:1.0", state: "exited", cpuPercent: null, memoryBytes: null, metricsState: "NOT_RUNNING", reasonCode: "NOT_RUNNING" }
  ],
  policy: { readOnly: true, dataClassification: "LOCAL_SENSITIVE", remoteUploadEligible: false }
};

function sourceWithContainerInventory(inventory: ContainerInventory, getCurrentSnapshot: ObserverDataSource["getCurrentSnapshot"] = async () => structuredClone(syntheticSnapshot)): ObserverDataSource {
  return {
    getCapabilities: async () => structuredClone(syntheticCapabilities),
    getCurrentSnapshot,
    getContainerInventory: async () => structuredClone(inventory)
  };
}
