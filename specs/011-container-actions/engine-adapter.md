# Dormant fixed Linux Docker Engine adapter

Date: 2026-11-01. Status: accepted for unused Linux adapter implementation only after exact coordinator/independent review.
Decision: [ADR 029](../../docs/adr/029-fixed-linux-action-engine.md). The dormant journal's current-head checks and merge
prerequisite are complete. Actual native-fixture execution still requires reviewed source and separate approval.
The full broker, credentials, grants, authenticated recovery, OS isolation, installation and activation remain gated.

## Closed surface and authority

Propose unused `internal/actionengine`, implementing only the existing `actioncore.Engine` Inspect, Start, Stop and
Restart methods. Add a fixed rootful-Linux `Open(ctx, approvedWitness)` and resource Close; no command, listener,
environment/context discovery, exported arbitrary endpoint/client/Doer, generic request method or dependency.
Public Open uses only `/engine/docker.sock` in a future separately approved private worker namespace. Require an
explicit trusted approval of device/inode/nonzero GID, a root-owned 0660 single-link socket, no access ACL, no symlink
and trusted root-controlled ancestry. Do not change/adopt/repair permissions or choose another endpoint on failure.
Recheck the exact approved witness before every request and dial; a replacement is unavailable, not auto-approved.

The witness detects node drift, not daemon identity or permission. EngineGeneration remains an independently
owner-approved private binding in core/journal metadata; never derive it from a socket inode, PID or version string.
Same-root hostile replacement is not an isolation guarantee. Socket access remains root-equivalent broad authority;
typed HTTP does not install a daemon authorization plugin or approve the future worker's OS confinement.
Close rejects new operations and closes owned transport resources; cancellation is not rollback. An installed
stop/join/upgrade lifecycle and unknown-outcome recovery are not supplied by this adapter.

## Fixed HTTP contract

- Initial bounded `GET /version` selects exact case-sensitive ApiVersion, MinAPIVersion and Os. Require linux,
  canonical bounded major/minor values, minimum <= maximum and **minimum <= 1.45 <= maximum**. The only issued
  version is **1.45**. Do not negotiate a higher version, fall back after refusal or read DOCKER_API_VERSION.
  Future higher-minimum daemons stay unsupported until a separately reviewed/tested adapter revision.
- IDs are exactly 64 lowercase hexadecimal bytes, validated before socket I/O; names, short IDs, aliases and escaping
  are not accepted. `GET /v1.45/containers/{id}/json` selects only exact Id and State.Status, requires byte-exact Id
  equality and returns only the core's closed states. Missing/unknown states cannot authorize a mutation; a missing
  target returns UnknownState, never an inferred replacement. Malformed identity/JSON is a finite unavailable error.
- Unknown extra response properties are tolerated, matching Docker's open schema. Critical keys are case-sensitive,
  unique and required; reject duplicate selected keys, wrong types, trailing JSON and excessive bodies. Do not rely
  on encoding/json's case-insensitive struct-field matching after a separate duplicate check. Raw inspect may include
  environment/arguments/paths: it exists only in bounded transient parsing and is never returned, logged or persisted.
- Start is exactly `POST /v1.45/containers/{id}/start`, no body/query. Stop/restart use their distinct paths and only
  integer query `t=1..30`, supplied from the already approved binding. No custom signal, headers, body, attach,
  checkpoint, update, create, exec, kill, remove or daemon control. The configured stop signal remains effective;
  Docker may force termination after grace. The adapter does not strengthen the non-atomic state check in ADR 027.
- Only HTTP 204 is a mutation acknowledgement. A 304/state race, other status, malformed response, cancellation,
  disconnect or lost acknowledgement returns a fixed error. After persisted intent core reports OUTCOME_UNKNOWN,
  not automatic retry or inferred success. Acknowledgement is not application health or enduring container state.

Own a dedicated HTTP/1 transport: Unix-only dial to the approved node, literal internal HTTP host, Proxy nil,
redirects refused, compression and keep-alives disabled, no cookies, inherited headers, idempotency headers or GetBody.
No explicit retry loop. Fresh HTTP/1 connections avoid Go's reused-connection retry branch, including zero-write POST
retries. Test this property on the actual pinned Go transport, not a fake Doer. No caller transport override.

