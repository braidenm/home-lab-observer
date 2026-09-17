# F1 activation evidence design

Status: accepted for implementation after independent review, 2026-09-17.
No activation authority or completed-profile proof.
Implements the remaining gate in [ADR 020](../../docs/adr/020-linux-installed-worker-composition.md),
not a relaxation of its disposable-VM or packaged release requirements.

## Question and current evidence

How can `start`, restart and endpoint refresh validate the actual worker boundary
without a permanent privileged daemon, an owner checkbox, or a reusable "safe"
flag? F1 currently installs stopped. Manager policy checks and four primitive
probes have passed, but those are not whole-profile acceptance. Root is trusted;
the checks protect against unsupported hosts, accidental drift and broken
isolation, not a malicious root administrator.

The supported first target remains Ubuntu 24.04, systemd 255, Linux amd64.
Unrelated local Observer and Compose installations must remain untouched. The
collector must not import transport or credential packages for self-testing.
The uploader must not gain a general file reader, command executor or arbitrary
network target. No test may reset, copy back or reprovision the installed ledger.

## Options

| Option | Security and operability | Cost and portability | Decision |
| --- | --- | --- | --- |
| Continue stopped installation | Safe while proof is missing, but no connected user outcome | Minimal code; not deliverable completion | Current fallback only |
| Root-owned durable acceptance receipt alone | Easy to operate, but stale after boot, policy or artifact drift | Small code; creates invalidation/replay obligations | Reject as startup authority |
| Separate generic sandbox test utility | Can test many controls, but different executable and mutable profile can diverge | More packaging and privileged test surface | Use only for disposable release fixtures |
| Fixed checks in the actual worker process, plus root-owned manager checks and release acceptance | Tests the process that will read observations/credentials; no independent success token | Narrow platform-specific code; modest startup delay | Selected for implementation |

## Recommended composition

1. Keep release-time destructive/fault acceptance separate. The exact bundle must
   pass disposable-VM installation, mount/UID, network, TLS, crash/ledger, refresh,
   rollback and reboot tests before it is offered for live canary activation.
   CI syntax validation, a security score and an operator acknowledgement do not
   replace these results. Failed proof leaves the profile unavailable.
2. Under the existing installation lease, `start` validates immutable artifact,
   root-owned configuration, exact unit bytes, current loaded restrictions and
   ancestor address policy. Refuse partial refresh/uninstall or unknown services.
3. Each real worker runs a small fixed startup check before opening handoff,
   collector adapters, credential content or ledger. It checks its actual numeric
   identity, no-new-privileges, required denied socket/IPC primitives, expected
   fixed filesystem visibility and absence of inherited network sockets. Only
   closed result codes are diagnostic. A failure is a non-restart terminal exit.
   A successful check does not assert complete filesystem collection coverage.
4. Do not rely on `ExecStartPre` to establish the actual process's mount view:
   systemd creates execution namespaces separately. Fixed checks belong before
   normal work in the same process. Keep process-local nondumpability/core-limit
   verification ahead of any secret input, including enrollment.
5. The root start transaction additionally needs bounded, same-generation network
   enforcement evidence. The proof must distinguish a denied connection from an
   absent listener/unavailable network: an owned dual-stack loopback fixture needs
   a successful baseline, denied attempts from the actual uploader context and
   zero accepted fixture traffic. Authorized fixed-host TLS must succeed in that
   same context without sending credentials or HTTP. No caller-supplied host,
   URL, shell or arbitrary port target is exposed by a worker.
6. Start collector/uploader only through the reviewed transaction. Any failed
   check stops and joins the exact owned workers; it does not broaden policy,
   restart on partially switched configuration or retry enrollment. Recheck on
   explicit start/restart/refresh. The first canary uses `Restart=no` and leaves
   boot activation disabled; `Restart=on-failure` cannot reuse an earlier proof.
   A later narrow root oneshot/timer must repeat this same transaction before
   unattended boot or crash recovery. This is a documented canary limitation,
   not the final unattended product promise.

## Bounded same-process rendezvous

Use one fixed root-owned volatile activation directory under the existing `/run`
coordination root, read-only bound at `/activation` in the uploader. Bind the
directory, not an individually replaced file. Root alone writes canonical bounded
request/commit records; uploader reads/traverses via its reviewed group. No
credential is included. Reject symlinks, unexpected owner/group/mode/ACL, extra
fields and records over 2 KiB. Do not extend the durable ledger or generic
handoff APIs with activation authority.

The root request binds a random nonce, artifact/configuration digests and policy
generation. It contains only two root-selected ephemeral port numbers for the
owned IPv4/IPv6 loopback fixture, never an address, URL, path or command. The
worker uses compiled loopback literals for these ports and the compiled HTTPS
origin for TLS. There is no user-facing network-probe endpoint. Root retains
both listeners through baseline/probe/baseline checks and counts connections;
unexpected fixture traffic fails closed. Baseline connections have a separate
phase and are not counted as worker-denial success.
Drain the initial baseline connections before opening the denial-counting phase,
then publish the request to the already paused, audited worker. Finish worker attempts and close their
sockets before ending that phase and running the final baseline.

