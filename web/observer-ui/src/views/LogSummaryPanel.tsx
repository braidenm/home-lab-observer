import type { ResourceState } from "../hooks/useObserverData";
import type { LogCounts, LogSeverityCounts, LogSourceSummary, LogSummary, TrendRange } from "../types";
import { titleCase } from "../components/format";
import { SectionStatus } from "../components/SectionStatus";
import { StateBadge } from "../components/StateBadge";

const ranges: TrendRange[] = ["1h", "6h", "24h", "7d"];
const severityKeys: Array<keyof LogSeverityCounts> = ["critical", "error", "warn", "info", "debug", "trace", "unknown"];

export function LogSummaryPanel({ summary, range, onRangeChange }: { summary: ResourceState<LogSummary>; range: TrendRange; onRangeChange: (range: TrendRange) => void }) {
  return (
    <section className="observer-panel observer-log-history" aria-labelledby="log-summary-title">
      <div className="observer-section-heading observer-section-heading--inside observer-log-history__heading">
        <div><p className="observer-kicker">Persisted rollups</p><h3 id="log-summary-title">Log metadata summary</h3><p>Captured metadata is counted locally without storing log bodies, identities, or event codes.</p></div>
        <div className="observer-segmented" role="group" aria-label="Log summary range">
          {ranges.map((item) => <button type="button" key={item} className={range === item ? "is-active" : ""} aria-pressed={range === item} onClick={() => onRangeChange(item)}>{item}</button>)}
        </div>
      </div>
      <p className="observer-log-history__privacy"><strong>Local-sensitive metadata</strong> · read-only · bodies and identity fields omitted · remote upload not eligible</p>
      <p className="observer-log-history__privacy">Operational history, not a forensic audit. On Windows, clearing a log and recreating an event with identical identity fields can evade reset detection.</p>
      <LogSummaryState summary={summary} />
    </section>
  );
}

function LogSummaryState({ summary }: { summary: ResourceState<LogSummary> }) {
  if (summary.status === "unsupported") return <div className="observer-log-history__state"><strong>No log summary endpoint</strong><p>This data source keeps the recent memory/session events below, but does not provide local persisted history.</p></div>;
  if (summary.status === "error") return <div className="observer-inline-error" role="alert"><strong>Log summary unavailable</strong><p>{summary.error}. Recent memory/session events continue independently.</p></div>;
  if (summary.status === "loading" || !summary.value) return <div className="observer-log-history__loading" role="status"><span>Loading log metadata summary…</span></div>;
  const value = summary.value;
  if (value.sources.length === 0) {
    return <div className="observer-log-history__state"><strong>Native log summaries are disabled</strong><p>No source is configured. Zero counts are not inferred, and this dashboard cannot enable collection.</p></div>;
  }
  return (
    <>
      <SectionStatus section={value} compact />
      <dl className="observer-log-history__totals">
        <SummaryMetric label="Captured in range" value={countLabel(value.counts, "captured")} />
        <SummaryMetric label="Discarded in range" value={countLabel(value.counts, "discarded")} />
        <SummaryMetric label="Historical coverage" value={titleCase(value.coverageState)} />
        <SummaryMetric label="Generated" value={formatTimestamp(value.generatedAt)} />
      </dl>
      <div className="observer-log-source-grid">
        {value.sources.map((source) => <LogSourceCard key={source.source} source={source} intervalSeconds={value.bucketIntervalSeconds} />)}
      </div>
      <p className="observer-table-note">Counts are captured observations, not every machine event. Coverage gaps remain visible even when delayed records were captured in the same time bucket.</p>
    </>
  );
}

function SummaryMetric({ label, value }: { label: string; value: string }) {
  return <div><dt>{label}</dt><dd>{value}</dd></div>;
}

