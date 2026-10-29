# ADR 024: Immutable rich installation policy and profile-bound upload state

Status: Accepted for the unused pure state-machine slice; ledger and installed activation remain gated.
Date: 2026-10-29.

## Context

The numeric canary has a deliberately narrow compiled sandbox, 16 KiB pending-body limit and schema-v1 ledger.
The unused combined envelope can carry a numeric host document plus sensitive workload inventory. Increasing a JSON
cap cannot establish consent, installed authority, durable retry safety or server approval. The uploader currently
revalidates numeric bodies during both state-machine and ledger operations; both must understand an explicit profile.

An old pending payload is an immutable request, not a mutable dashboard setting. The current state machine can retire
an expired request and admit/send a replacement in the same Step. A live policy toggle with a separate retirement
operation would therefore require a more complex coordinated authority boundary than this beta needs.

## Decision

Use a separately enrolled rich installation with one immutable, release-defined policy for its entire identity.
Numeric installation, credential, sequence, pending bytes, paths and sandbox remain untouched. Reuse the current
upload state machine and Linux ledger, adding closed code-owned profile selection rather than a parallel queue,
pluggable validator, caller-chosen byte limit or general policy engine.

The first implementation and disposable-VM proof is **process-only**: numeric host plus enabled processes; containers
are canonically disabled. It creates no Docker principal, helper, socket access or Docker unit. The credential-free
collector may see other users' ordinary proc entries with `ProtectProc=default`; it remains unprivileged and gains no
ptrace capability. Disabled process collection retains `ProtectProc=invisible`. This is an immutable profile choice,
not a runtime toggle. Uploader credentials and network access remain separate from acquisition.

Process-only is a staged implementation proof, not a promise to install an intermediate owner-host entry. Hold the
owner's rich rollout until a useful process-and-Docker profile has its own complete acceptance, avoiding repeated
enrollment. Docker later requires another fixed immutable profile and reviewed mediation, not enabling a switch on
the process-only installation. Observation never implies control authority.

### Policy trust and retry admission

A released, pinned profile definition fixes permitted groups and sandbox. The installer creates an exclusive
root-owned local policy record from an explicit approved enrollment choice, binding identity, definition digest and
effective groups. Its canonical digest must agree with the installed record, typed handoff and upload ledger. This
is a local operator record, **not an externally signed policy**; do not invent a local signing key or treat a hash
alone as authorization. Platform independently persists and enforces the registration's approved profile/groups.

Before credential loading or network use, the installed uploader checks the local record, release definition and
effective installed binding. It repeats profile/group/digest admission before every initial send and every retry.
Missing, changed or mismatched policy latches recovery before transport; immutable pending bytes are not rewritten,
retired to bypass the mismatch, or sent under a different policy. The pure ledger validates its fixed expected
binding, but cannot authenticate a root-owned file or replace the later installed-worker gate.

### No in-place policy replacement

Changing groups requires stopping/joining and fencing the old rich workers, then confirmed revocation of the old
remote credential. Any uncertainty blocks replacement and leaves the old installation stopped. Only then may an
explicit operation create a new server/connector identity, credential, ledger, sequence and separate active root.
Never adopt/reset the old ledger, reuse a credential, copy grants or bind old pending bytes into new services.

Retain old bounded private state offline and outside every new bind mount/path. A future reviewed retirement/removal
procedure must bound retained installations; this beta does not provide an unattended replacement loop or accumulate
arbitrarily many retired roots. Dynamic reconfiguration and a generic retirement primitive are deferred.

## Storage and compatibility

Keep numeric public APIs as exact numeric wrappers and preserve their schema-v1 SQL, 64-page/256 KiB database and
1 MiB total envelope. Add explicit profile-aware APIs sharing the same lifetime lock, one connection, full-record CAS,
commit-uncertainty latch and private filesystem checks. Bind profile and policy digest even on empty/terminal records.
Rich-only schema v2 uses a 288 KiB pending ceiling, 128-page/512 KiB database and proposed 2 MiB DB+journal envelope.
No migration, adoption, auto-provisioning on open, schema repair or new dependency is included.

The profile kind is rejected from the pre-SQL database header before SQLite could recover a different kind's hot
journal. Matching-kind recovery still belongs to SQLite; later logical-binding rejection is not a promise of bytewise
non-mutation during legitimate recovery. SQL size checks happen before allocating pending bodies; wire validation
and exact expected policy binding remain mandatory afterward.

`journal_size_limit` is not an active-journal quota. The proposed rich envelope is conditional on preserving DELETE
journaling, 4096-byte pages, verified maximum page count, `cache_spill=OFF`, one fixed UPDATE per transaction, no
savepoints/ATTACH/VACUUM, and actual maximum-payload/crash/ENOSPC proof. If that proof fails, revise the profile before
acceptance rather than silently raising global limits. See [the implementation contract](../../specs/013-connected-inventory/upload-profiles.md).

## Alternatives, evidence and rollback

In-place mutable consent introduces old-pending/retry races; silently widening numeric workers violates their existing
installed contract. A new ledger implementation duplicates recovery and CAS logic. Separate immutable enrollment with
profile-bound existing machinery is the smallest bounded choice; its cost is explicit later re-enrollment rather than
seamless group toggles.

SQLite documents [page limits](https://www.sqlite.org/pragma.html#pragma_max_page_count),
[cache spill](https://www.sqlite.org/pragma.html#pragma_cache_spill),
[retained journal limits](https://www.sqlite.org/pragma.html#pragma_journal_size_limit),
[header identities](https://www.sqlite.org/fileformat2.html#database_header) and
[rollback atomicity/recovery assumptions](https://www.sqlite.org/atomiccommit.html).
These support the design, not an unexecuted storage bound or physical power-loss claim. Systemd's
[ProtectProc documentation](https://www.freedesktop.org/software/systemd/man/255/systemd.exec.html#ProtectProc=)
explains visibility, not collection completeness or ptrace authority.

Before activation, remove an unused profile as a code-only rollback. After activation, stop/fence new rich workers,
preserve their identity and bounded private state, and revoke through the supported server path if retiring them.
The independent numeric installation continues. Compatible code rollback needs its own exact-profile proof; restoring
an older ledger or changing an active-root symlink is not a supported recovery technique.
