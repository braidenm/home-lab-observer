# Owner-operated connected Linux publication

Status: implementation slice; not evidence that a public candidate or owner-host
installation passed. The registered numeric workflow ID `397507617` is a
code-owned gate, independently read from the canonical fixed-path provider
record after PR 120 merged. There is no CLI override or name-only fallback.

## Authority and prerequisites

Run `scripts/connected_promote.py` from a clean, trusted checkout of the exact
current public repository default branch. Use native Python 3.12 or later,
Git and GitHub CLI on the owner's trusted Windows, macOS or Linux workstation.
On Windows use Windows Python and the existing Windows GitHub CLI login, not a
WSL login or copied credential. Invoke Python with `-B` to avoid bytecode files.
No third-party Python package is needed. This portability applies to the
publication tool, **not** to the Ubuntu-only connected-beta installation profile.

The owner's existing GitHub CLI stored credential authorizes repository reads,
the immutable-release setting check and intended tag/release writes. The program
does not request, print or export that credential. It clears ambient token,
GitHub configuration, Git configuration, pager and debug overrides, disables
interactive authentication and uses fixed shell-free GitHub CLI arguments for
`github.com/braidenm/home-lab-observer`. A compromised workstation or stored
credential remains a publication-authority compromise; this is not a sandbox.

Provider output is bounded, private diagnostics are suppressed and each command
has a timeout. No candidate binary or installer is executed on the publisher.
Candidate provenance and SPDX attestations are cryptographically verified for
each of the five candidate files; policy then checks certificate repository and
owner numeric IDs, workflow, exact source, hosted runner and run attempt.
These are unsigned-beta checksums and provenance, not independent publisher
package signing.

## Prepare the exact candidate for manual VM acceptance

After the manually dispatched trusted-main candidate workflow succeeds, supply
its public numeric run and artifact IDs. Use a **new** output directory:

```text
python -B scripts/connected_promote.py --prepare --run-id 123456789 --artifact-id 987654321 --output candidate-check
```

The example IDs are synthetic. This mode reads provider APIs and writes only
that new local directory; it does not create a tag, draft, release, VM, service
or enrollment. It gets the artifact size/digest/expiry from GitHub independently
of its sidecar, verifies the exact ZIP bytes and closed flat five-file inventory,
and checks the latest successful run attempt and all attestations. The candidate
source must equal the current clean main checkout. A prior attempt or rerun is
not interchangeable.

Verified files are written as non-executable data under `candidate-check/payload/`.
The sibling `candidate-identity.json` contains only bounded public candidate and
artifact identity, for constructing the later acceptance record. Never treat a
directory left by a failed command as verified; use a new directory after fixing
the failure. Preparation is not installed acceptance or publication permission.

