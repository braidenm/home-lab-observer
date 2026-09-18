# Compatible code selection and rollback: remaining design gate

Status: proposed implementation boundary, 2026-09-17. Independent read-only review
identified these gaps; no upgrade, rollback, release or activation approval is implied.
This completes neither [F1](installed-composition.md) nor its packaged acceptance.

## What exists and what does not

The installer retains verified immutable releases and pins one manifest digest in
the installed configuration. This is a useful basis for code rollback, but there is
no completed code-transition history identifying a previous compatible release.
The local interim refresh implementation stops and disables the owned workers
before publishing its legacy journal, so a failed stop does not leave an
uncompleted journal while workers may still run. This ordering correction does
not provide resume, staged CA retention, or crash-safe mixed-generation recovery.
It also runs the exact packaged `validate-existing-ledger` offline mode before
publication and again before completion. A changed private logical fingerprint
leaves refresh incomplete and the workers stopped; no pending request is decoded
into logs or operator output. This interim comparison does not replace the
required durable pre-transition witness or recovery codec. Both reads are now
paired with an anchored ext4 UUID, directory inode/generation and database
inode/generation comparison. The installer validates the fixed private ledger
directory and member set through no-follow descriptors before calling the
identity primitive. A substitute database with identical logical contents is
therefore refused before completion. This is an in-process comparison only;
the interrupted journal still cannot validate it after a reboot.
Directory order, timestamps and version strings must not select a rollback target.
Initial installation has no previous release to roll back to.

Three implementation gaps must be closed before claiming safe rollback:

1. The connected manifest identifies a package/profile but does not prove compatible
   ledger, pending wire bytes, credential, configuration and handoff semantics.
2. The existing offline `validate-ledger` operation checks pristine enrollment state.
   A used installation correctly fails that check. Preserve this promotion safeguard;
   add a separately named existing-state compatibility check, never weaken it.
3. Endpoint refresh completion binds the full installed configuration, including its
   artifact digest. Changing that digest independently would leave an inconsistent
   refresh journal. Do not delete or fabricate a refresh completion to permit it.

## Proposed smallest coherent transition

Use a bounded root-owned transition journal for either endpoint refresh or compatible
code selection, with exact previous/next configuration, operation type and expected
prior completion identity. Specify migration from the existing refresh journal before
implementation; accepted ADRs must not be silently rewritten.

A code-selection transition may change only the verified artifact identity and a
monotonic transition generation. Preserve owner/server/connector identity, dedicated
numeric principals, current endpoint policy, credential location and the exact ledger.
No ledger reset, copy-back, re-enrollment or backup restoration is a code rollback.
Record one explicit previous verified artifact only after a successful code selection.

The trusted current installer must verify both bundles against a frozen state-compatibility
contract. That contract must cover the durable ledger schema and sequence rules, pending
request bytes, credential/configuration formats, handoff and activation protocols.
Version labels or a package's own compatibility assertion are insufficient evidence.
The target's fixed offline existing-state validation and cross-version release fixtures
must also pass. Unsupported compatibility leaves both workers stopped for recovery.

Keep the installation lease through stop/join, preparation, exact artifact/unit/config
selection, manager reload, offline compatibility checks and durable completion. Restart
requires a fresh activation transaction; an older success record grants no permission.
Define every interrupted state and allowed recovery operation before implementing writes.

## Required review and proof

- Freeze the compatibility contract and journal migration in a focused ADR/spec update.
- Test each write/sync/reload interruption, mixed generations, unknown prior records,
  same-ledger preservation, retained pending bytes, terminal state and sequence monotonicity.
- Test compatible forward/back selection with two exact synthetic packaged releases;
  reject incompatible state and prove that initial enrollment validation remains strict.
- Include stop/refresh/code-selection contention, failed target validation, unavailable
  endpoint policy and cleanup failures. Never activate a partially switched generation.
- Run packaged VM restart, simulated power-loss and recovery checks. These are still
  prerequisites for the owner-server canary, not replaced by the activation unit tests.

Docker controls, Kubernetes and smart-device providers do not expand this work item.

## Proposed concrete contract for review

This section and [ADR 022](../../docs/adr/022-compatible-connected-code-transitions.md)
are a plan, not approved implementation or completed acceptance.

### Operations and compatibility authority

- `select-code <verified-bundle-directory> <manifest-sha256>` stages/verifies a
  target immutable release and selects it while stopped. The expected digest is
  obtained through the trusted release distribution, not copied from an untrusted
  target's self-description. Selecting the current digest is a refusal/no-op with
  no transition and no new rollback history.
