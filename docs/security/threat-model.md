# Threat model

**Reviewed:** 2026-09-09  
**Scope:** First read-only observer release

## Assets

- Connector credential and one-use enrollment secret.
- Machine identity and ownership binding.
- Host, process, service, container, filesystem, network, sensor, and log observations.
- Local history, configuration, release metadata, and update state.
- Docker/native OS read handles and future action credentials.

## Implemented source-preview boundary

The source preview implements native collection, opt-in Docker reads, the numeric host store, and local API/UI.
Upload, enrollment, native log sources, service adapters, and signed installers remain future release requirements below. The local bearer token is
generated with 256 bits of entropy, restricted to the current OS account, and accepted only in the Authorization header.
The dashboard stores it in tab-scoped session storage only after validation and provides a lock/forget action. This
protects against unrelated web origins, not malicious software already running as the same OS user or an administrator.

The listener accepts explicit IPv4 loopback only. Static assets are embedded and same-origin; credentials are never
placed in startup output. Source names are rendered as text. Numeric history excludes process identities and log bodies.
The database has a single process owner, bounded maintenance, and non-destructive corruption isolation. Preserved
quarantine files are outside the active-history budget and require deliberate owner cleanup after diagnosis.

Spec 006 adds an in-process Docker adapter bound to one explicitly configured local Unix socket or Windows named pipe.
Only fixed version, inventory and one-shot stats GET requests are issued, with no proxy environment, redirects, remote
endpoint, raw daemon forwarding or Docker CLI execution. Each cycle has a four-second budget, four stats workers,
500 output rows, a two-MiB inventory response bound and a 256-KiB stats-response bound. Normalized metadata stays in
memory and is served only through the protected specialized container API; it never enters history or upload.

This is an application-level read-only boundary, not a reduction in OS socket authority. A compromised observer process
with direct Docker access could exercise the authority of that socket. A separate constrained proxy/worker is a required
future container-package defense, not a protection claimed by this native foreground preview. Owners should not enable
Docker access unless they accept that privilege boundary. Windows/macOS live-engine validation remains outstanding.

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

## Spec 009 planned helper boundary

The optional Linux journal helper is an executable bundled with the same release, not a generic plugin. It is resolved
only beside the running observer and checked against a build-embedded digest and closed protocol/version. Both its
private request and normalized output are bounded; the environment excludes loader injection before exec. The main
observer must run without the helper's dynamic libraries. Native stalls/crashes/malformed output become unavailable
log collection, while the parent enforces deadlines and reaps children. No shell, arbitrary source, cursor in argv,
message-body acquisition, library installation or privilege change is authorized by enabling this capability.

The same-user helper separates failures, not hostile same-account authority. Native libraries and the host account
remain part of the trusted computing base. Artifact integrity, filesystem identity, selected-field acquisition,
permission-limited views, missing-loader fallback, private-checkpoint canaries and child-reaping tests are required
before shipping. This section describes accepted controls under implementation, not protections already in preview 2.
See [ADR 010](../adr/010-optional-linux-journal-helper.md).

Independent review additionally requires Linux helper core-dump suppression before private input/journal access:
set process-local non-dumpability and a zero core-size limit, failing closed if unavailable. This prevents ordinary
helper crashes from creating an extra unbounded private-data dump. It does not change host-wide crash-reporting policy
or protect memory from the host administrator. Verify the process-local policy in native tests.

## Explicit exclusions

The first release has no arbitrary shell, arbitrary file reader, arbitrary log path, arbitrary Docker operation, generic plugin execution, LAN listener, or remote configuration that can broaden local collection. Adding one requires a new threat model and ADR.

## Verification

Security acceptance includes secret-canary propagation tests, role/ACL tests, listener inventory, Host/Origin abuse tests, request and storage exhaustion tests, corrupt-state recovery, revoked/replayed enrollment, signed-artifact verification, and public-workflow runner-policy checks.