Proposed code-owned bounds: 2-second dial; 4-second version/inspect including body; 45-second mutation including
response, always further capped by caller context/core budget; 16 KiB response headers, 64 KiB version and 256 KiB
inspect bodies (read at most cap+1). Oversized otherwise legitimate inspect is unavailable, not partially trusted.
Do not parse/return daemon error messages or drain unbounded bodies for connection reuse. Unsupported OS returns a
finite unsupported result before socket I/O. No rootless, Windows or macOS control parity is claimed.

## Tests before any real daemon mutation

Ordinary-user fake Unix HTTP tests cover every method/path/query/body, full-ID/grace rejection before dial, version
intersection and future-min refusal, all core states, exact/case-sensitive ID and key validation, unknown properties,
secret canaries, oversize/header bounds, cancellation/deadlines, proxy/environment poisoning, redirects, malformed
responses, socket drift and lost-response exactly-one-POST behavior. No production validation bypass flag is allowed.

A private fixture endpoint adaptation may select a temporary test-owned Unix path and captured owner expectations;
the public constructor independently enforces the unchanged fixed rootful path/approval checks. Both use the same
socket-witness checks and HTTP/operation/decoder code. This is not a public arbitrary-path or forTest API. Synthetic
owner/ancestry adaptation is not acceptance of the rootful production namespace; public rejection tests remain required.
The later native Desktop fixture uses its explicitly approved actual rootful socket metadata, canonicalizing only
its known test namespace alias, never relaxing public ancestry rules or changing daemon socket/group permissions.

Only after test/source approval, create **one fixture at a time**, with at most two sequential source-built modes:
default graceful signal handling, then ignored SIGTERM. A single immutable container need not expose a mode-switch
control surface. Each has a separate uniquely named/labelled full ID; verify and clean up the first before creating
the second, never a concurrent fleet. Pin the static scratch fixture image to its exact inspected image ID.
Use UID65532, read-only root, network none, no ports,
host mounts/devices, all capabilities dropped, no-new-privileges, PID limit64, memory32 MiB/no extra swap and CPU0.25.
Before every fixture mutation AND cleanup verify captured full ID, unique run label, exact image identity and fixed
isolation settings. A label/name alone never grants production action authority. Uncertain ownership aborts; do not
enumerate/delete by prefix or touch an existing workload. Fixture creation/removal are test-only orchestration, not
additional adapter operations. Cleanup targets only the exact verified fixture ID, even after a failed assertion.

Exercise all three independently requested operations through Worker + the real journal + the actual adapter,
including graceful stop and exact retained replay without another restart (fixture StartedAt plus call-count proof).
The second fixed mode ignores its configured stop signal to verify finite grace without a separate kill endpoint.
Prove intent/storage failure prevents dispatch and retained uncertainty never repeats a mutation. No real mutation,
native success, owner authentication or remote authorization is claimed by accepting this contract. Root-access test invocation
against a local disposable fixture requires separate explicit approval; it is not permission to operate host workloads.

## Delivery and rollback

The manual entry is `node scripts/test-action-engine-native.mjs --allow-owned-fixtures`, only after exact source/test
review and separate execution approval. Preconditions: a clean reviewed checkout, Node22, the existing Go1.27.3
toolchain/dependency cache (offline), and an approved local Linux/amd64 daemon. No download, host package install or
socket/group change is performed. Windows/x64 uses the fixed Docker Desktop Linux named pipe and explicit
WSL Ubuntu root bridge to the same Linux test binary; other platforms refuse before mutation. The CLI gets a fresh
empty private Docker config and scrubbed environment, with an explicit local endpoint, not inherited credentials,
contexts, TLS or proxies. Before creation, compare CLI and Linux-adapter version/API/minimum/OS/architecture tuples.

