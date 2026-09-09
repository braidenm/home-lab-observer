import { useState } from "react";
import type { ObserverDataSource, TrendRange } from "./types";
import { useObserverData } from "./hooks/useObserverData";
import { OverviewView } from "./views/OverviewView";
import { TrendsView } from "./views/TrendsView";
import { WorkloadsView } from "./views/WorkloadsView";
import { LogsView } from "./views/LogsView";
import { HealthPrivacyView } from "./views/HealthPrivacyView";

type ViewId = "overview" | "trends" | "workloads" | "logs" | "health";

const views: { id: ViewId; label: string; shortLabel: string }[] = [
  { id: "overview", label: "Overview", shortLabel: "Overview" },
  { id: "trends", label: "Trends", shortLabel: "Trends" },
  { id: "workloads", label: "Workloads", shortLabel: "Workloads" },
  { id: "logs", label: "Logs", shortLabel: "Logs" },
  { id: "health", label: "Observer & privacy", shortLabel: "Observer" }
];

export interface ObserverDashboardProps {
  dataSource: ObserverDataSource;
  initialView?: ViewId;
  mode?: "local" | "demo" | "embedded";
  onViewChange?: (view: ViewId) => void;
}

export function ObserverDashboard({
  dataSource,
  initialView = "overview",
  mode = "local",
  onViewChange
}: ObserverDashboardProps) {
  const [activeView, setActiveView] = useState<ViewId>(initialView);
  const [range, setRange] = useState<TrendRange>("6h");
  const { data, error, loading, refreshedAt, refresh } = useObserverData(dataSource, range);

  const selectView = (view: ViewId) => {
    setActiveView(view);
    onViewChange?.(view);
  };

  return (
    <div className="observer-shell">
      <a className="observer-skip-link" href="#observer-content">Skip to dashboard content</a>
      <header className="observer-header">
        <div className="observer-brand">
          <div className="observer-brand__mark" aria-hidden="true"><span /><span /><span /></div>
          <div>
            <strong>Home Lab Observer</strong>
            <span>Local machine intelligence</span>
          </div>
        </div>
        <div className="observer-header__status">
          <span className={`observer-connection observer-connection--${mode}`}>{mode === "demo" ? "Synthetic demo" : mode === "local" ? "Local connection" : "Embedded view"}</span>
          <button type="button" className="observer-refresh" onClick={refresh} disabled={loading}>
            {loading ? "Refreshing…" : "Refresh"}
          </button>
        </div>
      </header>
      <nav className="observer-navigation" aria-label="Observer dashboard">
        {views.map((view) => (
          <button
            type="button"
            key={view.id}
            className={activeView === view.id ? "is-active" : ""}
            aria-label={view.label}
            aria-current={activeView === view.id ? "page" : undefined}
            onClick={() => selectView(view.id)}
          >
            <span className="observer-navigation__label">{view.label}</span>
            <span className="observer-navigation__short">{view.shortLabel}</span>
          </button>
        ))}
      </nav>
      <main id="observer-content" tabIndex={-1}>
        {error && (
          <section className="observer-error" role="alert">
            <strong>Observer data is unavailable</strong>
            <p>{error}</p>
            <button type="button" onClick={refresh}>Try again</button>
          </section>
        )}
        {!data && loading && <DashboardSkeleton />}
        {data && (
          <>
            {activeView === "overview" && <OverviewView overview={data.overview} />}
            {activeView === "trends" && <TrendsView trends={data.trends} range={range} onRangeChange={setRange} />}
            {activeView === "workloads" && <WorkloadsView workloads={data.workloads} />}
            {activeView === "logs" && <LogsView logs={data.logs} />}
            {activeView === "health" && <HealthPrivacyView health={data.health} />}
          </>
        )}
      </main>
      <footer className="observer-footer">
        <span>Read-only · loopback by default · bounded history</span>
        <span aria-live="polite">{refreshedAt ? `Refreshed ${refreshedAt.toLocaleTimeString()}` : "Loading observer state"}</span>
      </footer>
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <div className="observer-skeleton" role="status" aria-label="Loading dashboard">
      <div /><div /><div /><div />
      <span>Loading observer state…</span>
    </div>
  );
}
