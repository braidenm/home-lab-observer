# F1 connected bundle and reviewed service templates

Status: implementation authorized as part of ADR020's unactivated end-to-end profile.
Native release manifests, identity roles and archive validators remain unchanged.

## Closed bundle

The separate `observer-connected-bundle/v1` manifest contains a prerelease version,
40-lowercase-hex commit, linux/amd64 platform, the fixed Ubuntu24.04/systemd255 canary
profile, and a sorted exact list of three binaries and reviewed template resources.
The binaries are `observer-connected-collector`, `observer-connected-uploader`, and
`observer-connected-install`. Runtime identity uses a separate connected framing
record and roles collector/uploader/install; no legacy fallback or journal helper.

The four payload resources are `home-lab-observer-connected-collector.service.tmpl`,
`home-lab-observer-connected-uploader.service.tmpl`, `enrollment.properties.tmpl`,
and `installed-config.schema.json`. Together with the three binaries and
`connected-manifest.json`, the staged directory and flat archive contain exactly
eight regular files. Binary modes are 0755 and resource/manifest modes are 0644.
Windows-side verification cannot attest POSIX mode bits; Linux staging rechecks them.

`artifact_sha256` and the immutable release directory name mean SHA256 of canonical
manifest bytes. This binds every listed resource through its digest, size and mode.
It is distinct from the distribution archive SHA256. Neither digest alone proves
publisher authenticity; the caller must authenticate release checksums/attestation
before treating an expected digest as trusted. No installer elevates on a manifest
self-assertion alone.

The manifest itself is not self-listed. Unknown, duplicate, linked, malformed,
missing or extra files are refused; regular binaries are bounded, Linux/amd64 ELF
with no dynamic loader/imported libraries, correct Go command/build settings,
and exactly one matching connected identity record. Templates must equal the
embedded reviewed bytes, not merely a caller-provided digest. Canonical closed JSON
avoids duplicate/case/unknown-key ambiguity. Existing native-v2 rollback is untouched.

## Runtime identity

The sole linker variable is `main.releaseIdentity`. `connectedidentity.Resolve`
requires a canonical bounded record matching the expected role and actual supported
runtime; empty/development identity fails closed. Encode/Parse and bounded Scan are
used by release tooling without executing cross-target inputs. Malformed, duplicate,
dangling and chunk-spanning marker cases have deterministic synthetic tests. Identity
is package consistency evidence; authenticated release provenance is separate.

## Deterministic templates

`connectedunits.RenderUnits(connectedprofile.Config)` uses validated data and embedded
templates only. It returns the fixed collector/uploader unit bytes; callers cannot
supply template text, command paths, environment or arbitrary unit fragments.
`Resources` returns detached exact template bytes for the connected bundle.

Workers take no arguments. Collector uses the shared GID as primary; uploader uses
its private primary GID and the shared group only as supplementary. Exit codes
20/21/22 are non-restart credential/terminal/recovery states. Crash restart is bounded
and retains the separate C2 startup cooldown. Unit directives implement the accepted
profile, but effective inherited policy and actual namespace/syscall probes remain
mandatory before activation; template tests alone do not prove installed isolation.

Enrollment uses a separately reviewed private-pipe manager invocation, never a
secret in unit text, arguments or environment. `RenderEnrollmentProperties` accepts
only typed uploader UID/GID/shared-GID, artifact digest, addresses, and a closed mode.
It returns properties and the fixed executable with one fixed mode argument:
`enroll` binds staging with constrained egress; `validate-enrollment` binds staging
with private networking; `validate-ledger` binds only the final ledger with private
networking. Neither validation mode mounts a credential, handoff or enrollment
artifact outside its required scope. No fictitious connector ID is needed before
exchange. Grant/binding input and validation output use bounded private pipes.

## Build and verification interface

`scripts/build-connected.sh <prerelease-version> <full-commit> <new-output-directory>`
requires a clean matching checkout and builds the three separate static command roots.
The internal `cmd/connectedpack` build tool emits the connected identity records,
canonical manifest, deterministic flat tar.gz and `SHA256SUMS`. It never installs,
starts a service, publishes, or overwrites an existing output directory. A failed build
may leave a partial unpublishable directory; no completed checksum file is emitted
when required inputs are missing. Native-v2 archives and workflows are unaffected.

`connectedbundle.VerifyDirectory(directory, expectedManifestSHA256)` requires a
trusted expected manifest digest, verifies exact bounded contents and returns the
closed manifest. This is snapshot validation, not protection against later replacement
by the same owner. The privileged installer must reverify its fresh root-owned staged
copy before publishing it. Manifest SHA is not an archive authentication shortcut.

## Acceptance

- [x] Closed manifest and real synthetic Go ELF identity/resource correlation tests.
- [x] Deterministic unit rendering with malicious/invalid input refusal.
- [x] Explicit network/IPC/resource template assertions; native-v2 code unchanged.
- [ ] Actual three-command release build and artifact verification after composition.
- [ ] Independent review, followed by the separate installed positive/negative VM gates.

Synthetic packaging tests compile tiny command fixtures but never execute them or
install services. Symbolic-link creation is skipped only when Windows denies that
fixture operation; hard-link refusal remains tested. The separate manual primitive
fixture documents its own evidence and is not an installed-profile acceptance gate.