On 2026-10-26, a live preparation attempt was correctly refused when the Actions
artifact download endpoint returned HTTP 415 for an octet-stream request. The
adapter now uses the documented JSON media type for that endpoint's redirect to
ZIP bytes; release-asset downloads retain their separate octet-stream media type.
There is no fallback that admits an HTTP error body. This finding and its
synthetic regression tests are not evidence of successful preparation, VM
acceptance or publication. See [GitHub's artifact-download contract](https://docs.github.com/en/rest/actions/artifacts?apiVersion=2026-03-10#download-an-artifact).

The same live preflight then found that GitHub CLI 2.93 rejects combining
`--cert-identity` with `--signer-workflow` before cryptographic verification.
The adapter now selects only the exact certificate URI, retaining repository,
source/signer digests, main ref and hosted-runner restrictions. The independent
certificate policy still checks all signer/source fields and numeric identities
for both provenance and SPDX; no attestation requirement is relaxed. See the
[GitHub CLI verification manual](https://cli.github.com/manual/gh_attestation_verify).

That preflight also established that the generated SPDX attestation uses the
exact predicate `https://spdx.dev/Document/v2.3`. An unversioned prefix accepted
by the CLI was correctly rejected by the independent exact-match policy. The
code-owned vocabulary now contains only that SPDX 2.3 URI and SLSA provenance v1;
unversioned, different-version and prefix matches remain refused. See [GitHub's
SPDX verification instructions](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations#verifying-an-attestation-for-sboms).

A subsequent read-only diagnostic verified all five files from successful run
`40399849708`, artifact `12688271733`, against both exact predicates and the
unchanged certificate/subject policy. No downloaded command was executed and no
release was written. The corrected verifier still needs review/merge and a new
candidate before normal preparation, exact-byte VM acceptance and publication.

Run the separate reviewed bootstrap **and** activation VM scopes against those
exact bytes and independently pinned fixture inputs. Commit the completed
code-owned `docs/acceptance/connected-linux/<version>.json` record through an
independently reviewed PR. Every fixed result must be `PASS`; no raw terminals,
observations, tokens, grants or private host paths belong in that record.

## Publish without rebuilding

Update the trusted checkout to exact clean current main after the acceptance PR
merges, then run:

```text
python -B scripts/connected_promote.py --publish --version 0.1.0-canary.1
```

The version is illustrative, not an existing-release claim. The program reads
the canonical acceptance file from Git, not an arbitrary supplied path. The
candidate source must be a merged main ancestor, and the complete intervening
diff must be **only** that one non-executable regular acceptance-file addition.
Other changes require a new candidate/version and new VM evidence. Publication
does not rebuild or execute assets. Every Git read disables replace-object
references, and any legacy `info/grafts` file (including a dangling link) is
refused before provider access, so local graph overlays cannot hide a runtime
change from the ancestry/diff check.

A version-keyed exclusive lock in the repository's common Git directory prevents
simultaneous local publishers. The program verifies immutable releases are
enabled and rechecks clean current main, candidate attempt/artifact, tag, draft
binding and asset IDs/digests before every mutation. It downloads draft assets
independently to compare their actual bytes before publishing a non-latest
prerelease. The pre-mutation comparison includes only closed publication
authority: release ID, tag/source, name/body, draft/prerelease/immutable state and
each asset's ID/name/state/size/digest. Harmless download counters and provider
timestamps may change during verification; they cannot change those authority
fields or substitute asset bytes. After publication it requires immutable metadata and anonymously
downloads all four public files with no GitHub credential, bounded HTTPS-only
redirects and exact size/digest checks.

## Closed retries and failures

- Absent tag/release: create the exact candidate-source tag, then its bound draft.
- Exact tag without release: create only the bound draft.
- Exact bound draft: verify retained assets and upload only missing files.
- Exact immutable published release: verify metadata and anonymous bytes as an
  idempotent success; do not mutate it.

Unexpected tag source, draft content, asset IDs/digests, mutable published
release, stale main/attempt, expired candidate or failed anonymous download
refuses success. Atomic creation races are refetched and must converge to the
same candidate. There is no clobber, deletion, retagging or overwrite fallback.
Partial drafts and immutable failed-postcheck releases are retained for diagnosis.
After an interruption, rerun the same version from the still-valid exact main;
do not delete or repair provider state speculatively. A stale local lock requires
the owner to establish no publisher is active before removing that exact lock.

The release list is a bounded single page; if it cannot prove absence or
uniqueness, publication fails rather than overlooking another draft. Fixed
success/failure markers and public IDs replace raw provider errors. A failure
does not mean a prior provider mutation was undone.

## Separate remaining gates

Synthetic adapter tests establish policy and wiring, not live publication or VM
evidence. Public immutable publication precedes deployment-owned catalog
activation. Real server installation, useful owner-bound uploads, ordinary-user
sharing and grant/revoke/direct-access checks remain separate acceptance gates.
This tool does not enable automatic boot, unattended upgrades/removal, Docker
controls, Kubernetes, remote process details or remote logs.
