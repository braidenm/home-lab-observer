import { useId, useMemo, useState } from "react";
import type { ContainerInventory, ContainerInventoryItem, ContainerObservation, CurrentSnapshot } from "../types";
import type { ResourceState } from "../hooks/useObserverData";
import { formatBytes, titleCase } from "../components/format";
import { StateBadge } from "../components/StateBadge";
import { SectionStatus } from "../components/SectionStatus";

type WorkloadKind = "processes" | "services" | "containers";
const kinds: Array<{ value: WorkloadKind; label: string }> = [
  { value: "processes", label: "Processes" }, { value: "services", label: "Services" }, { value: "containers", label: "Containers" }
];
const singularKind: Record<WorkloadKind, string> = { processes: "process", services: "service", containers: "container" };

interface WorkloadsViewProps {
  snapshot: ResourceState<CurrentSnapshot>;
  containerInventory: ResourceState<ContainerInventory>;
}

export function WorkloadsView({ snapshot, containerInventory }: WorkloadsViewProps) {
  const id = useId();
  const [kind, setKind] = useState<WorkloadKind>("processes");
  const [query, setQuery] = useState("");
  const dedicatedContainers = containerInventory.status !== "unsupported";
  const section = kind === "containers"
    ? dedicatedContainers ? containerInventory.value : snapshot.value?.sections.containers ?? null
    : snapshot.value?.sections[kind] ?? null;
  const resource = kind === "containers" && dedicatedContainers ? containerInventory : snapshot;
  const normalized = query.trim().toLowerCase();
  const items = useMemo(() => section?.items.filter((item) => {
    if (!normalized) return true;
    const containerMetadata = "image" in item ? ` ${item.image} ${item.idAlias}` : "";
    return `${item.name} ${item.state}${containerMetadata}`.toLowerCase().includes(normalized);
  }) ?? [], [normalized, section]);

  const selectRelative = (index: number) => {
    const next = kinds[(index + kinds.length) % kinds.length];
    setKind(next.value);
    requestAnimationFrame(() => document.getElementById(`${id}-${next.value}`)?.focus());
  };

  return (
    <div className="observer-view" aria-labelledby="workloads-title">
      <div className="observer-view-heading"><div><p className="observer-kicker">Bounded observations</p><h2 id="workloads-title">Workloads</h2><p>Only fields in the accepted process, service, and container contracts are shown.</p></div></div>
      <section className="observer-panel observer-panel--flush">
        <div className="observer-toolbar">
          <div className="observer-segmented" role="tablist" aria-label="Workload type">
            {kinds.map((item, index) => (
              <button key={item.value} id={`${id}-${item.value}`} type="button" role="tab" aria-selected={kind === item.value} aria-controls={`${id}-panel`} tabIndex={kind === item.value ? 0 : -1} className={kind === item.value ? "is-active" : ""} onClick={() => setKind(item.value)} onKeyDown={(event) => {
                if (event.key === "ArrowRight") { event.preventDefault(); selectRelative(index + 1); }
                if (event.key === "ArrowLeft") { event.preventDefault(); selectRelative(index - 1); }
                if (event.key === "Home") { event.preventDefault(); selectRelative(0); }
                if (event.key === "End") { event.preventDefault(); selectRelative(kinds.length - 1); }
              }}>{item.label}</button>
            ))}
          </div>
          <label><span>Filter returned records</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={`Find ${singularKind[kind]}…`} disabled={!section} /></label>
        </div>
        <div id={`${id}-panel`} role="tabpanel" aria-labelledby={`${id}-${kind}`}>
          {!section && <WorkloadResourceState kind={kind} resource={resource} />}
          {section && <>
            <SectionStatus section={section} compact />
            {kind === "containers" && dedicatedContainers && <ContainerInventoryNotice inventory={section as ContainerInventory} />}
            {kind === "containers" && section.supportState === "DISABLED" && dedicatedContainers && <ContainerDisabled />}
            {items.length > 0 && <><p className="observer-table-scroll-hint">Scroll horizontally to view every column.</p><div className="observer-table-scroll" role="region" aria-label={`${kind} table; horizontally scrollable on small screens`} tabIndex={0}>
              {kind === "processes" && <ProcessTable items={items as CurrentSnapshot["sections"]["processes"]["items"]} />}
              {kind === "services" && <ServiceTable items={items as CurrentSnapshot["sections"]["services"]["items"]} />}
              {kind === "containers" && <ContainerTable items={items as Array<ContainerObservation | ContainerInventoryItem>} dedicated={dedicatedContainers} />}
            </div></>}
            {items.length === 0 && section.supportState !== "DISABLED" && <p className="observer-empty">{emptyMessage(section.supportState, section.collectionState, normalized.length > 0)}</p>}
          </>}
        </div>
      </section>
    </div>
  );
}

