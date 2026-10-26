# Connected Linux candidate and immutable publication

Status: proposed next slice; no publication or catalog enablement yet.
Scope: the separate Ubuntu 24.04/systemd 255/amd64 explicit-start beta, not the
six-target local preview, unattended lifecycle or Docker controls.

## Outcome and alternatives

Owners download four version-pinned public files without GitHub credentials:
archive, canonical connected manifest, generated installer and final checksums.
Use one manually dispatched trusted-main workflow to build/attest a candidate,
then an owner-operated checked-in local promotion program to publish unchanged
bytes only after exact-artifact manual VM acceptance. The owner's existing
GitHub CLI credential is the publication authority; never extract it or create
a new hosted administrative secret. The candidate workflow has no publication
authority or production credential.
Do not rebuild during publication: the embedded source identity changes bytes,
and a later rebuild is not the tested artifact. Do not add private runners,
production credentials or privileged guest installation to PR CI.

Alternatives: reusing the six-platform preview misrepresents connected support;
publishing a local workstation build lacks hosted provenance; one automatic
build-and-publish job cannot wait for genuine external VM acceptance. A full
package-manager/signing infrastructure remains later work. The existing
UNSIGNED_BETA_WITH_CHECKSUMS_AND_PROVENANCE label remains honest: GitHub provenance
and release attestations do not make this a separately publisher-signed package.
An all-hosted promoter would need a separately provisioned Administration-read
credential to verify immutable-release settings; `GITHUB_TOKEN` contents-write
does not grant that capability. Owner-operated promotion avoids that new secret
and makes the trust boundary explicit. It does not make workstation compromise
safe.

## Candidate

- Only the canonical public repository owner may dispatch or rerun, on main.
  Require exact current-main source at candidate authorization, a bounded
  `0.x.y-canary.N` version, and no existing tag or release for that version.
- Hosted build uses the pinned supported Go toolchain, reviewed three-role
  identities and `connectedpack`; source, helper and archive identities agree.
  Run focused tests and reproducibility of the exact connected payload; scan
  reachable dependencies. Do not repeat unrelated six-target preview builds.
- Emit only the four flat public assets plus bounded `candidate.json` metadata
  into one immutable Actions artifact. Metadata includes repository/workflow/run
  identity, exact run attempt, version, source, contract and every filename/size/SHA-256.
  Pin numeric repository `1362278980` and owner `44215293`, the workflow path
  and provider workflow ID, main ref and exact source SHA. Artifact name binds
  run ID and attempt; an artifact from a previous attempt is not interchangeable.
  The upload output/provider API supplies artifact ID and archive digest;
  metadata cannot contain the digest of its own enclosing archive. Acceptance
  binds that independently verified ID, digest, name, size and expiry. No paths
  from a private build host, environmental dumps, credentials or observations.
- Generate dependency SBOM/provenance attestations for final asset digests using
  pinned actions and isolated least-privilege permissions. An SBOM may remain
  an attestation/build artifact; it must not silently change the four-file
  download contract or final three-entry checksum membership.
- Keep artifacts for a bounded period. Expired or missing candidates require a
  new candidate/version and fresh evidence, not an unrecorded substitute build.

## Exact-artifact installed acceptance

Download the candidate by exact run/artifact identity and verify cryptographic
provenance, trusted repository/workflow/source/main identity and closed content
before manual VM use. Independently supplied pins must match candidate bytes.
Run both the bootstrap and activation scopes on that exact archive/manifest:
download refusal/valid stopped install/reboot, then useful acknowledged host
metrics, stop/start/retained identity/increasing sequences/explicit post-reboot
start. Keep the existing isolated 2-CPU/3-GiB/no-NIC/no-host-share budget.

Commit a schema-bounded public acceptance record through a small independently
reviewed PR: candidate run/artifact identity, source/version/asset pins, exact
image/receiver/probe/harness pins, fixed per-case results and explicit remaining
limits. Never publish raw consoles, terminal captures, grants or observations.
A failed/not-run case is not success. The record is release evidence, not a
durable service-activation capability: actual worker checks still run on every
explicit start. Applicable ADR 020/021 gates cannot be waived by a checkbox.

