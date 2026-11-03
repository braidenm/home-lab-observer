# Buildable delivery slices for the dormant action runtime

**Historical plan, superseded as the default roadmap on 2026-11-03 by
[the reuse-first plan](reuse-first-plan.md), now clarified by [ADR033](../../docs/adr/033-first-party-dashboard-and-management.md).**
Preserve this implementation/review record; do not continue the ten-layer rollout solely to complete it.
Its safety gates still apply if the dormant custom runtime is ever selected for activation.

2026-11-03. Delivery plan; review and verify each reconstructed head before publication.

The integrated local checkpoint is much larger than a useful review. Preserve its reviewed implementation
bytes while delivering acyclic, tested layers. A local checkpoint or library merge does not make controls
operable. No slice adds an installer, running service, credentials, enabled flag or permission to operate a host.
The working read-only collection and dashboard remain independent of these dormant action components.

Use the accepted Spec011/ADRs and reviewed `91a3862` implementation as the initial source of truth; later
reviewed storage-fault tests have their own pinned checkpoint. Do not copy shared uncommitted work, invent
temporary stubs, remove assertions or combine unrelated dependency updates to make a head build.

## Sequence

Each row is one coherent PR. Use an isolated, initially clean worktree based on the preceding delivered head.
Select only the named paths from their reviewed Git objects and verify their raw hashes before commit.

| Order | Scope | Required earlier layers |
| --- | --- | --- |
| 1 | Linked Spec011/ADR030/ADR031 documents, including capacity, paired-client, fault and delivery plans | Existing main |
| 2 | Whole `internal/actionprotocol/`, including its pinned synthetic fixture and README | 1 |
| 3 | Whole `internal/actionremote/`, including the ordinary refusal tests and explicitly tagged paired probe | 2 |
| 4 | Whole `internal/actionbudget/` and the five changed guarded-Engine source/test paths | 2 |
| 5 | All changed `internal/actionjournal/` paths together, including existing-file integration and reviewed v2 tests | 4 |
| 6 | Whole `internal/actionprofile/` and `internal/actionipc/` | 2, 4 |
| 7 | Broker policy, protected-source and ownership bookkeeping subset below | 4, 6 |
| 8 | Whole `internal/actionworker/` and `cmd/observer-action-worker/` | 3, 4, 6 |
| 9 | Whole `internal/actionadmin/` and `cmd/observer-action-admin/` | 4, 6 |
| 10 | Remaining `internal/actionbroker/` and `cmd/observer-action-broker/` | 5–9 |

Layer7 is exactly `policy.go`, `policy_binding.go`, `policy_test.go`, `broker_files.go`,
`broker_files_test.go`, `broker_files_linux.go`, `broker_files_linux_test.go`, `owned_workers.go`,
`owned_workers_test.go`, `owned_join.go` and `owned_join_linux_test.go`, under `internal/actionbroker/`.

Keep journal's existing-file changes with its new schema, guarded transaction, loaders and test helpers.
Keep protocol and HTTP fixture helpers within their owning complete packages. Linux listeners cannot be
published alone unchanged: `ErrRecoveryRequired`, `brokerState`, execution/control and root recovery pull the
remaining graph together. The final broker integration is consequently larger than the earlier slices;
review it as one consumed runtime boundary, not a series of broken heads. Do not refactor it merely to hit
a file-count target without a separate source-first review.

## Gates and evidence

These boundaries are source-derived proposals, not already green reconstructed heads. For each slice:

1. Verify the selected exact source closure and preserve all incoming main changes. Run the relevant local
   package checks, then existing required Windows/macOS checks and Linux race/vet without new skips or limits.
2. Keep physical storage testing in its existing closed private profile. Its code may compile in ordinary
   tests, but no physical PASS follows from an environment-gated skip or a previous v1 result. Layer5 must
   carry the reviewed v2 acceptance receipts, not silently defer a failing storage assertion.
3. Record root/independent source review, actual head checks and the exact merged source. Auto-merge only
   that checked head through the repository's normal required gates; no force, bypass or direct main push.
4. Describe precisely what was delivered and what remains dormant. Historical local receipts remain local
   evidence; do not relabel them as current-head managed CI, installation or production compatibility.

The paired probe is optional additional acceptance, never a skipped replacement for an ordinary journey.
Its fixed source digest selects the reviewed whole client closure, not necessarily an incomplete early layer.
Do not repin it to arbitrary manifests or promise native pairing before all its reviewed inputs are available.

After the layers are delivered, central recovery/finalization and validated retained queries, stronger
candidate/fallback compatibility floors, package/installed identity and lifecycle/VM checks remain required.
Only then operate a labelled disposable container for start/stop/restart. Owner buttons and any existing
workload operation remain gated on those results. Kubernetes controls remain out of this phase.
