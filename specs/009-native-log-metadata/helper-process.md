# Private helper process transport

Status: Implemented transport/reader/dispatcher slice; no runtime entrypoint or source activation yet.

The private transport accepts only a code-owned, already verified command from the
fixed platform resolver. It is internal and has no exported command/path runner.
Linux resolution, embedded digest/ownership checks, and Windows same-executable
verification are described in [helper-identity.md](helper-identity.md).

`NewReader` takes only the code-owned release version, commit and embedded Linux
digest. Construction performs no I/O; one instance is shared across configured
sources. Read validates input, resolves the fixed helper, uses private protocol
encoding/decoding and rejects uncorrelated output. Missing/mismatched helpers have
closed unavailable batches. Other process/protocol failures return a fixed error
for the collector's failed-attempt projection without asserting native support.
macOS returns explicit unsupported without touching the filesystem or launching.
Linux rejects the Windows-only application preset. Resolver errors are reduced to
canonical sentinels even if wrapped with private operating-system details.

Each transport instance has one non-queuing child slot. A request is bounded to
32 KiB before any launch; stdout retains at most 2 MiB. Stderr is discarded and
never returned or logged. Input and output are private pipes, never files or
arguments. The platform resolver must supply a minimal code-owned environment;
the transport rejects an unspecified environment rather than inherit it.
The working directory is the verified executable's directory, never the parent's
possibly untrusted current directory. Windows helpers have no visible console.

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

The reusable child dispatcher performs process hardening before reading a single
private input byte. Only a bounded, exactly correlated request may open the fixed
native reader. It emits one validated protocol response; expected native-open
failure is a closed unavailable batch. Invalid input, hardening failure, reader
error, cancellation or invalid batch exits unsuccessfully without diagnostics.
The native reader's cleanup runs before emitting output so cleanup-time failure
or cancellation cannot publish a premature success. Actual platform entrypoints
and mandatory Linux hardening wiring remain integration gates.

References: Go 1.27.1 [os/exec Cmd](https://pkg.go.dev/os/exec#Cmd) and
[Process.Kill](https://pkg.go.dev/os#Process.Kill). `WaitDelay` closes lingering
pipes but is not an absolute bound on an operating-system process wait.
