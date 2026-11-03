# Retained status after two-sided recovery finalization

2026-11-03. **Proposed paired implementation clarification; review required before code.**
Extends the existing admin/journal contracts without a new wire member, enum, command, migration or authority.
The four-route Platform execution cut remains separate; it does not implement finalization or relax its readers.

## Actual gap

The current Observer RetainedStatus decoder requires HELD for OUTCOME_UNKNOWN and EXPIRED_UNCONFIRMED.
Platform's actual retained-fact/coordination validator also rejects either immutable fact state with RELEASED;
its store rejects the expired/unconfirmed combination even before DTO construction. No finalizer exists yet,
so there is no legitimate published post-finalization state today. Merely widening a decoder would not fix
the source query, and releasing a lane cannot rewrite an original fact as success or confirmed expiry.

V39 already defines the separate recovery, approval and finalization tables. The future recovery delivery must
validate those actual records and their CAS alongside the unchanged aggregate fact. An enum/string, detached
projection, successful HTTP response or naked RELEASED row is not finalization evidence.

## Paired read semantics

RetainedStatus.state remains the actual immutable fact fold. RetainedStatus.lane_state remains current
operational coordination. After a VALIDATED committed finalization, the two additional observational combinations
are OUTCOME_UNKNOWN/RELEASED and EXPIRED_UNCONFIRMED/RELEASED. Request/claim/result remain the same actual retained
objects; no outcome/time/sequence is rewritten. CLAIMED/RELEASED and REQUESTED/RELEASED remain invalid.

Before returning either new combination, Platform's module-owned query must prove, under the same registration
fence and original physical participation, all existing retained-source/row equality plus:

- The exact original request, receipt digest, owner/server/installation and action bind the recovery record.
- Its selected actual approval and immutable finalization bind the same recovery/action/digest/ACK, approved
  owner decision and prepare identity; renewal follows the existing same-ACK rule.
- The original proposed/approved HELD lane and released lane are the exact guarded successor, without overflow;
  admission and runtime agree with the stored released revision and unchanged fact sequence/phase.
- The finalization was committed together with that exact lane release. Missing, corrupt, foreign or partially
  committed evidence is UNAVAILABLE, never an invented HELD/RELEASED or successful finalization.

The proof is a specific consumed recovery query, not a callback, exported permit or generic optional validator.
Existing schema shape alone is insufficient. All read paths that share retained status must use this validation;
no owner/worker path may bypass it through a looser DTO/store. No database or network I/O enters pure sourcing.

Observer's closed observational decoder can recognize these two combinations once the paired contract/vectors
are reviewed. That recognition grants NO proposal, ACK, dispatch or release authority. INIT and fresh recovery
proposal continue to require UNKNOWN/EXPIRED_UNCONFIRMED with HELD. No other state/lane combination is widened.

## Repeated CONTINUE and explicit STATUS

The fixed CONTINUE order is unchanged: RETAINED1, STATUS2, then CHECK3/FINALIZE4 only when a HELD recovery still
needs finalization; at most STATUS5 follows ambiguous FINALIZE. The independently joined session can end after
STATUS2. It must not issue CHECK or repeat FINALIZE merely because a previous reply was lost.

If RETAINED1 reports one of the validated released combinations, the root operation still fences/joins and
validates its exact protected mapping/source. It may return historical ACKED only when the actual immutable
local ACK exists AND fresh STATUS2 is FINALIZED, matches the selected current approval/recovery/prepare/revision,
and MatchFinalization binds that approval to the actual local ACK digest and exact released lane successor.
No new Inspect, CHECK, ACK write/import, finalization mutation or clock/approval renewal occurs in this branch.
Missing/mismatched ACK or finalization refuses; remote RELEASED/FINALIZED alone never manufactures local ACKED.

Standalone STATUS remains read-only and available for any uncertain prior operation. It reports local and
remote observations separately. Any UI or CLI claim that both sides are finalized must require the same actual
ACK digest/finalization match; neither local ACKED nor remote FINALIZED alone proves the other's state. A
mismatch remains explicit RECOVERY_REQUIRED, never automatic repair, second ACK or resumed execution.

Normal local-ACK/remote-HELD renewal still uses the original immutable ACK bytes and a deliberately renewed
owner approval bound to that digest. No requirement makes its old ACK prepare ID equal the renewed prepare ID.
Already-finalized lookup has no fresh release side effect and does not demand a newly fabricated owner decision.
Current ACTIVE ACTION/registration and immutable binding remain necessary; no observation-token substitution.

## Required source-first proof and rollout

Review the paired wire text and canonical positive/negative vectors FIRST. Then prove actual consumers:

- Platform actual finalization COMMIT atomically releases the exact lane, preserves original unknown/expired
  facts, and permits validated owner/worker retained readback. Forged released rows, missing/wrong finalization,
  wrong approval/ACK/revision, partial-commit and Long overflow refuse.
- Observer decoder accepts ONLY the two added observational combinations; INIT still rejects released input.
- Consumed real SQLite/Unix CONTINUE after lost FINALIZE returns the same historical ACK only after matching
  fresh STATUS; zero new inspection/ACK/check/finalize. Wrong selected approval or actual ACK digest refuses.
- Historical finalized lookup after approval expiry validates the original finalization time against that
  approval; it does not demand a currently unexpired approval or create another owner decision.
- Standalone STATUS and local-ACK/remote-HELD renewal distinguish both partial states without auto-resume.

Keep the current four-route execution cut's UNKNOWN/HELD contract unchanged until the consumed recovery facade
supplies this proof. Reader/source compatibility for BOTH candidate and fallback precedes the first finalization
write and any new released/unknown state. Package/installed/paired/disposable acceptance remains mandatory.
This document authorizes no host action, production flag, credential, new floor marker or canary.
