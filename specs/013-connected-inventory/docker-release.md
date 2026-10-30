# Closed Docker candidate and exact-byte publication proposal

Date: 2026-10-30. Status: approved PR-A implementation in progress; source review and CI pending.
No candidate dispatch, build, VM acceptance, public release, catalog or owner activation is authorized here.

PR-A now implements the fixed candidate source and cheap regression gates. The numeric wire/defaults remain v1;
the new Docker workflow is manual-only and has not been dispatched. Its registered workflow ID and prepare/promote
entry points remain PR B, not a fallback to the numeric workflow or publication policy.

## Outcome and owning contracts

Finish the release tooling before the next complete packaged VM run, so that its final source identity includes
the reviewed unit correction, candidate workflow, verifiers and promoter. Test that exact source/artifact once;
do not accept an old `4eee2be` package plus a diagnostic overlay as a release candidate.

[ADR 004](../../docs/adr/004-independent-release-supply-chain.md) owns independent public delivery;
[ADR 024](../../docs/adr/024-immutable-rich-installation-profiles.md),
[ADR 025](../../docs/adr/025-isolated-installed-docker-observation.md) and
[ADR 026](../../docs/adr/026-docker-reader-startup-barrier.md) retain immutable enrollment, Docker authority and
same-process startup boundaries. Reuse Spec 012's [release model](../012-connected-observation/connected-release.md),
[candidate contract](../012-connected-observation/candidate-build.md),
[promotion policy](../012-connected-observation/promotion-policy.md) and
[owner-operated publication](../012-connected-observation/owner-promotion.md) through a fixed Docker preset.
No new ADR, publisher credential, generic profile registry or second publication framework is proposed.

## Two coherent source PRs, then one frozen release source

1. **PR A — protected Docker candidate:** add a fixed
   `.github/workflows/connected-linux-docker-candidate.yml`, named Docker candidate validation/CLI and focused tests.
   It can build, compare and attest, never publish or install. Add the contract/progress documentation now, not after
   final candidate creation. Keep the numeric workflow, metadata v1, defaults and accepted files strict and unchanged.
2. After PR A merges, read its registered workflow numeric ID from the fixed provider path. This is a read, not a
   dispatch. **PR B — owner prepare/promote:** bind that exact ID in the fixed Docker promoter/policy, add its closed
   acceptance receipt and operator instructions, and run synthetic provider/race/wiring tests. No zero/None runnable
   fallback, arbitrary workflow override or candidate dispatch before the pin is merged.
3. After both PRs and the unit correction merge, pin their complete clean current-main commit **S**. Only then dispatch
   the protected Docker candidate and build the complete local VM baseline from S. Candidate creation/preparation
   require exact current main. Publish no older candidate merely because its runtime diff seems harmless.
4. Freeze S until proof is finished. After both cases pass, merge **only** the canonical
   `docs/acceptance/connected-linux-docker/<version>.json` addition. The promoter must prove S is an ancestor and the
   entire S-to-current-main diff is exactly that one regular 100644 addition. No task/index/spec/workflow/test edit
   is permitted in the interval. Any other change requires a fresh candidate and matching exact-byte proof.

Both source PRs must compile/test independently. Shared implementation may factor profile-neutral bounded I/O,
provider/Git/ZIP and release-state mechanisms behind two code-owned named presets; it must not accept caller-chosen
contracts, paths, roles, destinations or dynamically imported policy. Preserve numeric functions/default behavior and
golden v1 bytes with regression tests. Do not duplicate the entire promoter just to change names and one contract.
The exact factoring boundary is an implementation-review gate, not permission for a generic plugin interface.

## Fixed Docker candidate contract

The preset is only `RICH_PROCESSES_DOCKER_V1`, definition
`86f39c7ba444f5ef4560ba82cbf660a17c0d7810b6e49443cb509d9e85036f37`,
Ubuntu 24.04/systemd 255/Linux amd64 explicit-start beta. It never admits numeric or process-only archives.

The external download contract remains exactly four flat public files, in order:

1. `home-lab-observer-connected-rich-processes-docker_<version>_linux_amd64.tar.gz`
2. `connected-manifest.json`
3. `install-connected-rich-processes-docker-linux.sh`
4. `SHA256SUMS`, containing exactly archive/manifest/installer hashes in that order.

A separate canonical `candidate.json` uses `observer-connected-docker-candidate/v1`; it binds the fixed repository
and owner numeric IDs, new fixed workflow path/registered ID, main ref, exact owner/triggering actor, run/attempt,
S, bounded unused canary version, fixed profile/definition and each ordered asset size/SHA-256. It has no timestamp,
host path, enclosing artifact digest or credential. Keep metadata at most 4 KiB and the five-file payload at most
64 MiB; manifest at most 16 KiB, installer at most 64 KiB and checksums at most 4 KiB. Do not widen numeric limits.
Use distinct attempt-bound Docker artifact names; the independent comparison artifact is not a publishable candidate.

