# Spec 006 review and acceptance evidence

## Review approach

The collector, API/contract and reusable UI were implemented in separate worktrees. Reviewers inspect slices they did
not author; the integration owner verifies the packaged service and browser. Required hosted checks still gate merge.

## Findings and corrections

| Finding | Correction / regression evidence |
| --- | --- |
| Docker's per-core CPU convention would conflict with the host-capacity contract | Collector review requires valid counter deltas divided by system deltas, bounded to 0–100; missing/reset counters remain null |
| One-shot stats and engine minimum-version negotiation needed explicit handling | Fixed request/version boundary and fake-engine regressions are required before acceptance |
| Privacy table implied container history/upload despite the preview policy | Container row now explicitly says current memory only and no upload; further projections require a reviewed spec |
| Live Docker smoke could pass with permanently unavailable CPU | Wait for a valid bounded CPU reading and memory with AVAILABLE quality after warmup |
| Container metric interpretation and phone scrolling were implicit | Visible host-capacity CPU/engine-memory explanation, unavailable stopped metrics and horizontal-scroll guidance |

## Local evidence

- Repository policy check passes.
- OpenAPI/JSON Schema validation: five schemas, 12 valid and 10 invalid fixtures.
- Eleven synthetic handler responses validate, including dedicated container, limited and disabled views.
- Integrated UI typecheck, 29 component/adapter tests, library/demo builds and deterministic embedded build pass.
- Full Go suite, vet and real native snapshot schema validation pass on Windows.
- Packaged Edge smoke passes at 390/768/1440 pixels: protected real service/history, disabled container guidance,
  keyboard tabs, synthetic running/stopped/partial table, filtering, no page overflow and token lock/forget.
  The synthetic browser table is not presented as live-engine evidence; Linux CI tests that separately.
- Independent UI review inspected phone/desktop screenshots and approved readability, accessible table scrolling,
  keyboard navigation and honest zero/unavailable/partial states. Images stay local and are not published as host data.

## Hosted evidence

Pending the reviewed head of [PR 9](https://github.com/braidenm/home-lab-observer/pull/9). Linux CI additionally creates isolated synthetic running/stopped Docker workloads,
checks inventory and real CPU/memory, verifies privacy/authentication/limits and checks workloads remain unchanged.
Windows/macOS native tests and cross-builds do not imply certification against live Desktop engines.

## Remaining scope

This is a source-build, foreground, read-only preview. Native installers/background services, Linux container packaging
with a constrained proxy, Podman certification, OS services/logs/sensors and outbound Platform Demo enrollment/upload
remain separate specifications. No daemon is installed or enabled; no live user workload is mutated by this change.
