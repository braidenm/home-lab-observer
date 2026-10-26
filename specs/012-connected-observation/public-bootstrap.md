# Version-pinned connected Linux bootstrap

Status: proposed, review before implementation. Completes distribution for Spec 012;
it does not waive ADRs 020/021's remaining installed fault/lifecycle gates.

## Question and context

How can an ordinary owner install the separate native connected beta without a
GitHub account, build tools or a registry PAT? The existing `connectedpack` emits
a deterministic three-role archive, manifest and checksums. The Go root installer
already verifies exact bundle contents and host policy before its no-echo token
prompt, then installs stopped. It must remain the only enrollment/installation
transaction; a shell helper must not duplicate service/account/credential logic.

The package is Ubuntu 24.04/systemd 255/amd64 only. Root is trusted. Protect against
network substitution, accidental version mixing, mutable caller staging, unsafe
archive paths, ambient curl configuration, unknown installed state and token
disclosure. The sudo-authorized interactive installing account, including its
same-UID processes, is trusted: a private user staging directory excludes other
UIDs but does not defend against replacement by that same account before sudo.
The helper does not claim same-UID compromise resistance. A malicious root
administrator or compromised trusted publisher is not solved by a checksum.

## Alternatives and recommendation

| Choice | Consequences |
| --- | --- |
| Keep verified manual bundle staging | Smallest boundary, but root-owned ancestor requirements and several downloads make normal setup error-prone. Retain as technical fallback. |
| Version-specific Bash bootstrap with fixed isolated Python USTAR reader, then existing Go transaction | Selected: one public script, exact byte/member pins, standard Ubuntu Python 3.12. The tiny extractor is not an installation transaction or generic archive tool. |
| Bash plus GNU tar listing/extraction | Fewer source lines but reliable closed header admission is not expressed by parsing a human-oriented verbose listing; prefer explicit bounded raw-header checks. |
| General package-manager repository | More distribution policy than this fixed beta needs; revisit for signing, upgrades and broader OS support. |

Generate `install-connected-linux.sh` at build time from the verified manifest
and completed archive digest/size. It contains no secret or configurable provider.
Derive one HTTPS GitHub release URL from a canonical bounded `0.x.y-canary.N`
version under the fixed public `braidenm/home-lab-observer` repository. Do not use
moving latest tags or accept a user-supplied URL, download path, unit or command.
Add the script to final `SHA256SUMS` only after generation; checksums never include
their own digest. The Platform release catalog separately pins all four assets.

## Privileged boundary

1. The app emits a secret-free command downloading the exact public script to a
   fresh private user directory and verifying its catalog SHA-256 before sudo.
   No `curl | sh`, token argument/environment/file or GitHub credential.
2. Bootstrap accepts only one syntactically valid `srv_` resource reference,
   requires root and an attached terminal, sets a fixed system PATH and private
   umask, and validates root-owned/non-writable/non-link ancestors `/`, `/var`,
   `/var/lib`. It never adopts a caller-selected staging directory.
3. Create a fresh exclusive root-owned private staging directory under `/var/lib`.
   Require existing Bash/curl (>=8.4)/Python 3.12/coreutils/acl tools, never install system packages
   automatically. Remove inherited ACLs only from newly created bootstrap dirs.
4. Fetch only the exact archive with `-q` as curl's first argument, `--fail`,
   `--remove-on-error`, `--proxy ''` (ambient proxies intentionally unsupported),
   `--location`, `--proto '=https'`, `--proto-redir '=https'`, `--max-redirs 5`,
   `--connect-timeout 10`, `--max-time 180`, and `--max-filesize` equal to the
   generated exact archive size. Never use insecure TLS or caller CA overrides.
   Require curl >=8.4, which also bounds unknown-length streaming responses.
   Recheck stat size and the archive SHA-256 embedded in the generated script
   before parsing. A partial download is never eligible for extraction.
5. Run the embedded fixed extractor using `/usr/bin/python3 -I`: ignore Python
   environment/user-site/current-directory imports. Read bounded gzip/512-byte
   USTAR records; require exactly one occurrence of every generated canonical
   flat member in order, regular-file type only, exact size/mode and root UID/GID,
   no link target, prefix, PAX/GNU extensions, special files or directories.
   Validate each raw header checksum and exact canonical header bytes, including
   USTAR magic/version and zero padding, against the publisher's Go writer.
   Require the two zero end records and no trailing decompressed bytes. Create
   only exclusive regular files in the fresh empty root-private bundle directory,
   restore only generated fixed modes and verify each file digest (including the
   manifest pin) before executing the installer. Never extract into a live tree.
