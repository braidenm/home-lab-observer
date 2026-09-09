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
