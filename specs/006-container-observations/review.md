# Spec 006 review and acceptance evidence

## Review approach

The collector, API/contract and reusable UI were implemented in separate worktrees. Reviewers inspect slices they did
not author; the integration owner verifies the packaged service and browser. Required hosted checks still gate merge.

## Findings and corrections

| Finding | Correction / regression evidence |
| --- | --- |
| Docker's per-core CPU convention would conflict with the host-capacity contract | Valid counter deltas divided by system deltas, bounded to 0–100; missing/reset counters remain null, and absent/zero precpu uses bounded prior samples |
| One-shot stats and engine minimum-version negotiation needed explicit handling | Fixed GET query, minimum/maximum intersection and fake-engine regressions |
| Native Windows/unknown engine stats were not certified | Retain inventory, but omit running stats with ENGINE_OS_UNSUPPORTED unless the engine reports Linux |
| Null engine inventory could appear as healthy empty | Reject null with INVALID_RESPONSE; valid empty arrays remain healthy zero |
| Privacy table implied container history/upload despite the preview policy | Container row now explicitly says current memory only and no upload; further projections require a reviewed spec |
| Live Docker smoke could pass with permanently unavailable CPU | Wait for a valid bounded CPU reading and memory with AVAILABLE quality after warmup |
| Container metric interpretation and phone scrolling were implicit | Visible host-capacity CPU/engine-memory explanation, unavailable stopped metrics and horizontal-scroll guidance |
| Workload filters and legacy capabilities could confuse navigation | Reset filters on every tab-change path and explain the dedicated model in Observer & privacy |
| Client rejected known truncation when the larger total was unknown | Accept truncated=true with equal counts; keep legacy metric labels unqualified |
| API row cap could lose a source's known count | Compute total before the cap; regression covers the defensive alternate-source case |
| Native fixture paths could exceed macOS socket limits | Short private temporary socket directory, real named-pipe/Unix HTTP roundtrip, redirect rejection and cancellation tests |

The API consumes the collector's normalized typed cache, not arbitrary plug-in data. Alternate future sources must honor
that contract; adding a generic source/plugin boundary requires a separate validation/threat-model decision.

## Local evidence

- Repository policy check passes.
- OpenAPI/JSON Schema validation: five schemas, 12 valid and 10 invalid fixtures.
- Eleven synthetic handler responses validate, including dedicated container, limited and disabled views.
- Integrated UI typecheck, 31 component/adapter tests, library/demo builds and deterministic embedded build pass.
- Full Go suite, vet and real native snapshot schema validation pass on Windows.
- Packaged Edge smoke passes at 390/768/1440 pixels: protected real service/history, disabled container guidance,
  keyboard tabs, synthetic running/stopped/partial table, filtering, no page overflow and token lock/forget.
  The synthetic browser table is not presented as live-engine evidence; Linux CI tests that separately.
- Independent UI review inspected phone/desktop screenshots and approved readability, accessible table scrolling,
  keyboard navigation and honest zero/unavailable/partial states. Images stay local and are not published as host data.

## Hosted evidence

The first [PR 9](https://github.com/braidenm/home-lab-observer/pull/9) runtime run passed on Linux (2m54s), Windows
(2m7s), macOS (1m41s) and all six cross-build targets (2m32s). The final reviewed head must pass required checks again.
The initial policy failure was a synthetic token/pattern literal, corrected without weakening the scanner. An unrelated
Google apt repository checksum mismatch interrupted the browser dependency install; no integrity checks were bypassed.
Linux CI additionally creates isolated synthetic running/stopped Docker workloads,
checks inventory and real CPU/memory, verifies privacy/authentication/limits and checks workloads remain unchanged.
Windows/macOS native tests and cross-builds do not imply certification against live Desktop engines.

## Remaining scope

This is a source-build, foreground, read-only preview. Native installers/background services, Linux container packaging
with a constrained proxy, Podman certification, OS services/logs/sensors and outbound Platform Demo enrollment/upload
remain separate specifications. No daemon is installed or enabled; no live user workload is mutated by this change.
