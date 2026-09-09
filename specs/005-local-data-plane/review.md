# Spec 005 review and acceptance evidence

## Independent cross-slice review

The implementation was split across runtime/storage, HTTP, and embedded UI worktrees. Reviewers then inspected slices
they did not author; the integration owner ran real binary and browser tests and incorporated the findings. No reviewer
approval substitutes for required CI. PR 8 remains draft until the review corrections and final hosted checks pass.

| Finding | Correction and regression evidence |
| --- | --- |
| SQLite row deletion did not guarantee physical reclamation | Incremental auto-vacuum plus WAL checkpoints; real growth/reclaim test |
| Corruption recovery could confuse contention with corruption | Exclusive OS ownership lock; corruption-only quarantine, collision/sidecar preservation tests |
| Raw/rollup reads could race maintenance | One read transaction through `Store.ReadMetric`; last observation times survive downsampling |
| Cache copies could mutate shared state or emit null arrays | Deep status/data copies and slice cloning preserving empty arrays; API schema smoke |
| Windows process status omitted every otherwise readable process | Explicit unknown status for unsupported Windows field; native current-process regression |
| OS MIME registry changed JavaScript content type | Fixed embedded release-manifest media types; Windows hosted test regression |
| Response limits applied only after download | Bounded streaming reads with cancellation test |
| Local token creation briefly preceded restrictive Windows ACLs | Restrict directory/file before writing secret; ownership/ACL tests |
| UI unlock errors/storage availability could escape the gate | Safe construction failure and session-storage fallback; token canary and forget tests |
| Phone diagnostics hid simple signal values offscreen | Three-column signal table fits phone; wide diagnostic matrix has explicit scroll guidance |
| Overview could expose data despite denied collector state | State-aware projection and denied-data regression |
| Series support was inferred from metric names rather than collection | Explicit latest per-metric collector status, no unavailable historical-data exposure |

Readiness/latest-attempt, outage retention, checkpoint-busy handling, transient store recovery, schema-shape validation,
and strict client decoding findings are being corrected before final approval. This paragraph is replaced by the final
review result once those regressions and the integration suite pass.

## Local verification

- Full Go unit/integration suite and vet on Windows, including real native process availability.
- Root OpenAPI/JSON Schema/fixture tests and actual native snapshot validation.
- Actual service smoke: protected API, valid selected/degraded response envelopes, real metric history after successive
  collection sequences, no credential in API/process output, and graceful process/store reuse tests.
- Embedded dashboard: TypeScript, component/adapter tests, reusable-library/demo builds, deterministic embedded assets.
- Packaged Playwright: unlock, session reuse/forget, real trends, five sections, keyboard workload navigation, explicit
  unsupported collectors, and no page overflow at 390, 768 and 1440 CSS pixels. Screenshots inspected locally; no real
  machine screenshots or state files are published to the repository.

## Hosted acceptance

Required checks run in parallel on GitHub-hosted runners. The initial run completed Linux, macOS, browser and six-target
cross-build checks in roughly 1m40s–2m30s; its Windows MIME inconsistency was fixed. Final all-green evidence is recorded
after the reviewed head completes. Long-running release/install tests remain outside merge CI.

## Remaining product scope

This is a source-build foreground preview, not an installable release. Docker/Podman, OS services, hardware sensors,
native logs, signed installers/background services, and outbound Platform Demo enrollment/upload remain separate specs.
No arbitrary commands or remote-control endpoint are included. Active history has bounded incremental retention;
preserved recovery quarantines require owner cleanup and are not silently deleted.