The checked-in scratch-only build has no RUN/base pull/network. Verify exact image/full ID/run label and all fixed
caps before operations and cleanup; uncertain identity retains the fixture and refuses further cases. Real journal
cooldowns remain in force: the bounded manual run waits between independent requests rather than changing its clock.
The opt-in Go native entry is intentionally skipped in ordinary CI; that skip is not native proof. Cheap Node policy
tests run in the existing contracts task. Linux audit stages use fixed persistent `/var/tmp`, not session-scoped `/tmp`;
Windows retains its ordinary temporary directory. Each fresh stage is privately owned 0700 and its bounded synthetic
basename is emitted before daemon execution for recovery. Before CLI create, exclusively write and synchronize a
bounded private 0600 `intent-<mode>.json` containing source, run ID, image ID, mode and the exact derived fixture name;
an audit failure prevents creation. This intent does not claim that creation succeeded; it permits exact-name recovery
after a lost acknowledgement without enumeration. Immediately after a valid full create ID, before inspecting or
operating on it, exclusively write a bounded private 0600 `created-<mode>.json` containing source, run ID, image ID,
mode and captured full ID. Synchronize the file and, on Linux, its parent directory. This receipt records captured identity
only: it does not certify ownership, successful actions, removal or acceptance. Failure retains the stage and never
authorizes cleanup by name/prefix. The final bounded result record (source,
version, image/binary hashes, synthetic run ID and two exact fixture IDs with removal acknowledgements) are retained;
acknowledgement is not verified absence, which requires separate read-only exact-ID checks. No image prune or broad
cleanup is attempted. Output is closed markers,
never raw Docker/compiler/inspect/error content. This source contains no successful native-run or release claim.

One coherent dormant adapter/test PR after journal delivery; no unused wire-contract chain or observation refactor.
Run focused/full Go tests/vet, Linux race, contracts and independent review. The native fixture's exact image/source,
daemon/API tuple and limits must be recorded separately from synthetic coverage; API1.45 support on tested daemons is
not proof of every API between their advertised minimum and maximum. No production default/capability changes.
Removing unused adapter code leaves journal evidence and observation untouched. Future rollback disables dispatch,
preserves unknown fences and compatible journal readers, and never replays a request to compensate for cancellation.

## Local implementation evidence (2026-11-01)

Accepted specs preceded code. The initial fixed-Unix contract tests failed without the adapter, then passed for
exact routes/options, full IDs, version intersection, selected case-sensitive fields, malformed/oversized bodies,
UTF8, witness drift and finite errors. Actual pinned-transport Unix faults prove one POST after lost response, no
redirect/proxy fallback and cancellation/Close joining. Body-read cancellation is synchronized after handler flush,
not a short deadline that assumes the server was scheduled. Worker + the real journal + Unix HTTP retain both known
and unknown outcomes across reopening without another inspect/mutation; unavailable storage performs no Engine I/O.

Full Linux race plus final focused reruns/vet, complete Windows tests/vet, existing contracts (93 including thirteen new fixture policy tests),
Foundation43 and diff checks pass locally. Ordinary Linux Engine tests have one intentional native opt-in skip;
that skip is not actual Docker, rootful namespace, graceful/forced-exit or runtime compatibility proof. The separately
approved supervised native execution below supplies the bounded disposable-fixture evidence, not ordinary CI.

