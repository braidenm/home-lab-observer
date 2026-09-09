# Implementation plan: Reusable local dashboard

## Decision

Publish a small React package whose components consume a transport-neutral projection of the accepted Spec 002
contracts. Keep the exact two-endpoint local adapter and deterministic demo source behind explicit subpath exports so
embedded, standalone, and external consumers share the UI without sharing authentication or collectors.

## Package shape

```text
web/observer-ui/
  src/types.ts                 -> normalized view model and ObserverDataSource
  src/adapters/local-http.ts   -> loopback-only read transport
  src/data/synthetic.ts        -> deterministic offline demo data
  src/views/                   -> overview, trends, workloads, logs, health/privacy
  demo/                        -> standalone browser entry
```

React is a peer dependency. Vite emits split core/local/demo ES modules, declarations, and a component-scoped local stylesheet; a separate demo build proves
the package can run without the observer binary. Native HTML controls and focused React state keep the dependency and
accessibility surface small.

## Privacy design

- Process observations contain code-owned names and resource facts, but no account, owner, username, user identifier,
  arguments, or environment.
- Log metadata contains only the accepted observation time, source, severity, and event code. Human-readable labels are
  formatted from event codes, never body text.
- Message bodies use the exact omitted/redacted-local-only union and are absent from the safe-default demo snapshot.
- Absolute adapter origins are limited to loopback and never receive cookies or URL credentials.

## Validation and rollback

`npm run check` performs strict typechecking, Vitest coverage, library compilation, declaration emission, and standalone
demo compilation. CI runs that gate with a ten-minute timeout, preserving the repository-wide target below 15 minutes.
The slice owns no persisted data or runtime migration; rollback is a package/spec commit revert.

## Deferred integration

The Go runtime, embedded asset serving, browser-level packaged accessibility checks, and
Platform Demo adapter remain later specifications. Those integrations must preserve this slice's privacy and
transport boundaries without revising the accepted Spec 002 contract by implication.
