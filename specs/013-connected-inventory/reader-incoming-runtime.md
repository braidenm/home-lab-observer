# Docker reader native incoming-directory boundary

Status: approved plan; local implementation under source review, exact-package acceptance pending. Date:2026-10-30.
Evidence/alternatives: [version-matched research](../../docs/architecture-research/2026-10-30-reader-runtime-incoming.md).
Keep [ADR025](../../docs/adr/025-isolated-installed-docker-observation.md) and
[ADR026](../../docs/adr/026-docker-reader-startup-barrier.md) unchanged.

## Required behavior

1. Preserve the physical `reader-root/run/systemd/incoming`: exact root:root0755, empty, ACL-free, same existing
   root-owned scaffold. Do not change `uploaderScaffold`, generic `safeScaffold`, numeric or process-only checks.
2. Only the live Docker reader's `/run/systemd/incoming` changes from readable0755 to native exact root:root0600,
   ACL-free and denied. Its returned read state must be `denied=true` with no returned entries, never treat an unread
   directory as proven empty. Require read and traverse Access to fail with EACCES; write must also be denied
   (EACCES or EROFS). Reject0755, other modes/ownership, ACLs, successful read/access, malformed/unknown errors and
   observed procfs/sysfs. The other live paths retain their current exact checks.
3. Root independently proves the one code-owned source
   `/run/systemd/propagate/home-lab-observer-connected-rich-docker-reader.service`. Use a private compiled literal,
   with test-only equality to `connectedunits.RichProcessesDockerReaderUnit`, never a supplied name or directory.
   The worker dependency closure must not import the unit-rendering/installation package. Validate trusted ancestors,
   no symlink traversal, exact root:root0600 source, absent access/default ACL, bounded empty directory and non-API
   filesystem. Walk/pin every fixed parent component no-follow, requiring root-owned directories without group/world
   writes, not an assumed exact parent mode. Open the unit child relative to the pinned parent; compare the parent
   path/object and source path/FD before and after audit. No writes or permission repair.
4. Reuse `checkNamespaceView`'s pinned namespace-root callback so live `run/systemd/incoming` must identify that exact
   source. Return a tiny Dev/Ino value to the reader-only existing startup audit. Before request and after matching
   response, the same captured PID/InvocationID must have the same approved source identity; mismatch blocks commit.
   Preserve socket, inherited-FD, policy, descriptor-pass and challenge fences. Close audit descriptors on all exits.
5. Do not add a competing bind, `InaccessiblePaths=`, broader mount option or generic manager/path abstraction.
   Existing unit bytes, resource hashes, caps, paths exposed to the worker, collection/profile fields, schema and
   frozen definition `86f39c7ba444f5ef4560ba82cbf660a17c0d7810b6e49443cb509d9e85036f37` remain unchanged.
   The native denied manager bookkeeping is not a collection channel or additional user-controlled authority.

## Small coherent implementation map

- `internal/connectedruntime/docker_view_linux.go`: Docker-reader-only runtime distinction. Leave the generic
  physical and old uploader guards untouched; no accepted-mode OR fallback.
- `internal/connectedruntime/docker_namespace_linux.go`: private fixed-source descriptor audit through the existing
  pinned-root callback; a closed reader-specific result carries Dev/Ino. An acyclic `connectedunits` import still
  violates worker authority/dependency closures. Keep the exact unit literal private in runtime and bind it to the
  unit package only in a test; do not relax any command dependency test. Old namespace entry points stay strict.
- `internal/connectedinstall/docker_reader_activation_linux.go`: existing Docker-only audit seam captures the first
  identity and compares the second before commit, preserving all lifecycle phases and conservative cleanup.
- Focused runtime/namespace/lifecycle tests and the existing disposable Docker fixture assertions where needed.
  No collector, transport, enrollment, ledger, public API, dependency, workflow or generic manager redesign.

## TDD and acceptance

- Failing runtime fixture reproduces readable physical0755 versus live0600-denied; pass only the exact reader case.
  Negative mode/owner/group/ACL/access/error/filesystem/listing cases plus unchanged numeric/process/physical tests.
- Source fixtures: unknown unit/path cannot be supplied; symlink, foreign owner, writable ancestor, ACL, nonempty,
  API filesystem, parent link/replacement, substituted source FD/path and changed namespace inode fail. Assert bounded reads and descriptor
  closure. A denied child listing is not the root emptiness test.
- Lifecycle fixtures retain first-source identity before request, same identity after response, and reject a changed
  source/PID/InvocationID/socket/policy before commit. Cancellation/cleanup uncertainty remains recovery, no commit.
- Run focused Linux race/vet, required contract/packaging/dependency checks and ordinary platform CI without a new
  broad suite. Verify numeric/process definitions, SQL, fixtures and unit/resource bytes are unchanged.
- Freeze final clean merged source after all source changes. Build protected Docker candidate and same-source local
  numeric/process/Docker baselines, compare exact independent protected bytes and attestations, then complete both
  first-boot/reboot Docker cases with no diagnostic overlays. Require actual startup audits, useful rows, isolation,
  faults, retry, numeric coexistence, existing-rich refusal, caps and exact cleanup; another refusal reopens diagnosis.
- No accepted Docker VM, publication or owner-host upgrade is claimed by source or retained-state probe results.
  Canonical acceptance may be the only addition after final candidate source; all release/owner gates remain.

## Rollback and evidence handling

Before activation, remove the unused correction as code rollback. Never chmod a live propagation directory, modify
the daemon, reset/adopt a ledger, select an older binary against new state or repair an owner installation. Failure
keeps workers stopped where owned, retains private state and emits only finite outcomes. Existing accepted numeric
installation and process-only definition remain independent. Any later active rollback uses supported stop/revoke
boundaries; no automatic upgrade/uninstall/crash recovery is added.

The current probe's `READ_RUN_INCOMING_MODE` with complete recovery is retained-sandbox evidence, not proof that this
was the original startup failure. The root audit and complete exact-source packaged journey must establish the fix.

## Local source evidence (2026-10-30)

Root and peer approved the spec-first boundary before production edits. New focused tests first failed with the
missing fixed-source audit/identity, then passed with the implementation. Linux runtime and install suites pass race
and vet under an ordinary-user scope bounded to2CPU/1GiB/zero swap. Real owned temporary-directory adapter tests prove
rewinding after EOF, late-entry refusal, no-follow parent/source link refusal, retained-path substitution and closure;
their ownership normalization is test-only, not privileged host proof. Synthetic tests cover root ownership/ACL/mode,
API filesystems, read/access errors, zero identity, both paused audit comparisons and unchanged closed failure phases.
Old units/resources, numeric/process definitions, SQL and fixtures are unchanged. Independent final source review,
current-head CI, new protected exact-source bytes and the full no-overlay packaged VM remain pending.

PR150 CI subsequently rejected the production `connectedunits` dependency in the worker command closures. The
correction removes that production import, retains a fixed literal with test-only cross-package equality, and
requires all command dependency tests plus affected runtime/install race/vet to pass. An acyclic import alone was
not evidence of least-authority compatibility; no closure allowlist, sandbox or guard is weakened.