Two supervised preflights refused before image build/container creation or native test execution: missing offline
module metadata, then Windows path conversion through WSL's default shell. Cache preparation restored clean tracked
sums and offline verification/builds. The narrow bridge correction uses `wsl --exec` for both wslpath and the fixed
env/native command, preserving argument boundaries/backslashes rather than invoking the default Linux shell. Cheap
argument regressions cover Windows paths with spaces; these preflight corrections are not native Docker proof.
See [Microsoft WSL commands](https://learn.microsoft.com/en-us/windows/wsl/basic-commands) and current `wsl --help`.

Further supervised preflights correctly refused a Windows/Ubuntu daemon mismatch, then a Linux CLI without Buildx;
neither reached container creation. The Linux-only optional test prerequisite is now fixed Buildx **v0.38.1**,
official asset `buildx-v0.38.1.linux-amd64`, 68,366,498 bytes,
SHA256 `e25d5cd675092bb5d1d03632fdc46cdf3fede7b514dc5f023a105e00d3ce5ad4`.
The manual entry may add `--buildx-source <absolute-private-file>` only on Linux. Preparation/download is outside the
harness: require root-owned private 0700 parent and regular, nonsymlink, single-link 0700 file of exact size/hash;
copy exclusively to the fresh config's `cli-plugins/docker-buildx`, recheck the digest before CLI use and verify
the exact version read-only. No URL/hash/plugin-name overrides, runtime downloads, global installation, driver
creation/bootstrap or legacy-builder fallback. Keep `{}` config, fixed local daemon and `buildx build --builder default`
with no-pull/no-network scratch-only input. Record only the fixed version/hash in the bounded private receipt.
Before building, read only `buildx inspect default` without bootstrap and require exactly one running Docker-driver
node; its endpoint must be the fixed URI or the default alias whose separate default-context host equals that URI.
Reject other drivers, extra nodes, errors or malformed metadata, never create a builder or fall back.
Windows rejects this extra option; its existing installed-plugin behavior is unchanged and not proven by Linux tests.
Primary evidence: [CLI29.1.3 plugin lookup](https://raw.githubusercontent.com/docker/cli/v29.1.3/cli-plugins/manager/manager.go),
[Buildx0.38.1 testing-only manual download](https://raw.githubusercontent.com/docker/buildx/v0.38.1/README.md),
[official release](https://github.com/docker/buildx/releases/tag/v0.38.1) and
[Docker driver](https://docs.docker.com/build/builders/drivers/docker/). This driver uses the daemon's embedded BuildKit,
not an additional builder container. Compatibility with the tested daemon still requires actual source-reviewed proof.

A subsequent supervised Linux attempt built the synthetic image and created its first stopped fixture, then refused
ownership before native Start/Stop/Restart. Exact-ID read-only recovery confirmed the sole mismatch: create requested
`no-new-privileges=true` while both strict Node and Go ownership predicates expected `no-new-privileges:true`.
The correction requests the exact colon spelling; neither validator accepts both spellings or relaxes any isolation
check. The first attempt's success-only receipt was insufficient for recovery and its temporary stage was unavailable
from the later session; the persistent stage and immediate captured-identity receipt above close that audit gap.
Retained-fixture recovery/cleanup is separate from a future reviewed rerun; no successful native proof is implied.
Regression requirements: exact create/inspect spelling agreement, equal-spelling rejection by unchanged ownership
checks, platform-specific stage parent, bounded closed intent/created-receipt fields, exclusive private writes before
creation/first inspection respectively, and continued identity refusal/retention. No production Engine or Go ownership
change is needed. Separately approved exact-ID recovery subsequently removed the retained stopped fixture without
force, followed by an independent exact-ID absence check; this is cleanup evidence, not native action proof.
The focused harness regressions first failed on the mismatched create spelling/missing audit behavior, then passed
16/16 with zero skips; all 96 Node contract tests and canonical contract validation pass after the correction.
No production Go or strict ownership-predicate bytes changed in that harness correction.

## Supervised disposable Linux proof

The reviewed source `739ee6172636e8bb6860c437e4c914fd89242242` completed the separately approved manual Linux
harness with exit0 and `ACTION_ENGINE_NATIVE_PASS cases=2 scope=DISPOSABLE_PROTOCOL_ONLY`. The bounded private receipt
binds that source, the built image/test binaries, both captured full IDs and removal acknowledgements. Only synthetic
fixture identities are held privately; no private paths, recovered identifiers or daemon response bodies are published.

Actual compatibility evidence is **Docker29.1.3, linux/amd64, advertised API maximum1.52/minimum1.44, issued API1.45
only**, with the pinned private Buildx0.38.1 prerequisite above. This is not proof of every advertised API version or
another Docker implementation. The native coordinator + real journal + fixed adapter passed distinct start, restart
and graceful stop in the first immutable fixture, then start and ignored-SIGTERM forced stop with the one-second grace
in the second. Real journal cooldowns remained enabled. Retained replay did not restart again. Only one fixture existed
at a time, with the reviewed image/full-ID/run-label and isolation-cap checks before every operation and removal.

Both removals were acknowledged, and independent read-only checks of those two exact IDs confirmed absence against
the same freshly verified daemon tuple. No existing workload was operated on; fixture images and private audit stages
were not broadly pruned. This proves the scoped disposable protocol path, not an installed worker namespace, owner
authentication, remote grants/revocation, action-credential isolation, recovery authority, Windows/macOS support or a
release/canary acceptance gate. The adapter remains unused by any installed role. Fresh current-head PR CI, review and
normal governed delivery are still required before merging this dormant source slice; no observation capability changes.