Before worker start, stop/join both units under the installation lease and remove
only validated owned old request/commit files. Verify the fixed directory is empty
and pin that same directory inode for the worker bind and later root publication.
The reviewed uploader performs local checks and waits for a request; absence never
falls through into probes, credential access or ordinary work. Automatic restarts
are disabled. This absent-request barrier avoids another protocol phase.

While the actual worker is paused, root anchors its `/proc/<MainPID>/fd` directory
and inspects every descriptor entry, bounding the number of entries rather than
the largest descriptor number. Reject sockets, oversized enumeration, unreadable
entries or any ambiguity. Recheck manager invocation ID, MainPID, process start
identity and exact executable before and after. Check loaded stdio=null, no socket
activation and no configured/current descriptor store. Root then publishes the
fresh request into the same pinned activation directory, permitting probes.
Nondumpability is never lowered to make inspection work; missing host permissions
refuse activation. Lowering RLIMIT_NOFILE does not close inherited high descriptors
and is not a substitute for this audit. Root records only closed results, never
descriptor target strings. No host procfs is mounted into the uploader.

Before credential or ledger access, the same uploader process performs its
fixed local checks, denied loopback attempts and fixed-origin TLS handshake. It
publishes a bounded response in its existing private status directory, containing
the request binding, actual invocation ID and a newly generated process-local
challenge, plus closed check results. Root validates response metadata/schema,
exact manager invocation identity and loaded policy, baseline connectivity and
zero connections during the denial phase before publishing commit. The commit
must match the request, invocation and fresh worker challenge. The paused worker
rechecks its immutable binding and accepts it only in that startup state.

All rendezvous work has a monotonic 30-second overall deadline. Parent death
before commit, partial records, worker replacement, stale records or mismatch
fail terminally. Every new worker generates a new challenge, even if a previous
request/commit remains. Reboot removes volatile proof. Raw errors and fixture
port numbers are not status text. Cleanup closes owned listeners and joins only
the verified invocations; failure never starts an unverified replacement.

Collector checks remain syscall/filesystem-only; collector imports no TLS,
HTTP, credential or uploader rendezvous package. Root checks collector startup
and stops both owned workers on transaction failure. Collector-only activity
before upload approval does not authorize any external transmission.

No durable acceptance flag or user-supplied attestation is introduced. A diagnostic receipt may record
artifact/configuration digests, policy generation, boot identity, time and closed
results, but cannot itself authorize later activation or contain host observations.

## Proof, operations and recovery

- Unit-test every refusal, malformed/extra input, partial output and deadline.
  Exercise process hardening only in child tests, never by crashing a real worker.
- Package dependency-closure tests must still exclude HTTP/credentials from the
  collector and rich host adapters from the uploader.
- Disposable profile tests deliberately remove one restriction at a time and
  require rejection; verify positive numeric collection, TLS and SQLite still run.
- Correlate root observations with exact invocation identity, not PID alone;
  join fixture children/listeners before cleanup and never stop foreign units.
- Require stale-commit replay, replaced worker, parent death at each phase,
  missing IPv6 baseline, disabled filtering and changed policy-generation tests.
- Test absent-request waiting, same-directory replacement refusal, inherited
  socket descriptors above a lowered file-descriptor limit, oversized descriptor
  enumeration and denied root process inspection. No failed audit publishes a request.
- Emit fixed actionable states: acceptance missing, policy drift, unsupported
  primitive, failed runtime check, or recovery required. No raw socket errors,
  addresses, paths, credentials or unbounded labels enter diagnostics.
- Refresh and compatible code rollback preserve the same durable ledger and
  require fresh checks; an older successful receipt cannot authorize them.
- Headless/native Windows and macOS support needs equivalent reviewed boundaries;
  these Linux checks make no cross-platform guarantee. Docker controls and smart
  devices remain separate capabilities/specifications.

## Primary evidence and pitfalls

Sources accessed 2026-09-17, pinned to the supported v255 baseline:

- [systemd execution controls](https://github.com/systemd/systemd/blob/v255/man/systemd.exec.xml):
  per-execution filesystem namespaces, inherited descriptors and sandbox controls.
  A successful pre-start process is not the same namespace as the main worker.
- [systemd service lifecycle](https://github.com/systemd/systemd/blob/v255/man/systemd.service.xml):
  pre-start failure prevents normal start, but pre-start children are not a way to
  keep an independent long-running proof service alive.
- [systemd resource controls](https://github.com/systemd/systemd/blob/v255/man/systemd.resource-control.xml):
  inherited allow policy and kernel support require effective-policy and actual
  enforcement checks; setting an IP rule is not sufficient evidence.
- [systemd security analysis](https://github.com/systemd/systemd/blob/v255/man/systemd-analyze.xml):
  the score assesses a subset of unit settings, not complete application or IPC
  security. Use it as diagnostic context, never the activation decision.

No new owner product decision is required for these conservative checks. A
supported disposable-VM environment and real acceptance evidence are still
required before live activation; lack of either is reported, not waived.
