# F1: installed boundary acceptance findings

Status: research and acceptance checklist, 2026-09-11. No installed-profile approval, networking selection,
installer or service activation. Complements [ADR 014](../../docs/adr/014-linux-connected-canary-workers.md).

## Decisions still required

**Uploader networking:** RootDirectory removes filesystem paths, not host TCP loopback access. The fixed HTTP
origin limits normal application behavior; it does not isolate a compromised uploader. Unit design must select
and prove enforcement that permits required DNS/HTTPS while denying the local observer API and unauthorized
fixture sinks. Do not select a networking framework in this slice. Resolve DNS reachability in that design:
a host loopback resolver cannot simply be assumed reachable from another network namespace.
Actual destination discovery, allowlist refresh and the real-host enforcement gate remain unresolved.

**Collector filesystem coverage:** pinned gopsutil first reads `/proc/1/mountinfo`, then falls back to
`/proc/self/mountinfo`; usage calls operate in the caller's namespace. Successful collection alone cannot prove
host coverage. Compare eligible mount identities and capacities against an owned reference fixture, including
hidden/inaccessible mounts, bind mounts and truncation. Installation evidence must precede declaring coverage
verified; no owner checkbox substitutes for that proof. Otherwise retain unavailable/degraded remote output.

## Minimal uploader root and runtime checks

- Verify the exact static executable; supply no shell, package manager or broad host directory mounts.
- Provide a reviewed read-only CA bundle, resolver configuration and minimal hosts data. Go's Linux trust
  search includes `/etc/ssl/certs/ca-certificates.crt`; test actual certificate verification inside the root.
- Permit only reviewed runtime environment inputs; do not inherit trust-store, resolver or proxy overrides.
- Bind the handoff read-only, keep ledger/diagnostics private, and provide only the service's credential.
- Test Go startup, TLS entropy and SQLite under the final syscall policy without exposing host procfs merely
  for optional runtime cgroup discovery. Validate the packaged build, not a dynamically linked test substitute.
- Inspect the final mounted view. In systemd 255, several hardening options imply `MountAPIVFS`, which can
  introduce `/proc`, `/sys` and `/dev` into a filesystem root. `ProtectProc` hides other-user processes but is not
  equivalent to absent procfs. `ProcSubset=pid` hides numeric procfs inputs needed by the collector.
- `LoadCredential` exposes a private read-only service credential location; verify that interaction with the
  root directly. Address-family restrictions do not revoke inherited descriptors. IP controls require usable
  platform support: configuration text alone is not enforcement evidence.

## Acceptance checks on exact installed units

1. Distinct numeric principals: collector cannot read uploader credential/ledger or process environment,
   memory or descriptors; unrelated third user cannot access either private state.
2. Uploader reads canonical handoff but cannot create, remove or replace its entries. ACL/group/owner drift
   fails startup without repair. Host-secret fixtures, Docker sockets and collector history remain inaccessible.
3. Collector socket probes fail and fixture sinks receive nothing. Uploader's authorized HTTPS succeeds while
   local-API and unauthorized-sink probes fail. Inspect inherited descriptors as well as new socket attempts.
4. Missing CA, wrong hostname, expired certificate, DNS failure and redirect never produce an insecure fallback.
5. Prove filesystem coverage or publish incomplete/unavailable data. Mount changes invalidate unsupported claims.
6. Stop/cancellation joins workers; restart, expiry, revocation, response loss and storage pressure preserve
   sequence/pending-byte invariants. Verify bounded diagnostics and explicit rollback on the packaged artifact.

## Evidence versus remaining work

| Evidence as of 2026-09-11 | Status and limit |
| --- | --- |
| Read-only Ubuntu WSL preflight | systemd 255.4 is PID 1, running; cgroup-v2 controllers available. Not enforcement proof. |
| D1 synthetic Linux ledger tests | Executed, including process death/hot-journal fixtures. No power-loss claim. |
| Namespace-isolated 1 MiB tmpfs pressure fixture | Executed separately; actual ENOSPC behavior only, not persistent-device sync. |
| WSL 255 cgroup IPv4/IPv6 filtering | Owned transient-unit probes passed: baseline connection, deny-all timeout, then precise loopback exception restoring connection. Not full destination policy. |
| Exact installed unit UID/mount/credential/network/TLS probes | Not executed. WSL is a feasible first test environment only when each capability is exercised. |
| Physical-host mount coverage, supported ARM64 platform, reboot and VM power loss | Not executed; separate installed-release gates remain open. |

WSL observations describe its Linux guest, not the Windows host. All future fixtures must use synthetic data,
explicitly owned resources, bounded cleanup and fixed result output; no raw credentials or host observations.

The filtering probes used the same owned listener for each protocol: baseline connection succeeded;
`DynamicUser` plus `IPAddressDeny=any` denied connection with a two-second timeout; the precise
`127.0.0.1/32` or `::1/128` allow exception restored connection. Transient units used `--collect`, and
listeners were closed afterward. No host application was contacted. This demonstrates working cgroup IP
filtering on the tested WSL systemd 255 environment despite its `-BPF_FRAMEWORK` build label; that label alone
must not be treated as evidence that IP filtering is unavailable. These probes do not establish TLS, DNS,
installed-worker isolation, public-destination policy or enforcement on a different host.

## Primary sources and code evidence

Consulted 2026-09-11:

- [systemd v255 execution controls](https://github.com/systemd/systemd/blob/v255/man/systemd.exec.xml):
  RootDirectory, MountAPIVFS, ProtectProc, ProcSubset, LoadCredential and descriptor restrictions.
- [systemd v255 resource controls](https://github.com/systemd/systemd/blob/v255/man/systemd.resource-control.xml):
  IP enforcement prerequisites.
- [gopsutil v4.26.6 Linux disk adapter](https://github.com/shirou/gopsutil/blob/v4.26.6/disk/disk_linux.go):
  mountinfo preference/fallback and caller-view usage queries.
- [Go Linux root certificate search](https://go.dev/src/crypto/x509/root_linux.go) and
  [resolver selection](https://go.dev/src/net/conf.go); checked against the local pinned Go 1.27.1 sources.

Repository evidence: static build identity validation in `internal/releasepack/build_identity.go`, the D1
[durable ledger contract](durable-ledger.md), and the [worker plan](worker-plan.md). No network enforcement
or installed acceptance result is inferred from these library contracts.
