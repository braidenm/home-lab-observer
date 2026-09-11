# E1: Linux shared-read installed handoff

Status: Accepted for the uninstalled Linux E1 library and owned synthetic DAC/ACL fixture only.
Decision: [ADR 018](../../docs/adr/018-shared-read-handoff.md).

## API

New package `internal/sharedhandoff`, leaving `internal/handoff.Store` unchanged:

```go
type Policy struct {
    CollectorUID uint32
    SharedGID uint32
    UploaderUID uint32
    ServerID string
}
func OpenWriter(directory string, policy Policy) (*Writer, error)
func OpenReader(directory string, policy Policy) (*Reader, error)
func (*Writer) Publish(raw observation.Snapshot, identity remoteprojection.Identity) error
func (*Reader) Read(ctx context.Context, expectedServerID string) ([]byte, error)
func (*Writer) Close() error
func (*Reader) Close() error
```

Policy comes from future trusted root-provisioned configuration, not an untrusted request or autodetected user name.
UIDs/GID must be nonzero, collector and uploader distinct, server ID the canonical exchanged identity. Writer requires
effective UID=collector and effective primary GID=shared. Reader requires effective UID=uploader and shared group
membership (primary or supplementary). Both roles require real/effective/saved UIDs to equal their policy UID and
real/effective/saved GIDs to match one nonzero primary GID. capget requires empty effective, permitted and inheritable
capability sets; it does not check or empty the capability bounding set. Installed CapabilityBoundingSet and
NoNewPrivileges restrictions remain mandatory and separately tested. No latent saved UID/GID is accepted.
These are separate types, not a mode switch that grants Reader a publish API. Nil read context fails fixed unavailable.
The Reader structurally satisfies uploadstate.Source without depending on uploader credentials or runtime.

The library cannot enumerate every process/account authorized by a numeric group. Root provisioning must guarantee the
dedicated group has only the two intended principals and verify that separately. Current process group checks do not prove
global group exclusivity. Root and compromised collector remain trusted for filesystem policy; uploader has no write grant.

## Filesystem contract

Linux only initially; other platforms fail with fixed unsupported and no filesystem writes. Reuse only truly identical
ownerfs traversal checks and remoteprojection Encode/Validate. Do not share a permissive mode parameter with private Store.

- Existing dedicated directory, collector UID/shared GID, exactly 0750, no privileged/sticky bits or extended/default ACL.
- Ancestors are root/collector-owned, not writable by unrelated principals; root-owned sticky temporary ancestors are
  accepted only when the next checked component is root/collector-owned. Uploader cannot replace an ancestor.
- Pin root/directory handles. Use only `snapshot.json`, `.snapshot-next` and `.writer-lock`, no caller paths/names.
- Writer lock is empty, collector/shared owned, 0600 and exclusive OS lock. Reader never requires a read/write lock handle.
- Final snapshot is collector/shared owned, regular, single-linked, 0640, at most 16384 bytes.
- Fresh staging begins 0600. Encode/validate complete bytes, write, change only that freshly created handle to 0640, sync,
  atomically rename and sync directory. No group access to a partly written new staging file. Crash leftovers of staging
  may be 0600 or 0640; only Writer may remove a validated bounded fixed staging file. Existing unsafe entries are refused,
  not chmod/chowned or repaired. No process umask mutation.
- At most latest+staging 32 KiB plus empty lock. Bound enumeration and read lengths. Reader opens only the final slot
  read-only; no cleanup, creation, rename, truncate or chmod paths. Validate policy before any bytes are returned.
- Use O_NOFOLLOW/O_NONBLOCK, opened-handle checks, path/handle identity and bounded private publication race handling.
  Unlinked handles remain unreadable and return unavailable; multiple links/wrong identity/mode/ACL remain unsafe.
- Recheck pinned directory policy for operations, not only startup. No arbitrary tree scanning or file readers.

Handle-based `fgetxattr` queries only `system.posix_acl_access` and (directories) `system.posix_acl_default`, with zero-size
buffers. Reject any present attribute, even a benign extra ACL. Known absent ENODATA is accepted on the reviewed local
Linux POSIX ACL path; unsupported/error responses fail closed. No ACL parser or shell command is required in production.
Positive reader and named-user/default-ACL negative tests must prove actual behavior on the supported filesystem.

Identity.SourceID/Read expectedServerID must equal Policy.ServerID. Encode and Validate retain numeric-only canonical
privacy. Read does not fabricate freshness: the uploader checks collection time. Fixed errors distinguish unsupported,
unsafe, missing, unavailable, invalid document and writer busy, without path/OS/ACL/body values.

