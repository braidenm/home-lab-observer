# Home Lab Observer UI

`@braidenm/home-lab-observer-ui` is the reusable browser dashboard for Home Lab Observer. It contains no transport,
authentication, analytics, or hosted-service assumptions. Consumers provide an `ObserverDataSource` that returns the
normalized view contracts.

The observer binary can use `LocalHttpObserverDataSource` against its same-origin loopback API. Platform Demo should
provide its own authenticated adapter rather than trying to call or iframe another machine's loopback dashboard.

## Development

```bash
npm ci
npm run dev
npm run check
```

The demo uses deterministic synthetic data and makes no network requests. The production package emits an ES module,
TypeScript declarations, and `observer-ui.css`.

## Data boundary

The view model excludes process accounts, owners, user identifiers, arguments, and environment variables. A log event
`summary` is a short, code-owned, sanitized metadata label; adapters must never populate it from a raw log or message
body. Bodies remain a separate opt-in field, and the included local adapter expects them to be omitted by default.

## Use

```tsx
import { ObserverDashboard, LocalHttpObserverDataSource } from "@braidenm/home-lab-observer-ui";
import "@braidenm/home-lab-observer-ui/styles.css";

const dataSource = new LocalHttpObserverDataSource({ baseUrl: "http://127.0.0.1:9780" });

export function App() {
  return <ObserverDashboard dataSource={dataSource} />;
}
```

Absolute local HTTP URLs must use `localhost`, `127.0.0.1`, or `::1`. A relative base URL is accepted for the embedded
same-origin dashboard. The adapter sends no cookies, follows no redirects, and keeps an optional bearer token only in
memory. It transports an already-sanitized view model and does not turn source messages into display summaries.
