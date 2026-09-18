# Paused connected-worker descriptor audit

Status: implementation slice under Spec 012 activation design. This read-only
Linux helper is not an installer, sandbox, worker-start permit, or replacement
for packaged VM acceptance.

Before publishing an activation request, the root transaction must hold the
installation lease and keep the exact uploader invocation paused behind the
absent-request barrier. The audit pins a pidfd and procfs directory, verifies
the expected fixed executable inode and process start identity, enumerates
every descriptor entry twice (not merely numbers below `RLIMIT_NOFILE`), and
refuses sockets, malformed or oversized inventories, missing standard FDs,
process replacement and unavailable inspection. It reads descriptor type
only; target paths and contents never enter diagnostics. A short deadline is
checked between kernel operations, but cannot interrupt one blocked syscall.

The parent must independently confirm the manager InvocationID, MainPID,
loaded unit restrictions, no socket activation or FD store, and exact worker
identity before and after this audit. Two inventories are not atomic; the
paused trusted-worker barrier and stopped sibling are essential. A passing
result is diagnostic correlation evidence, not reusable authority.

Tests cover malformed process stat, descriptor-number extremes, socket and
stdio refusal, exited-process refusal and self-audit behavior. Effective
namespace, principal, network/TLS, credential and ledger behavior still need
the full installed disposable-VM acceptance gate.
