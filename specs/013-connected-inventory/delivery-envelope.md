# Atomic connected observation envelope

Status: contract slice; no upload admission, ledger migration, runtime wiring or installed-profile change.

## Outcome and decision

Use one new `home-lab-connected-observation/v1` document, profile `numeric-host-workloads/v1`, containing the
existing canonical native-host document and the new canonical workload document. Keep a single cycle, sequence,
pending body and retry digest. Do not introduce independent telemetry streams or combine arbitrary latest rows.
Sequence and SHA-256 retry digest remain external ledger/transport metadata over the exact envelope bytes, not body
fields. The receiver binds the admitted transport sequence to the one atomic projection transaction.

The envelope has exactly `schema_version`, `projection_profile`, `source_id`, `collector_version`, `collected_at`,
`host_metrics` and `workload_inventory`. Both components are embedded JSON objects in their existing canonical field
form, never JSON strings or base64. Alternate representations are rejected. Both are required, including explicitly disabled workload
groups. Each component must pass its own unchanged canonical validator. Root source, version and UTC cycle timestamp
must match both components exactly. Section-specific observation timestamps remain independent. Validation proves
neither freshness nor authenticated identity, persisted local consent, permitted remote profile or management authority.

The whole UTF-8 envelope is capped at 294912 bytes (288 KiB); host component stays capped at 16384 bytes and workload
component at 262144 bytes. This leaves bounded envelope overhead. There is no arbitrary extension map or raw snapshot.
Malformed, noncanonical, duplicate/unknown, cross-source/version/cycle, missing or oversized input returns a fixed
error without rejected values. Inputs are checked before constructing the envelope. Failed encoding returns no body.

## Why not separate uploads?

Separate independently sequenced documents require stream-aware ledger, atomic completeness and UI freshness rules.
The current installation already has one durable pending-body slot. A combined envelope reuses that model, avoids a
new scheduler/multiplexer and prevents joining host and workload rows from different admitted sequences. This does
not make an older section fresh: read models and UI must preserve each section's own `observed_at` for freshness,
never replacing it with envelope/host cycle time or receipt time.
Keep numeric-v1/legacy-v1 admission unchanged; silently attaching optional fields would weaken closed contracts.

## Later integration gates (not implemented here)

- Deploy receiver/read-model support first. Persist host compatibility, native host and workload projections in one
  transaction under the same admitted sequence; join only matching sequences. Numeric-only uploads must not reuse
  old workload rows. Existing ownership/revocation checks apply to every read.
- Bind the permitted profile to registration/credential state; payload booleans are not consent. Existing registration
  events replay with legacy-compatible behavior, not retrospective rich consent. Host-local group opt-in is independent.
- Define one rich installed compatibility profile. Current numeric admission, socket deny and foreign-process isolation
  remain intact until exact packaged proof. Preserve the working connector during any separately enrolled coexistence.
- Increase profile-aware handoff, ledger, transport and receiver caps together. The current ledger's 16384-byte body
  constraint and 262144-byte database cap cannot hold a maximum new envelope. Bound database/journal/staging/total disk,
  allocation and disconnected retry, not just HTTP bytes. Never rewrite a pending body to fit another profile.
- Prove old-format drain/reader floor and supported rollback, or choose separately enrolled coexistence. Never reset
  sequence, restore an old used ledger, move credentials to another identity or run an old binary against new state.

## Contract proof

Use synthetic canonical fixtures and tests for round-trip identity, disabled/partial groups, independent section times,
source/version/cycle mismatch, unknown/duplicate/missing fields, malformed encodings and component/document byte caps.
Prove existing upload admission still refuses this profile. No network, host collection or second Spring context is
needed. Installed VM, receiver transaction, local consent and owner/delegate journeys remain explicit later gates.

The combined-envelope design is an application choice. PostgreSQL's all-or-nothing transaction/row-lock guarantees
support the later same-sequence projection update, not automatic application correctness. See
[transactions](https://www.postgresql.org/docs/18/tutorial-transactions.html) and
[row locking](https://www.postgresql.org/docs/18/explicit-locking.html), reviewed 2026-10-29. Version new persisted
registration facts rather than editing existing event meaning; see
[Axon event versioning](https://docs.axoniq.io/axon-framework-reference/4.13/events/event-versioning/).
