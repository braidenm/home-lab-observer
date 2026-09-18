# Same-process uploader startup gate

Status: unused Linux library; installed activation is not accepted.

The connected uploader must not load its credential or ledger merely because a
service started. In the same process, it first checks its local restrictions,
then reads a root-owned canonical activation request from the fixed read-only
`/activation` directory. No request means no probes or ordinary work.

The request binds the installed artifact, configuration and policy generation
plus root-selected ports for owned IPv4 and IPv6 loopback fixtures. The worker
must fail the loopback connection attempts, complete a TLS handshake to the
compiled Platform host using the pinned local CA and selected addresses, and
publish a bounded response with its actual invocation ID and fresh challenge.
It waits for a matching root commit, then rechecks the installed configuration
before returning to its caller. It never takes a caller-selected URL or command.

This package owns only the uploader-side sequence. The root installer must
prove the fixture baseline and zero accepted probe traffic, inspect the paused
process and loaded systemd policy, bind the response to the manager invocation,
and stop/join both workers on failure. Normal CI's synthetic tests do not prove
kernel BPF enforcement, the packaged mount view, real TLS, credentials, or an
installed VM transaction. Those are release gates before an owner canary.
