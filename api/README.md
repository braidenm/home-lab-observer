# Public local API

[`openapi.v1.json`](openapi.v1.json) is the canonical OpenAPI 3.1 contract for the first local read-only surface. It
defines authenticated capability, current-snapshot, bounded metric-series and dedicated container-inventory reads plus
detail-free health probes. The runtime uses explicit loopback, validates Host and Origin, emits no CORS grant, and requires a separate
authenticated TLS design before any non-loopback listener exists.

The first contract intentionally has no configuration, collection, upload, enrollment, action, shell, file, log-source,
or lifecycle mutation route. `GET /api/v1/snapshots/current` supports only code-owned section names and bounded process,
container, and log limits. Response ceilings are 128 KiB for capabilities and 1 MiB for a current snapshot.
`GET /api/v1/metrics/series` requires one of four fixed ranges and one to six repeated, code-owned metric identifiers;
its response ceiling is 1 MiB. It exposes UTC values with explicit null gaps and the actual sample interval, not an
arbitrary metric query language.

`GET /api/v1/containers` reads the optional Docker collector's cached `observer-container-inventory/v1` model, with
`limit=1..500` (default 100) and a 1-MiB response ceiling. It includes stopped containers and represents missing CPU or
memory as null with a reason. It never starts a Docker query on an HTTP request. The existing snapshot/capabilities
schemas remain unchanged: their container section still describes the unsupported legacy projection, not this new
dedicated view. Container metadata is local-sensitive, current-only, and explicitly ineligible for remote upload.

Schemas live under [`../schemas/v1`](../schemas/v1). Run `npm test` to validate OpenAPI, schemas, fixtures, privacy
canaries, read-only semantics, bounds, and requirements traceability.

The legacy remote `home-lab-server-snapshot/v1` payload is a separate strict upload projection. Local history, richer
observations, and log message bodies do not become remotely eligible by appearing in this API.
