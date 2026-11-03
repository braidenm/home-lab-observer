# First-party Docker controls: disposable acceptance

Date: 2026-11-03. Status: supervised Engine/core/journal proof passed; not installed control activation.
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
- [x] Independently review the execution plan and private one-run orchestration.
- [x] Execute supervised cases; record finite results and exact-ID cleanup evidence.
- [ ] Merge this small acceptance receipt through required checks.
- [ ] Complete missing consumed worker/permission wiring before our UI buttons.

## Evidence

Unchanged merged product/test source `2fdf880bfea13403db454a38cb40ff2cb7cd6e3e`,
Go 1.27.3 cross-compiled Linux/amd64 with CGO disabled; offline module verification
passed and source remained clean. Actual Linux daemon tuple was
`29.1.3|1.52|1.44|linux|amd64`; issued operation API remains exactly 1.45.

The first run stopped at START and cleaned up its exact fixture. The separate
orchestration had copied the executable into the image context with mode 0700,
preventing Dockerfile UID 65532 from executing it. The original merged harness
builds that fixture as 0755. An independently reviewed staging-only correction
restored 0755 inside the private 0700 build context; no product bytes or host
account/socket permissions changed. The failed stage was retained, not retried.

A fresh separately supervised run passed both sequential modes:

- Graceful fixture: START, RESTART with changed start witness, STOP with exit 0.
- Ignore-SIGTERM fixture: START, bounded STOP with forced exit 137.
- Both: retained request replay made no new Engine mutation; exact-ID cleanup
  acknowledged and independent exact-ID filtered inventory confirmed absence.

Finite final witness:

```text
start=pass stop=pass restart=pass forced_stop=pass duplicate_replay=pass fixtures=2 cleanup=absent
```

SHA-256 build inputs: fixture
`4575c558e0bc31915b9799733987085414469c5a1e0c670937a28032abf102b4`,
native test binary
`343470ceefcfb873fa4a74660309f44db8b85eef54a74354b8c738666c3ff15b`.
Private hash-bound receipts retain synthetic IDs and image identity; no production
container identity, credential, log or host topology is published here.
This result still does not prove installed worker or Platform authorization.
