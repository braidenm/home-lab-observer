import { useId, useMemo, useState } from "react";
import type { CurrentSnapshot } from "../types";
import { formatBytes } from "../components/format";
import { StateBadge } from "../components/StateBadge";
import { SectionStatus } from "../components/SectionStatus";

type WorkloadKind = "processes" | "services" | "containers";
const kinds: Array<{ value: WorkloadKind; label: string }> = [
  { value: "processes", label: "Processes" }, { value: "services", label: "Services" }, { value: "containers", label: "Containers" }
];

export function WorkloadsView({ snapshot }: { snapshot: CurrentSnapshot }) {
  const id = useId();
  const [kind, setKind] = useState<WorkloadKind>("processes");
  const [query, setQuery] = useState("");
  const section = snapshot.sections[kind];
  const normalized = query.trim().toLowerCase();
  const items = useMemo(() => section.items.filter((item) => !normalized || `${item.name} ${item.state}`.toLowerCase().includes(normalized)), [normalized, section.items]);

  const selectRelative = (index: number) => {
    const next = kinds[(index + kinds.length) % kinds.length];
    setKind(next.value);
    requestAnimationFrame(() => document.getElementById(`${id}-${next.value}`)?.focus());
  };

  return (
    <div className="observer-view" aria-labelledby="workloads-title">
      <div className="observer-view-heading"><div><p className="observer-kicker">Bounded current snapshot</p><h2 id="workloads-title">Workloads</h2><p>Only fields in the accepted process, service, and container contracts are shown.</p></div></div>
      <section className="observer-panel observer-panel--flush">
        <div className="observer-toolbar">
          <div className="observer-segmented" role="tablist" aria-label="Workload type">
            {kinds.map((item, index) => (
              <button key={item.value} id={`${id}-${item.value}`} type="button" role="tab" aria-selected={kind === item.value} aria-controls={`${id}-panel`} tabIndex={kind === item.value ? 0 : -1} className={kind === item.value ? "is-active" : ""} onClick={() => setKind(item.value)} onKeyDown={(event) => { if (event.key === "ArrowRight") selectRelative(index + 1); if (event.key === "ArrowLeft") selectRelative(index - 1); }}>{item.label}</button>
            ))}
          </div>
          <label><span>Filter returned records</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={`Find ${kind.slice(0, -2)}…`} /></label>
        </div>
        <div id={`${id}-panel`} role="tabpanel" aria-labelledby={`${id}-${kind}`}>
          <SectionStatus section={section} compact />
          <div className="observer-table-scroll" role="region" aria-label={`${kind} table; horizontally scrollable on small screens`} tabIndex={0}>
            {kind === "processes" && <ProcessTable items={items as CurrentSnapshot["sections"]["processes"]["items"]} />}
            {kind === "services" && <ServiceTable items={items as CurrentSnapshot["sections"]["services"]["items"]} />}
            {kind === "containers" && <ContainerTable items={items as CurrentSnapshot["sections"]["containers"]["items"]} />}
          </div>
          {items.length === 0 && <p className="observer-empty">{emptyMessage(section.supportState, section.collectionState, normalized.length > 0)}</p>}
        </div>
      </section>
    </div>
  );
}

function ProcessTable({ items }: { items: CurrentSnapshot["sections"]["processes"]["items"] }) {
  return <table className="observer-table"><caption className="observer-visually-hidden">Processes: name, state, CPU, memory, and process ID</caption><thead><tr><th scope="col">Name</th><th scope="col">State</th><th scope="col">CPU</th><th scope="col">Memory</th><th scope="col">Process ID</th></tr></thead><tbody>{items.map((item) => <tr key={item.pid}><th scope="row">{item.name}</th><td><StateBadge state={item.state} /></td><td>{item.cpuPercent.toFixed(1)}%</td><td>{formatBytes(item.memoryBytes)}</td><td>{item.pid}</td></tr>)}</tbody></table>;
}

function ServiceTable({ items }: { items: CurrentSnapshot["sections"]["services"]["items"] }) {
  return <table className="observer-table"><caption className="observer-visually-hidden">Services: name, state, and configured start mode</caption><thead><tr><th scope="col">Name</th><th scope="col">State</th><th scope="col">Start mode</th></tr></thead><tbody>{items.map((item) => <tr key={item.name}><th scope="row">{item.name}</th><td><StateBadge state={item.state} /></td><td>{item.startMode}</td></tr>)}</tbody></table>;
}

function ContainerTable({ items }: { items: CurrentSnapshot["sections"]["containers"]["items"] }) {
  return <table className="observer-table"><caption className="observer-visually-hidden">Containers: name and image, state, CPU, memory, and alias</caption><thead><tr><th scope="col">Name / image</th><th scope="col">State</th><th scope="col">CPU</th><th scope="col">Memory</th><th scope="col">Alias</th></tr></thead><tbody>{items.map((item) => <tr key={item.idAlias}><th scope="row"><strong>{item.name}</strong><small>{item.image}</small></th><td><StateBadge state={item.state} /></td><td>{item.cpuPercent.toFixed(1)}%</td><td>{formatBytes(item.memoryBytes)}</td><td>{item.idAlias}</td></tr>)}</tbody></table>;
}

function emptyMessage(support: string, collection: string, filtered: boolean): string {
  if (filtered) return "No returned records match this filter.";
  if (support !== "SUPPORTED") return `No records: collector support is ${support}.`;
  if (collection !== "OK") return `No records: collection state is ${collection}.`;
  return "Supported collection completed with zero records.";
}
