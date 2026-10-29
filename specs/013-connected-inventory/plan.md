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
3. **Opt-in rich runtime:** settle the installed observation profile and local policy in ADR 023. Reuse the collectors
   behind a minimal, credential-free acquisition role. Any Docker-authority helper is separately mediated and confined;
   the network uploader receives only an allowlisted bounded document, never socket access or raw inspect data.
4. **Delivery and migration:** apply profile-aware limits throughout handoff, ledger, transport and receiver; preserve
   exact retries and monotonic sequence. Implement and rehearse the supported profile-changing install/upgrade or
   separately enrolled coexistence path. Do not bypass current installed admission to accelerate the canary.
   First define the unused [combined envelope contract](delivery-envelope.md), so host and workloads share one cycle
   and pending upload instead of introducing independent streams. This contract does not increase runtime limits.
5. **Useful dashboard:** host overview plus searchable/bounded Containers, Processes and Storage views. Keep empty,
   disabled, failed, truncated and stale explicit. Add Services and aggregate trends as separately coherent changes;
   don't make container visibility wait for an entire log/history implementation.
6. **Release and accept:** independent review, small PRs, governed merges, exact artifact publication, isolated VM and
   Docker fixtures, then owner-host opt-in. Verify actual owner/delegated reads and revocation, resource budgets and
   legacy coexistence. Only after read-only usability proceed to Spec 011 broker/execution/grants/UI slices.

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
disable the new group/broker and keep credentials, pending bytes, sequence and unresolved action evidence intact.
Exact compatible code rollback and profile replacement must be proved; neither restore an old ledger nor assume
changing a release-directory symlink is safe.
