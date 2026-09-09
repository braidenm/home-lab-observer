# Spec 004: Reusable local dashboard

**Status:** Accepted  
**Owner:** Repository owner  
**Created:** 2026-09-09  
**Accepted:** 2026-09-09

## User outcome

An owner can inspect useful current and historical machine observations through a responsive local dashboard, while a
browser demo or another application can reuse the same UI against a different transport. The interface explains
capability gaps and privacy behavior instead of fabricating data or exposing sensitive source content.

## Slice boundary

This specification delivers the reusable TypeScript and React dashboard package under `web/observer-ui`:

- a transport-neutral `ObserverDataSource` and normalized view model;
- a loopback-constrained local HTTP adapter;
- deterministic synthetic fixtures and a standalone browser demo;
- overview, trend, workload, log, and observer health/privacy views; and
- package typecheck, tests, production builds, and GitHub-hosted validation.

This slice does **not** change the accepted Spec 002 API/schema contract. It does not implement the Go runtime, native
collectors, persistence, an HTTP listener, dashboard embedding, upload, installers, or release artifacts. A later
runtime-integration slice must map the canonical API contract into this view model or replace the adapter without
coupling components to collectors.

## Functional requirements

- **R1 Transport-neutral UI:** Components MUST depend on `ObserverDataSource`, not fetch, collectors, storage, or a
  hosting application. Consumers MAY provide another authenticated adapter.
- **R2 Safe local transport:** An absolute local adapter URL MUST use HTTP(S) on `localhost`, `127.0.0.1`, or `::1`.
  Same-origin relative use MAY be supported. Requests MUST be read-only, omit cookies, reject redirects, avoid tokens in
  URLs, and reject credential-bearing or non-loopback absolute URLs.
- **R3 Offline demo:** The demo MUST use deterministic synthetic data, require no observer runtime, and make no network
  request through its data source.
- **R4 Useful views:** The package MUST provide Overview, Trends, Workloads for process/service/container records, Logs
  with source/severity/context controls, and Observer Health/Privacy views.
- **R5 Honest capability states:** Unsupported, disabled, unavailable, permission-denied, and available-zero states MUST
  remain distinguishable where the view model provides them.
- **R6 Process privacy:** The default process model MUST exclude accounts, owners, usernames, user identifiers, raw
  arguments, and environment variables. The UI MUST NOT infer or display them.
- **R7 Log privacy:** Log bodies MUST be visibly off by default. `LogEvent.summary` MUST be a short, code-owned,
  sanitized metadata label and MUST NOT contain text copied from a raw log or message body. Any future body remains a
  separate opt-in, redacted, bounded, local-only field.
- **R8 Accessible responsive shell:** Navigation, filters, tables, state, and loading/error feedback MUST use semantic
  accessible controls and remain usable at a 390 CSS-pixel viewport without hiding core data.
- **R9 Local assets:** The package and demo MUST load no analytics, tracking, external runtime script, font, or CDN
  asset.
- **R10 Verified package:** Dependencies MUST be locked, typecheck and tests MUST cover privacy and adapter boundaries,
  production builds MUST emit the reusable library and standalone demo, and GitHub-hosted validation MUST target less
  than 15 minutes with read-only repository permissions.

## Acceptance evidence

- `npm run check` in `web/observer-ui` runs strict TypeScript validation, deterministic tests, the library build, and
  the standalone demo build.
- Adapter tests cover loopback acceptance, credential/non-loopback rejection, header-only bearer handling, omitted
  cookies, rejected redirects, error status, and JSON media type.
- Fixture and component tests prove process identity fields and log bodies are absent by default, privacy text is
  visible, capability failure states remain distinct, filters work, and navigation exposes accessible names.
- `.github/workflows/dashboard.yml` runs the same gate on a GitHub-hosted runner with read-only permissions and a
  ten-minute timeout.

## Compatibility and follow-up

The package is pre-release. Additive optional view fields are compatible; removing or changing a required field needs
a major package version or a documented migration before a stable release. Runtime embedding and mapping to the
canonical Spec 002 payloads require a separate specification and acceptance evidence. Platform Demo supplies its own
adapter and does not iframe another machine's loopback UI.
