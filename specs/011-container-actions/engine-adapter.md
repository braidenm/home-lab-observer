# Dormant fixed Linux Docker Engine adapter

Date: 2026-11-01. Status: draft for exact coordinator/independent review, not code or mutation approval.
Decision candidate: [ADR 029](../../docs/adr/029-fixed-linux-action-engine.md). The complete dormant journal must pass
all current-head CI and merge before adapter implementation. Exact document review is a separate prerequisite.
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

Only after test/source approval, create one uniquely named/labelled disposable container from a source-built static
scratch fixture image pinned to its exact inspected image ID. Use UID65532, read-only root, network none, no ports,
host mounts/devices, all capabilities dropped, no-new-privileges, PID limit64, memory32 MiB/no extra swap and CPU0.25.
Before every fixture mutation AND cleanup verify captured full ID, unique run label, exact image identity and fixed
isolation settings. A label/name alone never grants production action authority. Uncertain ownership aborts; do not
enumerate/delete by prefix or touch an existing workload. Fixture creation/removal are test-only orchestration, not
additional adapter operations. Cleanup targets only the exact verified fixture ID, even after a failed assertion.

Exercise all three independently requested operations through Worker + the real journal + the actual adapter,
including graceful stop and exact retained replay without another restart (fixture StartedAt plus call-count proof).
Test a fixture that ignores its configured stop signal to verify finite grace without a separate kill endpoint.
Prove intent/storage failure prevents dispatch and retained uncertainty never repeats a mutation. No real mutation,
native success, owner authentication or remote authorization is claimed by this draft. Root-access test invocation
against a local disposable fixture requires separate explicit approval; it is not permission to operate host workloads.

## Delivery and rollback

One coherent dormant adapter/test PR after journal delivery; no unused wire-contract chain or observation refactor.
Run focused/full Go tests/vet, Linux race, contracts and independent review. The native fixture's exact image/source,
daemon/API tuple and limits must be recorded separately from synthetic coverage; API1.45 support on tested daemons is
not proof of every API between their advertised minimum and maximum. No production default/capability changes.
Removing unused adapter code leaves journal evidence and observation untouched. Future rollback disables dispatch,
preserves unknown fences and compatible journal readers, and never replays a request to compensate for cancellation.
