# Publish a native preview

Native previews are published from this public repository only. Neither the private infrastructure repository nor its
legacy container package participates in the build. Consumers need no GitHub account or package token.

## What is verified

```text
Reviewed PR + required checks -> merged main
                                  |
Owner checks immutability -> manual explicit-version dispatch
                                  |
Six native archives + SBOM -> six native runner checks + vulnerability scan
                                  |
Final-byte provenance -> draft upload + download verification -> immutable publication
```

The release payload is six OS/architecture archives, two install helpers, `release-manifest.json`, `observer.spdx.json`,
and `SHA256SUMS`. Archives contain exactly one directory and four regular files; build output, credentials, arbitrary
source directories and debugging artifacts are not accepted. The manifest describes archives; the checksum file covers
all ten payload files, and provenance covers every final downloadable file, including the checksums.

Normal PR checks build the six targets once, then test native installation in parallel on Windows, Linux and macOS.
Publication additionally runs the native artifacts on both supported architectures for each OS. The release workflow
does not run on PR events and only its final jobs receive attestation or publication permissions.

## Owner preflight and dispatch

1. Merge the reviewed delivery changes through required checks. Do not dispatch an unmerged branch.
2. In this repository's **Settings → General → Releases**, verify release immutability is enabled. An authenticated
   repository administrator can also check it with the command below. Require `enabled: true`.
3. Choose a new `0.1.0-preview.N` version with a positive number and no existing release/tag. Do not reuse a failed
   version whose release/tag already exists.
4. Run **Actions → Publish native preview → Run workflow** on `main`, enter the version and explicitly confirm the
   immutability preflight. Wait for every publication job; a created draft is not a successful release.

```sh
gh api -H 'X-GitHub-Api-Version: 2026-03-10' repos/braidenm/home-lab-observer/immutable-releases
gh workflow run native-release.yml --ref main -f version=0.1.0-preview.1 -F immutability_confirmed=true
```

The settings API requires administrator-read permission, which the workflow deliberately does not have. Do not add an
owner PAT to CI to automate that check. The dispatcher attests to the preflight; after publishing, the workflow requires
the public release API to report `immutable: true` and downloads the assets without authentication to verify them again.
This is a trusted-maintainer boundary: a repository administrator can change repository settings outside CI.

## Failure and rollback

The workflow fails closed on a wrong branch, invalid identity, existing release/tag, vulnerability finding, native test
failure, malformed manifest, checksum mismatch or unverified publication. Inspect the failed step and fix it in a new
reviewed PR. Partial draft releases are not automatically published or deleted. Never replace an existing download to
repair a release: publish a new explicit preview version after reviewing the failure.

Consumers can retain a previously verified archive or select a retained installed version. Program rollback does not
downgrade or restore a database; preserve state and follow version-specific compatibility guidance.

## Preview versus GA

These Windows/macOS previews are not publisher-signed/notarized. Checksums and GitHub provenance do not remove OS trust
prompts or substitute for a platform signing identity. See [installation](install.md), [ADR 004](adr/004-independent-release-supply-chain.md)
and [ADR 007](adr/007-native-preview-installation.md). Native GA requires the separately managed signing/notarization identities.
