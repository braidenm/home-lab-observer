import "./styles.css";

export { ObserverDashboard } from "./ObserverDashboard";
export type { ObserverDashboardProps } from "./ObserverDashboard";
export { LocalHttpObserverDataSource, ObserverTransportError } from "./adapters/local-http";
export type { LocalHttpObserverDataSourceOptions } from "./adapters/local-http.types";
export {
  SyntheticObserverDataSource,
  syntheticHealth,
  syntheticLogs,
  syntheticOverview,
  syntheticTrends,
  syntheticWorkloads
} from "./data/synthetic";
export type * from "./types";
