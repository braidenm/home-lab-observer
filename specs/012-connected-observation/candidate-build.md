# Hosted connected Linux candidate

Status: build/attestation slice of [the connected release plan](connected-release.md).
It creates no tag/release/catalog, installs nothing and runs no candidate command.
Only Ubuntu 24.04/systemd 255/amd64 explicit-start beta is represented.

## Authorization and resource bounds

The manual workflow is fixed to `.github/workflows/connected-linux-candidate.yml`
in public repository ID `1362278980`, owner ID `44215293`. Owner dispatch and
rerun identities must agree in both workflow context and provider run-attempt
records. The exact source must be current `main` at authorization. The provider's
registered workflow ID is read from its fixed path and correlated with the run;
the later promotion policy independently pins that numeric ID after registration.
An existing version tag or release refuses. API errors are not interpreted as
absence. Only canonical bounded `0.x.y-canary.N` versions are accepted.

One hosted Linux build uses Go 1.27.3, fixed Linux/amd64/v1, no CGO/workspace/VCS
stamping or ambient Go configuration. It builds the three reviewed roles twice
in separate paths, packs each through `connectedpack`, and byte-compares all four
public assets. It executes the trusted build tool, never a generated candidate
command. Focused packaging/identity tests run once; reachable vulnerability
analysis covers the three released command roots once. This is not the six-OS
preview or privileged/manual VM acceptance. Jobs have 3/8/4-minute deadlines;
there is no private runner or release-write permission. Candidates retain seven
days; missing/expired bytes need a new candidate and matching proof.

## Metadata contract

`scripts/connected_candidate.py` uses Python 3.12 standard library only and
provides reusable pure `validate_metadata`, `decode_metadata`, `encode_metadata`,
`asset_names`, `artifact_name`, `validate_payload_bytes` and bounded Linux
filesystem `validate_payload` functions. The cross-platform byte verifier never
extracts files; the caller must separately admit ZIP/filesystem identities.
Invalid data raises the fixed `connected_candidate_invalid` ValueError. JSON is
compact ASCII, exact ordered fields, a single final LF, no duplicate/unknown
keys, no noncanonical spellings, and at most 4 KiB. Positive numeric identifiers
are at most JavaScript's safe-integer maximum; booleans are never identifiers.

Fields in order:

1. `schema_version`: `observer-connected-candidate/v1`.
2. `repository_id`, `owner_id`: the fixed numeric identities above.
3. `workflow_path`, `workflow_id`: fixed path and correlated positive provider ID.
4. `ref`: `refs/heads/main`.
5. `run_id`, `run_attempt`: exact positive provider run and attempt.
6. `source`: exact lowercase 40-character source commit.
7. `version`: bounded connected canary version.
8. `contract_sha256`: lowercase digest of the code-owned compatibility descriptor.
9. `assets`: exactly four ordered records, each ordered `name`, `size`, `sha256`:
   `home-lab-observer-connected_<version>_linux_amd64.tar.gz`,
   `connected-manifest.json`, `install-connected-linux.sh`, `SHA256SUMS`.

The five flat regular, single-link, 0644 files are bounded to 64 MiB combined
(including metadata reserve). Manifest is at most 16 KiB; script/checksums at most
64 KiB each. Checksums contain exactly archive/manifest/script entries in that
order. Payload validation verifies manifest source/version/target/profile and
contract correlation and each exact asset size/digest through opened directory
descriptors. It does not generically extract archives or claim that a self-asserted
checksum authenticates an asset. The Go packer is the canonical content verifier;
hosted cryptographic provenance and later immutable evidence establish origin.

One immutable Actions artifact is named
`connected-linux-<version>-<run_id>-<run_attempt>`. `candidate.json` cannot contain
its own enclosing ZIP's digest/ID. Upload outputs and the independent provider
artifact record supply that ID, digest, name, size and expiry for later acceptance.
An earlier rerun attempt's artifact cannot substitute for this attempt.

## Attestation boundary and tests

A separate hosted job downloads the build's exact artifact ID, validates the
five-file payload, scans the actual final archive directly using pinned Syft for
SPDX (not a directory scan that might ignore the nested archive), then revalidates
bytes. A closed coverage guard requires all three ELF file hashes to match the
manifest, the exact outer archive digest, three main modules/Go 1.27.3 runtimes,
and the gopsutil, x/sys and sqlite versions selected by trusted `go.mod`. Empty,
outer-only or incomplete scans refuse; this guard supplements Syft discovery,
not an independent proof that no dependency was missed. SBOM is outside the candidate payload and is retained
as a signed attestation predicate. Pinned `actions/attest` generates dependency
attestations for all five subjects, and build provenance for all five subjects
plus the exact SBOM digest. Only this job gets OIDC/attestations write; no job
gets contents write. The candidate build itself has no publication authority.

Synthetic Python tests cover canonical roundtrip, duplicate/unknown keys, bounds,
actor/ref/repository/run-attempt/source mismatches, provider workflow correlation,
version conflicts, changed files, checksum/manifest mismatches, links/modes,
closed artifact content and actual create/verify CLI wiring. Existing Contracts
CI runs these lightweight checks and shell syntax without compiling three roles
or invoking the manual release/VM pipeline. Workflow assertions verify manual
hosted triggers, exact artifact transfer and narrow attestation permissions.

Primary inputs verified 2026-10-26: [actions/attest v4.2.2](https://github.com/actions/attest/tree/1e69f48acb82d1966a394da916b4c1698aa569d6),
[artifact transfer](https://docs.github.com/en/actions/tutorials/store-and-share-data),
[Syft](https://github.com/anchore/syft/releases/tag/v1.55.0), and
[govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck).
Actions are pinned to reviewed full commit SHAs, not moving major tags.

Direct-target behavior was exercised locally with pinned Syft 1.55.0 against the
previously tested `d16e521` archive: 27 packages and all three exact ELF SHA-256
values were present, including core dependency versions and the outer archive
digest. Only Syft executed; no candidate command ran. The primary
[supported scan-target documentation](https://oss.anchore.com/docs/guides/sbom/scan-targets/)
documents generic `.tar.gz` archives as direct targets. A newly hosted candidate
must still pass the same guard on its own exact bytes.

This slice is not exact-artifact installed acceptance, immutable publication,
anonymous-download proof, real-server enrollment or owner/delegate sharing proof.

## Isolated interpreter cleanliness correction

The first hosted dispatch, [run 40395987272](https://github.com/braidenm/home-lab-observer/actions/runs/40395987272),
failed the builder's clean-source precondition before packaging or upload. Its
tests passed, but `python3 -I` ignores `PYTHONDONTWRITEBYTECODE` and imported tests
created untracked bytecode. This was reproduced with the exact isolated command.
All controlled isolated Python invocations now explicitly pass `-B`; a workflow
regression enforces that flag. The same focused suites must leave no bytecode
behind before the actual builder runs. Keep the clean-source requirement intact;
do not ignore generated files or delete dirty source inside the release job.
No candidate, VM acceptance or publication is claimed for the failed dispatch.

[Python 3.12 command-line guidance](https://docs.python.org/3.12/using/cmdline.html)
documents both isolated mode's ignored environment and explicit `-B` behavior
(checked 2026-10-26). A new dispatch uses the reviewed merged correction source.
