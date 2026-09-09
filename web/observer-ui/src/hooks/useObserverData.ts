import { useCallback, useEffect, useState } from "react";
import type { CurrentSnapshot, ObserverCapabilities, ObserverDataSource, TrendRange, TrendSnapshot } from "../types";

export interface ResourceState<T> {
  value: T | null;
  status: "loading" | "ready" | "error" | "unsupported";
  error: string | null;
}

const loading = <T,>(): ResourceState<T> => ({ value: null, status: "loading", error: null });

export function useObserverData(dataSource: ObserverDataSource, range: TrendRange) {
  const [refreshKey, setRefreshKey] = useState(0);
  const [capabilities, setCapabilities] = useState<ResourceState<ObserverCapabilities>>(loading);
  const [snapshot, setSnapshot] = useState<ResourceState<CurrentSnapshot>>(loading);
  const [trends, setTrends] = useState<ResourceState<TrendSnapshot>>(
    dataSource.getTrends ? loading : { value: null, status: "unsupported", error: null }
  );
  const [refreshedAt, setRefreshedAt] = useState<Date | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setCapabilities((current) => ({ ...current, status: "loading", error: null }));
    setSnapshot((current) => ({ ...current, status: "loading", error: null }));
    setTrends((current) => dataSource.getTrends ? { ...current, status: "loading", error: null } : { value: null, status: "unsupported", error: null });

    const settle = <T,>(promise: Promise<T>, update: (state: ResourceState<T>) => void) => {
      void promise.then(
        (value) => { if (!controller.signal.aborted) { update({ value, status: "ready", error: null }); setRefreshedAt(new Date()); } },
        (error: unknown) => { if (!controller.signal.aborted) update({ value: null, status: "error", error: safeMessage(error) }); }
      );
    };
    settle(dataSource.getCapabilities(controller.signal), setCapabilities);
    settle(dataSource.getCurrentSnapshot(controller.signal), setSnapshot);
    if (dataSource.getTrends) settle(dataSource.getTrends(range, controller.signal), setTrends);
    return () => controller.abort();
  }, [dataSource, range, refreshKey]);

  const refresh = useCallback(() => setRefreshKey((current) => current + 1), []);
  const isLoading = capabilities.status === "loading" || snapshot.status === "loading" || trends.status === "loading";
  return { capabilities, snapshot, trends, refreshedAt, isLoading, refresh };
}

function safeMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Observer data could not be loaded";
}
