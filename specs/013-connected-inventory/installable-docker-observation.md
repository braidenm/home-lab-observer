# Installed process and Docker observation

Status: engineering plan accepted in ADR 025; no implementation or activation. Process-only packaged VM proof is the
first gate. The useful owner release is process-and-Docker, not another intermediate process-only server entry.

## Complete outcome and fixed scope

On Ubuntu 24.04/systemd 255/amd64, an owner explicitly enrolls a new `RICH_PROCESSES_DOCKER_V1` installation, starts it,
sees real host/process/running-and-stopped container rows in the hosted dashboard, and shares read-only access with
another user. Names and image references are sensitive and visible to authorized server viewers. No Kubernetes,
container control, log bodies, system service management or automatic daemon changes belong to this delivery.

Reuse the existing local Docker adapter, pure container model, workload/combined projection, uploader and installer
safety machinery. No dependency upgrade, general proxy, callback plugin or second telemetry stream is selected.
Existing local Windows/macOS previews stay useful; connected parity and rootless/Desktop transports need separate proof.

## Three observation roles, one installation

| Role | Authority and input | Output / forbidden authority |
| --- | --- | --- |
| Docker reader | One explicitly approved local rootful Unix socket; fixed Engine GET adapter | Typed bounded latest container file; no listener, credential, raw inspect forwarding or arbitrary request |
| Host/process collector | Existing process visibility and read-only typed Docker file | One canonical combined cycle; socket creation/Engine access and upload credentials remain denied |
| Uploader | Existing approved handoff, private ledger and scoped delivery credential | Allowlisted HTTPS only; no Engine socket, raw process acquisition or helper control channel |

The installer remains a separate operator executable, not a fourth continuously running observation worker. Audit all
new principals before enrollment and keep the helper's socket authority out of collector/uploader supplementary groups.
Initial support binds one fixed local rootful socket; refuse unsupported endpoints without widening permissions.
Do not discover `DOCKER_HOST`, contexts or SSH/TLS credentials. Do not install/restart Docker or modify daemon settings.

Reader code issues only existing version negotiation, all-container listing and non-streaming per-container stats:
tested API intersection `1.41..1.45`, 4-second cycle budget, fan-out at most 4, version body 64 KiB, list body 2 MiB and
each stats body 256 KiB. No inspect/logs/events/exec/action endpoint is added. Validate the actual daemon's advertised
minimum/maximum; outside the tested intersection is unsupported, never a guessed fallback. Include stopped containers.
CPU remains host-capacity based, `0..100`; absent/warming/reset samples and stopped live metrics remain null.

Keep only current contract fields. Health, restart count, uptime, network and block-I/O are absent, not fabricated.
A future field extension must have its own source, null/quality semantics and tests before being advertised.

## File boundary, freshness and privacy

Implement a private fixed latest/staging file pair, not an IPC server or daemon response cache. Freeze its exact closed
encoding and definition binding during implementation review. It carries only sanitized typed container fields,
section observation time/quality/count/truncation and installation/profile/policy binding. No raw daemon body, engine
ID mapping for actions, socket path, environment, labels, ports, mounts, commands or free-form failure text crosses it.

Cap the record at 128 KiB: section at most 130,560 bytes plus at most 512 bytes of closed metadata; two slots at most
256 KiB, no history. Deterministically truncate before publication if encoded size/500-row bounds require it. This is
an additional explicitly budgeted transient helper boundary, not a larger upload/ledger budget. Use no-follow dirfd
operations, exact producer/consumer ownership and modes, single-link regular files, bounded allocation, complete-write
atomic publication and stable-inode reads. A collector must not trust a mutable path or copy unchecked bytes onward.

Repeat policy/definition admission before acquisition/publication/read/retry. Require an unambiguous observation time;
the implementation must freeze a bounded age/skew rule and prove stalled/helper-restart behavior. Never timestamp old
helper rows with a new cycle to make them fresh. Missing, stale, denied or malformed input yields the appropriate fixed
unavailable/partial section, not a healthy empty list. Per-section time remains independent of the combined cycle.
Apply the existing secret-pattern/Cc/Cf/UTF-8 filters and privacy tests before file persistence and again at egress.
The helper file is sensitive and bounded even though credential-free. Diagnostics expose fixed outcomes only.

Systemd confinement must allow only the helper's pathname Unix socket while retaining private networking, no Internet
families, no capabilities/ptrace and no access to credentials or ledgers. Collector socket denial and uploader
mount/proc/network boundaries stay intact. Preserve existing role memory/task/stop limits; measure the new helper
within an explicit reviewed cap before release, rather than silently raising existing caps. GET-only behavior is
trusted code, not daemon enforcement: a compromised reader still has root-equivalent socket authority.

## Immutable approval, upload and durable state

- New server approval `RICH_PROCESSES_DOCKER_V1` enables both groups. Process-only remains processes=true,
  containers=false; numeric and legacy approval never accept combined rich bodies. Do not infer approval from JSON.
- Freeze a new collection definition covering helper identity, socket scope, file boundary, installed resources,
  principals, limits and schema. Root-owned exclusive policy/configuration binds its exact digest and resolved IDs.
  The exchange must reflect independently persisted approved profile and definition, not echo caller claims.
- Add closed upload kind `2` with exact schema/user_version `3`, `profile_kind=2`, existing application_id and lifetime
  lock. Keep numeric kind/schema `0/1` and process-only `1/2` SQL/header semantics unchanged. Reject wrong kind/version
  before SQLite can recover a hot journal; correct-kind recovery remains SQLite-owned. No migration or adoption.
