import type { OverviewSnapshot } from "../types";
import { formatDuration, formatMeasurement, formatRelative } from "../components/format";
import { StateBadge } from "../components/StateBadge";

export function OverviewView({ overview }: { overview: OverviewSnapshot }) {
  return (
    <div className="observer-view" aria-labelledby="overview-title">
      <section className="observer-intro-panel">
        <div>
          <p className="observer-kicker">Current state</p>
          <h2 id="overview-title">{overview.host.displayName} is steady</h2>
          <p>
            One item needs attention. Observations are {overview.freshnessSeconds} seconds old and remain on this
            machine.
          </p>
        </div>
        <div className="observer-intro-panel__facts">
          <StateBadge state={overview.status} />
          <span>{overview.host.operatingSystem}</span>
          <span>{overview.host.architecture}</span>
          <span>Up {formatDuration(overview.host.uptimeSeconds)}</span>
        </div>
      </section>

      <section aria-labelledby="resource-heading">
        <div className="observer-section-heading">
          <div>
            <p className="observer-kicker">Resource pulse</p>
            <h3 id="resource-heading">Saturation at a glance</h3>
          </div>
          <span className="observer-caption">Updated {formatRelative(overview.observedAt)}</span>
        </div>
        <div className="observer-metric-grid">
          {overview.metrics.map((metric) => (
            <article className="observer-metric-card" key={metric.id}>
              <div className="observer-metric-card__topline">
                <span>{metric.label}</span>
                <span className={`observer-trend observer-trend--${metric.trend}`}>{metric.trend}</span>
              </div>
              <strong>{formatMeasurement(metric.measurement)}</strong>
              <p>{metric.detail}</p>
              <div className="observer-meter" aria-label={`${metric.label} ${formatMeasurement(metric.measurement)}`}>
                <span
                  style={{ width: `${Math.min(metric.measurement.value ?? 0, 100)}%` }}
                  className={metric.measurement.value !== null && metric.measurement.value >= 72 ? "observer-meter__fill--attention" : ""}
                />
              </div>
            </article>
          ))}
        </div>
      </section>

      <section className="observer-panel" aria-labelledby="changes-heading">
        <div className="observer-section-heading observer-section-heading--inside">
          <div>
            <p className="observer-kicker">Recent changes</p>
            <h3 id="changes-heading">What changed, not just what exists</h3>
          </div>
          <span className="observer-caption">Last 6 hours</span>
        </div>
        <div className="observer-notice-list">
          {overview.notices.map((notice) => (
            <article className="observer-notice" key={notice.id}>
              <StateBadge state={notice.state} />
              <div>
                <strong>{notice.title}</strong>
                <p>{notice.detail}</p>
              </div>
              <time dateTime={notice.occurredAt}>{formatRelative(notice.occurredAt)}</time>
            </article>
          ))}
        </div>
      </section>
    </div>
  );
}
