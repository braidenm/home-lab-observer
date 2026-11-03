# First-party Docker controls: disposable acceptance

Date: 2026-11-03. Status: supervised proof planned; not installed control activation.
Scope follows [ADR033](../../docs/adr/033-first-party-dashboard-and-management.md),
[ADR029](../../docs/adr/029-fixed-linux-action-engine.md) and the existing
[Engine test contract](engine-adapter.md). No new library, UI or operation protocol.

## Reuse and plan

Use the already merged `actionengine`, `actioncore`, `actionjournal` and existing
`TestOwnedNativeFixture`, not Docker CLI start/stop commands standing in for our code.
The native test executes our typed methods through our core/journal, verifies actual
container state and restart identity, and proves retained requests do not mutate again.

Build the existing fixture and native test binary from one exact clean merged source,
using pinned Go 1.27.3, Linux/amd64, CGO disabled and verified offline modules.
Transfer the hash-bound binaries and public source to a fresh owner-private stage
on an authorized Linux host; this avoids installing a compiler or changing Docker
permissions there. Before effects, verify source/helper/binary identities and that
the CLI and native adapter reach the same compatible daemon. Use the existing
fixture-ownership guards and argument builders; do not relax them for convenience.

## Acceptance and limits

- Two sequential synthetic containers only: one handles SIGTERM; one ignores it.
  Run each as non-root, no network/ports/host mounts/capabilities, read-only root,
  0.25 CPU, 32 MiB, 64 PIDs, no restart policy or container log collection.
- Only their newly captured full IDs, exact fixture image, run label and expected
  confinement can be mutated or removed. Never select an existing workload by name.
- Prove START, RESTART and STOP through the existing native test; restart must
  change the start witness, graceful stop exits cleanly, ignored SIGTERM requires
  the configured bounded forced stop, and retained replay adds no operation.
- Preserve real cooldown/timeouts. No mutation retry or inferred success after
  timeout. A failure retains its private stage/evidence and stops this run.
- Verify identity again before exact-ID cleanup, then independently verify filtered
  exact-ID absence. Preserve private image/build receipts; never prune unrelated data.
- Report finite results, not raw inspect, daemon errors, environment, logs or secrets.

This is actual Engine/core/journal proof, not actual Platform owner authentication,
installed worker isolation, end-to-end claim/result delivery or release acceptance.
Those remain required before enabling our UI controls. Observation/read sharing
and existing workloads remain unchanged. No general container-create/delete API
is introduced by creating/removing a test fixture outside the application.

## Tasks

- [x] Select unchanged merged native test and record a source-first bounded plan.
- [ ] Independently review the execution plan and private one-run orchestration.
- [ ] Execute once; record finite results and exact-ID cleanup evidence.
- [ ] Merge this small acceptance receipt through required checks.
- [ ] Complete missing consumed worker/permission wiring before our UI buttons.

## Evidence

Pending. Do not mark complete from successful compilation or a simulated Engine.
