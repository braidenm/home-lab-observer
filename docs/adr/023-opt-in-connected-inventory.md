# ADR 023: Opt-in rich connected inventory profiles

Status: Proposed; first contract and installed-profile review pending.
Date: 2026-10-29.

## Context

The owner installed numeric canary 1 successfully and requested the originally intended system/workload dashboard.
Local process and Docker collectors exist, but their observations are not upload-eligible. The accepted connected
profile cannot see foreign processes or open Docker sockets. Its compiled compatibility contract also prevents a
code-only update from quietly changing installed isolation. Legacy receiver container metrics cannot represent nulls.

## Proposed decision

Retain numeric-host/v1 unchanged and add explicit versioned workload egress contracts, host-local opt-in and a
separately accepted rich observation runtime. Reuse normalized collectors, not local JSON or Docker inspect responses.
Names and image references are sensitive metadata even after text validation. Keep current workload rows latest-only;
add aggregate numeric history separately. No raw logs, commands, accounts, connections, environment or mount paths.

Keep acquisition separate from the authenticated uploader. A fixed-operation Docker reader still possesses broader
daemon authority; its small local mediation boundary is trusted code, not a claim that GET-only methods remove that
authority. Review its concrete socket/process confinement before release. Never give Docker handles to the browser,
uploader or numeric worker. Management remains separately disabled under Spec 011 with independent credentials/grants.

Do not select the final profile-changing migration by convenience. Compare explicit validated replacement/coexistence
with extension of the existing installed transition protocol; preserve credentials, used sequence and pending bytes.
The current canary remains intact until exact-profile packaged acceptance and owner opt-in. This proposal supersedes
none of ADRs 012, 020, 021 or 022 and does not authorize widening their compiled profile.

## Alternatives and fit

| Option | Benefit | Limitation |
| --- | --- | --- |
| Keep numeric native and use legacy Compose separately | Existing rich inventory path | Two limited installs, legacy mandatory metrics, not a rich standalone native product |
| Serialize the local snapshot and loosen the current sandbox | Fast-looking integration | Silent privacy/authority expansion, broken installed admission and unknown-field leakage |
| Explicit rich profile using existing adapters | Fits native/headless product and owner control | Requires actual pipeline, install/migration and receiver proof |

Recommend the third option; keep the first usable during rollout and reject the second. A separate contract is not
itself another daemon. Introduce only runtime roles justified by concrete privilege separation, not one service per
data group or an unnecessary general plugin platform.

## Evidence, consequences and rollback

See [the dated research](../architecture-research/008-connected-inventory.md) and
[Spec 013](../../specs/013-connected-inventory/spec.md). Existing gopsutil and fixed GET-only Docker adapters are reused;
no library upgrade is hidden in feature work. Inspect the actual Engine API range before promising compatibility.
PrivateNetwork changes observation scope and ProtectProc can hide other users' processes; a successful collection
inside a sandbox does not prove full host coverage. Test that distinction in an actual packaged VM.

Contract-first tests prove allowlists, boundedness, nullable metrics and compatibility; VM/daemon/owner acceptance
prove deployment. Default-off local groups and unchanged legacy/numeric profiles provide staged rollback. Disabling
new collection does not reset upload identity or erase unresolved control audit. Final installed profile and rollout
method remain engineering decisions requiring independent review, not additional inferred owner commitments.
