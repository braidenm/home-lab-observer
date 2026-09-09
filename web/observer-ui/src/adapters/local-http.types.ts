export type ObserverTransportRequest = Record<string, string | number | boolean | undefined>;

export interface LocalHttpObserverDataSourceOptions {
  baseUrl?: string;
  bearerToken?: string;
  fetcher?: typeof fetch;
}
