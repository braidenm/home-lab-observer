import type { TrendSeries } from "../types";
import { formatMeasurement } from "./format";

export function TrendChart({ series }: { series: TrendSeries }) {
  const values = series.points.flatMap((point) => (point.value === null ? [] : [point.value]));
  const min = values.length > 0 ? Math.min(...values) : 0;
  const max = values.length > 0 ? Math.max(...values) : 1;
  const span = Math.max(max - min, 1);
  const path = series.points
    .map((point, index) => {
      const x = series.points.length === 1 ? 50 : (index / (series.points.length - 1)) * 100;
      const y = point.value === null ? 95 : 92 - ((point.value - min) / span) * 74;
      return `${index === 0 ? "M" : "L"}${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");
  const current = series.points.at(-1)?.value ?? null;

  return (
    <article className="observer-chart-card">
      <div className="observer-chart-card__heading">
        <div>
          <p className="observer-eyebrow">{series.label}</p>
          <strong>{formatMeasurement({ value: current, unit: series.unit, availability: series.availability })}</strong>
        </div>
        <StateKey color={series.color} />
      </div>
      <svg
        className={`observer-chart observer-chart--${series.color}`}
        viewBox="0 0 100 100"
        preserveAspectRatio="none"
        role="img"
        aria-labelledby={`chart-${series.id}-title chart-${series.id}-description`}
      >
        <title id={`chart-${series.id}-title`}>{series.label} trend</title>
        <desc id={`chart-${series.id}-description`}>
          {values.length} samples ranging from {min.toFixed(1)} to {max.toFixed(1)}.
        </desc>
        <line x1="0" y1="25" x2="100" y2="25" className="observer-chart__grid" />
        <line x1="0" y1="58" x2="100" y2="58" className="observer-chart__grid" />
        <line x1="0" y1="92" x2="100" y2="92" className="observer-chart__grid" />
        <path d={`${path} L100,96 L0,96 Z`} className="observer-chart__area" />
        <path d={path} className="observer-chart__line" />
      </svg>
      <div className="observer-chart-card__range" aria-hidden="true">
        <span>Earlier</span>
        <span>Now</span>
      </div>
    </article>
  );
}

function StateKey({ color }: { color: TrendSeries["color"] }) {
  return <span className={`observer-chart-key observer-chart-key--${color}`} aria-hidden="true" />;
}