- Retain 294,912-byte combined body; host 16,384, workloads 262,144 and each section 130,560. Preserve one global
  sequence/pending request, exact-byte SHA-256 retries, full-record CAS and uncertainty recovery. Ledger stays
  4096-byte pages/128-page/512-KiB DB with 2-MiB DB+journal/lock envelope. Re-prove the same bound for schema v3.
- Fresh separate enrollment/state only. Never copy credentials, grants, sequences, pending bytes or a used ledger.
  Successful Docker-rich installation requires no existing rich root; an existing numeric installation may coexist.
  An existing process-only rich root must cause pre-mutation refusal with its files/state byte-for-byte unchanged.
  Do not create another rich state root or claim simultaneous rich installations. Replacement remains a separate
  future operation requiring stop/join/fence plus confirmed revocation and bounded retained-state handling under ADR 024.

## Hosted integration and rollout order

1. Finish exact packaged process-only VM acceptance. Keep this Docker implementation blocked until that proof passes.
2. Deploy Platform readers for the new immutable profile, DB constraint and versioned event/serializer/replay support
   before any writer emits it. Add a separate default-off Docker readiness feature; no persisted new-key event before
   readers/readiness. Record permanent event-reader floors; disabling/revoking does not undo them.
3. In one coherent Observer implementation slice, connect fixed reader, file, profile/kind/schema-v3, installer, units,
   artifact identities and runnable disposable Docker/VM fixtures. Do not repeat a chain of unused contract-only PRs.
   Split only when review size demands it, keeping activation off and one complete acceptance target.
4. Extend the existing combined receiver and nullable authorized container read model/UI. Keep registration-row locking
   across revoke/admission and atomic head/native/workload persistence; join exact matching sequences. Preserve raw-body
   digest idempotency, timestamp/skew/quality distinctions and existing owner/viewer authorization. No new read route,
   grant system or admin-only exception is needed just to show approved inventory. Gate-off semantics must distinguish
   suspended retained data from never-collected/empty; revocation deletes latest rows through the supported path.
5. After reader deployment and exact artifact acceptance, permit explicit owner-selected profile enrollment/writer,
   verified release catalog and installer. Publish checksums/provenance/SBOM for the complete four-executable bundle
   (collector, Docker reader, uploader, installer); closed candidate/promotion verification must include the new reader
   and resources. Do not reuse a three-executable manifest or merely bump an image tag.
6. Use the supported installation on the owner host only after separate authorization. Verify actual fresh hosted rows,
   ordinary delegate reads, outsider denial and revocation, then measure steady/disconnected resource use. Documentation
   must distinguish source/VM/release/owner acceptance; none is claimed by this plan.

Reuse the current dashboard's bounded tables and meaningful null/quality states. On desktop and phone verify readable
names/images, loaded-versus-total counts, partial/truncated/stale/denied/stopped/healthy-empty states and sensitive-data
notice. Observation aliases and PIDs are display data, never action targets. Spec 011 controls require independent
grants, credentials, broker and ambiguity audit; this package cannot receive start/stop/restart requests.

## Required evidence, not substituted by source tests

- Synthetic adapter/file tests: exact GET paths/methods, version intersection, redirects/oversize/timeout/fan-out,
  running/stopped/denied/empty/partial counters, text canaries, deterministic bounds, malformed/foreign/stale file,
  ownership/link/race checks, no socket/credential inheritance and missing helper. Keep tests deterministic and cheap.
- Real SQLite profile tests: kind2/schema3 exclusive provisioning; every old/new cross-kind hot-journal open refuses
  before recovery without byte changes; exact retry/ack/expiry/CAS; crash before/after commit, cancellation/uncertainty,
  maximum valid wire and independent SQL ceiling, journal peak and isolated ENOSPC. No reused database identifiers.
- Exact packaged Ubuntu VM: disposable running/stopped containers only, separately pinned synthetic image; real foreign
  process; actual helper/collector/uploader UIDs, socket/mount/syscall/network/credential denials and current invocation
  lifecycle. Never manipulate unrelated host workloads. A daemon acknowledgement is not application health.
- Fault/resource proof: daemon unavailable/restarted/slow/denied, helper stopped/stale/tampered, response loss/reboot and
  exact pending retry, no output-body/secret leakage, bounded disk/CPU/RAM/logging, complete cleanup of only owned
  fixtures. Run separate acceptance cases: (a) successful numeric-plus-Docker-rich coexistence with no prior rich root,
  preserving numeric bytes/state; (b) an existing process-only rich root causes pre-mutation refusal and remains
  byte-for-byte unchanged. Neither case proves two rich installations can coexist. Do not silently relax failed caps.
- Real backend/shared-context owner/delegate journey and desktop/phone UX review; no extra Spring context or broad
  duplicate E2E suite. Routine PR CI stays under 15 minutes; long packaged/lifecycle fixtures are explicit manual
  release gates, never described as passed because synthetic tests ran.
- Immutable exact source/artifact/provenance/SBOM and acceptance record, operator install/start/status/stop instructions,
  known partial-state behavior and safe revocation. No automatic upgrade/uninstall/crash recovery promise.

## Remaining engineering freeze points

Review exact helper frame, age/skew rule, rootful socket identity/replacement handling, principal/path layout and helper
resource cap together with executable tests. Record the new definition digest only once those inputs are fixed. These
are delegated engineering decisions, not a product blocker or permission to guess installed isolation. Do not begin
runtime implementation before the process VM gate. See [ADR 025](../../docs/adr/025-isolated-installed-docker-observation.md)
for alternatives, authority risks and primary evidence; Kubernetes and Spec 011 activation remain separate.
