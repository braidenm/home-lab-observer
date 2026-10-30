# Closed Docker promotion and acceptance

Date: 2026-10-30. Status: root and independent design review approved; implementation and exact-head CI pending.
No candidate dispatch, build, VM, release or owner operation is authorized by this source slice.

## BOUNDARY
PR-A merged as 7fa9afdb6a31e36da52134088d049545183d8710. Root independently
read the new fixed workflow registration: ID 401377430, active, canonical path
.github/workflows/connected-linux-docker-candidate.yml. Pin that literal before
implementation; no None/zero/name-only/CLI override. Numeric ID 397507617 stays.

## ONE COHERENT SOURCE PR
1. _connected_promotion_core.py: extract profile-neutral pure JSON/ZIP/certificate,
   provider-record/evidence-diff and release-state mechanics. Reuse the actual two
   immutable, identity-allowlisted candidate presets from PR-A, not a new registry.
   Numeric and Docker policy facades bind their respective preset; no caller policy.
2. _connected_promote_core.py: move the existing bounded process/provider/Git,
   filesystem staging/version lock, prepare and release state machine once.
   A private fixed if-identity binding chooses the two code-owned candidate/policy
   facades. Never accept caller modules, contract files, destinations or URLs.
3. Keep connected_promotion_policy.py and connected_promote.py numeric public
   signatures, defaults, return values, CLI markers, record/body bytes and existing
   tests. Preserve Provider constructor and injected runner/fake seams. Add only
   connected_docker_promotion_policy.py and connected_docker_promote.py facades.
   Numeric validate_provenance still returns None; a private certificate matcher
   returns its matched verified statement for the Docker-specific SPDX check.
4. Add closed Docker receipt/provider/wiring/race tests and operator documentation.
   Do not edit packer, bootstrap templates, units, installed identities or budgets.
   Bootstrap identity/resource verification remains the exact PR-A verifier using
   the six resource and three bootstrap Git blobs at candidate source S.

## PROPOSED EXACT ACCEPTANCE RECORD
Path: docs/acceptance/connected-linux-docker/<version>.json
Schema: observer-connected-docker-acceptance/v1
Bounded duplicate-free JSON <=32768 bytes; fixed keys, SHA256 lowercase64, integer
IDs/sizes exclude booleans, fixed UTC timestamps, no extra observations/paths/logs.
Top-level keys: schema_version, candidate, candidate_sha256, artifacts, vm.
candidate: complete canonical Docker candidate metadata, workflow_id=401377430.
candidate_sha256: SHA256 of actual canonical candidate.json bytes, including LF.
artifacts: exactly primary and independent, each exactly id,name,sha256,size,
expires_at. Positive distinct IDs; primary name from the fixed candidate and
independent name is exactly primary name + '-independent'. ZIP digests may match:
independent builders can legitimately produce identical enclosing ZIP bytes.
Both expiries must be future and match provider records exactly.
vm keys: scope, source, image_sha256, receiver_sha256, probe_sha256,
harness_sha256, baseline, docker_inputs, cases, all_pass, cleanup.
scope = rich-docker; source equals candidate.source S.
baseline keys: build_record_sha256, numeric, processes.
numeric/processes each have exactly archive_sha256, manifest_sha256,
installer_sha256, checksums_sha256. These bind both four-file same-S baselines;
the reviewed build-record hash binds source bundle/toolchain and full31 inventory.
Docker candidate's public four assets are already bound by candidate.assets;
the operator must compare their exact bytes to the local same-S Docker baseline.
docker_inputs keys: docker_ce_sha256, docker_ce_cli_sha256, containerd_sha256,
fixture_sha256, rootfs_sha256. Require the three code-owned DEB pins in the frozen
docker_scope.py and the separately reviewed source-built fixture/rootfs pins.
cases keys are exactly rich-docker and rich-docker-refusal. Each case has exactly
first_boot, reboot, case, all equal PASS. Closed mapping to existing harness:
rich-docker -> HLO_VM_DOCKER_FIRST_BOOT / HLO_VM_DOCKER_REBOOT_PASS /
              HLO_VM_CASE_PASS rich-docker.
rich-docker-refusal -> HLO_VM_DOCKER_REFUSAL_FIRST_BOOT /
                      HLO_VM_DOCKER_REFUSAL_REBOOT_PASS /
                      HLO_VM_CASE_PASS rich-docker-refusal.
all_pass = PASS maps only to HLO_VM_ALL_PASS after both cases.
cleanup keys qemu,mounts,loops each PASS: independently confirm no owned live
process/mount/loop resources, while retaining regular evidence disks. These are
reviewed operator postconditions, not fabricated existing serial markers.
No dozens of granular self-asserted feature flags. The pinned twelve-file harness
owns granular privacy/isolation/retry checks; any fail/not-run/unknown refuses.
This record is reviewed evidence, not an unforgeable VM attestation or activation.

