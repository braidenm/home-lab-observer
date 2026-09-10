# Versioned schemas

`v1/` contains JSON Schema 2020-12 contracts for capabilities, the current sanitized local snapshot, bounded metric
series, container inventory, diagnostics health, fixed-grid native log summaries, and RFC 9457-style Problem Details.
`v1/fixtures/manifest.json` declares which synthetic fixtures must pass and which must fail.

Within a released major version, optional response fields may be added and consumers must ignore unknown response
fields. Removing or renaming fields, changing meaning/type/unit/privacy, narrowing a published bound, or adding a
required input creates a new major schema and API path with migration and rollback evidence.

Contract objects are closed so producers cannot emit accidental fields. This does not permit clients to reject a newer
response solely because it contains an additive optional field; client forward-compatibility is a separate required
behavior. Incoming upload payloads remain strict.

Metric identifiers, units, privacy meaning, and range semantics are stable within v1. Adding an identifier requires a
coordinated schema, OpenAPI, fixture, and client update; changing an existing identifier's meaning or unit is breaking.
The series contract intentionally has no arbitrary label map, selector, SQL, expression, or custom range escape hatch.
The log summary contract exposes fixed source/severity dimensions and explicit historical coverage without log bodies,
event codes, identities, paths, native payloads, or arbitrary query dimensions.

Run `npm test` from the repository root. Fixtures contain reserved domains and synthetic identifiers only.
