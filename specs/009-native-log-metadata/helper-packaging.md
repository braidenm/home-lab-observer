# Optional Linux helper release packaging

Status: Accepted implementation interface; publication and native artifact smoke remain separate gates.

`releasepack.Config.SchemaVersion` and `--schema-version` select the closed release
schema. Empty/default remains `observer-release/v1`; `observer-release/v2` is explicit.
V1 keeps exactly six staged primary binaries and the original four-file archives.
V2 requires those same six names plus `observer-journal-helper_linux_amd64` and
`observer-journal-helper_linux_arm64`. No additional, missing or linked inputs are
accepted. Linux archives add exactly `observer-journal-helper`; other platforms
retain their four files. V2 emits required content profiles and helper metadata
(explicit JSON null on non-Linux); v1 never emits these additional keys.

The release builder builds `./cmd/observer-journal-helper` first, hashes its final
bytes and includes that digest in the primary's authoritative identity. Both use
`-trimpath -ldflags '-s -w -X main.releaseIdentity=RECORD'`. Legacy `main.version`
and `main.commit` flags remain for v1 only. The record is generated with
`internal/buildidentity.Encode` and has this canonical ASCII framing:

`HLO-RELEASE-IDENTITY-V1[role|version|commit|os|arch|helper_sha_or_dash]END-HLO-IDENTITY`

Role is `observer` or Linux-only `journal-helper`. Version is the existing bounded
prerelease, commit is 40 lowercase hex, platform is an exact supported target,
and the digest is required only for a Linux observer (otherwise literal `-`).
The entire record is at most 256 bytes; binary scanning is streaming and capped
at 200 MiB with fixed chunk/overlap memory. Exactly one complete canonical record
must occur. Duplicate, dangling, oversized and malformed candidates fail.
Framing literals in parser code use separated runtime construction so the parser
does not itself introduce a dangling candidate; actual stripped six-target builds
containing the parser test this property.

Go's trimpath builds intentionally omit linker flags from build information
(Go source `cmd/go/internal/load/pkg.go`, issue 52372). Therefore build info verifies
GOOS/GOARCH, trimpath and static primary CGO mode; the framed record provides
version/commit/digest. Runtime uses this record authoritatively whenever nonempty,
with no fallback on invalid record or role/platform mismatch. Empty records
preserve development and v1 behavior only; v2 packaging requires the record.
This validates trusted release inputs, not publisher authentication or proof
against a malicious compiler. Archive attestation is the authenticity boundary;
the digest proves pair consistency. Runtime independently hashes the adjacent
helper before invocation. Native runtime wiring remains a separate integration gate.

Manifest parsing is bounded and closed, including required exact-case keys,
duplicate rejection, exact integers and one JSON value. Archive hashes cover
final bytes; verification also checks exact member names/types, per-member and
aggregate bounds, and Linux helper bytes against manifest size/hash. Existing
200 MiB executable and 220 MiB archive/expanded bounds are not increased for a
second executable.

Online installers fetch the manifest covered by SHA256SUMS. Offline v2 requires
an explicit manifest and checksum file alongside the archive/checksum; owners
are responsible for authenticating the local checksum source. Its selected asset must
match the already verified archive hash, size, version and platform. A missing
manifest allows only the original exact four-file v1 profile, never silent
acceptance of a missing v2 helper. Installers stage the complete pair before
atomic version selection. Installation never starts the helper, elevates,
installs libsystemd or restarts a running instance. Existing v1 directories remain
valid rollback targets; explicit restart and observation-state preservation remain.

Installer implementation is the next commit; this first contract/library slice
does not yet claim installer acceptance. Verification uses synthetic archives and temporary program roots. Required
cases include missing/extra/linked helpers, mixed version/commit/architecture,
wrong embedded digest, helper tampering, malformed profile metadata, bounded
expansion, interrupted pair staging and v2-to-v1 rollback. Native release smoke
must separately prove actual release-built pairs; publication is not enabled by
this library/installer change alone.
