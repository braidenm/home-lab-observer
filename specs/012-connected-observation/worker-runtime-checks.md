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

## systemd 255 empty scaffolding correction

The first packaged activation run on 2026-10-25 passed installation but refused
startup. The accepted stopped-install evidence already showed that systemd creates
empty root-owned mount-point directories even with `MountAPIVFS=no`; requiring
those pathnames to be absent therefore rejects the supported profile itself.

Keep `/home` absent. Before launching either worker, root validates the exact
physical uploader scaffold, root ownership, modes, no ACLs and empty leaves,
including `/root`, `/usr`, `/var`, `/proc`, `/sys`, `/dev/mqueue` and
`/run/systemd/incoming`. Reject extra children and links. In the actual uploader,
the readable host-tree placeholders must remain exact empty directories and
must not be procfs/sysfs; `/root` must retain its exact metadata and deny both
reading and search. Root validates the inaccessible leaf before launch rather
than trusting permission denial as proof of emptiness. No host tree is mounted
or made visible by this correction. The pinned live namespace must match the
physical root and fixed bind sources. With `PrivateIPC=yes`, `/dev/mqueue` may be
either the unchanged inert placeholder (`MountAPIVFS=no`) or an exact root-owned,
empty mode-1777 `MQUEUE_MAGIC` mount. Both branches require a different IPC
namespace from the coordinator; neither admits the host's message queues.
See [systemd 255 PrivateIPC](https://www.freedesktop.org/software/systemd/man/255/systemd.exec.html#PrivateIPC=),
rechecked 2026-10-25. Active namespace identity and mount checks
use the stable [Linux 6.8 mqueue filesystem magic and root mode](https://github.com/torvalds/linux/blob/v6.8/ipc/mqueue.c)
because this value is not exported by the pinned x/sys package. These checks
remain packaged-VM acceptance, not a unit-test claim.

Fixed lifecycle refusal phases identify installed state, host profile, principals,
service state and the runtime gate without copying raw errors. VM diagnostics
classify only this closed vocabulary. Installation, credential and ledger
admission are unchanged; no failure permits a policy fallback or reenrollment.
