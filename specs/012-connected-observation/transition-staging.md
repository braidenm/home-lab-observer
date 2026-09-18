# Durable connected transition staging: proposed write boundary

Status: design gate; implementation, independent review and power-loss proof pending.

This narrows the [compatible transition plan](compatible-code-transition.md).
It does not authorize installation, activation or a live migration. The first
implementation must remain Linux/amd64, root-owned and stopped-worker-only.

The pure stage-admission slice validates a caller-supplied exact inventory of
fixed file names and bytes before any installer write/resume logic uses them.
It requires a canonical preparation witness matching the canonical proposal,
bounded old/new CA bytes matching the recorded hashes, and either no new-format
predecessor or both its canonical journal and exact receipt. A retained
predecessor must lead to the proposal's previous configuration, resource
hashes, compatibility contract and code-history pointer. Extra, missing or
oversized entries refuse. This check does not establish file ownership,
durability, legacy-journal migration, ledger identity or worker state; the
installer must prove those separately from anchored descriptors.

The pure resource-derivation slice renders both installed configurations,
fixed hosts files and reviewed systemd units from the canonical proposed
record, then requires every resulting SHA-256 to match the record. It copies
the caller-supplied old/new CA inputs. This rejects a syntactically
valid record that claims different unit bytes or a substituted CA. The caller
must still establish the supplied CA's stage provenance and inspect each
actual active file through anchored ownership/mode checks; these expected
bytes are not write authority.

The Linux active-resource reader uses only the five fixed installed paths and
the root-owned, no-follow bounded file reader with exact installed 0644 modes.
It detaches each byte sequence,
then accepts only exact previous/next code-derived bytes, including mixed
interrupted states, before passing hashes to the pure classifier. This read
does not establish ext4 write durability, the private ledger witness, or
stopped-worker state. Those remain mandatory before replacement, cleanup, or
activation.

The separate Linux stage-filesystem slice provides anchored `InspectAt` and
`PublishAt` primitives, not an installer command. Both require root and an
exact root-owned config directory on ext4. Inspection permits only the fixed
private stage inventory, modes, ownership, no links/ACLs, bounded regular
files, and matching canonical proposal/preparation bytes. Publication refuses
occupied names, writes/syncs the preparation first, then creates/syncs the
private stage and each fixed member exclusively, and reopens all evidence for
byte comparison before returning. Any uncertain write or injected post-sync
interruption leaves the evidence intact and requires recovery; it does not
silently retry, delete, publish the active journal, or start workers. The
caller still owes installation lease, stopped-worker, predecessor, private
ledger, and eventual forward-recovery checks. Root-owned synthetic ext4 tests
exercise both successful reopen and failures at each durable publication step;
they do not substitute for VM power-loss proof.

The draft installer's normal `start`, `restart`, and endpoint `refresh` paths
now refuse any fixed preparation/stage marker under the installation lease,
including a dangling symlink. This is a residue fence only: it neither reads
partial evidence as valid nor chooses abort/forward recovery. The stopped
worker, predecessor and ledger checks still precede any future publication.

The pure staged classifier accepts only an already valid complete stage, the
actual active transition journal/receipt bytes, and caller-verified active
resource hashes plus the private ledger witness. The authoritative journal
and completion are read through fixed root-owned, no-follow, bounded
names under the anchored ext4 config directory. The reader preserves separate
absence of each name and refuses empty, linked, broad, foreign or oversized
files; it does not infer completion or permit cleanup. Before publication, the
journal/receipt must still be the exact retained predecessor pair (or both
absent for the first new-format transition) and every active resource must
match the previous set. After publication, the journal must equal the exact
proposal. A retained predecessor receipt is recognized as *old* evidence, not
completion of the new proposal; its unexpected absence after publication
refuses rather than inventing a missing-file transition. A new exact receipt
is accepted only with all next resources. It reports preparation pending,
forward recovery required or
completion cleanup pending. None of these is a worker startup, abort or cleanup
 permit; filesystem ownership, sync and stopped-worker proofs are external.

A read-only composition now requires the stage's decoded record to equal its
exact canonical proposal, re-derives code-owned previous/next resource bytes,
rejects active bytes outside those sets, and joins the fixed journal/receipt
with a caller-supplied private ledger witness in the staged classifier. This
returns only a diagnostic phase. Its caller must independently establish
anchored reads, the actual ledger witness, the installation lease and stopped
workers; no phase grants replacement, cleanup or activation authority. The
composition accepts detached evidence rather than importing the stage
filesystem package that also owns publication primitives.

## Fixed evidence

- `transition-preparing.json` in the root-owned config directory contains
  canonical `observer-connected-transition-preparing/v1` bytes. Its exact
  proposed-record digest, predecessor resources and private ledger witness
  are intent evidence, not a replacement for the journal.