- `rollback` has no target argument. It selects only the previous artifact in the
  last completed code transition, after the same compatibility checks. Initial
  installation reports `NO_PREVIOUS_CODE`; directory ordering is never consulted.
  A completed rollback records the code it replaced as the new previous target.
  Endpoint refresh preserves that pointer rather than making the current code
  its own rollback target.
- `resume-transition` has no target or policy arguments. It may complete only the
  exact retained interrupted journal. It never abandons that journal, substitutes
  a new release or restores an older ledger. Unknown/ambiguous state refuses.
- All operations retain the installation lease. The executing installer must be
  the exact current root-owned immutable installer (inode/build identity/manifest),
  not an arbitrary older executable claiming the installer role. Recovery admits
  only the journal's exact previous/next installer with the same known contract.

Introduce `observer-connected-bundle/v2` and a separate connected identity v2;
native-v2 distribution remains unchanged. Bind an exact compatibility-contract
SHA to the manifest and all three binary identities. The canonical code-owned
contract describes: D1 schema/application ID/page/journal policy and C1 sequence/
terminal semantics; pending canonical numeric-host/v1 bytes; credential v1;
installed config v1; shared handoff v1; activation v1; and the new transition
record format. Resolve/verification accepts only the compiled known contract.
No arbitrary accepted-epochs list, semver range or target-supplied migration runs.
The initial epoch requires exact reviewed resource/template bytes and the same
Linux isolation profile; changing those requires another reviewed contract.

The package descriptor and binary identity agreement catch packaging mistakes;
they do not prove program behavior. Cross-version build/fixture evidence and
trusted distribution remain mandatory. Bundles lacking the contract cannot be
selected or used as automatic rollback targets. Existing experimental v1 bundles
do not acquire compatibility authority merely because their version looks similar.

### Used-ledger validation without state rollback

Keep `validate-ledger` pristine-only. Add the closed mode
`validate-existing-ledger` to the target uploader and its exact offline policy:
dedicated uploader principal, no sockets/IPC, only the existing ledger write bind
required by SQLite recovery, and no credential/handoff/enrollment bind. Its private
input carries only the expected binding/principals; its output is a versioned
bounded logical-state fingerprint, never pending bytes or raw errors.

The trusted current uploader and the verified target uploader each open the same
existing ledger sequentially, validate/load it and close successfully without
calling any state-changing C1 method. Fingerprint canonical binding, watermark,
acknowledgement state, terminal state and exact pending metadata/body bytes.
Fingerprints must match. Used, pending and valid terminal records are accepted as
compatible; terminal state is preserved, never made resumable by code selection.
Fingerprints are private comparison data and never enter operator output, logs
or hosted status. Both workers must be joined under the lease before either read.
Ordinary age expiry must not discard a pending record during validation. Missing,
busy, corrupt, unsupported or mismatched state refuses selection.

SQLite may recover/synchronize its physical journal on open/close; do not promise
byte-identical database files. Preserve the same directory/database inode, logical
record and pending request bytes. No database copy, Provision, reset, sequence
allocation, acknowledgement, credential access or HTTP request is permitted.

### One journal, bounded history and migration

Use fixed root-owned `transition.json` (at most 16 KiB) and
`transition-complete` (at most 256 bytes). A record contains a version, closed
operation (`refresh` or `code-select`), exact previous/next configurations,
predecessor-completion SHA (or explicit initial), previous-code before/after,
known compatibility-contract SHA and hashes of the exact affected CA/hosts/unit
bytes. It also binds the pre-transition private logical-ledger fingerprint and
ledger binding/identity evidence durably before any active-file replacement;
resumed validation must match that witness, not a new in-memory baseline. Pin
filesystem identity plus directory/database inode, not a transient mount ID or
boot-dependent device number alone. The supported same-filesystem reboot mapping
must be proven in the VM fixture; ambiguous/remounted/restored identities refuse.
A receipt binds the canonical record digest. Retain one completed record,
not an unbounded history. It is a recovery witness, never a worker startup permit.

Every transition increments the existing policy generation, including code-only
selection: it is the installed execution-policy generation, not a DNS-only counter.
Code selection changes only artifact and generation, preserving identities,
principals, addresses, credential and ledger. Refresh changes only addresses,
CA trust and generation, preserving artifact and previous-code pointer. Refuse
overflow. Freeze these operation-specific comparisons before writing adapters.

