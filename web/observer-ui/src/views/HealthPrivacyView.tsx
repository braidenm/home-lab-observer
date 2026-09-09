import type { ObserverHealthSnapshot } from "../types";
import { formatBytes, formatRelative } from "../components/format";
import { StateBadge } from "../components/StateBadge";

export function HealthPrivacyView({ health }: { health: ObserverHealthSnapshot }) {
  const storagePercent = (health.storage.usedBytes / health.storage.limitBytes) * 100;
  return (
    <div className="observer-view" aria-labelledby="health-title">
      <div className="observer-view-heading">
        <div>
          <p className="observer-kicker">Self-observation</p>
          <h2 id="health-title">Observer health and privacy</h2>
          <p>See what the observer can access, what it stores, and whether anything leaves this machine.</p>
        </div>
        <StateBadge state={health.status} label={`Observer ${health.status}`} />
      </div>
      <div className="observer-health-grid">
        <article className="observer-panel observer-panel--padded">
          <p className="observer-kicker">Running build</p>
          <strong className="observer-large-value">v{health.version}</strong>
          <p>Commit {health.buildCommit} · started {formatRelative(health.startedAt)}</p>
        </article>
        <article className="observer-panel observer-panel--padded">
          <p className="observer-kicker">Bounded local history</p>
          <strong className="observer-large-value">{formatBytes(health.storage.usedBytes)}</strong>
          <p>of {formatBytes(health.storage.limitBytes)} · {health.storage.retentionHours} hour limit</p>
          <div className="observer-meter" aria-label={`Storage ${storagePercent.toFixed(0)} percent used`}>
            <span style={{ width: `${storagePercent}%` }} />
          </div>
        </article>
        <article className="observer-panel observer-panel--padded">
          <p className="observer-kicker">Remote connection</p>
          <strong className="observer-large-value">{health.remoteUpload.enabled ? "Connected" : "Local only"}</strong>
          <p>{health.remoteUpload.policySummary}</p>
        </article>
      </div>

      <section className="observer-panel observer-panel--flush" aria-labelledby="collectors-title">
        <div className="observer-section-heading observer-section-heading--inside observer-section-heading--padded">
          <div><p className="observer-kicker">Capabilities</p><h3 id="collectors-title">Collector status</h3></div>
          <span className="observer-caption">Missing is not reported as zero</span>
        </div>
        <div className="observer-table-scroll" role="region" aria-label="Collector health" tabIndex={0}>
          <table className="observer-table">
            <thead><tr><th scope="col">Collector</th><th scope="col">Availability</th><th scope="col">Last success</th><th scope="col">Duration</th><th scope="col">Reason</th></tr></thead>
            <tbody>
              {health.collectors.map((collector) => (
                <tr key={collector.id}>
                  <th scope="row">{collector.label}</th>
                  <td><StateBadge state={collector.availability} /></td>
                  <td>{collector.lastSuccessAt ? formatRelative(collector.lastSuccessAt) : "Never"}</td>
                  <td>{collector.durationMilliseconds === null ? "—" : `${collector.durationMilliseconds} ms`}</td>
                  <td>{collector.reason ?? "Ready"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="observer-privacy-grid" aria-labelledby="privacy-title">
        <article className="observer-privacy-card observer-privacy-card--primary">
          <p className="observer-kicker">Active privacy boundary</p>
          <h3 id="privacy-title">Private by default</h3>
          <ul>
            <li><span aria-hidden="true">✓</span> Loopback-only dashboard and API</li>
            <li><span aria-hidden="true">✓</span> Process arguments excluded</li>
            <li><span aria-hidden="true">✓</span> Environment variables excluded</li>
            <li><span aria-hidden="true">✓</span> Log message bodies disabled</li>
            <li><span aria-hidden="true">✓</span> Remote upload disabled</li>
          </ul>
        </article>
        <article className="observer-privacy-card">
          <p className="observer-kicker">Policy counters</p>
          <dl className="observer-counter-list">
            <div><dt>Redacted fields</dt><dd>{health.counters.redactedFields}</dd></div>
            <div><dt>Dropped by bounds</dt><dd>{health.counters.droppedRecords}</dd></div>
            <div><dt>Collection failures</dt><dd>{health.counters.collectionFailures}</dd></div>
            <div><dt>Queued uploads</dt><dd>{health.counters.queuedUploads}</dd></div>
          </dl>
        </article>
      </section>
    </div>
  );
}
