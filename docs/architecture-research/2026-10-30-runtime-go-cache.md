# Give runtime its own Go build-cache key

Status: local contracts and independent review pass; exact-head CI and observed cache save/restore pending.
Access date: 2026-10-30. Owning plan: [Spec 013](../../specs/013-connected-inventory/plan.md).

## Evidence and limits

[PR 146](https://github.com/braidenm/home-lab-observer/pull/146) passed all 17 checks across nine workflows after removing
the duplicate normal Linux suite: runtime Linux 7m28s, Windows 2m39s and macOS 1m03s. Every Linux package still runs
with race detection, handoff stress still repeats 25 times, and vet/schema/native/background/Docker checks remain.
The earlier cancelled job is not counted as green. Keep the existing eight-minute runtime cap.

The earlier Linux setup reported an exact cache-key hit and about 19 MB restored, followed by 1m44s normal tests and
5m01s race tests. This is consistent with a partial build cache, not proof of its contents or which job created it.
The cache-creator hypothesis does not justify deleting caches, retrying CI, changing runners or skipping tests.

The exact pinned `actions/setup-go` revision is `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e`:

- [Default paths](https://github.com/actions/setup-go/blob/b7ad1dad31e06c5925ef5d2fc7ad053ef454303e/src/package-managers.ts)
  select `go.mod` for the dependency hash and cache both `GOMODCACHE` and `GOCACHE`; the default is not `go.sum`.
- [Restore implementation](https://github.com/actions/setup-go/blob/b7ad1dad31e06c5925ef5d2fc7ad053ef454303e/src/cache-restore.ts)
  keys by operating system, architecture, Linux image, resolved Go version and input-file hash, not workflow/job identity.
- [Save implementation](https://github.com/actions/setup-go/blob/b7ad1dad31e06c5925ef5d2fc7ad053ef454303e/src/cache-save.ts)
  returns without saving after an exact primary-key hit. Its
  [post action](https://github.com/actions/setup-go/blob/b7ad1dad31e06c5925ef5d2fc7ad053ef454303e/action.yml) runs only on success.
  [GitHub documents immutable cache contents](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching).

At baseline `4528c9c3b9a4a96639cfb17fa233ab411005669b`, all 18 Go workflows omit an explicit Go cache dependency path,
including lightweight compilation fixtures.
Thus jobs on the same cache scope/platform/toolchain can share a key, and a lightweight first successful writer can
leave later full jobs repeatedly restoring a small cache that they cannot enrich. This mechanism is confirmed by
configuration and action source; the actual reported cache's history remains unproved.

## Small correction and acceptance

Set only runtime's existing `cache-dependency-path` to exactly these three checked-in files:

1. `go.mod` retains direct/transitive module-version invalidation.
2. `go.sum` also binds checksum changes.
3. `.github/workflows/runtime.yml` separates this full workload from lightweight jobs and invalidates on build-plan changes.

The action's [pinned guidance](https://github.com/actions/setup-go/blob/b7ad1dad31e06c5925ef5d2fc7ad053ef454303e/docs/advanced-usage.md#parallel-builds)
supports multiline paths and build-setting inputs, and recommends distinct keys for parallel builds. Do not hash all
Go sources, add per-run keys, custom save/restore actions, cache eviction or broad workflow edits for this slice.
Those options add churn/complexity without being required to isolate runtime. Source changes remain Go's content-based
build-cache concern; unchanged immutable entries will not acquire later incremental outputs until the key changes.

The cheap contract must first fail on the old configuration, then pass for the exact three-file key. Cover omitted,
extra, duplicate and wrong inputs, and ensure no other Go setup block claims runtime's workflow salt. Existing race-tag,
full-suite/stress, job/matrix/deadline and schema/smoke guards remain. No dependency or runtime behavior changes.

Fresh exact-head CI must pass before merge. A first successful cold run must populate this new key; then observe the
later restore/save messages and durations through approved CI diagnostics. Salt isolation does not promise the first
run becomes faster or that caches are a correctness dependency. No public Docker release or packaged VM acceptance is
established by this CI adjustment.
