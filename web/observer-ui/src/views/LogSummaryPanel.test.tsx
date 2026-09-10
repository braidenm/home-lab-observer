import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import logSummaryFixture from "../../../../schemas/v1/fixtures/valid/log-summary-1h.json";
import { mapLogSummary } from "../adapters/log-summary";
import type { ResourceState } from "../hooks/useObserverData";
import type { LogSummary } from "../types";
import { LogSummaryPanel } from "./LogSummaryPanel";

afterEach(cleanup);

const ready = (value: LogSummary): ResourceState<LogSummary> => ({ value, status: "ready", error: null });

describe("LogSummaryPanel", () => {
  it("keeps positive counts visible under gap and unknown coverage and preserves null as unavailable", async () => {
    const user = userEvent.setup();
    const summary = mapLogSummary(logSummaryFixture);
    const { rerender } = render(<LogSummaryPanel summary={ready(summary)} range="1h" onRangeChange={() => undefined} />);

    expect(screen.getByText("Local-sensitive metadata")).toBeTruthy();
    expect(screen.getByText(/bodies and identity fields omitted/i)).toBeTruthy();
    expect(screen.getByRole("img", { name: /System captured severity histogram/i }).querySelector(".observer-log-histogram__bucket--gap")).toBeTruthy();
    expect(screen.getByRole("img", { name: /System captured severity histogram/i }).querySelector(".observer-log-histogram__bucket--unknown")).toBeTruthy();

    await user.click(screen.getByText("View accessible bucket data"));
    const table = screen.getByRole("region", { name: "System log summary bucket data" });
    expect(table.getAttribute("tabindex")).toBe("0");
    const rows = within(table).getAllByRole("row");
    expect(rows[1].textContent).toMatch(/Gap · 0s1/);
    expect(rows[2].textContent).toMatch(/Unknown · 0s1/);

    const unknown = structuredClone(summary);
    unknown.sources[0].buckets[1].counts = null;
    unknown.sources[0].counts = { captured: 1, discarded: 1 };
    unknown.counts = { captured: 1, discarded: 1 };
    rerender(<LogSummaryPanel summary={ready(unknown)} range="1h" onRangeChange={() => undefined} />);
    await user.click(screen.getByText("View accessible bucket data"));
    expect(within(screen.getByRole("region", { name: "System log summary bucket data" })).getAllByText("Unavailable").length).toBeGreaterThan(0);
  });

  it("keeps unsupported source status visible without drawing an unknown-only chart", () => {
    const summary = mapLogSummary(logSummaryFixture);
    const source = summary.sources[0];
    source.status = { supportState: "UNSUPPORTED", collectionState: "NOT_RUN", freshness: "UNKNOWN", observedAt: null, attemptedAt: null, coverageThrough: null, reasonCode: "PLATFORM_UNSUPPORTED" };
    source.coverageState = "UNKNOWN";
    source.coveredSeconds = 0;
    source.counts = null;
    source.buckets = source.buckets.map((bucket) => ({ ...bucket, coverageState: "UNKNOWN", coveredSeconds: 0, reasonCode: "NOT_YET_OBSERVED", counts: null }));
    summary.supportState = "UNSUPPORTED";
    summary.collectionState = "NOT_RUN";
    summary.freshness = "UNKNOWN";
    summary.observedAt = null;
    summary.reasonCode = "PLATFORM_UNSUPPORTED";
    summary.coverageState = "UNKNOWN";
    summary.counts = null;

    render(<LogSummaryPanel summary={ready(summary)} range="1h" onRangeChange={() => undefined} />);

    expect(screen.getAllByText("Unsupported").length).toBeGreaterThan(0);
    expect(screen.getByText(/unknown grid is omitted; no zero history is inferred/i)).toBeTruthy();
    expect(screen.queryByRole("img", { name: /captured severity histogram/i })).toBeNull();
  });

  it("explains a disabled summary without implying zero history or offering configuration", () => {
    const summary = structuredClone(mapLogSummary(logSummaryFixture));
    summary.sources = [];
    summary.supportState = "DISABLED";
    summary.collectionState = "NOT_RUN";
    summary.freshness = "UNKNOWN";
    summary.observedAt = null;
    summary.reasonCode = "LOG_SOURCES_DISABLED";
    summary.coverageState = "UNKNOWN";
    summary.counts = null;
    render(<LogSummaryPanel summary={ready(summary)} range="1h" onRangeChange={() => undefined} />);
    expect(screen.getByText("Native log summaries are disabled")).toBeTruthy();
    expect(screen.getByText(/Zero counts are not inferred/i)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /enable|configure|install/i })).toBeNull();
  });

  it("exposes an independently operable fixed range selector", async () => {
    const user = userEvent.setup();
    let selected = "";
    render(<LogSummaryPanel summary={ready(mapLogSummary(logSummaryFixture))} range="1h" onRangeChange={(range) => { selected = range; }} />);
    const ranges = screen.getByRole("group", { name: "Log summary range" });
    expect(within(ranges).getByRole("button", { name: "1h" }).getAttribute("aria-pressed")).toBe("true");
    await user.click(within(ranges).getByRole("button", { name: "7d" }));
    expect(selected).toBe("7d");
  });

  it("keeps the complete table in a labelled keyboard-scroll region at 390px", async () => {
    const user = userEvent.setup();
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
    window.dispatchEvent(new Event("resize"));
    render(<LogSummaryPanel summary={ready(mapLogSummary(logSummaryFixture))} range="1h" onRangeChange={() => undefined} />);
    await user.click(screen.getByText("View accessible bucket data"));
    expect(screen.getByText(/Scroll sideways to inspect every severity column/i)).toBeTruthy();
    const region = screen.getByRole("region", { name: "System log summary bucket data" });
    expect(region.classList.contains("observer-table-scroll")).toBe(true);
    expect(region.getAttribute("tabindex")).toBe("0");
    expect(within(region).getAllByRole("columnheader")).toHaveLength(12);
  });
});
