import { useId } from "react";
import type { TrendSeries } from "../types";
import { formatValue } from "./format";
import { StateBadge } from "./StateBadge";

export function TrendChart({ series }: { series: TrendSeries }) {
  const id = useId();
  const values = series.points.flatMap((point) => (point.value === null ? [] : [point.value]));
  const min = values.length > 0 ? Math.min(...values) : 0;
  const max = values.length > 0 ? Math.max(...values) : 1;
  const span = Math.max(max - min, 1);
  const paths: string[] = [];
  const isolated: Array<{ x: number; y: number }> = [];
  let currentPath = "";
  series.points.forEach((point, index) => {
    if (point.value === null) {
      if (currentPath) paths.push(currentPath);
      currentPath = "";
      return;
    }
    const x = series.points.length === 1 ? 50 : 2 + (index / (series.points.length - 1)) * 96;
    const y = 92 - ((point.value - min) / span) * 74;
    if (series.points[index - 1]?.value == null && series.points[index + 1]?.value == null) isolated.push({ x, y });
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
        <div><StateBadge state={series.supportState} /><StateKey color={series.color} /></div>
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
        {isolated.map((point, index) => <path key={`point-${index}`} d={`M${point.x},${point.y} l0.01,0`} className="observer-chart__line observer-chart__point" />)}
      </svg>
      <div className="observer-chart-card__range" aria-hidden="true">
        <span>Earlier</span>
        <span>Now</span>
      </div>
      <p className="observer-muted">{values.length} observed {values.length === 1 ? "point" : "points"} · {series.gapCount} gaps{series.truncated ? " · response truncated" : ""}{series.reasonCode ? ` · ${series.reasonCode.replaceAll("_", " ").toLowerCase()}` : ""}</p>
    </article>
  );
}

function StateKey({ color }: { color: TrendSeries["color"] }) {
  return <span className={`observer-chart-key observer-chart-key--${color}`} aria-hidden="true" />;
}
