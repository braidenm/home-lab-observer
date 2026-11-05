# Immutable public approval set for the first connected profile

2026-11-04. Implementation slice under owner-approved ADR034; no installation or activation approval.

## Specification and rationale

The first locked-outcome profile does not support target replacement, policy edits, re-enrollment or new aliases as
recovery. Its existing `RunWorker` calls `RefreshSnapshot`, which reads the central head and currently validates
only binding before a publication. Tighten this consumed path: an existing head must describe the exact proposed
public entry set before any POST. This prevents a restart from silently replacing the central target/revision set.
It is not a substitute for current owner/operation authorization, private full-ID mapping or durable local locks.

Freeze the WHOLE public set: targets, aliases, operations, parameters/digests, policy/capability revisions and count.
Cooldowns are observational, not part of that immutable set. First publication remains possible only after the
existing verified null head; that response cannot independently prove that the server never lost old state.
Missing/corrupt storage and unsupported installation/binding changes still require separate refusal proof.

## Plan

Reuse `MatchSnapshotHead`, already called between the authenticated GET and single POST. Recompute the existing
`SnapshotDigest` using the proposed entries and the retained head revision/advertised/expiry times, then compare
the authenticated head digest. Publication ID is not in that preimage; use a valid fixed placeholder, not a new
nonce or persistent file. Expired UNAVAILABLE heads retain enough metadata even though their entries are hidden.
Reject malformed partial head metadata without panic. Keep exact binding checks and existing CAS: concurrent
changes can still reject the POST, which must never be retried automatically. No wire/schema/dependency changes.

This deliberately removes policy-edit refresh from the first profile; a future reviewed mapping-edit/recovery
release must supersede this restriction. It does not contain a compromised trusted worker using the API directly,
nor bind private Engine generation/full IDs. Root trust and supported-flow limits remain those of ADR034.

## Acceptance and tasks

- [x] RED: changed target/alias/operation/grace/revisions/count reached POST through the real TLS client before the fix.
- [x] GREEN: null first head, unchanged READY/EMPTY and expired UNAVAILABLE heads retain exact one-POST CAS behavior.
- [x] Cooldown-only differences do not change the digest; partial/malformed head refuses without panic.
- [x] Concurrent CAS conflict and lost publication response remain finite failures with no retry.
- [ ] Focused protocol/client/worker tests, Linux race/vet, independent review and current-head CI before merge.

No Engine call, migration, new domain fact, journal reset, credential or host/flag change is part of this slice.

Local verification: all protocol/client/worker tests and vet pass on Windows; the same three packages pass ordinary-user
Linux race tests and vet (1.812s, 2.133s and 7.023s respectively). No existing test assertion or timeout was relaxed.
