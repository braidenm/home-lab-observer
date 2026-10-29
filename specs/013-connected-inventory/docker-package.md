# Dormant installed Docker package implementation

Status: implementation in progress after the separately reviewed process-only VM gate; no owner installation,
approved production enrollment writer or published rich release. This is part of one coherent ADR 025 package,
not a separately activated contract. Numeric and process-only definitions/resources remain unchanged.

## Review-size scope — 2026-10-29

This is source layer 3 of 3, following [implemented bounded data plane/storage](docker-data-plane.md) and
[dormant three-worker composition](docker-worker-runtime.md). It completes the privileged installation, exact resource
rendering, four-executable bundle/bootstrap and build entry point. The three independently compiling/tested source
layers reduce review size while retaining one installed acceptance target; no earlier layer was an owner release.
The [merged process-only acceptance record](../../docs/acceptance/connected-rich-processes-2026-10-29.md) is preserved.
The draft Docker definition is not independently persisted server approval. New-source exact-head CI, final-source
reproducible artifacts, actual packaged Docker VM and hosted/owner acceptance are still required before release.

## Fixed binding

`RICH_PROCESSES_DOCKER_V1` uses kind 2/schema 3, both groups enabled, the existing combined/body/ledger budgets and
the SAME exclusive rich configuration, state and release roots. A pre-existing rich root refuses before mutation;
numeric may coexist. Four exact commands are `observer-connected-rich-docker-{collector,reader,uploader,install}`.
Exact units are `home-lab-observer-connected-rich-docker-{collector,reader,uploader}.service`.

Principals are `hlo-rich-docker-collector`, `hlo-rich-docker-uploader`, `hlo-rich-docker-reader`, handoff group
`hlo-rich-docker-read` and frame group `hlo-rich-docker-frame`. Collector/uploader/reader UIDs are distinct;
UploaderGID/SharedGID/FrameGID/approved SocketGID are distinct. UID and GID numeric domains may overlap normally.
The reader's primary group is FrameGID; its private status remains UID-owned 0700. Only its unit receives SocketGID
as a supplementary group, never persistent NSS daemon membership. Collector's primary SharedGID plus service-only
FrameGID permits frame reads; uploader has only its primary group plus SharedGID. No observation action authority.

Persist approved SocketGID, not a permanent socket inode. Explicit root Start validates fixed `/run/docker.sock`
as root-owned 0660 with the approved nonzero group and trusted parents, then binds that exact inode at
`/engine/docker.sock` in the minimal reader root. The activation witness binds actual socket device/inode and reader
PID/invocation. Replacement is unavailable until supported Stop/Start revalidation; no automatic adoption, socket
permission repair or daemon restart. Reader root contains only config/policy, executable, frame/status and socket
bindings plus reviewed inert systemd scaffolding; no host process/device/credential/ledger or uploader-activation access.
The separate read-only reader `/activation` mount carries only the existing typed request/response/commit startup
handshake. Before commit the reader performs no Engine I/O or frame publication. Its fresh same-invocation response
proves primitive checks finished and all socket FDs closed, allowing the unchanged root descriptor audit. Root audits
the exact PID/invocation, namespace and pinned socket before request publication and again before publishing commit.
`ConfigSHA256` hashes the canonical versioned reader-startup envelope and fixed reader role, not the uploader's raw
configuration. Both activation channels are masked from the frame-reading collector. Codec port fields are fixed inert
1024 values, rejected if different and never probed. A separate permit protocol and timing/sleep-based audits were
rejected; see [ADR 026](../../docs/adr/026-docker-reader-startup-barrier.md).

Frame directory is `<rich-state>/docker-frame`, mounted `/frame` for the reader. It samples every 15 seconds after
a joined at-most-four-second acquisition; frame freshness is 45 seconds with five-second future tolerance. Two
128-KiB slots have a 256-KiB transient ceiling, preserving real section observation time. Reader caps are 128 MiB,
32 tasks, 25% CPU, GOMAXPROCS=2 and Go memory target 64 MiB; Stop=15 seconds, Restart=no, null standard output/error.
Existing collector/uploader bounds are unchanged. A canonical draft definition binds these facts; its release digest
is frozen only after exact resources, independent review and packaged VM/resource acceptance, before remote approval.