Migration accepts legacy refresh files only when both exist, are exact canonical
root-owned records, their completion matches, and their next configuration equals
the current verified configuration. A no-refresh installation requires both absent.
Unknown, incomplete or mixed legacy records remain recovery-required. Preserve
legacy evidence, with its exact digest admitted as the predecessor of the first
new transition; do not rewrite its completion to match a new artifact. Once a
new completed journal exists, its predecessor link is authoritative and old legacy
files are only retained evidence, not a second current-state predicate. A legacy
manifest without the known compatibility contract still refuses code selection;
journal migration alone does not upgrade executable compatibility.

The pure migration-admission slice implements only the byte-level legacy
`observer-connected-refresh/v1` record/receipt check. It admits both exact
canonical bytes together when the record's next configuration equals a valid
caller-supplied installed configuration, and returns the SHA-256 of the exact
legacy completion as the proposed predecessor link. It rejects missing,
extra, malformed, conflicting or mixed-generation bytes. It performs no
filesystem I/O and cannot prove root ownership, durable synchronization,
worker stoppage or current package compatibility; the installer must establish
those before using the result. No transition or activation command is wired by
this slice.

For refresh, stage the exact bounded new CA bytes before publishing the journal;
the journal binds their hash. No mutable host-CA reread or fresh DNS result can
change a resumed transition. Exact journal-owned staging is bounded to one pending
generation. A staging failure before journal publication is recovery-visible,
not silently deleted or interpreted as committed. The implementation must freeze
the staging name/metadata and replacement/sync ordering in tests before writes.
The record's resource hashes are not sufficient to reconstruct bytes: retain the
exact bounded old/new CA sources required for comparison/resume, or an equally
strict immutable resource source, through durable completion. Never fetch a new
CA bundle during interrupted recovery and treat it as the recorded target.

### Ordered transaction and recovery

1. Acquire the lease; admit exact current or retained recovery state; verify
   current/target immutable bundles and executing installer. Stop/join the exact
   owned pair, then audit principals and supported host. No unknown unit is stopped.
2. Prepare typed next state/resources; run both fixed offline used-state checks;
   pin ledger identity and retain only bounded fingerprint evidence. Freeze the
   predecessor and stage any required refresh CA. No automatic enrollment occurs.
3. Publish/synchronize the complete next journal before replacing active units,
   configuration or CA/hosts. Once published, all normal start/refresh/selection
   commands refuse until this exact transition is completed. Stop/status remain
   available when owned identity can be established safely.
4. Replace only files whose exact current bytes equal the recorded previous or
   next bytes, using fixed exclusive staging and durable replace. A resume may
   finish a mixed previous/next set forward; a third value refuses. Reload the
   manager, verify the exact selected profile, repeat target ledger validation and
   compare logical fingerprint/inode before publishing completion last.
5. Return `SELECTED_STOPPED` or `REFRESHED_STOPPED`. Activation is a separate fresh
   ADR 021 transaction. Previous-code history becomes usable only after durable
   completion. A sync error remains recovery-required even if bytes look complete;
   reopening validates every artifact and synchronizes the boundary before success.

No cancellation or failure automatically switches files back. A completed code
rollback is a new forward transition in the installed generation with the same
durable ledger. Unknown state, missing target/staging, failed compatibility or
foreign manager identity remains stopped/recovery-required for explicit handling.

### Implementation order and completion gates

#### Pure journal codec slice

The first implementation slice is a side-effect-free `internal/connectedtransition`
codec, with no installer import or command wiring. It freezes the canonical compact
JSON `observer-connected-transition/v1` record (16 KiB maximum): operation,
previous/next installed configs, optional predecessor completion SHA-256,
optional previous-code before/after digests, recognized contract digest,
previous/next resource hash sets (CA, hosts, both units, installed config), and
the pre-transition ledger logical SHA-256 plus ext4 UUID/directory/database
inode-and-generation witness. Completion is exactly
`observer-connected-transition-complete/v1\n` + record SHA-256 hex + `\n`.
All hashes are lowercase SHA-256; empty predecessor and previous-code values
mean no prior completed transition/code selection, never an inferred target.

Decoding requires byte-for-byte canonical re-encoding and rejects unknown keys,
invalid config/ledger witnesses, overflow, wrong operation deltas, and malformed
hashes. Refresh preserves artifact and previous-code pointer while changing only
generation/addresses; code selection preserves addresses and all non-artifact
identity while incrementing generation, and sets previous-code-after to the
replaced artifact. The codec recomputes the installed-config and fixed-hosts
hashes from the embedded configs; resource sets remain complete even when
individual bytes do not change. This codec does not validate actual files, compiled compatibility
authority, trusted target packages, predecessor chain, or an installed ledger;
those are installer responsibilities. The existing descriptor still says
transition protocol `not-implemented`. A passing codec must not be interpreted
as permission to select, resume, roll back, or activate code.

