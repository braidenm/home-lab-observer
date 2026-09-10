import { useMemo, useState } from "react";
import type { ResourceState } from "../hooks/useObserverData";
import type { CurrentSnapshot, LogSummary, ObserverCapabilities, Severity, TrendRange } from "../types";
import { formatEventCode, formatRelative } from "../components/format";
import { StateBadge } from "../components/StateBadge";
import { SectionStatus } from "../components/SectionStatus";
import { LogSummaryPanel } from "./LogSummaryPanel";

const severities: Severity[] = ["CRITICAL", "ERROR", "WARN", "INFO", "DEBUG", "TRACE", "UNKNOWN"];

export function LogsView({ snapshot, capabilities, summary, range, onRangeChange }: {
  snapshot: ResourceState<CurrentSnapshot>;
  capabilities: ObserverCapabilities | null;
  summary: ResourceState<LogSummary>;
  range: TrendRange;
  onRangeChange: (range: TrendRange) => void;
}) {
  const logs = snapshot.value?.sections.logs;
  const [source, setSource] = useState("all");
  const [severity, setSeverity] = useState<Severity | "all">("all");
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<number | null>(null);
  const sources = [...new Set((logs?.items ?? []).map((item) => item.metadata.source))].sort();
  const events = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return (logs?.items ?? []).map((event, index) => ({ event, index })).filter(({ event }) =>
      (source === "all" || event.metadata.source === source) &&
      (severity === "all" || event.metadata.severity === severity) &&
      (!normalized || `${event.metadata.eventCode} ${event.metadata.source}`.toLowerCase().includes(normalized))
    );
  }, [logs?.items, query, severity, source]);
  const context = selected === null ? [] : (logs?.items ?? []).slice(Math.max(0, selected - 1), selected + 2);
  const retainedBodies = (logs?.items ?? []).filter((item) => item.body.state === "REDACTED_LOCAL_ONLY").length;

  return (
    <div className="observer-view" aria-labelledby="logs-title">
      <div className="observer-view-heading"><div><p className="observer-kicker">Contract log metadata</p><h2 id="logs-title">Logs and event context</h2><p>Event-code labels are derived from code-owned metadata, never from body text.</p></div></div>
      <LogSummaryPanel summary={summary} range={range} onRangeChange={onRangeChange} />
      {!snapshot.value && <section className="observer-panel observer-panel--padded"><strong>Recent memory/session events unavailable</strong><p>The persisted metadata summary above loads independently. No current events or zero counts are inferred.</p></section>}
      {snapshot.value && logs && <><aside className="observer-privacy-callout" aria-labelledby="body-policy-title">
        <div className="observer-privacy-callout__icon" aria-hidden="true">Aa</div>
        <div><strong id="body-policy-title">Recent memory/session events</strong><p>Default body state: {capabilities?.policy.logBodies.defaultState ?? "Policy unavailable"}. Snapshot profile: {snapshot.value.privacy.profile}. Message bodies upload eligible: {snapshot.value.privacy.messageBodiesUploadEligible ? "yes" : "no"}.</p></div>
        <span className="observer-policy-chip">{retainedBodies} redacted local {retainedBodies === 1 ? "body" : "bodies"}</span>
      </aside>
      <section className="observer-panel observer-panel--flush">
        <SectionStatus section={logs} compact />
        <div className="observer-toolbar observer-toolbar--logs">
          <label><span>Source</span><select value={source} onChange={(event) => setSource(event.target.value)}><option value="all">All returned sources</option>{sources.map((item) => <option key={item} value={item}>{item}</option>)}</select></label>
          <label className="observer-search-field"><span>Filter safe metadata</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Event code or source…" /></label>
          <div className="observer-severity-filter" aria-label="Severity filter"><button type="button" className={severity === "all" ? "is-active" : ""} aria-pressed={severity === "all"} onClick={() => setSeverity("all")}>All</button>{severities.map((item) => <button key={item} type="button" className={severity === item ? "is-active" : ""} aria-pressed={severity === item} onClick={() => setSeverity(item)}>{item}</button>)}</div>
        </div>
        <div className="observer-log-summary"><span><strong>Current returned snapshot</strong></span><span><strong>{events.length}</strong> events in view</span><span><strong>{snapshot.value.privacy.redactionCount}</strong> snapshot redactions</span><span><strong>{snapshot.value.privacy.droppedCount}</strong> snapshot records dropped</span></div>
        <ol className="observer-log-list" aria-label="Log event timeline">{events.map(({ event, index }) => (
          <li key={`${event.metadata.observedAt}-${event.metadata.source}-${index}`} className={selected === index ? "is-selected" : ""}>
            <button type="button" onClick={() => setSelected(index)} aria-expanded={selected === index}>
              <time dateTime={event.metadata.observedAt}>{formatRelative(event.metadata.observedAt)}</time><StateBadge state={event.metadata.severity} />
              <span className="observer-log-list__main"><strong>{formatEventCode(event.metadata.eventCode)}</strong><span>{event.metadata.source} · {event.metadata.eventCode} · body {event.body.state}</span></span><span className="observer-log-list__context">View context</span>
            </button>
          </li>
        ))}</ol>
        {events.length === 0 && <p className="observer-empty">{emptyMessage(logs.supportState, logs.collectionState, query || source !== "all" || severity !== "all")}</p>}
      </section>
      {selected !== null && <section className="observer-panel" aria-labelledby="context-title"><div className="observer-section-heading observer-section-heading--inside"><div><p className="observer-kicker">Before and after</p><h3 id="context-title">Event context</h3></div><button className="observer-text-button" type="button" onClick={() => setSelected(null)}>Close context</button></div><p className="observer-context-note">Context contains exact metadata and body-state information only; redacted text is not displayed.</p><div className="observer-context-grid">{context.map((event, index) => <ContextEvent key={`${event.metadata.observedAt}-${index}`} event={event} />)}</div></section>}</>}
    </div>
  );
}

function ContextEvent({ event }: { event: CurrentSnapshot["sections"]["logs"]["items"][number] }) {
  return <article><div><StateBadge state={event.metadata.severity} /><time dateTime={event.metadata.observedAt}>{formatRelative(event.metadata.observedAt)}</time></div><strong>{event.metadata.eventCode}</strong><dl><div><dt>Source</dt><dd>{event.metadata.source}</dd></div><div><dt>Body state</dt><dd>{event.body.state}</dd></div>{event.body.state === "REDACTED_LOCAL_ONLY" && <div><dt>Body redactions</dt><dd>{event.body.redactionCount}</dd></div>}</dl></article>;
}

function emptyMessage(support: string, collection: string, filtered: string | boolean): string {
  if (filtered) return "No returned events match these metadata filters.";
  if (support !== "SUPPORTED") return `No events: collector support is ${support}.`;
  if (collection !== "OK") return `No events: collection state is ${collection}.`;
  return "Supported collection completed with zero events.";
}
