# Spec 006: Read-only Docker container observations

**Status:** Accepted for implementation under the owner's approved continuation plan

## Outcome and boundary

An owner explicitly enables a local Docker connection and sees running and stopped containers in Workloads, with
names/images, state, and CPU/memory readings where available. Unavailable readings remain null with a reason, not zero.
This slice excludes Podman-specific compatibility, service managers, logs, sensors, installers, remote upload and actions.
The broad collector roadmap is delivered as separate focused specifications rather than one large change.

## Requirements

- R1: Disabled by default. `observer serve --docker-endpoint ENDPOINT` opts in to one explicit local Unix socket or
  Windows named pipe. No environment/context discovery, TCP, SSH, remote pipe, daemon mutation or privilege escalation.
- R2: A transport-isolated Docker reader issues only GET version, all-container inventory and bounded one-shot stats.
  Negotiate an explicitly supported Engine API range; incompatible engines produce UNSUPPORTED. Do not claim Podman or
  Windows-container stats support without evidence. Never forward the socket/API through HTTP.
- R3: Bound each cycle to four seconds, inventory to 500 records/two MiB, stats bodies to 256 KiB each, and stats fan-out
  to four requests. Cancellation must close work. Failed per-container readings retain inventory with null metrics and
  stable quality reasons. Host collection and the local dashboard remain available on engine failure.
- R4: Publish `GET /api/v1/containers?limit=1..500` (default 100), a dedicated `observer-container-inventory/v1` contract.
  Keep the original snapshot/capabilities schemas unchanged; their container section describes the legacy snapshot only.
  The new view has its own support/freshness/collection state, counts, truncation and effective data policy.
- R5: Inventory includes stopped containers; metrics for stopped/non-running records are unavailable, not fabricated.
  CPU is percent of engine-host capacity using valid counter deltas, memory is bytes of reported usage. Zero is valid
  only when measured. Missing/reset/invalid counters remain null. No identifiers enter numeric history.
- R6: Only code-owned fields survive normalization: hashed container alias, bounded sanitized name/image, allowlisted
  state, nullable metrics, and quality code. No commands, environment, mounts, labels, ports, raw IDs or daemon errors
  are returned, persisted or logged. Container metadata is local-sensitive and not upload-eligible.
- R7: The endpoint uses existing bearer, Host/Origin, no-CORS, method, timeout and response-size protections. The dashboard
  exposes configuration instructions when disabled, honest failure/empty/truncated states, and mobile/keyboard usability.
- R8: An optional data-source method supplies the new view without breaking existing consumers or the synthetic demo.
  Headless operation needs no Docker CLI. Native Unix/Windows transports have tests; real Linux Docker smoke in CI runs
  with a synthetic container and verifies a read-only daemon interaction. Required CI remains parallel and under 15 min.

## Acceptance

Test endpoint rejection, allowlisted HTTP paths/methods, version incompatibility, absent/denied engines, malformed and
oversized bodies, redirects, secret-canary filtering, timeouts, fan-out limits, stats errors/reset, disabled/empty states,
running/stopped records, stable aliases, response limits, schema validation, CLI options and UI states. Review code and
responsive UI independently before auto-merge. Native Windows/macOS transport tests do not imply live-engine certification.
