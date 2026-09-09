import { useId, useState } from "react";
import type { ObserverDataSource, TrendRange } from "./types";
import { useObserverData, type ResourceState } from "./hooks/useObserverData";
import { OverviewView } from "./views/OverviewView";
import { TrendsView } from "./views/TrendsView";
import { WorkloadsView } from "./views/WorkloadsView";
import { LogsView } from "./views/LogsView";
import { HealthPrivacyView } from "./views/HealthPrivacyView";

export type ViewId = "overview" | "trends" | "workloads" | "logs" | "health";
const views: Array<{ id: ViewId; label: string; shortLabel: string }> = [
  { id: "overview", label: "Overview", shortLabel: "Overview" }, { id: "trends", label: "Trends", shortLabel: "Trends" },
  { id: "workloads", label: "Workloads", shortLabel: "Workloads" }, { id: "logs", label: "Logs", shortLabel: "Logs" },
  { id: "health", label: "Observer & privacy", shortLabel: "Observer" }
];

export interface ObserverDashboardProps {
  dataSource: ObserverDataSource;
  initialView?: ViewId;
  mode?: "local" | "demo" | "embedded";
  onViewChange?: (view: ViewId) => void;
}

export function ObserverDashboard({ dataSource, initialView = "overview", mode = "local", onViewChange }: ObserverDashboardProps) {
  const contentId = useId();
  const [activeView, setActiveView] = useState<ViewId>(initialView);
  const [range, setRange] = useState<TrendRange>("6h");
  const { capabilities, snapshot, trends, refreshedAt, isLoading, refresh } = useObserverData(dataSource, range);
  const selectView = (view: ViewId) => { setActiveView(view); onViewChange?.(view); };
  const connectionLabel = mode === "demo" ? "Synthetic demo" : mode === "embedded" ? "Embedded data source" : "Local API";
  const boundary = mode === "demo" ? "Synthetic data source; no observer connection" : mode === "embedded" ? "Transport and egress are controlled by the embedding application" : capabilities.value ? `${capabilities.value.policy.readOnly ? "Read-only" : "Policy-reported writable"} · default bind ${capabilities.value.policy.defaultBind}` : "Local API policy unavailable";

  return (
    <div className="observer-shell">
      <a className="observer-skip-link" href={`#${contentId}`}>Skip to dashboard content</a>
      <header className="observer-header"><div className="observer-brand"><div className="observer-brand__mark" aria-hidden="true"><span /><span /><span /></div><div><strong>Home Lab Observer</strong><span>Machine observations</span></div></div><div className="observer-header__status"><span className={`observer-connection observer-connection--${mode}`}>{connectionLabel}</span><button type="button" className="observer-refresh" onClick={refresh} disabled={isLoading}>{isLoading ? "Refreshing…" : "Refresh"}</button></div></header>
      <nav className="observer-navigation" aria-label="Observer dashboard">{views.map((view) => <button type="button" key={view.id} className={activeView === view.id ? "is-active" : ""} aria-label={view.label} aria-current={activeView === view.id ? "page" : undefined} onClick={() => selectView(view.id)}><span className="observer-navigation__label">{view.label}</span><span className="observer-navigation__short" aria-hidden="true">{view.shortLabel}</span></button>)}</nav>
      <main id={contentId} tabIndex={-1}>
        <ResourceError label="Capabilities" resource={capabilities} onRetry={refresh} />
        <ResourceError label="Current snapshot" resource={snapshot} onRetry={refresh} />
        {activeView === "overview" && <SnapshotGate resource={snapshot}>{snapshot.value && <OverviewView snapshot={snapshot.value} />}</SnapshotGate>}
        {activeView === "trends" && <TrendsView trends={trends.value} status={trends.status} error={trends.error} range={range} onRangeChange={setRange} />}
        {activeView === "workloads" && <SnapshotGate resource={snapshot}>{snapshot.value && <WorkloadsView snapshot={snapshot.value} />}</SnapshotGate>}
        {activeView === "logs" && <SnapshotGate resource={snapshot}>{snapshot.value && <LogsView snapshot={snapshot.value} capabilities={capabilities.value} />}</SnapshotGate>}
        {activeView === "health" && <HealthPrivacyView capabilities={capabilities.value} snapshot={snapshot.value} mode={mode} />}
      </main>
      <footer className="observer-footer"><span>{boundary}</span><span aria-live="polite">{snapshot.value ? `Snapshot ${snapshot.value.collectionState}` : refreshedAt ? `Last response ${refreshedAt.toLocaleTimeString()}` : "Awaiting data"}</span></footer>
    </div>
  );
}

function ResourceError<T>({ label, resource, onRetry }: { label: string; resource: ResourceState<T>; onRetry: () => void }) {
  if (resource.status !== "error") return null;
  return <section className="observer-error" role="alert"><strong>{label} unavailable</strong><p>{resource.error}</p><button type="button" onClick={onRetry}>Try again</button></section>;
}

function SnapshotGate({ resource, children }: { resource: ResourceState<unknown>; children: React.ReactNode }) {
  if (resource.value) return children;
  if (resource.status === "loading") return <div className="observer-skeleton" role="status" aria-label="Loading current snapshot"><div /><div /><div /><div /><span>Loading current snapshot…</span></div>;
  return <section className="observer-panel observer-panel--padded"><strong>Current snapshot unavailable</strong><p>Other independently loaded dashboard information remains available.</p></section>;
}
