import type { CurrentSnapshot } from "../types";
import { formatBytes, formatDuration, formatRelative } from "../components/format";
import { StateBadge } from "../components/StateBadge";
import { SectionStatus } from "../components/SectionStatus";

export function OverviewView({ snapshot }: { snapshot: CurrentSnapshot }) {
  const overview = snapshot.sections.overview;
  const host = overview.data;
  const filesystems = snapshot.sections.filesystems;
  const memoryPercent = host && host.memoryTotalBytes > 0 && host.memoryUsedBytes <= host.memoryTotalBytes
    ? (host.memoryUsedBytes / host.memoryTotalBytes) * 100 : null;
  const storageTotal = filesystems.items.reduce((sum, item) => sum + item.totalBytes, 0);
  const storageUsed = filesystems.items.reduce((sum, item) => sum + item.usedBytes, 0);
  const storagePercent = storageTotal > 0 && storageUsed <= storageTotal ? (storageUsed / storageTotal) * 100 : null;
  const gaps = Object.entries(snapshot.sections).filter(([, section]) => section.supportState !== "SUPPORTED" || section.collectionState !== "OK" || section.freshness !== "CURRENT" || section.reasonCode || ("truncated" in section && section.truncated));

  return (
    <div className="observer-view" aria-labelledby="overview-title">
      <section className="observer-intro-panel">
        <div>
          <p className="observer-kicker">Current snapshot</p>
          <h2 id="overview-title">{host?.hostAlias ?? "Overview unavailable"}</h2>
          <p>Sequence {snapshot.sequence} collected {formatRelative(snapshot.observedAt)} in {snapshot.durationMilliseconds} ms.</p>
        </div>
        <div className="observer-intro-panel__facts">
          <StateBadge state={snapshot.collectionState} label={`Snapshot ${snapshot.collectionState}`} />
          {host && <><span>{host.os}</span><span>{host.architecture}</span><span>Up {formatDuration(host.uptimeSeconds)}</span></>}
        </div>
      </section>

      <section aria-labelledby="resource-heading">
        <div className="observer-section-heading">
          <div><p className="observer-kicker">Contract facts</p><h3 id="resource-heading">Host resources</h3></div>
          <span className="observer-caption">Null and missing observations remain unavailable</span>
        </div>
        {host ? (
          <div className="observer-metric-grid">
            <Metric label="Memory used" value={memoryPercent === null ? "Unavailable" : `${memoryPercent.toFixed(1)}%`} percent={memoryPercent} detail={`${formatBytes(host.memoryUsedBytes)} of ${formatBytes(host.memoryTotalBytes)}`} />
            <Metric label="Logical processors" value={host.cpuLogicalCount.toLocaleString()} percent={null} detail="Capacity; CPU utilization is not in v1" />
            <Metric label="Filesystem used" value={storagePercent === null ? "Unavailable" : `${storagePercent.toFixed(1)}%`} percent={storagePercent} detail={`${formatBytes(storageUsed)} of ${formatBytes(storageTotal)}`} />
            <Metric label="Filesystems returned" value={`${filesystems.returnedCount} of ${filesystems.totalCount}`} percent={null} detail={filesystems.truncated ? "Result is truncated" : "Complete bounded result"} />
          </div>
        ) : <DataGap text={overview.reasonCode ?? overview.collectionState} />}
      </section>

      <section className="observer-panel observer-panel--padded" aria-labelledby="overview-quality-title">
        <div className="observer-section-heading"><div><p className="observer-kicker">Data quality</p><h3 id="overview-quality-title">Overview section</h3></div></div>
        <SectionStatus section={overview} />
      </section>

      <section className="observer-panel" aria-labelledby="gaps-heading">
        <div className="observer-section-heading observer-section-heading--inside">
          <div><p className="observer-kicker">Collection gaps</p><h3 id="gaps-heading">Signals needing interpretation</h3></div>
          <span className="observer-caption">{gaps.length} of 7 sections</span>
        </div>
        {gaps.length === 0 ? <p className="observer-empty">All sections report supported, current, complete collection.</p> : (
          <div className="observer-notice-list">{gaps.map(([name, section]) => (
            <article className="observer-notice" key={name}>
              <StateBadge state={section.supportState !== "SUPPORTED" ? section.supportState : section.collectionState} />
              <div><strong>{name}</strong><p>{section.reasonCode ?? ("truncated" in section && section.truncated ? `${section.returnedCount} of ${section.totalCount} returned` : `Freshness: ${section.freshness}`)}</p></div>
              <span>{section.observedAt ? formatRelative(section.observedAt) : "Not run"}</span>
            </article>
          ))}</div>
        )}
      </section>
    </div>
  );
}

function Metric({ label, value, detail, percent }: { label: string; value: string; detail: string; percent: number | null }) {
  return <article className="observer-metric-card"><div className="observer-metric-card__topline"><span>{label}</span></div><strong>{value}</strong><p>{detail}</p><div className="observer-meter" aria-label={`${label}: ${value}`}>{percent !== null && <span style={{ width: `${Math.min(Math.max(percent, 0), 100)}%` }} />}</div></article>;
}

function DataGap({ text }: { text: string }) { return <p className="observer-empty"><StateBadge state="UNKNOWN" /> {text}</p>; }
