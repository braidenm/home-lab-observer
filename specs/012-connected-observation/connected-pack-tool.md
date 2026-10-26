# Closed connected release packer

Status: build-only Spec 012 slice. The command itself cannot install, enroll,
start, download, publish, select or roll back a connected worker. It now also
renders the separate [public bootstrap](public-bootstrap.md); that helper remains
unpublished until its disposable-VM/download gates pass.

`connectedpack identity` emits the canonical V2 identity for one of the three
fixed Linux/amd64 roles and the compiled compatibility digest. `pack` accepts
only an exact three-binary input directory, a valid prerelease version and
source commit, and a newly created output directory. It copies bounded inputs
exclusively, includes only the four reviewed resources, verifies the exact V2
bundle, and writes a deterministic archive, manifest, version-pinned
`install-connected-linux.sh` and `SHA256SUMS` with checksums last. The script pins
the completed archive and each canonical USTAR member. Checksums cover archive,
manifest and script, never themselves. Public bootstrap packaging accepts only
bounded canonical `0.x.y-canary.N` versions and archives at most 256 MiB.
A failed build may leave an incomplete new output directory;
it must never be published or treated as an installable release. The tool does
not delete or overwrite caller data to hide that failure.

Tests cover closed options, refusal to emit final checksums for missing
binaries, and one real synthetic ELF pack journey checking all final asset
digests and helper pins. Bootstrap tests cover closed rendering, malformed
archives and joined cancellation with terminal preservation. Bundle-level tests
cover content and archive integrity. The later
build script, signed release provenance, actual command roots, two-release
compatibility and packaged VM acceptance are separate gates.
