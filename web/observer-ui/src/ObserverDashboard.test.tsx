import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { ObserverDashboard } from "./ObserverDashboard";
import { SyntheticObserverDataSource, syntheticCapabilities, syntheticSnapshot } from "./data/synthetic";
import type { ObserverDataSource } from "./types";

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
});
