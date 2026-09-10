# Windows same-binary helper composition

Status: implemented entrypoint and synthetic composition tests; native activation, host-log fixtures and release
publication remain separate gates.

On Windows amd64/arm64, the verified observer executable accepts exactly one private child invocation:
`__log-helper` with no following arguments. The fixed resolver is the only production caller. This dispatch happens
before the public CLI and before normal diagnostic output, so malformed helper identity, arguments or input exit
unsuccessfully and silently. Other platforms do not recognize the private mode.

The child validates its nonempty authoritative `observer` release identity for the running Windows OS and architecture
before reading stdin. It then delegates to the existing bounded helper dispatcher: one closed private request from
stdin, one validated and correlated response on stdout, no stderr diagnostics, a fixed two-second context and no
history, HTTP or configuration authority. Input decoding completes before the fixed WEVTAPI reader is opened. The
entrypoint offers no channel, query, path, remote host, body, provider-name or arbitrary argument surface.

The fixed reader composition is `eventnative.NewFactory` behind `eventreader.New`. It selects only the System or
Application source encoded by the already-validated private request. Query/thread/handle ownership and the accepted
operational continuation-anchor limitation remain defined in [windows-reader.md](windows-reader.md). Windows needs no
Linux dumpability setup; the parent already launches the verified same executable hidden, with a minimal code-owned
environment and working directory, while the shared dispatcher still runs its required pre-input preparation seam.

Synthetic tests inject the reader and preparation seams; they do not read or register a host event channel. They prove
identity and argument rejection before private input, malformed request rejection before native open, cleanup before a
closed response, protocol correlation, canary non-reflection and exact hidden dispatch. Packaged process deadline,
kill/reap and trusted-executable resolution remain covered by the shared transport tests. Actual owned Windows event
fixtures, runtime activation and release publication are still required before claiming end-to-end support.
