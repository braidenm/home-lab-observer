import { useMemo, useState } from "react";
import type { LogEvent, LogSnapshot, Severity } from "../types";
import { formatRelative } from "../components/format";
import { StateBadge } from "../components/StateBadge";

const severities: Severity[] = ["critical", "error", "warning", "info", "debug"];

export function LogsView({ logs }: { logs: LogSnapshot }) {
  const [source, setSource] = useState("all");
  const [severity, setSeverity] = useState<Severity | "all">("all");
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const events = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return logs.events.filter((event) =>
      (source === "all" || event.sourceId === source) &&
      (severity === "all" || event.severity === severity) &&
      (!normalized || `${event.summary} ${event.code} ${event.unit}`.toLowerCase().includes(normalized))
    );
  }, [logs.events, query, severity, source]);
  const selectedIndex = logs.events.findIndex((event) => event.id === selected);
  const context = selectedIndex < 0 ? [] : logs.events.slice(Math.max(0, selectedIndex - 1), selectedIndex + 2);

  return (
    <div className="observer-view" aria-labelledby="logs-title">
      <div className="observer-view-heading">
        <div>
          <p className="observer-kicker">Allowlisted event metadata</p>
          <h2 id="logs-title">Logs and event context</h2>
          <p>Start with severity, source, code, and safe structured fields. Expand context without widening collection.</p>
        </div>
      </div>
      <aside className="observer-privacy-callout" aria-labelledby="body-policy-title">
        <div className="observer-privacy-callout__icon" aria-hidden="true">Aa</div>
        <div>
          <strong id="body-policy-title">Message bodies are off by default</strong>
          <p>
            Summaries are code-owned, sanitized labels—not source message text. Bodies require an explicit allowlist
            and are redacted before persistence. No source has bodies enabled.
          </p>
        </div>
        <span className="observer-policy-chip">0 enabled</span>
      </aside>
      <section className="observer-panel observer-panel--flush">
        <div className="observer-toolbar observer-toolbar--logs">
          <label>
            <span>Source</span>
            <select value={source} onChange={(event) => setSource(event.target.value)}>
              <option value="all">All enabled sources</option>
              {logs.sources.map((item) => (
                <option key={item.id} value={item.id} disabled={item.availability !== "supported"}>
                  {item.label}{item.availability !== "supported" ? ` — ${item.availability}` : ""}
                </option>
              ))}
            </select>
          </label>
          <label className="observer-search-field">
            <span>Filter safe metadata</span>
            <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Code, unit, summary…" />
          </label>
          <div className="observer-severity-filter" aria-label="Severity filter">
            <button type="button" className={severity === "all" ? "is-active" : ""} aria-pressed={severity === "all"} onClick={() => setSeverity("all")}>All</button>
            {severities.map((item) => (
              <button key={item} type="button" className={severity === item ? "is-active" : ""} aria-pressed={severity === item} onClick={() => setSeverity(item)}>
                {item}
              </button>
            ))}
          </div>
        </div>
        <div className="observer-log-summary">
          <span><strong>{events.length}</strong> events in view</span>
          <span><strong>{logs.redactedFieldCount}</strong> fields redacted</span>
          <span><strong>{logs.droppedEventCount}</strong> events dropped by bounds</span>
        </div>
        <ol className="observer-log-list" aria-label="Log event timeline">
          {events.map((event) => (
            <li key={event.id} className={selected === event.id ? "is-selected" : ""}>
              <button type="button" onClick={() => setSelected(event.id)} aria-expanded={selected === event.id}>
                <time dateTime={event.at}>{formatRelative(event.at)}</time>
                <StateBadge state={event.severity} />
                <span className="observer-log-list__main">
                  <strong>{event.summary}</strong>
                  <span>{event.sourceLabel} · {event.unit} · {event.code}</span>
                </span>
                <span className="observer-log-list__context">View context</span>
              </button>
            </li>
          ))}
        </ol>
        {events.length === 0 && <p className="observer-empty">No events match these safe metadata filters.</p>}
      </section>
      {selected && (
        <section className="observer-panel" aria-labelledby="context-title">
          <div className="observer-section-heading observer-section-heading--inside">
            <div>
              <p className="observer-kicker">Before and after</p>
              <h3 id="context-title">Event context</h3>
            </div>
            <button className="observer-text-button" type="button" onClick={() => setSelected(null)}>Close context</button>
          </div>
          <p className="observer-context-note">Context includes metadata and allowlisted structured fields only.</p>
          <div className="observer-context-grid">
            {context.map((event) => <ContextEvent key={event.id} event={event} selected={event.id === selected} />)}
          </div>
        </section>
      )}
    </div>
  );
}

function ContextEvent({ event, selected }: { event: LogEvent; selected: boolean }) {
  return (
    <article className={selected ? "is-selected" : ""}>
      <div><StateBadge state={event.severity} /><time dateTime={event.at}>{formatRelative(event.at)}</time></div>
      <strong>{event.code}</strong>
      <dl>
        {Object.entries(event.structuredFields).map(([key, value]) => (
          <div key={key}><dt>{key.replaceAll("_", " ")}</dt><dd>{String(value ?? "unavailable")}</dd></div>
        ))}
      </dl>
    </article>
  );
}
