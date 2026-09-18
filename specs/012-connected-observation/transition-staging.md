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

The pure staged classifier accepts only an already valid complete stage, the
actual active transition journal/receipt bytes, and caller-verified active
resource hashes plus the private ledger witness. Before publication, the
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
ledger logical or inode change, stale proposal, stop failure and concurrent
commands. A disposable supported ext4 VM must prove normal reboot and
abrupt power-off at selected in-flight boundaries with a used pending ledger;
the previously completed stopped-ledger witness is not this proof. No owner
server canary precedes independent review and these results.
