# Public local API

[`openapi.v1.json`](openapi.v1.json) is the canonical OpenAPI 3.1 contract for the first local read-only surface. It
defines authenticated capability and current-snapshot reads plus detail-free health probes. The declared server is
explicit loopback; the future runtime must validate Host and Origin, emit no CORS grant, and require a separate
authenticated TLS design before any non-loopback listener exists.

The first contract intentionally has no configuration, collection, upload, enrollment, action, shell, file, log-source,
or lifecycle mutation route. `GET /api/v1/snapshots/current` supports only code-owned section names and bounded process,
container, and log limits. Response ceilings are 128 KiB for capabilities and 1 MiB for a current snapshot.

Schemas live under [`../schemas/v1`](../schemas/v1). Run `npm test` to validate OpenAPI, schemas, fixtures, privacy
canaries, read-only semantics, bounds, and requirements traceability.

The legacy remote `home-lab-server-snapshot/v1` payload is a separate strict upload projection. Local history, richer
observations, and log message bodies do not become remotely eligible by appearing in this API.