- A single root-owned `transition-stage` directory (mode 0700) holds only
  fixed names: `proposal.json`, `old-ca.pem`, `new-ca.pem`,
  `predecessor.json` and `predecessor-complete`. The last two are absent
  only for a first transition without a completed new journal. Legacy
  `refresh.json` and `refresh-complete` remain separately intact. CA files
  are individually bounded by 1 MiB; proposal by 16 KiB; predecessor journal
  and receipt by their protocol maxima. No arbitrary path or recursive cleanup
  is accepted. The directory and config parent must be on a filesystem that
  supports the required file and directory synchronization; failure refuses.
- Active `transition.json` and `transition-complete` are fixed root-owned
  names. The stage retains exact predecessor bytes before replacing either.
  An old completion beside a newly published journal is not a new completion;
  it is recognized only after verifying the staged predecessor bytes and their
  digest. The classifier receives logical absence of a *new* receipt then.

## Ordered write and recovery states

1. Under the installation lease, verify the completed predecessor and
   existing files, stop/join both exact owned workers, and pin the private
   logical/physical ledger witness. Construct a valid proposed record, then
   publish/sync the preparation witness before staging any resource bytes.
2. Create the stage exclusively. Copy only verified exact old/new CA and
   predecessor evidence; write the canonical proposal. Sync each file, stage
   directory and its parent. Reopen and compare every byte/hash to the
   preparation witness and proposed record before authoritative publication.
3. Publish/sync `transition.json` before replacing any active resource.
   From this point, `abort-preparation` is forbidden. All normal start,
   refresh and code-selection operations refuse until exact forward recovery
   completes. A missing/foreign stage, record or ledger is recovery-required.
4. Replace each fixed active file only when its current bytes equal the
   recorded previous or next bytes. Sync each replaced file and parent.
   Reopen exact resources and ledger, verify the selected package and loaded
   manager properties, then publish/sync the new completion last.
5. With exact completion and all next resources verified, remove only
   recognized stage entries and `transition-preparing.json`, remove the
   now-empty stage directory, and sync parents. Residual staging is
   `COMPLETE_CLEANUP_PENDING` and blocks another transition; it is not
   permission to repeat the transition or start workers.

### Interrupted active-file replacement

Each of the five fixed active resources needs a proposal-bound, role-specific
temporary name in its own parent directory:
`.observer-transition-<role>-<full lowercase SHA-256 of proposal bytes>`,
where `<role>` is one of `ca`, `hosts`, `collector`, `uploader`, or `config`.
The old generic `.install-next` slot is not transition evidence and must never
be silently adopted. Create the temporary file exclusively through an
anchored descriptor, with exact root ownership, mode, no ACL and one link.
The primitive refuses any role, target, installed mode or byte bound outside
the five reviewed fixed mappings.
Write only the code-derived next bytes and sync the file. Then verify that the
active target is still exactly previous or next
and retains the same no-follow inode immediately before replacement. Rename
over the previous target when needed, then sync the parent directory.
Do not treat file sync as directory-entry durability; Linux documents that
the containing directory also needs an explicit sync
([fsync(2)](https://man7.org/linux/man-pages/man2/fsync.2.html)).

On resume after authoritative journal publication, inspect only the
proposal-bound fixed temporary names. If a temporary file is root-owned,
unlinked elsewhere, correctly confined and bounded, and the active target
still matches the proposal's previous or next bytes, discard that one
temporary file, sync its parent, and regenerate from the retained stage. A
partial temporary write is not evidence about the active target. An unknown
name, wrong owner/type/mode/link/ACL, unsupported filesystem, or active bytes
outside the exact previous/next set remains a stopped recovery refusal. A
crash after rename but before parent sync must reopen and classify the actual
target and sync its parent even if the target already has next bytes; it must
never assume that the rename persisted. After all resources
match next, exact installed modes, loaded manager properties, package and
private ledger witnesses must be rechecked before publishing completion.

Before step 3, `abort-preparation` may clean only a verified preparation
whose authoritative journal was never published, whose active resources and
ledger still match the completed predecessor, and whose stage contains only
known owned entries. It must never silently re-resolve DNS, reread host CA,
replace a proposal, restore a ledger or follow a symlink. A missing fixed
entry is not reconstructed from the current host. Every uncertain sync,
rename, manager reload or cleanup result stays stopped for explicit recovery.

## Acceptance matrix

Fault-inject every file create/write/sync/close, stage/active rename,
directory sync, manager reload and completion/cleanup boundary. For each
interruption, reopen from disk in a new process and assert one of:
abortable unchanged preparation, exact forward-only recovery, completed
cleanup pending, or explicit refusal. Test old/new/mixed resources, retained
old receipt, unknown entries, wrong ownership/link/mode, CA substitution,
proposal-bound partial/foreign temporary files, power loss on each active
rename/parent sync, ledger logical or inode change, stale proposal, stop
failure and concurrent commands. A disposable supported ext4 VM must prove
normal reboot and abrupt power-off at selected in-flight boundaries with a
used pending ledger;
the previously completed stopped-ledger witness is not this proof. No owner
server canary precedes independent review and these results.
