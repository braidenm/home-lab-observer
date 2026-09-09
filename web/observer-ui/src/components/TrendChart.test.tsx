import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { TrendChart } from "./TrendChart";
import type { TrendSeries } from "../types";

afterEach(cleanup);

it("shows isolated observations without counting gaps as measured points", () => {
  const series: TrendSeries = {
    id: "cpu.utilization.percent", label: "CPU utilization", unit: "percent", color: "mint",
    supportState: "SUPPORTED", collectionState: "PARTIAL", freshness: "CURRENT", reasonCode: "COLLECTION_GAPS",
    pointCount: 3, gapCount: 2, truncated: false,
    points: [{ at: "2026-09-09T12:00:00Z", value: null }, { at: "2026-09-09T12:01:00Z", value: 42 }, { at: "2026-09-09T12:02:00Z", value: null }]
  };
  const { container } = render(<TrendChart series={series} />);
  expect(screen.getByText(/1 observed point · 2 gaps/)).toBeTruthy();
  expect(container.querySelectorAll(".observer-chart__point").length).toBe(1);
  expect(container.querySelectorAll(".observer-chart__line:not(.observer-chart__point)")[0]?.getAttribute("d")).not.toContain("L");
});
