import { useCallback, useEffect, useState } from "react";
import type { DashboardData, ObserverDataSource, TrendRange } from "../types";

interface ObserverDataState {
  data: DashboardData | null;
  error: string | null;
  loading: boolean;
  refreshedAt: Date | null;
}

export function useObserverData(dataSource: ObserverDataSource, range: TrendRange) {
  const [refreshKey, setRefreshKey] = useState(0);
  const [state, setState] = useState<ObserverDataState>({
    data: null,
    error: null,
    loading: true,
    refreshedAt: null
  });

  useEffect(() => {
    const controller = new AbortController();
    setState((current) => ({ ...current, loading: true, error: null }));
    Promise.all([
      dataSource.getOverview(controller.signal),
      dataSource.getTrends(range, controller.signal),
      dataSource.getWorkloads({}, controller.signal),
      dataSource.getLogs({}, controller.signal),
      dataSource.getObserverHealth(controller.signal)
    ])
      .then(([overview, trends, workloads, logs, health]) => {
        setState({ data: { overview, trends, workloads, logs, health }, error: null, loading: false, refreshedAt: new Date() });
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setState((current) => ({
            ...current,
            error: error instanceof Error ? error.message : "Observer data could not be loaded",
            loading: false
          }));
        }
      });
    return () => controller.abort();
  }, [dataSource, range, refreshKey]);

  const refresh = useCallback(() => setRefreshKey((current) => current + 1), []);
  return { ...state, refresh };
}