Evidence may be committed after the candidate source: the publication code runs
from current trusted main and verifies the candidate source is a merged main
ancestor. The complete intervening diff must contain **only** the one canonical
`docs/acceptance/connected-linux/<version>.json` addition: regular non-executable
file, no symlink, rename, deletion or mode change. Reject every other changed
path, including tasks/index docs, workflows, policy, runtime, harness or packer.
Update progress docs after publication. Ancestry alone is insufficient. A
runtime/packaging change requires a new candidate and matching installed proof.

## Publication and rollback

The local promoter runs from exact clean trusted main, verified equal to the
current default-branch SHA through the provider API. Recheck that main remains
unchanged before every mutation and immediately before publication. Accept only
a bounded candidate run ID and code-owned acceptance-record identity, never a
provider URL or executable path. Clear ambient `GH_TOKEN`/`GITHUB_TOKEN`
overrides and use fixed shell-free `gh` argv, fixed github.com/repository and
numeric IDs. Never call `gh auth token`, copy credentials to environment/files,
print raw provider output, or execute downloaded candidate assets.

Verify successful canonical candidate workflow, current run attempt,
owner/trigger/ref/source/run correlation, numeric repository/owner identity,
strict evidence-only ancestry, exact artifact ID/digest/name/size/expiry, all
four asset digests/sizes, canonical manifest/helper/checksum consistency,
cryptographic attestations and complete matching acceptance. Verify provenance
signer workflow/ref, source SHA and numeric sourceRepositoryIdentifier, not
repository display name alone. Provider text is data, never shell fragments or
authorization evidence alone. Extract only bounded flat regular ZIP members;
reject duplicates, unknowns, paths, links, oversized or changed bytes.

Acquire a version-keyed local process lock. Assemble the closed assets in a
draft release/tag bound to the tested source; independently
download draft bytes and compare, then publish as a non-latest prerelease only
when repository immutable-release protection is verified enabled. Immediately
before publish recheck settings, main SHA, tag SHA, exact draft binding and all
asset IDs/digests. Never clobber, overwrite, delete or silently rebind a release.
Closed retry states are: absent tag/release creates the exact tag/draft; exact
tag without release creates its draft; exact bound draft verifies existing and
uploads only missing assets; exact immutable published release verifies as
idempotent success. Wrong tag SHA, changed/unexpected draft content/assets or
mutable published release fails and is retained. Atomic provider creation races
must refetch and revalidate this same state, never adopt a different candidate.
After publishing require actual immutable release metadata and an anonymous,
credential-free download/digest/size check of every asset. Preserve fixed stage
diagnostics and the public candidate/release identity, not raw tool output.

Only then generate Platform Demo's deployment-owned catalog from those verified
public bytes. Catalog activation and exact-main app deployment are separate
infrastructure work. A separate owner-bound native canary must coexist with the
legacy connector; grant/view/revoke/direct-access acceptance remains separate.
Rollback disables new setup metadata and preserves installed state and existing
Compose service. Never mutate an immutable release or reset a credential/ledger.

## Proof and primary sources (2026-10-26)

Automated tests reject untrusted ref/actor/run correlation, stale/mismatched
source or artifact, unknown/duplicate/path-bearing assets, changed bytes,
missing/failed/mismatched acceptance, absent/expired artifacts, conflicting version bindings,
and non-immutable publication. Test actual script wiring and least-privilege
workflow permissions with synthetic provider responses; mocks are not live
publication or anonymous-download evidence. Manual jobs remain outside PR CI.

- [GitHub immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases): stage all assets before publication; actual immutable protection binds assets/tag and generates release attestation.
- [GitHub artifact attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations): attest final bytes with narrowly scoped OIDC/attestation permissions and verify signer/build identity, not a checksum alone.
- [Workflow artifact identity and retention](https://docs.github.com/en/actions/tutorials/store-and-share-data): artifacts carry specific run identity and bounded retention; download the exact candidate rather than substituting a later build.
- Existing repository [release boundary ADR](../../docs/adr/004-independent-release-supply-chain.md), [public bootstrap](public-bootstrap.md), and [manual VM scope](bootstrap-vm-acceptance.md) remain authoritative. New work adds no server-control permission.
