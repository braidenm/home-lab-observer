# Spec 005 review and acceptance evidence

## Independent cross-slice review

The implementation was split across runtime/storage, HTTP, and embedded UI worktrees. Reviewers then inspected slices
they did not author; the integration owner ran real binary and browser tests and incorporated the findings. No reviewer
approval substitutes for required CI. PR 8 is eligible for auto-merge only after all final hosted checks pass.

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
| Readiness stayed healthy with failed/stale collection, while fresh observer signals were marked stale | Latest-attempt state and per-section freshness; failure/recovery/selection regression tests |
| Collector/sequence failure paused retention | Independent bounded maintenance on failure paths; scheduler regression tests |
| Busy checkpoint looked successful; transient degradation stayed latched | Real held-reader checkpoint test, operation-specific recovery, preserved failure counters |
| Same-version incompatible database escaped recovery | Canonical affinity/nullability/PK/layout/vacuum validation and quarantine regressions |
| Client accepted invalid states and sensitive extra fields | Closed nested decoders, valid fixture compatibility, log union and unsupported-data regression tests |
| One chart observation was invisible and gaps counted as samples | Isolated point marker, honest observed-point count, component regression |

Final independent cross-review found no remaining release blocker in the source-preview scope. Its non-blocking client
truncation compatibility note was also corrected and tested: a source may report known truncation without knowing a
larger total. The integration owner verified the final diff, schema scenarios, and packaged UI.

## Local verification

- Full Go unit/integration suite and vet on Windows, including real native process availability.
- Root OpenAPI/JSON Schema/fixture tests and actual native snapshot validation.
- Actual service smoke: protected API, valid selected/degraded response envelopes, real metric history after successive
  collection sequences, no credential in API/process output, and graceful process/store reuse tests.
- Embedded dashboard: TypeScript, 21 component/adapter tests, reusable-library/demo builds, deterministic embedded assets.
- Packaged Playwright: unlock, session reuse/forget, real trends, five sections, keyboard workload navigation, explicit
  unsupported collectors, and no page overflow at 390, 768 and 1440 CSS pixels. Screenshots inspected locally; no real
  machine screenshots or state files are published to the repository.

## Hosted acceptance

Required checks run in parallel on GitHub-hosted runners. After correcting the initial Windows MIME inconsistency, all
seven checks passed with the slowest at 2m11s. The exact reviewed-head result remains auditable in
[PR 8's required checks](https://github.com/braidenm/home-lab-observer/pull/8/checks), which gate auto-merge.
Long-running release/install tests remain outside merge CI.

## Remaining product scope

This is a source-build foreground preview, not an installable release. Docker/Podman, OS services, hardware sensors,
native logs, signed installers/background services, and outbound Platform Demo enrollment/upload remain separate specs.
No arbitrary commands or remote-control endpoint are included. Active history has bounded incremental retention;
preserved recovery quarantines require owner cleanup and are not silently deleted.