Validate the actual archive, not just the outer checksum: exactly ten unique manifest entries (four executable roles
and six fixed resources), plus the embedded identical manifest, means eleven regular tar members. Check canonical
names, modes, headers, resource bytes/digests, source/version/profile/definition and all four ELF identities. Preserve
existing per-binary/resource limits and explicitly bound aggregate streamed decompression; reject duplicate/unknown
members, links, path traversal, trailing/noncanonical data and mixed-source or substituted-role binaries. No host
execution or unbounded extraction of a candidate is allowed.
Trust the six resource blobs and three fixed bootstrap-source blobs at S, not the caller's working-tree line endings.
The verifier reproduces the installer template as bytes and compares it exactly; it never runs that installer.

## Independent builders and attestations

Use two separate fresh GitHub-hosted Ubuntu 24.04 jobs, pinned supported Go/actions and the same S/version/target.
Neither may download or reuse the other's compiled output or Go build cache. Verify module content and keep the
second builder's cache fresh; do not describe the existing same-builder/shared-cache two-pass equality as independent
reproducibility. Each may reuse the existing Docker four-role build script unchanged. This is independence of execution,
not a different trust organization or independently implemented compiler.
Initialize each fresh cache through a first run step using the runner-provided `RUNNER_TEMP` and `GITHUB_ENV`, before
Go setup; reject existing entries. Do not use the unavailable `runner` expression context in job-level `env`.

A required comparison job downloads their exact immutable artifact IDs and compares all four final public files
byte-for-byte. If both generate deterministic candidate metadata, compare that fifth file too. Missing, failed,
cancelled, stale-attempt, changed or incomplete comparison fails the candidate. Only the primary exact artifact name/ID
can enter preparation. Keep comparison in the reviewed workflow graph, not a self-asserted acceptance boolean.
Immediately before the SBOM/attestation steps, one `verify-current` gate must correlate fresh provider authorization
with downloaded metadata's exact source/version/workflow/run/attempt. A failed-job rerun cannot sign prior-attempt
build bytes under a newer invocation; separate successful authorization and payload checks do not establish this binding.
Require positive, distinct primary/secondary artifact IDs and distinct fixed names/output mappings. Download each
builder's own upload ID exactly once, never one builder's ID into both comparison directories. Identical IDs or names
refuse even when bytes match; synthetic workflow/adapter negatives must demonstrate this anti-self-comparison gate.

Only after comparison passes may the attestation job scan the primary actual archive with pinned Syft. A real
four-ELF fixture must prove coverage, each ELF's manifest hash, outer archive digest, four main modules/Go runtimes
and the discovered dependency inventory plus required trusted core-module versions; an empty/outer-only scan or
numeric three-ELF inventory refuses. This discovery is not exhaustive dependency coverage or a reachability proof. Run
reachable vulnerability analysis for all four command roots, without executing generated commands.

Keep build/comparison jobs contents/actions read-only; only the attestation job receives narrowly scoped OIDC and
attestations write, with no contents/release write. Attest every final candidate subject with SLSA provenance v1 and
retain the existing certificate/source/ref/numeric-ID/hosted-runner/run-attempt checks. The one SPDX
`https://spdx.dev/Document/v2.3` attestation has **only the archive** as its subject: that is the artifact Syft describes.
Do not attach that archive SBOM to the installer, checksums, manifest or candidate metadata. The five retained candidate
files receive provenance; the signed archive SPDX predicate authenticates its SBOM contents. Raw SBOM bytes are not
a sixth retained file or a separately retained provenance subject, and later verification must not claim otherwise.
Provenance proves origin, not runtime acceptance or independent package signing.

The existing numeric publication policy's broader legacy SPDX subject scope is compatibility debt for a separate
follow-up, not a best-practice template. Do not silently alter old numeric attestations or numeric v1 acceptance here.

## Prepare, VM and public receipt

The Docker prepare path verifies registered workflow/run/attempt, successful comparison-dependent graph, artifact
ID/digest/name/size/expiry from the provider, closed ZIP/payload and the exact attestation set below.
The exact verification set is provenance for all five retained files plus one archive-subject signed SPDX predicate;
validate its authenticated four-ELF/module coverage, not an unretained SBOM file or a fabricated SBOM-file provenance check.
It writes only a new private local directory; it does not execute files, enroll, stage a VM or mutate GitHub.

From the same S, reuse the complete local baseline builder for numeric/process/Docker packages, receiver, prerequisite
probe, twelve-file harness and synthetic Docker workload. Compare every protected Docker public byte to the local
Docker baseline before VM use. Pin all baseline fixtures and the signed Canonical/Docker inputs separately. Both
protected clean builders must already agree; local shared-cache equality is not their substitute.

Run the existing two-case [Docker VM proof](docker-vm-proof.md) with the protected candidate's exact Docker bytes,
same-S numeric/process inputs and no diagnostic overlay/probe. Keep two fresh overlays, both first boot and reboot,
no NIC/host share, 2 vCPU/3 GiB/12-GiB overlays and existing bounded marker/shutdown checks. Require numeric coexistence,
existing-rich-root pre-mutation refusal, actual rows/null metrics/privacy, role/socket/credential denials, paused-reader
audit, socket replacement with the same held invocation, policy-bound pending bytes, exact reboot retry and cleanup.

