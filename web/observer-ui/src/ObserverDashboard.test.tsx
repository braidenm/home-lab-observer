import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { ObserverDashboard } from "./ObserverDashboard";
import { SyntheticObserverDataSource } from "./data/synthetic";

afterEach(cleanup);

describe("ObserverDashboard", () => {
  it("renders useful overview data and explicit local-only status", async () => {
    render(<ObserverDashboard dataSource={new SyntheticObserverDataSource()} mode="demo" />);

    expect(await screen.findByRole("heading", { name: "studio-node is steady" })).toBeTruthy();
    expect(screen.getByText("Synthetic demo")).toBeTruthy();
    expect(screen.getByText("Storage pressure is building")).toBeTruthy();
    expect(screen.getByText(/loopback by default/i)).toBeTruthy();
  });

  it("navigates to workload inventory and filters without exposing process arguments", async () => {
    const user = userEvent.setup();
    render(<ObserverDashboard dataSource={new SyntheticObserverDataSource()} />);
    await screen.findByRole("heading", { name: "studio-node is steady" });
    await user.click(screen.getByRole("button", { name: "Workloads" }));

    const region = screen.getByRole("region", { name: "Process inventory" });
    expect(within(region).getByText("postgres")).toBeTruthy();
    await user.type(screen.getByPlaceholderText("Find process…"), "platform");
    expect(within(region).queryByText("postgres")).toBeNull();
    expect(screen.queryByText(/--password|authorization=/i)).toBeNull();
  });

  it("makes the default log-body policy visible and provides metadata context", async () => {
    const user = userEvent.setup();
    render(<ObserverDashboard dataSource={new SyntheticObserverDataSource()} />);
    await screen.findByRole("heading", { name: "studio-node is steady" });
    await user.click(screen.getByRole("button", { name: "Logs" }));

    expect(screen.getByText("Message bodies are off by default")).toBeTruthy();
    expect(screen.getByText("0 enabled")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /Collection cycle completed/i }));
    expect(screen.getByRole("heading", { name: "Event context" })).toBeTruthy();
    expect(screen.getByText(/metadata and allowlisted structured fields only/i)).toBeTruthy();
  });

  it("shows unsupported and permission-denied capabilities distinctly", async () => {
    const user = userEvent.setup();
    render(<ObserverDashboard dataSource={new SyntheticObserverDataSource()} />);
    await screen.findByRole("heading", { name: "studio-node is steady" });
    await user.click(screen.getByRole("button", { name: "Observer & privacy" }));

    expect(screen.getByText("Hardware sensors")).toBeTruthy();
    expect(screen.getAllByText("Unavailable").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Permission Denied").length).toBeGreaterThan(0);
    expect(screen.getByText("Loopback-only dashboard and API")).toBeTruthy();
  });
});
