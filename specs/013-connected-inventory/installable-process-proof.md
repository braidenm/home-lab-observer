# Fixed process-only package: disposable installation proof

## Outcome and scope

Deliver one complete, VM-installable `RICH_PROCESSES_V1` package, rather than more unbound libraries. This is an
intermediate proof for the useful process-and-Docker release. Do not install it on the owner's host, create an interim
production server entry, or expose a production installer. Numeric installation and remote admission remain unchanged.

This implements ADR 024, the combined envelope and profile-bound storage plan. The first owner rich release remains
process-and-Docker, with a distinct immutable profile; Docker controls remain Spec 011, not observation authority.

## Fixed roles and ownership

- Three exact rich binaries: collector, uploader, installer. No numeric worker accepts a `--profile` argument.
- Fixed rich `/etc`, `/var/lib` and `/opt` roots, exact OS principals and systemd units, and one active installation.
- Root-owned canonical collection policy binds server/connector identity, resolved principals, enabled groups and the
  exact released collection definition. Its digest binds handoff, durable state and worker admission.
- Artifact identity, network addresses and network policy generation remain separately audited operational values.
  Same-definition compatibility is necessary but is not sufficient to authorize an artifact/address refresh: this
  process-only slice implements fresh installation only and exposes no refresh witness or invocation. A new collection
  definition needs a new separately enrolled identity. No ledger/credential copying, adoption or sequence reset.
- Collector can read process metadata but has no credential, socket creation or Docker handle. Uploader sees only
  approved handoff, protected credential and bounded ledger, not host processes or a Docker endpoint.

Reuse private safe file/tool/lifecycle machinery inside existing security-owning packages. Public constructors select
only exact named profiles. Do not export caller-provided paths, resources, SQL, ownership or generic installation plans.
Do not duplicate an entire installer or weaken numeric role admission for convenience.

## Collection and handoff

Collect at most 200 returned processes after at most 4096 metadata attempts. Use a ten-second cancellable collection
cycle and the existing bounded gopsutil provider. Preserve unavailable, denied, partial and truncated states. CPU is a
lifetime average relative to one core and can exceed 100%; it is not instantaneous host-capacity CPU.

Compose numeric host and workload documents from one snapshot/cycle. The installed collector does not assert complete
host-filesystem coverage merely because a namespace exposes some mounts. Raw account, command arguments, environment,
paths and unrestricted logs are not part of this profile. Names are sensitive, not anonymous.
Private-network loopback counters are not host traffic: installed network acquisition is disabled, rather than
publishing apparently healthy host totals from the collector's private namespace.

The rich handoff is a canonical `observer-connected-rich-handoff/v1` record with exact profile, server, installed policy
digest and embedded combined observation object. Its maximum is 288 KiB plus 512 bytes; latest and staging files together
cannot exceed twice that bound. Existing numeric handoff remains 16 KiB. Validate the complete body and exact group
claims before publishing and reading. Reader returns exact original producer body bytes, not a reserialized equivalent.

Recheck installed policy before collection, before publishing, before credential access and before every upload retry.
Policy mismatch is terminal recovery, not permission to expire/rewrite the pending body. New profile enrollment and
ledger-v2 construction are fixed rich siblings; no mutable profile selector widens numeric enrollment. The server must
echo the exact `RICH_PROCESSES_V1` profile and collection-definition digest before the exchange is accepted; a legacy
server that silently ignores those request fields produces an ambiguous exchange and cannot activate rich collection.

## Package and lifecycle acceptance

The exact bundle contains only reviewed rich members/resources and role-specific identities with the same collection
definition digest. Mixed numeric/rich roles, altered definitions/resources and unknown members fail before enrollment.
Installation leaves services stopped. Start, stop and status remain explicit; no automatic reboot/start/restart promise.

On a fresh Ubuntu 24.04/systemd255 fixture, use synthetic enrollment and a bounded local receiver to prove:

1. Numeric and rich packages coexist without modifying numeric hashes/configuration/units/ledger/status.
2. Distinct real principals and effective manager policy enforce credential, network, process and Docker isolation.
   A foreign-UID synthetic process is visible to the collector; no arguments/environment are collected.
3. A valid process body larger than 16 KiB persists in the rich ledger; response loss/restart retries identical sequence,
   bytes and digest, then acknowledgement advances state. Real SQLite crash/storage tests remain separate mandatory proof.
4. Tampered installed policy fails closed and preserves pending bytes. No Docker role, group, helper, unit or socket
   exists in this process-only bundle.
5. Bounded status/error output contains no observation body,
   secret, process name or private endpoint.

The VM is not the entire fault matrix. Focused binding/admission tests must separately prove rejection of altered
profile, digest, configuration and executable definition before secret/network access, and bundle identity/resource
mismatches before enrollment. Focused lifecycle transaction tests must prove cancellation and uncertain manager results
cannot stop a substituted/restarted invocation. Record those test results separately; do not attribute them to this VM.

Keep long installed lifecycle/fault tests outside default PR checks; run focused unit/contract boundaries and existing
cross-platform checks normally. No additional Spring context, disabled required check or production workload as fixture.

## Completion gate

Source/tests are not acceptance. Before this slice is complete, record exact reviewed revision, packaged hashes, native
fixture evidence and independent code/security review. Before owner rollout, implement and prove the separate Docker
profile, compatible receiver/read model and safe installation transition; publish immutable checksummed artifacts.
