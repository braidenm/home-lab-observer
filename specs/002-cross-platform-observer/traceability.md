# Spec 002 requirements traceability

| Requirement | Contract evidence | Fixture evidence | Automated assertion |
| --- | --- | --- | --- |
| R1 | `api/openapi.v1.json` paths | All valid fixtures | OpenAPI validation and read-only method scan |
| R2 | `capabilities-v1.schema.json` support-state enum | Linux and Windows capabilities; invalid state | AJV valid/invalid expectations |
| R3 | `current-snapshot-v1.schema.json` sections | Linux current snapshot | AJV validation and required-section checks |
| R4 | Shared section status definition | Supported, denied, and unsupported sections | Unsupported-with-items invalid fixture |
| R5 | OpenAPI snapshot query parameters | Invalid-query Problem Details | Exact maximum/default assertions |
| R6 | OpenAPI `x-max-response-bytes` | Current snapshot fixture | UTF-8 fixture byte ceiling assertion |
| R7 | Capabilities and snapshot privacy objects | Safe-default fixtures | Prohibited-key, credential, and private-address scan |
| R8 | Log metadata/body definitions and conditional body policy | Omitted and redacted-local-only logs | Upload-eligible-body invalid fixture |
| R9 | `problem-details-v1.schema.json` and media type | Valid invalid-query problem; stack-trace problem | AJV valid/invalid expectations |
| R10 | Spec compatibility policy and schema version constants | Manifest pins schema IDs | Expected schema IDs and OpenAPI version assertion |
| R11 | Fixture manifest | `valid/` and `invalid/` trees | Every manifest entry evaluated; both outcomes required |
| R12 | `scripts/validate-contracts.mjs` | All fixtures | `npm test` and Contracts workflow |
| R13 | Capability platform object and support states | Linux and Windows capability fixtures | Synthetic-identifier/privacy scan |
| R14 | API/schema READMEs and snapshot privacy projection | Snapshot privacy object | Legacy projection constant assertion |

The validation script fails if any R1-R14 identifier is absent from this matrix or the accepted specification.
