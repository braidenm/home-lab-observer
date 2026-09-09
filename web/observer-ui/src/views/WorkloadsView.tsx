import { useMemo, useState } from "react";
import type { Workload, WorkloadKind, WorkloadSnapshot } from "../types";
import { formatBytes, formatRelative, titleCase } from "../components/format";
import { StateBadge } from "../components/StateBadge";

const kinds: { value: WorkloadKind; label: string }[] = [
  { value: "process", label: "Processes" },
  { value: "service", label: "Services" },
  { value: "container", label: "Containers" }
];

export function WorkloadsView({ workloads }: { workloads: WorkloadSnapshot }) {
  const [kind, setKind] = useState<WorkloadKind>("process");
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<"cpu" | "memory" | "name">("cpu");
  const items = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return workloads.items
      .filter((item) => item.kind === kind && (!normalized || `${item.name} ${item.state}`.toLowerCase().includes(normalized)))
      .sort((left, right) => {
        if (sort === "name") return left.name.localeCompare(right.name);
        if (sort === "memory") return (right.memoryBytes ?? -1) - (left.memoryBytes ?? -1);
        return (right.cpuPercent ?? -1) - (left.cpuPercent ?? -1);
      });
  }, [kind, query, sort, workloads.items]);

  return (
    <div className="observer-view" aria-labelledby="workloads-title">
      <div className="observer-view-heading">
        <div>
          <p className="observer-kicker">Workload inventory</p>
          <h2 id="workloads-title">Processes, services, and containers</h2>
          <p>Resource facts stay useful without collecting process arguments, environment variables, or mount details.</p>
        </div>
      </div>
      <section className="observer-panel observer-panel--flush">
        <div className="observer-toolbar">
          <div className="observer-segmented" role="tablist" aria-label="Workload type">
            {kinds.map((item) => (
              <button
                key={item.value}
                type="button"
                role="tab"
                aria-selected={kind === item.value}
                className={kind === item.value ? "is-active" : ""}
                onClick={() => setKind(item.value)}
              >
                {item.label}
              </button>
            ))}
          </div>
          <div className="observer-filter-row">
            <label>
              <span>Filter</span>
              <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={`Find ${kind}…`} />
            </label>
            <label>
              <span>Sort</span>
              <select value={sort} onChange={(event) => setSort(event.target.value as typeof sort)}>
                <option value="cpu">CPU</option>
                <option value="memory">Memory</option>
                <option value="name">Name</option>
              </select>
            </label>
          </div>
        </div>
        <div className="observer-table-scroll" role="region" aria-label={`${titleCase(kind)} inventory`} tabIndex={0}>
          <table className="observer-table">
            <thead>
              <tr>
                <th scope="col">Name</th>
                <th scope="col">State</th>
                <th scope="col">CPU</th>
                <th scope="col">Memory</th>
                <th scope="col">Started</th>
                <th scope="col">Details</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => <WorkloadRow key={item.id} item={item} />)}
            </tbody>
          </table>
        </div>
        {items.length === 0 && <p className="observer-empty">No matching {kind} observations.</p>}
        <p className="observer-table-note">
          Showing {items.length} of a bounded {workloads.limits[limitKey(kind)]} {kind} records.
        </p>
      </section>
    </div>
  );
}

function WorkloadRow({ item }: { item: Workload }) {
  const detail = item.kind === "process"
    ? `PID ${item.processId}`
    : item.kind === "service"
      ? `${item.manager} · ${item.startup}${item.restartCount === null ? "" : ` · ${item.restartCount} restarts`}`
      : `${item.health ?? "health unavailable"} · ${item.restartCount} restarts`;
  return (
    <tr>
      <th scope="row">
        <strong>{item.name}</strong>
        {item.kind === "container" && <small>{item.image}</small>}
      </th>
      <td><StateBadge state={normalizeWorkloadState(item.state)} label={item.state} /></td>
      <td>{item.cpuPercent === null ? "Unavailable" : `${item.cpuPercent.toFixed(1)}%`}</td>
      <td>{formatBytes(item.memoryBytes)}</td>
      <td>{item.startedAt ? formatRelative(item.startedAt) : "—"}</td>
      <td>{detail}</td>
    </tr>
  );
}

function normalizeWorkloadState(state: string): "running" | "active" | "exited" | "inactive" | "denied" {
  return (["running", "active", "exited", "inactive", "denied"].includes(state) ? state : "inactive") as
    "running" | "active" | "exited" | "inactive" | "denied";
}

function limitKey(kind: WorkloadKind): keyof WorkloadSnapshot["limits"] {
  return kind === "process" ? "processes" : kind === "service" ? "services" : "containers";
}