## Acceptance and privileged fixture proposal

Ordinary tests cover policy validation, saved-group consistency, owned-handle mode/link/ACL checks, output bounds and
unsupported stubs. The explicitly opted-in privileged fixture covers unknown names, size bounds, partial writes,
sync/rename failures, wrong binding, distinct-principal denial, concurrent replacement and cleanup. Linux static builds remain dependency-free
beyond existing x/sys and projection packages; no networking, service install or root operations in production.

After explicit review of setup, a bounded root test orchestrator may create an owned native `/tmp` fixture:

1. Outer root-owned 0700 temporary directory; inside, a minimal 0755 chroot and a copy of only the static test executable.
2. Create only fixture handoff/resources, assign numeric synthetic collector/uploader/third identities to child processes
   using setgroups/setgid/setuid at exec; never modify host account/group databases or create services.
3. Collector child publishes; uploader child reads exactly the canonical document but direct OS write/create/remove/rename
   attempts fail. Unrelated third child fails direct read/traversal. Children prove empty effective, permitted and
   inheritable capability sets after exec, not an empty bounding set or the complete installed service policy.
4. Add named-user ACL and default-ACL fixtures inside the owned tree; policy rejects them even when mode bits still match.
5. Wrong UID/GID and second writer fail; writer can replace while reader never returns partial/unvalidated bytes.
6. Bound subprocess deadlines/output; reap every child before deleting exact owned temporary fixtures. Output only fixed
   evidence/result codes, never projection contents. No host native log query, network use or Docker socket.

The owned WSL Ubuntu root-orchestrated fixture approach was independently approved before implementation. This authorizes
only newly created validated temporary resources, not existing paths, host accounts, services or deployment changes.
If root/ACL prerequisites are unavailable, report acceptance unexecuted; never silently pass the installed security gate.
The installed service namespace/credential restrictions and real host deployment remain later E/F/G slices.

## Tasks

- [x] Independent API/policy and privileged fixture design approval.
- [x] Separate Linux writer/reader library and no-side-effect unsupported stubs.
- [x] Synthetic boundaries, fresh staging publication, race and fixed-error tests.
- [x] Actual distinct-identity read/write denial and ACL fixtures in isolated owned environment.
- [x] Full Go tests/vet and Linux focused repetition.
- [x] Independent final code/security review (root and native reviewer; no blockers).
- [x] Add path-filtered/manual GitHub-hosted Ubuntu privileged fixture workflow.
- [ ] Observe successful hosted fixture execution before claiming CI acceptance.
- [x] Keep no network/worker/install activation, and owner-private regression behavior unchanged.

## Local execution evidence

On 2026-09-11 the static Linux amd64 test executable ran under Ubuntu WSL in the explicitly approved root-orchestrated
fixture. Ten repeated complete runs passed, including distinct-UID collector/uploader/third-user kernel denial assertions,
named-user and default ACL rejection, wrong primary/supplementary group rejection, empty effective/permitted/inheritable capability sets, and
250 writer publications concurrent with 1000 reader attempts per run. Only complete canonical reads or unavailable were
accepted during publication. Fixed output is `SHARED_HANDOFF_DAC_ACL_PASS` in verbose mode; child output is capped and
discarded, not copied into user-visible diagnostics. No host user/group, service, network or native log operation occurred.

Each root fixture creates an outer native `/tmp` directory, stages a static binary and marker into a new chroot, drops
child identities at exec, waits/reaps children under deadlines, and cleans only its owned temporary tree. Normal tests
skip the privileged fixture unless explicitly enabled with `OBSERVER_SHARED_ACCEPTANCE=1`; enabling it without root fails.
Acceptance automation must explicitly opt in and inspect its result, not mistake an ordinary skip for installed support.
Linux arm64 compiled successfully but has not executed locally. Windows full tests exercise unsupported/no-write stubs.

These results prove scoped DAC/ACL behavior, not installed systemd isolation, remote upload, root provisioning correctness,
global group membership exclusivity, durability across power loss or full canary acceptance.

The `Shared handoff acceptance` workflow compiles the static fixture on a GitHub-hosted Ubuntu runner and explicitly
enables the privileged test three times with a five-minute job limit. It runs for relevant implementation/dependency/policy
changes and manual dispatch. No private runner or secret is used. This exercises the opted-in failure/concurrency tests;
the ordinary runtime suite alone skips them. Hosted execution evidence is pending until the workflow runs successfully.
