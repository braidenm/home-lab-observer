# Spec 004: Reusable local dashboard

**Status:** Accepted  
**Owner:** Repository owner  
**Created:** 2026-09-09  
**Accepted:** 2026-09-09

## User outcome

An owner can inspect the accepted current machine snapshot through a responsive dashboard, while a browser demo or
another application can reuse the same UI against a different transport. Optional trend providers can add history;
the local v1 adapter never invents a series endpoint or infers history from one snapshot.

## Slice boundary

This specification delivers the reusable TypeScript and React dashboard package under `web/observer-ui`:

- a transport-neutral `ObserverDataSource` and normalized view model;
- a loopback-constrained adapter for the exact Spec 002 capabilities and current-snapshot endpoints;
- deterministic synthetic fixtures and a standalone browser demo;
- overview, trend, workload, log, and observer health/privacy views; and
- package typecheck, tests, production builds, and GitHub-hosted validation.

This slice does **not** change the accepted Spec 002 API/schema contract. It does not implement the Go runtime, native
collectors, persistence, an HTTP listener, dashboard embedding, upload, installers, or release artifacts. A later
runtime-integration slice must serve the accepted contract without coupling components to collectors.

## Functional requirements

- **R1 Transport-neutral UI:** Components MUST depend on `ObserverDataSource`, not fetch, collectors, storage, or a
  hosting application. Consumers MAY provide another authenticated adapter.
- **R2 Safe local transport:** An absolute local adapter URL MUST use HTTP(S) on `localhost`, `127.0.0.1`, or `::1`.
  The default MUST be `http://127.0.0.1:9847`. It MUST call only `GET /api/v1/capabilities` and
  `GET /api/v1/snapshots/current`, omit cookies, reject redirects, avoid tokens in URLs, enforce response ceilings, and
  parse safe Problem Details. Same-origin relative use MAY be supported explicitly.
- **R3 Offline demo:** The demo MUST use deterministic synthetic data, require no observer runtime, and make no network
  request through its data source.
- **R4 Useful views:** The package MUST provide Overview, Trends, Workloads for process/service/container records, Logs
  with source/severity/context controls, and Observer Health/Privacy views. Trends MUST be visibly unsupported for a
  source without an explicit series method.
- **R5 Honest capability states:** The view model and UI MUST preserve support, collection, freshness, nullable reason,
  observation time, total count, returned count, and truncation independently. Unknown future states are neutral;
  unavailable/null data is never plotted or displayed as zero; supported successful empty lists remain healthy zero.
- **R6 Process privacy:** The default process model MUST exclude accounts, owners, usernames, user identifiers, raw
  arguments, and environment variables. The UI MUST NOT infer or display them.
- **R7 Log privacy:** The log model MUST be the exact closed Spec 002 union: metadata contains observation time, source,
  severity, and event code; body state is `OMITTED` or the bounded `REDACTED_LOCAL_ONLY` object. It MUST expose no
  summary, arbitrary structured-field map, or raw-body property. Presentation labels derive only from event code.
- **R8 Partial availability:** Capabilities, current snapshot, and optional trends MUST load independently; one failed
  read MUST NOT hide successful reads.
- **R9 Accessible responsive shell:** Navigation, filters, tables, state, and loading/error feedback MUST use semantic
  accessible controls and remain usable at a 390 CSS-pixel viewport without hiding core data.
- **R10 Local assets and scope:** CSS MUST be component-scoped. The core package export MUST exclude the local adapter
  and demo fixtures. The package and demo MUST load no analytics, tracking, external runtime script, font, or CDN
  asset.
- **R11 Verified package:** Dependencies MUST be locked, typecheck and tests MUST cover privacy and adapter boundaries,
  production builds MUST emit the reusable library and standalone demo, and GitHub-hosted validation MUST target less
  than 15 minutes with read-only repository permissions.

## Acceptance evidence

- `npm run check` in `web/observer-ui` runs strict TypeScript validation, deterministic tests, the library build, and
  the standalone demo build.
- Adapter tests load the real accepted Linux, Windows, log-opt-in, and Problem Details fixtures and cover exact endpoint
  paths, port, mapping, loopback/credential rejection, header-only bearer handling, media types, and response ceilings.
- Fixture and component tests prove process identity fields and log bodies are absent by default, privacy text is
  visible, capability failure states remain distinct, filters work, and navigation exposes accessible names.
- `.github/workflows/dashboard.yml` runs the same gate on a GitHub-hosted runner with read-only permissions and a
  ten-minute timeout.

## Compatibility and follow-up

The package is pre-release. Additive optional view fields are compatible; removing or changing a required field needs
a major package version or a documented migration before a stable release. Runtime embedding requires a separate
specification and acceptance evidence. Platform Demo supplies its own adapter and does not iframe another machine's
loopback UI.