function LogSourceCard({ source, intervalSeconds }: { source: LogSourceSummary; intervalSeconds: number }) {
  const label = source.source === "system" ? "System" : "Application";
  const chartHidden = source.status.supportState === "UNSUPPORTED" && source.buckets.every((bucket) => bucket.coverageState === "UNKNOWN" && bucket.counts === null);
  return (
    <article className="observer-log-source" aria-labelledby={`log-source-${source.source}`}>
      <div className="observer-log-source__heading">
        <div><p className="observer-kicker">{source.source} source</p><h4 id={`log-source-${source.source}`}>{label} log metadata</h4></div>
        <StateBadge state={source.status.supportState} />
      </div>
      <dl className="observer-log-source__status">
        <div><dt>Collection</dt><dd><StateBadge state={source.status.collectionState} /></dd></div>
        <div><dt>Freshness</dt><dd><StateBadge state={source.status.freshness} /></dd></div>
        <div><dt>Coverage</dt><dd><StateBadge state={source.coverageState} /></dd></div>
        <div><dt>Reason</dt><dd>{source.status.reasonCode ? titleCase(source.status.reasonCode) : "None"}</dd></div>
        <div><dt>Last attempt</dt><dd>{source.status.attemptedAt ? formatTimestamp(source.status.attemptedAt) : "Not attempted"}</dd></div>
        <div><dt>Coverage watermark</dt><dd>{source.status.coverageThrough ? formatTimestamp(source.status.coverageThrough) : "Unavailable"}</dd></div>
      </dl>
      <div className="observer-log-source__counts"><span><strong>{countLabel(source.counts, "captured")}</strong> captured</span><span><strong>{countLabel(source.counts, "discarded")}</strong> discarded</span></div>
      {chartHidden ? <p className="observer-log-source__empty">This source is unsupported here. Its unknown grid is omitted; no zero history is inferred.</p> : <LogHistogram source={source} intervalSeconds={intervalSeconds} label={label} />}
    </article>
  );
}

function LogHistogram({ source, intervalSeconds, label }: { source: LogSourceSummary; intervalSeconds: number; label: string }) {
  const maximum = Math.max(1, ...source.buckets.map((bucket) => bucket.counts?.captured ?? 0));
  return (
    <div className="observer-log-histogram-wrap">
      <div className="observer-log-legend" aria-label="Severity legend">{severityKeys.map((severity) => <span key={severity}><i className={`observer-log-severity observer-log-severity--${severity}`} aria-hidden="true" />{titleCase(severity)}</span>)}</div>
      <ul className="observer-log-coverage-legend" aria-label="Coverage legend">
        {(["FULL", "PARTIAL", "GAP", "UNKNOWN"] as const).map((state) => <li key={state}><i className={`observer-log-coverage-key observer-log-coverage-key--${state.toLowerCase()}`} aria-hidden="true" />{titleCase(state)}</li>)}
      </ul>
      <div className="observer-log-histogram" role="img" aria-label={`${label} captured severity histogram. Complete bucket values and coverage are available in the table below.`}>
        {source.buckets.map((bucket) => (
          <div key={bucket.at} className={`observer-log-histogram__bucket observer-log-histogram__bucket--${bucket.coverageState.toLowerCase()}`} aria-hidden="true">
            <div className="observer-log-histogram__stack">
              {severityKeys.map((severity) => <span key={severity} className={`observer-log-severity--${severity}`} style={{ height: `${((bucket.counts?.severity[severity] ?? 0) / maximum) * 100}%` }} />)}
            </div>
          </div>
        ))}
      </div>
      <div className="observer-log-histogram__range"><time dateTime={source.buckets[0]?.at}>{source.buckets[0] ? formatTimestamp(source.buckets[0].at) : "No buckets"}</time><span>{intervalSeconds / 60}m buckets</span><time dateTime={source.buckets.at(-1)?.at}>{source.buckets.at(-1) ? formatTimestamp(source.buckets.at(-1)!.at) : "No buckets"}</time></div>
      <details className="observer-log-bucket-details">
        <summary>View accessible bucket data</summary>
        <p className="observer-table-scroll-hint">Scroll sideways to inspect every severity column.</p>
        <div className="observer-table-scroll" role="region" tabIndex={0} aria-label={`${label} log summary bucket data`}>
          <table className="observer-table observer-log-bucket-table">
            <thead><tr><th scope="col">Time</th><th scope="col">Coverage</th><th scope="col">Captured</th><th scope="col">Discarded</th>{severityKeys.map((severity) => <th scope="col" key={severity}>{titleCase(severity)}</th>)}<th scope="col">Reason</th></tr></thead>
            <tbody>{source.buckets.map((bucket) => <tr key={bucket.at}>
              <th scope="row"><time dateTime={bucket.at}>{formatTimestamp(bucket.at)}</time></th>
              <td>{titleCase(bucket.coverageState)} · {bucket.coveredSeconds}s</td>
              <td>{countLabel(bucket.counts, "captured")}</td><td>{countLabel(bucket.counts, "discarded")}</td>
              {severityKeys.map((severity) => <td key={severity}>{bucket.counts ? bucket.counts.severity[severity].toLocaleString() : "Unavailable"}</td>)}
              <td>{bucket.reasonCode ? titleCase(bucket.reasonCode) : "None"}</td>
            </tr>)}</tbody>
          </table>
        </div>
      </details>
    </div>
  );
}

function countLabel(counts: LogCounts | null, key: keyof LogCounts): string { return counts === null ? "Unavailable" : counts[key].toLocaleString(); }
function formatTimestamp(value: string): string { return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }).format(new Date(value)); }
