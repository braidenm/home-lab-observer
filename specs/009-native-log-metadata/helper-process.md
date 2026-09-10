# Private helper process transport

Status: Implementation slice; no runtime entrypoint or source activation yet.

The private transport accepts only a code-owned, already verified command from the
future fixed platform resolver. It is internal and has no exported command/path
runner. Linux resolution, embedded digest and ownership checks, and Windows
same-executable verification remain separate required integration gates.

Each transport instance has one non-queuing child slot. A request is bounded to
32 KiB before any launch; stdout retains at most 2 MiB. Stderr is discarded and
never returned or logged. Input and output are private pipes, never files or
arguments. The platform resolver must supply a minimal code-owned environment;
the transport rejects an unspecified environment rather than inherit it.

The caller has at most two seconds including launch and I/O, constrained by its
parent context. Cancellation or output overflow kills the direct child. One Wait
owner reaps it, with a 100 ms pipe cleanup bound. The caller allows a further
100 ms cleanup grace, then returns a fixed failure if the OS has not completed
reaping. The occupied slot is retained until actual completion, so subsequent
collections cannot accumulate unreaped children or waiting goroutines. The helper
never launches descendants; this is not a process-tree management API.

No userspace program can promise immediate reaping of a process stuck in kernel
uninterruptible I/O. We therefore bound caller delay and resource multiplicity,
not claim that a kill signal always completes by a deadline. A retained reaper
has no Store, collector, HTTP, or diagnostic access. Stop need not wait indefinitely
for it; process exit delegates residual OS cleanup to the operating system.

Only a successful child exit and a complete bounded packet can reach the separate
strict protocol decoder. Timeout, cancellation, overflow, launch failure, or
nonzero exit returns no partial output and only fixed private error sentinels.
Command arguments, stderr, operating-system errors and request contents are never
formatted into those errors.

Tests use synthetic test-binary children, not native host logs, to cover exact
size boundaries, overflow, nonzero exit, cancellation, timeout, privacy, concurrent
slot refusal and reap-before-reuse. Injectable process lifecycle seams cover an
OS wait that outlasts the caller; production never exposes those seams.

References: Go 1.27.1 [os/exec Cmd](https://pkg.go.dev/os/exec#Cmd) and
[Process.Kill](https://pkg.go.dev/os#Process.Kill). `WaitDelay` closes lingering
pipes but is not an absolute bound on an operating-system process wait.
