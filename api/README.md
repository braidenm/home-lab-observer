# Public contracts

Home Lab Observer will publish its read-only local API as OpenAPI 3.1 and its stored/uploaded payloads as versioned JSON Schemas. Contracts are introduced by an accepted implementation specification, include valid and invalid fixtures, and run compatibility checks in CI.

The legacy remote `home-lab-server-snapshot/v1` projection remains an integration compatibility boundary during migration. Local rich observations and historical logs do not get added to that bounded latest-state payload.