## Local interfaces and acceptance

Named `connectedprofile.RichProcessesDockerConfig/Policy` and encode/decode/load/digest/binding functions are closed
siblings, not a caller-defined profile registry. Separate reader identity admission checks its real IDs, required
groups, no-new-privileges and zero capabilities. Named enrollment/transport/storage/runtime/installer siblings enforce
this definition without widening old constructors. The reader uses only the existing bounded GET adapter, sanitized
typed file and fixed local socket. It never accepts a URL, HTTP request or listener configuration.

Foundation unit tests precede runtime integration. Exact identity/resources/bundle tests, fresh enrollment echo,
cross-kind storage/hot-journal proof, actual sandbox/lifecycle/daemon-fault VM proof and owner/delegate release gates
remain mandatory under [the installation plan](installable-docker-observation.md). Source composition alone proves
none of those deployed outcomes.

## Source implementation evidence (not installed acceptance)

The dormant composition now has the four fixed binary identities, six exact unit/schema resources, canonical bundle
verification and deterministic archive/bootstrap construction. Fresh enrollment requires an independently approved
exact profile/definition echo and provisions kind 2/schema 3; older constructors refuse adoption. Installation leaves
all services stopped/disabled, preserves partial state, and never installs/reconfigures a Docker daemon. Explicit
Start rechecks the persisted group and pinned current socket, starts the reader through the two paused audits, then
the existing uploader/collector activation proof. Stop joins only captured invocations; status includes the reader
separately so missing Docker acquisition does not become a healthy-empty inventory.

| Role | Persistent groups | Service-only supplementary grant | Writable observation state |
| --- | --- | --- | --- |
| Collector | Handoff primary group | Frame group | Combined handoff, collector status |
| Reader | Frame primary group | Approved socket group | Two bounded frame slots, reader status |
| Uploader | Private primary plus handoff membership | Handoff only | Private ledger, uploader status |

Local validation: Windows full Go tests/vet; Linux installer/enrollment/policy/resource/bundle/bootstrap/packer tests
(including the real four-role synthetic ELF archive journey); focused Linux race/vet; real root identity, socket and
frame DAC fixtures; Node 80 contract tests, runtime snapshot and foundation policy. Synthetic startup negatives cover
preexisting/replaced invocation, both audits, stale response/status, uncertain start, cancellation and publication
failure. These do not prove an installed Docker service, real daemon fault isolation, reboot or owner/delegate use.
Independent review, exact-head CI, actual reproducible package build, the complete disposable VM target, immutable
artifact publication and hosted reader-first approval remain separate release gates. No public rich installer or
owner-host activation is enabled by this source evidence.

### Layer 3 local checks — 2026-10-29

The aggregate 119 implementation/test/script/workflow Git blobs match full-composition review source
`70cadaf9b6bfcfd8542291385571ffa50ef8ce5b` exactly; only split documentation and merged process-only acceptance updates
differ. This layer adds 44 of those files over dormant workers `381fafac3834e76d146b554aff71f93b9b7ef2f0`.
Go1.27.3 Windows/Ubuntu WSL full tests and vet, focused Linux installer/policy/units/bundle/bootstrap/packer/installer
command race tests, and the actual root temporary-socket pinned-witness/ACL/link/replacement fixture pass locally.
The synthetic four-role ELF/archive journey is included in real Linux tests, not a mocked manifest-only check.
Node22.22.3 80 contracts, schema fixtures, real Windows collect-once, foundation43, Bash syntax, ShellCheck0.9.0,
local links, diff hygiene and exact original-profile/resource preservation checks pass.

These checks are not this split's exact-head CI, a final-source artifact, actual packaged Docker VM acceptance or
independently approved enrollment. The earlier two-pass `70cadaf` build is preliminary; adding the accepted VM harness
and merging the source layers changes source identity and requires fresh exact-source artifact pins and acceptance.
