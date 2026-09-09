import type { TrendRange, TrendSnapshot } from "../types";
import { TrendChart } from "../components/TrendChart";

const ranges: { value: TrendRange; label: string }[] = [
  { value: "1h", label: "1 hour" }, { value: "6h", label: "6 hours" },
  { value: "24h", label: "24 hours" }, { value: "7d", label: "7 days" }
];

export function TrendsView({ trends, status, error, range, onRangeChange }: {
  trends: TrendSnapshot | null;
  status: "loading" | "ready" | "error" | "unsupported";
  error: string | null;
  range: TrendRange;
  onRangeChange: (range: TrendRange) => void;
}) {
  return (
    <div className="observer-view" aria-labelledby="trends-title">
      <div className="observer-view-heading">
        <div><p className="observer-kicker">Optional history source</p><h2 id="trends-title">Trends</h2><p>The accepted v1 local API exposes a current snapshot, not historical series.</p></div>
        {trends && <div className="observer-segmented" aria-label="Trend time range">{ranges.map((item) => (
          <button key={item.value} type="button" className={range === item.value ? "is-active" : ""} aria-pressed={range === item.value} onClick={() => onRangeChange(item.value)}>{item.label}</button>
        ))}</div>}
      </div>
      {status === "unsupported" && <Unavailable title="No series endpoint" detail="Connect a separate history data source or use the synthetic demo. No chart is inferred from the current snapshot." />}
      {status === "loading" && !trends && <Unavailable title="Loading trend source" detail="Current snapshot views remain available independently." />}
      {status === "error" && <Unavailable title="Trend source unavailable" detail={error ?? "The optional trend source failed."} />}
      {trends && <div className="observer-chart-grid">{trends.series.map((series) => <TrendChart key={series.id} series={series} />)}</div>}
    </div>
  );
}

function Unavailable({ title, detail }: { title: string; detail: string }) {
  return <section className="observer-panel observer-panel--padded"><strong>{title}</strong><p>{detail}</p></section>;
}
