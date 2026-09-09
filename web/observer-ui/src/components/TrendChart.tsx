import { useId } from "react";
import type { TrendSeries } from "../types";
import { formatValue } from "./format";

export function TrendChart({ series }: { series: TrendSeries }) {
  const id = useId();
  const values = series.points.flatMap((point) => (point.value === null ? [] : [point.value]));
  const min = values.length > 0 ? Math.min(...values) : 0;
  const max = values.length > 0 ? Math.max(...values) : 1;
  const span = Math.max(max - min, 1);
  const paths: string[] = [];
  let currentPath = "";
  series.points.forEach((point, index) => {
    if (point.value === null) {
      if (currentPath) paths.push(currentPath);
      currentPath = "";
      return;
    }
    const x = series.points.length === 1 ? 50 : (index / (series.points.length - 1)) * 100;
    const y = 92 - ((point.value - min) / span) * 74;
    currentPath += `${currentPath ? " L" : "M"}${x.toFixed(2)},${y.toFixed(2)}`;
  });
  if (currentPath) paths.push(currentPath);
  const current = series.points.at(-1)?.value ?? null;

  return (
    <article className="observer-chart-card">
      <div className="observer-chart-card__heading">
        <div>
          <p className="observer-eyebrow">{series.label}</p>
          <strong>{formatValue(current, series.unit)}</strong>
        </div>
        <StateKey color={series.color} />
      </div>
      <svg
        className={`observer-chart observer-chart--${series.color}`}
        viewBox="0 0 100 100"
        preserveAspectRatio="none"
        role="img"
        aria-labelledby={`${id}-title ${id}-description`}
      >
        <title id={`${id}-title`}>{series.label} trend</title>
        <desc id={`${id}-description`}>
          {values.length > 0 ? `${values.length} numeric samples ranging from ${min.toFixed(1)} to ${max.toFixed(1)}. Missing samples are gaps.` : "No numeric samples are available."}
        </desc>
        <line x1="0" y1="25" x2="100" y2="25" className="observer-chart__grid" />
        <line x1="0" y1="58" x2="100" y2="58" className="observer-chart__grid" />
        <line x1="0" y1="92" x2="100" y2="92" className="observer-chart__grid" />
        {paths.map((path, index) => <path key={index} d={path} className="observer-chart__line" />)}
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
