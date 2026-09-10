import { useCallback, useEffect, useState } from "react";
import type { ContainerInventory, CurrentSnapshot, DiagnosticsHealth, LogSummary, ObserverCapabilities, ObserverDataSource, TrendRange, TrendSnapshot } from "../types";

export interface ResourceState<T> {
  value: T | null;
  status: "loading" | "ready" | "error" | "unsupported";
  error: string | null;
}

const loading = <T,>(): ResourceState<T> => ({ value: null, status: "loading", error: null });

export function useObserverData(dataSource: ObserverDataSource, range: TrendRange, logRange: TrendRange) {
  const [refreshKey, setRefreshKey] = useState(0);
  const [capabilities, setCapabilities] = useState<ResourceState<ObserverCapabilities>>(loading);
  const [snapshot, setSnapshot] = useState<ResourceState<CurrentSnapshot>>(loading);
  const [trends, setTrends] = useState<ResourceState<TrendSnapshot>>(
    dataSource.getTrends ? loading : { value: null, status: "unsupported", error: null }
  );
  const [containerInventory, setContainerInventory] = useState<ResourceState<ContainerInventory>>(
    dataSource.getContainerInventory ? loading : { value: null, status: "unsupported", error: null }
  );
  const [diagnosticsHealth, setDiagnosticsHealth] = useState<ResourceState<DiagnosticsHealth>>(
    dataSource.getDiagnosticsHealth ? loading : { value: null, status: "unsupported", error: null }
  );
  const [logSummary, setLogSummary] = useState<ResourceState<LogSummary>>(
    dataSource.getLogSummary ? loading : { value: null, status: "unsupported", error: null }
  );
  const [refreshedAt, setRefreshedAt] = useState<Date | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setCapabilities((current) => ({ ...current, status: "loading", error: null }));
    setSnapshot((current) => ({ ...current, status: "loading", error: null }));
    setTrends((current) => dataSource.getTrends ? { ...current, status: "loading", error: null } : { value: null, status: "unsupported", error: null });
    setContainerInventory((current) => dataSource.getContainerInventory ? { ...current, status: "loading", error: null } : { value: null, status: "unsupported", error: null });
    setDiagnosticsHealth((current) => dataSource.getDiagnosticsHealth ? { ...current, status: "loading", error: null } : { value: null, status: "unsupported", error: null });

    const settle = <T,>(promise: Promise<T>, update: (state: ResourceState<T>) => void) => {
      void promise.then(
        (value) => { if (!controller.signal.aborted) { update({ value, status: "ready", error: null }); setRefreshedAt(new Date()); } },
        (error: unknown) => { if (!controller.signal.aborted) update({ value: null, status: "error", error: safeMessage(error) }); }
      );
    };
    settle(dataSource.getCapabilities(controller.signal), setCapabilities);
    settle(dataSource.getCurrentSnapshot(controller.signal), setSnapshot);
    if (dataSource.getTrends) settle(dataSource.getTrends(range, controller.signal), setTrends);
    if (dataSource.getContainerInventory) settle(dataSource.getContainerInventory(controller.signal), setContainerInventory);
    if (dataSource.getDiagnosticsHealth) settle(dataSource.getDiagnosticsHealth(controller.signal), setDiagnosticsHealth);
    return () => controller.abort();
  }, [dataSource, range, refreshKey]);

  useEffect(() => {
    const controller = new AbortController();
    if (!dataSource.getLogSummary) {
      setLogSummary({ value: null, status: "unsupported", error: null });
      return () => controller.abort();
    }
    setLogSummary((current) => ({ ...current, status: "loading", error: null }));
    void dataSource.getLogSummary(logRange, controller.signal).then(
      (value) => {
        if (!controller.signal.aborted) {
          setLogSummary({ value, status: "ready", error: null });
          setRefreshedAt(new Date());
        }
      },
      (error: unknown) => {
        if (!controller.signal.aborted) setLogSummary({ value: null, status: "error", error: safeMessage(error) });
      }
    );
    return () => controller.abort();
  }, [dataSource, logRange, refreshKey]);

  const refresh = useCallback(() => setRefreshKey((current) => current + 1), []);
  const isLoading = capabilities.status === "loading" || snapshot.status === "loading" || trends.status === "loading" || containerInventory.status === "loading" || diagnosticsHealth.status === "loading" || logSummary.status === "loading";
  return { capabilities, snapshot, trends, containerInventory, diagnosticsHealth, logSummary, refreshedAt, isLoading, refresh };
}

function safeMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Observer data could not be loaded";
}