A second pure classifier may accept a previously decoded record, caller-verified
active resource hashes and ledger witness, plus either no receipt or the exact
record-bound receipt. Every resource must equal its recorded previous or next
hash. An absent receipt is always recovery-required, even if all files still
match previous or already match next; an exact receipt is complete only if every
resource matches next. A third resource value, changed ledger or non-exact
receipt refuses. The caller must establish root-owned file identity, retained
predecessor/staging evidence, stopped workers and durable synchronization. The
classifier does not perform I/O, repair, switch a release or authorize restart.

#### Independent review refinements (still proposed)

Before implementing the codec or writes, freeze these additional recovery states:

- **Preparing:** publish a bounded preparation witness before staging resources.
  A crash before authoritative journal publication cannot reconstruct missing new
  CA bytes. A narrowly scoped `abort-preparation` may remove only verified owned
  preparation files while active resources still match the completed predecessor.
  Unknown entries or changed active resources refuse cleanup. It does not undo a
  published transition; after publication, recovery remains forward-only.
  The first pure-codec slice defines canonical `observer-connected-transition-preparing/v1`
  bytes (at most 4 KiB) with the proposed transition-record SHA-256, exact previous
  installed configuration and resource hashes, predecessor-completion SHA-256 (or
  explicit initial), and the same private logical/physical ledger witness as the
  proposed transition. It must derive from a valid proposed record and reject
  extra keys, alternate encodings, invalid hashes and mismatched config/hosts
  hashes. Pure comparison with the exact proposed record must reject a changed
  valid proposal as well as an invalid one. This is only a compact
  intent/witness; it does not retain staged CA bytes, prove filesystem ownership
  or authorize cleanup. Before any future
  abort, the installer must separately prove that no authoritative journal was
  published, the completed predecessor is exact, workers are stopped, every
  active resource and ledger witness is unchanged, and staging contains only
  known owned entries. This slice has no installer wiring.
- **Predecessor retention:** retain exact previous journal and completion bytes
  in bounded staging before replacing `transition.json`. Their hashes alone do
  not preserve the evidence needed to validate the predecessor after replacement.
- **Completed, cleanup pending:** publish and synchronize completion last, then
  verify and remove only fixed owned staging entries and synchronize that
  directory. Residual staging blocks a new transition until this cleanup succeeds.
  Never recursively discard an unrecognized directory to make the next run work.
- **Durability:** synchronize staged files, staging directory and its parent;
  synchronize the authoritative journal's parent after publication and each
  active resource's parent after replacement. Atomic rename alone is not a
  durable completion guarantee.

The narrow reboot identity profile remains an implementation/review gate. A
filesystem UUID plus directory/database inode and inode-generation witness is a
candidate; availability, anchored-descriptor queries, replacement rejection and
reboot stability need proof on the explicitly supported filesystem. Unsupported
filesystems must refuse transitions. No local identity scheme here detects a
restored/cloned filesystem preserving those identifiers: restore requires
revocation and re-enrollment, not a claimed local anti-rollback guarantee.

The [existing-ledger witness slice](used-ledger-witness.md) is only the logical
comparison primitive. It does not implement any of these lifecycle states.
The proposed [durable staging boundary](transition-staging.md) freezes the
fixed names, ordering and interruption matrix for the next installer review;
it is not implementation or release evidence.

#### Delivery sequence

1. Review this ADR/plan; freeze exact compatibility bytes and journal codec with
   pure operation/migration/previous-code tests. Do not add a generic migration API.
2. Add offline used-state mode and exact manager properties, with real D1 fixtures:
   pristine, acknowledged, pending, terminal, exhausted, corrupt and incompatible.
3. Refactor only refresh into the same fixed journal transaction and add explicit
   code selection/rollback/resume adapters. Fault-test every publish/sync/reload,
   stop failure, cancellation and uncertain completion. Keep old strict promotion.
4. Package two exact compatible synthetic releases; exercise forward selection,
   subsequent refresh, rollback and resumed mixed state with unchanged ledger
   identity/pending bytes. Refuse old contract/profile/credential/schema variants.
5. Independently review and run the full disposable-VM mount/principal/egress,
   restart, simulated power-loss and recovery matrix before any live activation.
   These gates are not satisfied by this proposed document or synthetic unit tests.
