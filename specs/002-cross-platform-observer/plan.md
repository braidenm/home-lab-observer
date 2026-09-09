# Implementation plan: Local API contracts and compatibility fixtures

## Decision

Land a contract-only slice before runtime code. OpenAPI owns HTTP paths, parameters, security, media types, and response
ceilings. Standalone JSON Schemas own payload semantics. A manifest maps synthetic fixtures to schemas and expected
validity. One Node script validates the whole set and enforces repository-specific privacy/read-only invariants that a
generic schema validator cannot express.

## Artifacts

- `api/openapi.v1.json`: OpenAPI 3.1 local read-only surface.
- `schemas/v1/*.schema.json`: JSON Schema 2020-12 contracts.
- `schemas/v1/fixtures/{valid,invalid}`: synthetic examples and negative proofs.
- `schemas/v1/fixtures/manifest.json`: fixture expectation registry.
- `scripts/validate-contracts.mjs`: OpenAPI/schema/fixture/policy validation.
- `.github/workflows/contracts.yml`: five-minute GitHub-hosted CI job.
- `traceability.md`: R1-R14 evidence map.

## Validation flow

```text
npm test
  -> parse and validate OpenAPI 3.1
  -> compile JSON Schema 2020-12 documents
  -> validate every manifest fixture
  -> require invalid fixtures to fail
  -> assert GET-only API and loopback server/security policy
  -> assert parameter and response ceilings
  -> scan valid fixtures for prohibited keys/credential shapes/private addresses
  -> verify each accepted requirement has automated or documented evidence
```

## Compatibility and rollback

This slice has no runtime or persisted-state migration. If a contract is wrong before implementation, amend v1 with
fixtures while it has no released producer. After a runtime release implements v1, incompatible correction creates v2
beside v1. Rollback reverts the contract commit and its CI workflow; no machine state or Platform payload changes.

## CI budget

Use Node 22, `npm ci --ignore-scripts`, AJV 2020-12, format validation, and OpenAPI parsing on `ubuntu-latest`. The job
has read-only contents permission and a five-minute timeout, leaving the repository-wide pull-request budget below
15 minutes.

## Follow-on implementation gates

- Runtime handlers MUST be generated from or contract-tested against these files.
- Collector/store work MUST produce the current snapshot fixture shape before opening a listener.
- UI work MUST consume the same API rather than a private collector object.
- Upload work MUST explicitly map local observations down to the legacy remote-safe schema.
- Any LAN listener, message-body upload, or mutation endpoint requires a new specification and threat-model change.