## PREPARE / COMPARISON / CURRENT ATTEMPT
Docker prepare accepts run-id, primary artifact-id, independent-artifact-id and a
NEW output directory. Explicit IDs avoid introducing artifact-discovery pagination.
Read each exact provider record, validate fixed names/run/source/repository IDs,
digest/size/expiry and reject identical IDs before download. Latest run must be
successful/completed, exact owner/trigger/main/source/workflow and current attempt.
Read fixed /actions/runs/R/attempts/N/jobs?per_page=100&page=1: require the complete
five unique jobs authorize/build/independent/compare/attest, all successful,
same documented head_sha/run_id, from that exact attempt endpoint; refuse
unknown/missing/duplicate/skipped/failed/cancelled. Do not require an invented
per-job run_attempt field: the official response example has no such property.
This is corroborating provider evidence, not independent authority over workflow
code: reviewed S and its certificate-bound successful graph remain mandatory.
Download both exact ZIPs once, verify each external digest/size, closed five-file
inventory and metadata. Validate both PR-A Docker payloads using trusted Git blobs,
then literally compare all five files. Never substitute the primary ID twice.
Docker ZIP limit = payload64MiB +1MiB overhead, matching PR-A comparison; numeric
ZIP limit remains64MiB. Decompressed inner tar cap remains838982144 streaming.
Write only verified primary payload + candidate-identity.json in a new private
directory; include both public artifact records. Failed directories are retained,
not adopted on retry. No downloaded candidate or installer is executed.

## CRYPTOGRAPHIC VERIFICATION
Use the stored native gh credential, same bounded fixed argv and full certificate
checks. Require SLSA provenance v1 for all five retained candidate files.
For archive SPDX only: gh cryptographic success -> SAME matched certificate-bound
statement, exact https://spdx.dev/Document/v2.3, sole archive name/SHA256 subject ->
validate THAT statement.predicate with the four-role/hash/project/stdlib/core-module
guard, against the verified manifest and source-S go.mod. Never use another result's
predicate, a newly scanned/local SBOM or an unretained SBOM file/provenance subject.
Keep provider JSON/verification output <=1MiB and bounded subprocess timeouts.
Actual46KB fixture fits. Preserve numeric legacy SPDX scope as compatibility debt.

## PUBLISH / RETRIES
Keep exact clean current-main owner/provider checks, no replace refs and no grafts;
prove S ancestor and raw diff exactly one regular100644 addition at the Docker
canonical path. No runtime/doc/workflow change may hide in the acceptance interval.
Keep the SAME version-keyed common-Git-directory lock across profiles, because
both publish into the same global v<version> tag namespace. No per-profile race gap.
Before EVERY mutation recheck main, immutable setting, latest run/attempt and jobs,
BOTH artifact records/expiry, tag/source, exact draft/body/flags and asset authority.
Same closed CREATE_TAG/CREATE_DRAFT/UPLOAD_MISSING/PUBLISH/VERIFY_PUBLISHED states;
no overwrite/delete/clobber or caller text. Download draft bytes independently;
compare authority fields excluding harmless counters; publish non-latest immutable
prerelease only, then anonymously verify all four exact public sizes/digests.
Wrong profile/version/body/tag/artifact state refuses and remains for diagnosis.

## FIXED FACADE API / FILE OWNERSHIP

The pure-policy slice owns only `_connected_promotion_core.py`, the existing numeric policy facade,
`connected_docker_promotion_policy.py` and its new focused test file. All numeric public functions remain.
The Docker facade retains the same helper names and primary `validate_artifact`/`unpack_candidate`, adding only
`validate_independent_artifact(info,record)`, `unpack_independent_candidate(bytes,record)`,
`validate_attempt_jobs(response,record)` and `validate_archive_spdx(results,candidate,manifest_bytes,required_modules)`.
Generic Docker `validate_provenance` admits SLSA only; the dedicated archive-SPDX function is its only SPDX path,
returns None on success and validates the predicate of that same matched sole-archive statement. Numeric generic
provenance keeps its existing two-predicate behavior and None return. Export a fixed SPDX predicate constant if needed.
The operator owns all other implementation/CLI/docs/wiring files and alone stages/commits the completed slice.
No parallel writer edits, stages or commits another slice's files.

## TEST / IMPLEMENTATION GATES
Root/peer approve this schema/factoring before code. Then new isolated tree from
merged PR-A main, TDD and unchanged numeric suites/golden/body signatures first.
Add matched-SPDX-vs-unrelated predicate, sole-subject, four-role coverage, rerun,
missing/failed comparison job, self-ID/name, archive/ZIP tamper, expiry, source-only
receipt diff, credential environment/fixed argv, every pre-mutation race and draft
counter, Windows stored-auth prepare, anonymous postcheck and CLI wiring tests.
No new Spring/Go runtime context, external-state mutation or expensive PR VM test.
After B and ALL source prerequisites merge, freeze final S; only then candidate,
same-S baseline comparison, both VM cases, canonical receipt and promotion.

## PRIMARY SOURCES CHECKED (2026-10-30)
https://cli.github.com/manual/gh_attestation_verify
https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations
https://docs.github.com/en/rest/actions/artifacts?apiVersion=2026-03-10
https://docs.github.com/en/rest/actions/workflow-jobs?apiVersion=2026-03-10#list-jobs-for-a-workflow-run-attempt
https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases
Source read: existing numeric policy/promoter/runbook; Docker run_vm/scope/guest;
same-source31-asset builder record shape; exact bootstrap render/templates.
