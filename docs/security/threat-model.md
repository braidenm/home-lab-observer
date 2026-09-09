# Threat model

**Reviewed:** 2026-09-09  
**Scope:** First read-only observer release

## Assets

- Connector credential and one-use enrollment secret.
- Machine identity and ownership binding.
- Host, process, service, container, filesystem, network, sensor, and log observations.
- Local history, configuration, release metadata, and update state.
- Docker/native OS read handles and future action credentials.

## Trust boundaries

```text
machine sources -> collector -> sanitizer/store -> local API/UI
                                      |
                                      -> uploader -> TLS -> Platform API

future actions use separate credentials, process, IPC, and allowlists
```

- Machine sources are untrusted input even when local; names and log bodies may contain hostile markup or secret-looking data.
- The collector has machine-read authority but no Platform credential.
- The uploader has a scoped connector credential and outbound network but no machine-source handles.
- The local web role has sanitized read access only and binds loopback.
- Platform Demo authenticates owners/delegates independently and cannot remotely enable sensitive local collection.
- Release workflows are privileged publishers; public pull-request workflows are untrusted.

## Primary threats and controls

| Threat | Required controls |
| --- | --- |
| Credential disclosure | Private prompt/token file, scoped exchange, OS ACLs, structured-log filtering, no URL/argv/environment transport |
| Sensitive telemetry disclosure | Data minimization, explicit source/body opt-in, pre-storage redaction, bounded schemas, local/remote policy separation |
| Local API reached through browser attacks | Explicit loopback bind, bearer session, Host/Origin validation, no CORS, CSP, anti-framing, no mutation endpoints |
| Docker/OS authority escalation | Separate worker and account, constrained proxy/IPC, fixed operations, no raw socket/API forwarding |
| Resource exhaustion | Request/query/body limits, collector deadlines, bounded queues/history/WAL, downsampling, per-source rate/drop accounting |
| Malicious source names or logs | Normalize length/encoding, render as text, no HTML interpretation, fixed structured-field allowlist |
| Replay or cross-owner upload | One-use enrollment, tenant/resource-bound renewable credential, monotonic sequence/idempotency, server revocation |
| Supply-chain compromise | Clean public history, pinned actions/toolchains, no PR secrets/self-hosted runners, checksums, SBOM, attestations, protected immutable releases |
| Unsafe update/rollback | Manual default, signed manifest, final-byte verification, side-by-side versions, health-confirmed switch, compatible state/backup |
| Capability confusion | Explicit supported/degraded/disabled/denied states and freshness; never map missing data to zero/healthy |

## Explicit exclusions

The first release has no arbitrary shell, arbitrary file reader, arbitrary log path, arbitrary Docker operation, generic plugin execution, LAN listener, or remote configuration that can broaden local collection. Adding one requires a new threat model and ADR.

## Verification

Security acceptance includes secret-canary propagation tests, role/ACL tests, listener inventory, Host/Origin abuse tests, request and storage exhaustion tests, corrupt-state recovery, revoked/replayed enrollment, signed-artifact verification, and public-workflow runner-policy checks.
