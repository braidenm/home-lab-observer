# Connected Linux publication admission

Status: pure-policy slice of [the release plan](connected-release.md). This code
does not fetch, execute, publish, grant access or mutate installed state.

`scripts/connected_promotion_policy.py` consumes the candidate metadata contract
and admits one bounded `observer-connected-acceptance/v1` JSON record. Its exact
top-level fields are `schema_version`, `candidate`, `candidate_sha256`, `artifact`
and `vm`. `candidate` is the complete canonical candidate metadata; the separate
digest binds its actual bytes. `artifact` contains exactly positive `id`, the
attempt-bound `name`, ZIP `sha256`, positive bounded `size` and UTC `expires_at`.
`vm` contains exact image/receiver/probe/harness SHA-256 pins plus a `results`
object. Every named bootstrap and activation result must be `PASS`; not-run,
failed, unknown and missing results refuse. Raw consoles or observations do not
belong in the record. Independently reviewed evidence is not an activation token.

The closed ZIP verifier returns five flat data files in memory, not an extracted
filesystem. It rejects duplicate/unknown/path-bearing/link/encrypted members,
oversized or changed content and mismatched metadata. Candidate bytes still need
the shared payload checker and cryptographic provenance; hashes alone are not
origin authentication.

The provider contract binds current successful workflow run **and attempt**,
numeric repository/owner, owner/triggering actor, source/ref/path/workflow ID,
artifact ID/name/digest/size/expiry and correlated workflow-run source. Provenance
policy applies only to a successful `gh attestation verify` result. It checks
verified certificate signer/config/source/numeric identities, hosted environment,
workflow dispatch, exact run invocation and matching subject digest. It does not
promote workflow-controlled statement predicates to independent authority.

`git diff --raw --no-abbrev --no-renames -z SOURCE HEAD` must describe exactly
one new 100644 `docs/acceptance/connected-linux/<version>.json`; the later caller
must separately prove ancestry and clean current trusted main. No unrelated doc,
workflow, runtime or policy delta is permitted between candidate and promotion.

Pure release-state classification returns only `CREATE_TAG`, `CREATE_DRAFT`,
`UPLOAD_MISSING`, `PUBLISH` or `VERIFY_PUBLISHED`. Existing exact draft assets
remain unchanged. Wrong source/body/title/flags/asset identity, unexpected or
duplicate assets, and mutable published state refuse. There is no overwrite,
delete, clobber, credential or generic URL operation here.

Both provenance and SPDX dependency attestations retain this certificate/subject
boundary; a caller cannot select an arbitrary predicate type. The candidate's
trusted build also checks actual SBOM coverage of the three packaged Go roles.

Eight synthetic Python 3.12 test methods cover these boundaries in milliseconds,
without installed software, real grants or network writes. The provider/CLI slice
must additionally test real fixed argv wiring, bounded I/O, credential isolation,
main/settings/state rechecks, recovery races and anonymous post-publication
verification. This policy alone is not a working or accepted public release.

Primary evidence (accessed 2026-10-26):
[GitHub CLI attestation verification](https://cli.github.com/manual/gh_attestation_verify)
distinguishes certificate identity from workflow-controlled predicates;
[Actions artifact API](https://docs.github.com/en/rest/actions/artifacts)
defines the external artifact ID/digest/expiry/run binding. Existing ADR 004 and
the parent release plan define migration, authority, rollout and rollback.
