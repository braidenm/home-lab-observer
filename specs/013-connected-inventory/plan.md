# Rich connected inventory implementation plan

## Boundaries

Reuse `internal/collector` and `internal/containerobs` for normalized read-only observations, with their existing
dependency injection and bounded scans/fan-out. Keep observation acquisition, explicit egress projection, private
handoff, authenticated delivery, hosted read models and UI independent. Do not repurpose the uploader as a Docker client.
No new dependency is selected by this plan; gopsutil v4.26.9 is the current source baseline.

The current connected worker imports `numerichost`, not the rich collector. Its socket syscall deny, private network,
invisible foreign proc entries and fixed installed compatibility profile are deliberate. A source import or new JSON
field cannot establish that host data is visible or that the richer runtime is isolated.

## Small delivery sequence

1. **Truthful dashboard now:** existing Platform Demo Spec 108 presents uncollected sections without zero counts;
   preserve healthy-empty and temporary-failure semantics, native values and legacy inventory. This changes no egress.
2. **Bounded workload contract:** add pure explicit process/container projection, synthetic fixtures, strict schemas and
   quality/boundary tests. Add a corresponding backend parser/receiver contract before any producer wire activation.
   Preserve nullable container metrics instead of changing the legacy mandatory-numeric DTO. Exact first-slice fields,
   consent, quality mappings and budgets are in [the executable profile](workload-profile.md).
3. **Opt-in rich runtime:** use the separately enrolled immutable policy in [ADR 024](../../docs/adr/024-immutable-rich-installation-profiles.md).
   First prove process-only acquisition without any Docker role/socket; this staging proof does not require an interim
   owner-host entry. Hold the useful rich owner rollout until the process-and-Docker profile is accepted. Reuse the collectors
   behind a minimal, credential-free acquisition role. Any Docker-authority helper is separately mediated and confined;
   the network uploader receives only an allowlisted bounded document, never socket access or raw inspect data.
4. **Delivery and migration:** apply profile-aware limits throughout handoff, ledger, transport and receiver; preserve
   exact retries and monotonic sequence. Implement and rehearse the supported profile-changing install/upgrade or
   separately enrolled coexistence path. Do not bypass current installed admission to accelerate the canary.
   First define the unused [combined envelope contract](delivery-envelope.md), so host and workloads share one cycle
   and pending upload instead of introducing independent streams. This contract does not increase runtime limits.
   Implement the [profile-bound state and storage contract](upload-profiles.md) in separate coherent slices: adapter-free
   model prerequisite, unused pure state machine, then exact-profile durable ledger. Only afterward wire approved
   enrollment/policy, bounded handoff/transport and receiver; no current worker silently selects the rich profile.
   The next coherent runtime slice is the [VM-installable fixed process package](installable-process-proof.md), not another
   standalone contract. Its process-only profile is staging evidence, not an intermediate owner-host installation.
   After that exact packaged VM proof passes, deliver the [coherent process-and-Docker installation](installable-docker-observation.md)
   under [ADR 025](../../docs/adr/025-isolated-installed-docker-observation.md): fixed credential-free file-producing
   Docker reader, distinct immutable profile/kind/schema-v3, reader-first hosted integration and one complete artifact
   acceptance target. This design adds no implemented helper, socket access or owner rollout.
5. **Useful dashboard:** host overview plus searchable/bounded Containers, Processes and Storage views. Keep empty,
   disabled, failed, truncated and stale explicit. Add Services and aggregate trends as separately coherent changes;
   don't make container visibility wait for an entire log/history implementation.
6. **Release and accept:** independent review, small PRs, governed merges, exact artifact publication, isolated VM and
   Docker fixtures, then owner-host opt-in. Verify actual owner/delegated reads and revocation, resource budgets and
   legacy coexistence. Only after read-only usability proceed to Spec 011 broker/execution/grants/UI slices.

### Review-size adjustment — 2026-10-30

Keep the single installed process-and-Docker acceptance target, but review its implemented source in three coherent
layers: [bounded data plane and storage](docker-data-plane.md), then three dormant fixed workers, then privileged
installation and complete four-executable packaging. Each layer must compile and test independently; later layer
references must not appear before their implementation. No layer alone approves a release or exposes owner enrollment.
Preserve the process-only VM evidence and all remaining Docker/hosted/owner gates; do not call this an activated profile.

## API, state and observability

Observation uploads remain replaceable latest telemetry, not invented Axon domain events. Local opt-in and future
control/grant lifecycle are distinct state boundaries that need their own explicit model; no state write is added by
the contract-only phase. Platform registration/grants retain their existing domain ownership and user-ID isolation.
Inventory payloads never enter logs, diagnostics, metric labels or browser failure artifacts. Emit bounded outcomes,
counts, durations and drop/truncation metrics, not raw names/IDs/rejected values.

## Tests and rollback

Use deterministic unit/contract tests for field allowlists, unavailable/partial/null/empty states, invalid numbers,
hostile strings, oversized rows/documents, consent suppression and source mismatch. Use the real receiver and a small
owner/delegate HTTP journey for integration. No second full Spring context, DirtiesContext or duplicate broad suite.
Use manual disposable fixtures for daemon faults and installed isolation, with only redacted fixed result records.

Before activation, removing an unused contract/profile is a code rollback with no data migration. After activation,
stop/fence new rich workers and keep credentials, pending bytes, sequence and unresolved action evidence intact.
Group changes require ADR 024's confirmed revocation and separately enrolled identity, not a live policy toggle.
Exact compatible code rollback and profile replacement must be proved; neither restore an old ledger nor assume
changing a release-directory symlink is safe.