The canonical `observer-connected-docker-acceptance/v1` JSON binds the full candidate metadata/digest and independent
provider artifact identity, exact image/Docker package/synthetic fixture/receiver/probe/harness/baseline pins, and a
closed per-case/result vocabulary. Every required result must be PASS; unknown/missing/failed/not-run refuses. Record
only bounded public hashes, identities and finite evidence, never raw console, rows, names, grants or credentials.
Specify the exact receipt field/result vocabulary in PR B before S freezes, not after seeing a failing run.

## Promotion, activation and proof requirements

Reuse stored native GitHub CLI authentication, token/debug isolation, fixed shell-free endpoints and bounded output.
Retain the version lock, replacement/graft refusal, clean exact-main/immutable-setting rechecks before every mutation,
draft/source/asset authority checks, closed retries, no overwrite/delete/clobber, independent draft-byte downloads,
non-latest immutable prerelease and anonymous credential-free final byte verification. The Docker receipt path and
workflow identity must never be accepted by the numeric promoter or vice versa. Evidence-only ancestry is mandatory.

Before implementation approval, review the named preset factoring and receipt fields. Before delivery, tests must
prove unchanged numeric golden bytes/behavior; cross-profile substitution refusal; four-role/ten-entry archive and
actual Syft coverage; both builder outcomes/comparison; exact attempt/source/artifact binding; provider CLI wiring,
mutation races, stale main, changed assets, expiry and anonymous postcheck. PR CI stays cheap; builds/VM/publish remain
manual protected/operator work, not private PR runners or production installation.

This remains `UNSIGNED_BETA_WITH_CHECKSUMS_AND_PROVENANCE`. Public publication does not activate Platform's catalog,
feature intent, rich enrollment/writer or owner server. Those retain reader-first deployment, consent and live
owner/viewer/share/revoke acceptance. Docker controls and Kubernetes remain out of scope.

## PR-A local source evidence (2026-10-30)

- The existing numeric golden metadata SHA-256 remains
  `d53709af7315321a622170312d9e8f7f9c5ed8f4585fd0cfa699ab1dd15604ba`. Numeric workflow bytes, public signatures,
  defaults and legacy publication policy are unchanged; the shared core accepts only its two actual immutable presets.
- Linux runs all 66 candidate/promotion regression tests without skips, including strict archive streaming, boundary
  identities, isolated CLI imports, hostile caller Git-environment refusal, literal five-file comparison and provider
  self-comparison/expiry/source refusals. The archive boundary received independent source review; the complete
  PR-A source and exact-head CI are separate pending gates.
- As a read-only verifier fixture, the old `4eee2be` four-role archive passed and its 24,331-byte installer was
  reproduced exactly from that source's fixed Git blobs. This is neither a build of current S nor VM acceptance.
- Verified Syft 1.55.0 scanned that old archive under bounded offline execution. Its actual 46,964-byte SPDX output
  SHA-256 is `82b4becd0e26fc91a7c573438239af6252147beb58ec9e45010d9ce5bbcd076c`; the guard accepts four exact ELF
  hashes, four project/stdlib occurrences and required gopsutil/x/sys/sqlite versions. The committed fixture retains
  only the consumed fields from that actual output (30 discovered packages), not a claim of exhaustive dependencies.
  The source/tool comparison was read-only and no generated executable was run.
- Cheap workflow tests check two distinct hosted builders, fresh separate caches, fixed upload-ID/name mapping,
  download-each-own-ID-once, comparison dependency, four vulnerability roots and archive-only SPDX/five-file provenance.
  Their negative mutations refuse self-download, substituted output maps, shared-cache enablement and omitted comparison.
- Final review identified two test-first corrections: earlier-attempt metadata now refuses against the fresh attesting
  invocation before downstream work, and fresh caches use a supported run-step environment rather than invalid job
  context expressions. Workflow and CLI regression checks cover both; these are delivery fixes, not runtime authority.
- No candidate, attestation, public release, catalog, feature intent, enrollment or owner-server operation has run for
  this slice. Cached same-builder two-pass equality is not independent clean-builder reproducibility; that protected
  comparison and complete same-source VM proof remain required after all source tooling merges.

## Primary sources checked 2026-10-30

- [Artifact attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations): narrowly scoped OIDC/attestation permissions and actual final subjects; not runtime safety.
- [Immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases): attach all assets to a draft before publication locks assets/tag; mutable notes/flags are not asset integrity.
- [Actions artifact API](https://docs.github.com/en/rest/actions/artifacts?apiVersion=2026-03-10): independent ID, digest, expiry and workflow-run binding; metadata alone cannot attest its enclosing ZIP.
- [GitHub CLI attestation verification](https://cli.github.com/manual/gh_attestation_verify): verify signer/source identity and exact predicates, not workflow-controlled claims alone.
- [Actions context availability](https://docs.github.com/en/actions/reference/workflows-and-actions/contexts): job-level environment cannot use `runner`; initialize private caches at the supported run-step boundary.
