# Home Lab Observer UI

`@braidenm/home-lab-observer-ui` is the reusable browser dashboard for Home Lab Observer. Its root export contains the
transport-neutral component and normalized view contracts only. Consumers provide an `ObserverDataSource`; the local
HTTP adapter and synthetic demo source are explicit subpath imports.

The observer binary can use `LocalHttpObserverDataSource` against its loopback API. It reads only the accepted
capabilities, current-snapshot, and bounded metric-series `GET` contracts. Platform Demo provides its own authenticated
adapter rather than trying to call or iframe another machine's loopback dashboard.

## Development

```bash
npm ci
npm run dev
npm run check
```

The demo uses deterministic synthetic data and makes no network requests. The production package emits an ES module,
TypeScript declarations, and `observer-ui.css`.

## Data boundary

The view model excludes process accounts, owners, user identifiers, arguments, and environment variables. Its log
record is an exact closed metadata/body union: metadata contains only observation time, source, severity, and event
code; a body is either omitted or the bounded `REDACTED_LOCAL_ONLY` contract object. There is no summary, arbitrary
structured-field map, or raw-body property. Display labels are derived only from `event_code`, never message text.

## Use

```tsx
import { ObserverDashboard } from "@braidenm/home-lab-observer-ui";
import { LocalHttpObserverDataSource } from "@braidenm/home-lab-observer-ui/local";
import "@braidenm/home-lab-observer-ui/styles.css";

const dataSource = new LocalHttpObserverDataSource({
  baseUrl: "http://127.0.0.1:9847",
  bearerToken: readTokenFromInstallerOwnedStorage()
});

export function App() {
  return <ObserverDashboard dataSource={dataSource} />;
}
```

The default origin is `http://127.0.0.1:9847`. Absolute URLs must use `localhost`, `127.0.0.1`, or `::1`; an explicit
empty base URL selects same-origin embedding. The adapter sends no cookies, follows no redirects, keeps an optional
bearer token only in memory, enforces the contract response ceilings, and parses Problem Details without exposing raw
responses. Trends request the six code-owned metric identifiers at `/api/v1/metrics/series`; the adapter preserves null
gaps, actual sample resolution, support state, and truncation metadata. The optional data-source method lets remote
consumers omit local history rather than inventing it from snapshots.
