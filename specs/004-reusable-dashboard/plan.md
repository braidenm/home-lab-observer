# Implementation plan: Reusable local dashboard

## Decision

Publish a small React package whose components consume a transport-neutral view model. Keep the local HTTP adapter and
deterministic demo source beside that interface so embedded, standalone, and external consumers share the UI without
sharing authentication or collector implementations.

## Package shape

```text
web/observer-ui/
  src/types.ts                 -> normalized view model and ObserverDataSource
  src/adapters/local-http.ts   -> loopback-only read transport
  src/data/synthetic.ts        -> deterministic offline demo data
  src/views/                   -> overview, trends, workloads, logs, health/privacy
  demo/                        -> standalone browser entry
```

React is a peer dependency. Vite emits one ES module, declarations, and a local stylesheet; a separate demo build proves
the package can run without the observer binary. Native HTML controls and focused React state keep the dependency and
accessibility surface small.

## Privacy design

- Process observations contain code-owned names and resource facts, but no account, owner, username, user identifier,
  arguments, or environment.
- Log summaries are code-owned sanitized labels. They are not excerpts, previews, or copies of raw source messages.
- Message bodies remain separately modeled, visibly disabled by default, and absent from synthetic fixtures.
- Absolute adapter origins are limited to loopback and never receive cookies or URL credentials.

## Validation and rollback

`npm run check` performs strict typechecking, Vitest coverage, library compilation, declaration emission, and standalone
demo compilation. CI runs that gate with a ten-minute timeout, preserving the repository-wide target below 15 minutes.
The slice owns no persisted data or runtime migration; rollback is a package/spec commit revert.

## Deferred integration

The Go runtime, canonical API-to-view mapping, embedded asset serving, browser-level packaged accessibility checks, and
Platform Demo adapter remain later specifications. Those integrations must preserve this slice's privacy and
transport boundaries without revising the accepted Spec 002 contract by implication.
