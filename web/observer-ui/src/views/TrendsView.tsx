import type { TrendRange, TrendSnapshot } from "../types";
import { formatRelative } from "../components/format";
import { StateBadge } from "../components/StateBadge";
import { TrendChart } from "../components/TrendChart";

const ranges: { value: TrendRange; label: string }[] = [
  { value: "1h", label: "1 hour" },
  { value: "6h", label: "6 hours" },
  { value: "24h", label: "24 hours" },
  { value: "7d", label: "7 days" }
];

export function TrendsView({
  trends,
  range,
  onRangeChange
}: {
  trends: TrendSnapshot;
  range: TrendRange;
  onRangeChange: (range: TrendRange) => void;
}) {
  return (
    <div className="observer-view" aria-labelledby="trends-title">
      <div className="observer-view-heading">
        <div>
          <p className="observer-kicker">Bounded history</p>
          <h2 id="trends-title">Trends and correlated events</h2>
          <p>Compare resource shape with workload and observer events at the same point in time.</p>
        </div>
        <div className="observer-segmented" aria-label="Trend time range">
          {ranges.map((item) => (
            <button
              key={item.value}
              type="button"
              className={range === item.value ? "is-active" : ""}
              aria-pressed={range === item.value}
              onClick={() => onRangeChange(item.value)}
            >
              {item.label}
            </button>
          ))}
        </div>
      </div>
      <div className="observer-chart-grid">
        {trends.series.map((series) => <TrendChart key={series.id} series={series} />)}
      </div>
      <section className="observer-panel" aria-labelledby="correlation-heading">
        <div className="observer-section-heading observer-section-heading--inside">
          <div>
            <p className="observer-kicker">Timeline context</p>
            <h3 id="correlation-heading">Events in this window</h3>
          </div>
          <span className="observer-caption">{Math.round(trends.sampledEverySeconds / 60)} minute samples</span>
        </div>
        <ol className="observer-event-strip">
          {trends.annotations.map((annotation) => (
            <li key={annotation.id}>
              <time dateTime={annotation.at}>{formatRelative(annotation.at)}</time>
              <StateBadge state={annotation.severity} />
              <div>
                <strong>{annotation.label}</strong>
                <span>{annotation.source}</span>
              </div>
            </li>
          ))}
        </ol>
      </section>
    </div>
  );
}
