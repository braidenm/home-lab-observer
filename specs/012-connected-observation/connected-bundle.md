# Exact connected-canary bundle verification

Status: implementation slice under Spec 012 and ADR 022. This is a read-only
bundle verifier and deterministic archive writer, not a downloader, release
publisher, installer, code-selection operation or rollback permit.

The closed bundle contains exactly three Linux/amd64 Go ELF commands and four
reviewed unit/schema resources, plus one canonical bounded manifest. Each
entry has a fixed name, mode, size limit and SHA-256. V1 retains its exact
legacy format for explicit diagnosis but carries no recognized compatibility
contract. V2 requires the compiled descriptor digest and matching V2 identity
in each of the three binaries. A declared contract is not self-authenticating.

Verification requires an independently trusted manifest digest, a dedicated
safe directory, exact inventory, bounded regular files, no unsafe links,
unchanged file identity during reading, exact content hashes, static Linux
ELF/Go build settings and one matching role identity per binary. Resource
bytes must equal the compiled templates, not merely a manifest claim. The
archive writer rehashes bytes as it copies and emits a deterministic flat
tar.gz with closed names, modes and timestamps.

Tests use tiny synthetic Linux Go ELF binaries without executing them and
cover malformed manifests, mixed generations, recomputed self-assertions,
changed files, wrong roles/resources, extra entries and archive determinism.
The privileged installer must still verify its freshly staged final copy,
release provenance, two-release state compatibility, ledger identity,
interrupted transition recovery and disposable-VM behavior before selecting
or activating code. This library does not satisfy those gates.
