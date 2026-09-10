# Native release smoke profiles

Status: implementation checkpoint; workflow activation remains a separate release decision.

The existing native delivery command discovers the package profile only from the
checksum-covered `release-manifest.json`. It accepts the closed
`observer-release/v1` and `observer-release/v2` manifests and rejects unknown or
mixed profiles. V1 keeps the original four-file archive and installer invocation.
V2 requires exactly five files in each Linux archive, including
`observer-journal-helper`; Darwin and Windows remain exact four-file archives.

Before any executable runs, bundle and anonymous-download smoke verify the exact
release file allowlist, every checksum, the manifest schema and immutable asset
identity, and every archive member name. For both Linux archives, smoke extracts
only the already allowlisted package into an owned temporary directory and checks
the helper's regular-file type, size, and SHA-256 against that archive's manifest
record. It never starts the helper or queries a host journal.

Native smoke continues to run the primary observer version, authenticated local
service, dashboard, graceful background lifecycle, and supported Windows manager
checks. A v2 offline install additionally receives the local manifest and
`SHA256SUMS`; the installer invokes the staged primary observer's bounded release
identity verification hook. Linux smoke then checks the installed helper bytes
against the same manifest record. Installation still starts neither executable,
and uninstall and v1 rollback behavior are unchanged.

This checkpoint does not activate a workflow or publish a v2 release. A release
workflow must provide final v2 artifacts on native Linux, macOS, and Windows
runners and pass this smoke before any support claim or publication.
