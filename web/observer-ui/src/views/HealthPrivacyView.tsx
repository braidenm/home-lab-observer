import type { CurrentSnapshot, ObserverCapabilities, SectionName } from "../types";
import { formatBytes, formatRelative, formatValue, titleCase } from "../components/format";
import { StateBadge } from "../components/StateBadge";

const sectionNames: SectionName[] = ["overview", "filesystems", "processes", "services", "containers", "logs", "observer"];

export function HealthPrivacyView({ capabilities, snapshot, mode }: { capabilities: ObserverCapabilities | null; snapshot: CurrentSnapshot | null; mode: "local" | "demo" | "embedded" }) {
  const transport = mode === "demo" ? "Synthetic demo transport" : mode === "embedded" ? "Transport supplied by the embedding application" : capabilities ? `Default API bind ${capabilities.policy.defaultBind}` : "Local transport policy unavailable";
  const uploadEligible = capabilities?.collectors.filter((item) => item.uploadEligible).length;
  return (
    <div className="observer-view" aria-labelledby="health-title">
      <div className="observer-view-heading"><div><p className="observer-kicker">Reported policy and quality</p><h2 id="health-title">Observer health and privacy</h2><p>Every sentence below is derived from the active data source and display mode.</p></div>{snapshot && <StateBadge state={snapshot.collectionState} label={`Snapshot ${snapshot.collectionState}`} />}</div>

      <div className="observer-health-grid">
        <Fact title="Observer" value={capabilities ? `v${capabilities.observer.version}` : "Unavailable"} detail={capabilities ? `${capabilities.observer.mode} · ${capabilities.platform.os}/${capabilities.platform.architecture}` : "Capabilities request did not produce data"} />
        <Fact title="Transport" value={transport} detail={capabilities ? `CORS ${capabilities.policy.corsEnabled ? "enabled" : "disabled"}` : "No transport policy reported"} />
        <Fact title="Retention ceiling" value={capabilities ? formatBytes(capabilities.policy.retentionBytes) : "Unavailable"} detail={capabilities ? `${capabilities.policy.retentionDays} day limit` : "Capabilities unavailable"} />
      </div>

      <section className="observer-panel observer-panel--flush" aria-labelledby="collectors-title">
        <div className="observer-section-heading observer-section-heading--inside"><div><p className="observer-kicker">Complete state envelope</p><h3 id="collectors-title">Collector and section status</h3></div><span className="observer-caption">Missing is never rendered as zero</span></div>
        <p className="observer-table-note">Scroll the table horizontally to inspect all collection details on smaller screens.</p>
        <div className="observer-table-scroll" role="region" aria-label="Collector and section status table; horizontally scrollable on small screens" tabIndex={0}>
          <table className="observer-table"><caption className="observer-visually-hidden">Capability support plus snapshot collection, freshness, reason, count, and truncation for every section</caption><thead><tr><th scope="col">Section</th><th scope="col">Capability</th><th scope="col">Snapshot support</th><th scope="col">Collection</th><th scope="col">Freshness</th><th scope="col">Observed</th><th scope="col">Returned / total</th><th scope="col">Truncated</th><th scope="col">Reason</th><th scope="col">Class / upload</th></tr></thead>
            <tbody>{sectionNames.map((name) => {
              const capability = capabilities?.collectors.find((item) => item.name === name);
              const section = snapshot?.sections[name];
              const list = section && "totalCount" in section ? section : null;
              return <tr key={name}><th scope="row">{titleCase(name)}</th><td>{capability ? <StateBadge state={capability.supportState} /> : "Unavailable"}</td><td>{section ? <StateBadge state={section.supportState} /> : "Unavailable"}</td><td>{section ? <StateBadge state={section.collectionState} /> : "Unavailable"}</td><td>{section ? <StateBadge state={section.freshness} /> : "Unavailable"}</td><td>{section?.observedAt ? formatRelative(section.observedAt) : "Not observed"}</td><td>{list ? `${list.returnedCount} / ${list.totalCount}` : "Not applicable"}</td><td>{list ? (list.truncated ? "Yes" : "No") : "Not applicable"}</td><td>{section?.reasonCode ?? capability?.reasonCode ?? "None"}</td><td>{capability ? `${capability.dataClassification} / ${capability.uploadEligible ? "eligible" : "not eligible"}` : "Unavailable"}</td></tr>;
            })}</tbody>
          </table>
        </div>
      </section>

      <section className="observer-privacy-grid" aria-labelledby="privacy-title">
        <article className="observer-privacy-card observer-privacy-card--primary"><p className="observer-kicker">Active reported policy</p><h3 id="privacy-title">Privacy facts</h3><ul>
          <li><span aria-hidden="true">•</span>{capabilities ? `Read-only: ${capabilities.policy.readOnly ? "yes" : "no"}` : "Read-only policy unavailable"}</li>
          <li><span aria-hidden="true">•</span>{capabilities ? `Default log body state: ${capabilities.policy.logBodies.defaultState}` : "Log body policy unavailable"}</li>
          <li><span aria-hidden="true">•</span>{snapshot ? `Body upload eligible: ${snapshot.privacy.messageBodiesUploadEligible ? "yes" : "no"}` : "Body upload policy unavailable"}</li>
          <li><span aria-hidden="true">•</span>{snapshot ? `Remote projection: ${snapshot.privacy.remoteProjection}` : "Remote projection unavailable"}</li>
          <li><span aria-hidden="true">•</span>{uploadEligible === undefined ? "Collector upload policy unavailable" : `${uploadEligible} of ${capabilities!.collectors.length} collectors upload eligible`}</li>
        </ul></article>
        <article className="observer-privacy-card"><p className="observer-kicker">Snapshot privacy</p><dl className="observer-counter-list"><div><dt>Profile</dt><dd>{snapshot?.privacy.profile ?? "Unavailable"}</dd></div><div><dt>Redactions</dt><dd>{snapshot?.privacy.redactionCount ?? "Unavailable"}</dd></div><div><dt>Dropped</dt><dd>{snapshot?.privacy.droppedCount ?? "Unavailable"}</dd></div></dl>{snapshot && <p>Excluded: {snapshot.privacy.excludedFields.map(titleCase).join(", ") || "none reported"}</p>}</article>
      </section>

      {snapshot && <section className="observer-panel observer-panel--flush" aria-labelledby="signals-title"><div className="observer-section-heading observer-section-heading--inside"><div><p className="observer-kicker">Observer section</p><h3 id="signals-title">Self-observation signals</h3></div></div><div className="observer-table-scroll" role="region" aria-label="Observer signals table" tabIndex={0}><table className="observer-table"><caption className="observer-visually-hidden">Observer signal name, state, and exact value</caption><thead><tr><th scope="col">Signal</th><th scope="col">State</th><th scope="col">Value</th></tr></thead><tbody>{snapshot.sections.observer.items.map((item) => <tr key={item.name}><th scope="row">{titleCase(item.name)}</th><td><StateBadge state={item.state} /></td><td>{formatValue(item.value, item.unit)}</td></tr>)}</tbody></table></div></section>}
    </div>
  );
}

function Fact({ title, value, detail }: { title: string; value: string; detail: string }) { return <article className="observer-panel observer-panel--padded"><p className="observer-kicker">{title}</p><strong className="observer-large-value">{value}</strong><p>{detail}</p></article>; }