function WorkloadResourceState({ kind, resource }: { kind: WorkloadKind; resource: ResourceState<unknown> }) {
  if (resource.status === "loading") return <p className="observer-empty" role="status">Loading {kind === "containers" ? "container inventory" : "current snapshot"}…</p>;
  if (resource.status === "error") return <div className="observer-inline-error" role="alert"><strong>{kind === "containers" ? "Container inventory unavailable" : "Current snapshot unavailable"}</strong><p>{resource.error}</p></div>;
  return <p className="observer-empty">No {kind} data source is available.</p>;
}

function ContainerInventoryNotice({ inventory }: { inventory: ContainerInventory }) {
  return <p className="observer-table-note">Dedicated container inventory · read-only · local-sensitive · remote upload not eligible. CPU is percent of engine-host capacity; memory is engine-reported usage. Stopped containers have unavailable metrics.{inventory.truncated ? ` Showing ${inventory.returnedCount} of ${inventory.totalCount}.` : ""}</p>;
}

function ContainerDisabled() {
  return (
    <div className="observer-container-guidance">
      <h3>Container observations are disabled</h3>
      <p>Restart the local observer with one explicit Docker endpoint. Use <code>observer serve --docker-endpoint unix:///var/run/docker.sock</code> for a local Unix socket, or <code>observer serve --docker-endpoint npipe:////./pipe/docker_engine</code> for the local Windows named pipe.</p>
      <p>The observer does not discover an endpoint from the environment and does not install, enable, or reconfigure Docker.</p>
    </div>
  );
}

function ProcessTable({ items }: { items: CurrentSnapshot["sections"]["processes"]["items"] }) {
  return <table className="observer-table"><caption className="observer-visually-hidden">Processes: name, state, CPU, memory, and process ID</caption><thead><tr><th scope="col">Name</th><th scope="col">State</th><th scope="col">CPU</th><th scope="col">Memory</th><th scope="col">Process ID</th></tr></thead><tbody>{items.map((item) => <tr key={item.pid}><th scope="row">{item.name}</th><td><StateBadge state={item.state} /></td><td>{item.cpuPercent.toFixed(1)}%</td><td>{formatBytes(item.memoryBytes)}</td><td>{item.pid}</td></tr>)}</tbody></table>;
}

function ServiceTable({ items }: { items: CurrentSnapshot["sections"]["services"]["items"] }) {
  return <table className="observer-table"><caption className="observer-visually-hidden">Services: name, state, and configured start mode</caption><thead><tr><th scope="col">Name</th><th scope="col">State</th><th scope="col">Start mode</th></tr></thead><tbody>{items.map((item) => <tr key={item.name}><th scope="row">{item.name}</th><td><StateBadge state={item.state} /></td><td>{item.startMode}</td></tr>)}</tbody></table>;
}

function ContainerTable({ items, dedicated }: { items: Array<ContainerObservation | ContainerInventoryItem>; dedicated: boolean }) {
  return <table className="observer-table observer-container-table"><caption className="observer-visually-hidden">Containers: name and image, state, host-capacity CPU, engine-reported memory, metrics quality, and alias</caption><thead><tr><th scope="col">Name / image</th><th scope="col">State</th><th scope="col">CPU (host)</th><th scope="col">Memory</th>{dedicated && <th scope="col">Metrics</th>}<th scope="col">Alias</th></tr></thead><tbody>{items.map((item) => <tr key={item.idAlias}><th scope="row"><strong>{item.name}</strong><small title={item.image}>{item.image}</small></th><td><StateBadge state={item.state} /></td><td>{item.cpuPercent === null ? "Unavailable" : `${item.cpuPercent.toFixed(1)}%`}</td><td>{formatBytes(item.memoryBytes)}</td>{dedicated && "metricsState" in item && <td><StateBadge state={item.metricsState} />{item.reasonCode && <small>{titleCase(item.reasonCode)}</small>}</td>}<td>{item.idAlias}</td></tr>)}</tbody></table>;
}

function emptyMessage(support: string, collection: string, filtered: boolean): string {
  if (filtered) return "No returned records match this filter.";
  if (support !== "SUPPORTED") return `No records: collector support is ${support}.`;
  if (collection !== "OK") return `No records: collection state is ${collection}.`;
  return "Supported collection completed with zero records.";
}
