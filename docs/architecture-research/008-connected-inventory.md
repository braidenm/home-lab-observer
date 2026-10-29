# Expanding connected observation without widening implicit authority

Research/access date: 2026-10-29. Scope: Spec 013 read-only expansion and sequencing before Spec 011 Docker actions.

## Question and actual evidence

How can the hosted dashboard gain useful process/container data while preserving local-first use, owner-scoped access,
bounded operation and the working numeric installation? Current source baseline d9935b8 uses gopsutil v4.26.9, Go 1.27,
separate `collector`/`containerobs` adapters, and a connected `numerichost` worker. Its unit denies socket/socketpair,
uses PrivateNetwork and ProtectProc=invisible; numeric projection excludes all workload rows. Container read-model
ADR 006 intentionally preserves null metrics outside the old mandatory-numeric snapshot. ADR 022 is not an accepted
profile-changing upgrade mechanism. No copied user data or private infrastructure is needed for this decision.

## Primary sources and applicable conclusions

- [gopsutil v4.26.9](https://pkg.go.dev/github.com/shirou/gopsutil/v4@v4.26.9), pinned repository baseline, BSD-3-Clause:
  reusable host/process adapters and platform differences. Keep acquisition behind existing providers and do not
  replace maintained collection with ad hoc shell parsing. Release compatibility does not establish sandbox visibility.
  Its pinned [CPUPercent implementation](https://github.com/shirou/gopsutil/blob/v4.26.9/process/process.go#L365-L382)
  computes lifetime CPU time / age *100, which can exceed 100 across cores; it is not a recent host-capacity sample.
  The current normalizer's 0..100 filter can omit busy processes.
  The first rich contract exposes this basis and limitation rather than silently changing acquisition semantics.
- [systemd 255 execution environment](https://www.freedesktop.org/software/systemd/man/255/systemd.exec.html), deployed
  native-beta target: PrivateNetwork creates a namespace with loopback only; ProtectProc controls foreign process
  visibility. Preserve truthful scope and independently test any revised installed profile. Disabling a sandbox flag
  is an authority change, not a harmless metric fix. Filesystem-backed Unix sockets can remain reachable despite a
  private network namespace; mount visibility and syscall policy also matter.
- [Docker socket protection](https://docs.docker.com/engine/security/protect-access/) and
  [daemon authorization](https://docs.docker.com/engine/extend/plugins_authorization/): authenticated daemon access can
  carry host-root authority. Read-only filesystem mounts do not constrain HTTP operations. A small fixed reader/broker
  is an explicit trusted boundary; do not expose or forward the socket or silently configure a daemon plugin.
- [Docker Engine v1.54 reference](https://docs.docker.com/reference/api/engine/version/v1.54/): inspect fixed resource
  operations and per-version compatibility when implementing adapters; this reference is not evidence that every
  deployed daemon supports that version. Keep the adapter's negotiated/tested range explicit.
- [Beszel security](https://beszel.dev/guide/security), rolling docs identifying 0.21.1: agent-initiated connectivity
  is a practical centralized-viewing pattern. Preserve outbound authenticated HTTPS rather than requiring inbound
  ports or copying a custom handshake/machine-fingerprint ownership scheme. Upstream's documented SSH host-key
  limitation is not adopted; existing SSH operator access continues strict known-host verification.

The established-project comparison in [research 007](007-open-source-alignment.md) remains useful for Netdata's
local/central separation and Cockpit's visible privileges. Its older version claims are not fresh implementation
proof. The current [Cockpit privilege guide](https://docs.cockpit-project.org/cockpit-guide/main/guide/privileges.html)
confirms visible administrative-access state backed by the host session; reuse its clear UX, not its broad host login
authority. [Netdata's process view](https://learn.netdata.cloud/docs/live-view/processes) combines grouped summaries
and drill-down while treating command-line visibility as elevated access. Our smaller contract excludes command lines.
The versioned gopsutil package documentation was available when the upstream tree URL was not.

## Alternatives, use cases and pitfalls

Keep numeric + legacy Compose as a migration fallback; it avoids new collection authority but does not deliver the
native product. Adopting an existing dashboard would reduce custom collection work but would still need our owner/
delegate boundary and deployment/license review. Extend existing adapters through explicit opt-in profiles for this
portfolio/standalone product rather than embedding an entire orchestrator or general remote shell.

Normal collection must include stopped Docker containers without fake live metrics. Denied stats must not remove
otherwise readable inventory. A namespace-local counter cannot be relabeled host network usage. Process names and
image references may contain private data; bounded text is not anonymity. Collection deadlines, row truncation, daemon
failure, counter resets, owner revocation and disconnected upload must remain independently visible. Observation IDs
and changing PIDs are not restart authorization. A successful Docker operation is not proof of application health.

## Recommendation, proof and operation

Use the staged plan in [Spec 013](../../specs/013-connected-inventory/plan.md): truthful UI, explicit DTO/schema and
receiver fixtures, reviewed opt-in installed runtime, pipeline/migration proof, then packaged and real-owner acceptance.
Only afterward enable separately specified actions. Measure CPU/memory/disk and timeout behavior on a disposable
fixture before the owner's host. Never test restarts on real application/game containers as a smoke test.

Keep numeric v1 fixtures unchanged; prove malicious extra fields and raw text cannot escape explicit assignments.
Prove parser compatibility and owner/delegate denial through real HTTP, not only a DTO unit test. Test crash/retry and
used-ledger preservation for upgrades, and observe actual hosted rows before claiming rollout. Default-off new groups
and preserving the working old profile provide a reversible rollout. No new signing/GA or platform parity claim.

## Owner decisions

The owner authorized richer data and independently permissioned Docker start/stop/restart, leaving engineering choices
to us. No additional product question blocks the bounded read-only plan. Exact installed profile, consent persistence
and migration need documented independent technical review; do not convert this delegation into blanket control
grants or an exception to release/security gates.
