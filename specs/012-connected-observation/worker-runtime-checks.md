# Fixed same-process Linux worker checks

Status: implementation slice under Spec 012 and ADR 020. These unused
same-process checks are one part of activation evidence, not acceptance of a
unit or permission to start a connected worker.

The future collector and uploader call fixed Linux/amd64 checks before
opening private state or observations. The primitive probe requires actual
denial of role-specific sockets, Unix socket pairs, System V shared memory
and `io_uring_setup`; a successful operation is closed or cleanup-attempted
and always refuses. An absent feature, resource exhaustion or unrelated error
does not masquerade as isolation. Fixed view checks require the collector's
minimal numeric `/proc` sources without private uploader paths, and require
the uploader's restricted root to lack host `/proc`, `/sys` and other host
trees while its few approved directories are read-only.

The package has no network client, listener, credential reader, installer or
collector initialization. Unit tests inject primitive outcomes and verify
both denials and unexpected-success cleanup. They do not prove the loaded
systemd profile, inherited-descriptor closure, effective IP restrictions,
TLS, or installed principal. Root must audit the paused actual worker's
descriptors and policy; disposable-VM tests must verify the complete packaged
invocation before any service activation.