6. Invoke the verified Go installer with bundle directory, manifest digest and
   server reference. Only Go reads the one-use grant from its no-echo terminal
   prompt; bootstrap neither reads nor captures it. Reuse current preflight,
   lease, uncertain-exchange recovery and stopped-install behavior unchanged.
7. Print exact installed-manager Start/Status/Stop commands derived from the
   pinned manifest. Never start/enable services automatically. Preserve actual
   installed state on failure; cleanup removes only the fresh bootstrap's fixed
   public files/directories, refuses unexpected contents and never recurses into
   configuration, credentials, ledger, existing releases or a caller path.

## Proof and operation

Render tests reject invalid/noncanonical versions, commits, unknown contracts,
unsafe/missing manifest members and invalid digest/size. Prove deterministic
generation, closed fixed URLs, no credentials, shell syntax, literal quoting,
bounded downloads, exact member/file verification and non-recursive cleanup.
Repinned malformed archive fixtures must reject duplicate names, symlinks,
hardlinks, special entries, absolute/`..` paths, extensions, oversized bodies,
wrong metadata/checksums, truncation and trailing data before installer execution.
The download fixture must follow an allowed synthetic HTTPS redirect and refuse
an HTTP downgrade or a chain beyond the fixed redirect limit.
Use owned synthetic fixtures; ordinary tests cannot mutate real accounts/units.
The actual generated helper must also pass a disposable no-NIC VM first-install
journey with the scoped synthetic receiver/download fixture, plus a corrupted
download refusal before any grant exchange. A mocked successful command is not
installed acceptance. Rerun the exact release artifact after runtime changes.

Release publication is a separate small slice: reviewed merged-source identity,
hosted build/vulnerability scan/provenance, manual exact-artifact VM acceptance,
draft with closed asset allowlist, verify downloaded draft bytes, then immutable
publication and anonymous byte/digest check. Do not expose the setup catalog or
install on the owner host before the applicable ADR 020/021 gates pass. A missing
gate is reported rather than replaced by an acknowledgement checkbox.

Rollback disables new setup metadata and leaves the existing Compose connector
untouched. An uncertain native installation stays stopped for explicit recovery;
do not reset its credential/ledger or attempt automatic removal/upgrade.
Diagnostic output is fixed stages and public artifact identity, not tokens,
HTTP bodies, host observations or arbitrary tool errors in hosted records.

## Primary evidence, accessed 2026-10-26

- [GitHub immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases): stage all assets in a draft before publication; immutable tag/assets and release attestations do not replace independent local byte checks.
- [curl manual](https://curl.se/docs/manpage.html), supported Ubuntu curl baseline: `-q` disables curlrc; protocol, TLS, timeout and download-size controls are explicit. Redirect following must not permit an HTTP downgrade.
- [Go traversal-resistant file APIs](https://go.dev/blog/osroot): lexical validation alone does not prevent filesystem races. The existing Go bundle/installer uses anchored roots; bootstrap avoids mutable shared staging and never extracts unverified network bytes into a live tree.
- [Python 3.12 tarfile security](https://docs.python.org/3.12/library/tarfile.html#extraction-filters): archive members can carry links and special metadata; do not treat generic extraction defaults as a security boundary. This design instead pins the entire publisher archive and checks its exact closed members before extraction.
- [Go archive/tar formats](https://pkg.go.dev/archive/tar#Format): the actual publisher uses `FormatUSTAR` and `TypeReg` in `internal/connectedbundle/archive.go`; validate its exact closed output, not the broader formats a generic tar reader accepts.
- [Python 3.12 gzip](https://docs.python.org/3.12/library/gzip.html): bounded decompression, CRC/truncation failures and possible concatenated members require explicit total/read/EOF handling; a valid gzip wrapper alone says nothing about safe archive contents.

These are a narrow fixed-artifact implementation choice, not a claim that shell
scripts are a general privileged package manager. No owner decision is open for
the helper itself; remaining installed safety gates remain mandatory.
