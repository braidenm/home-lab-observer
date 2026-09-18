# Closed connected release packer

Status: build-only Spec 012 slice. This command cannot install, enroll, start,
download, publish, select or roll back a connected worker.

`connectedpack identity` emits the canonical V2 identity for one of the three
fixed Linux/amd64 roles and the compiled compatibility digest. `pack` accepts
only an exact three-binary input directory, a valid prerelease version and
source commit, and a newly created output directory. It copies bounded inputs
exclusively, includes only the four reviewed resources, verifies the exact V2
bundle, and writes a deterministic archive, manifest and `SHA256SUMS` with
checksums last. A failed build may leave an incomplete new output directory;
it must never be published or treated as an installable release. The tool does
not delete or overwrite caller data to hide that failure.

Tests cover closed options and refusal to emit final checksums for missing
binaries. Bundle-level tests cover content and archive integrity. The later
build script, signed release provenance, actual command roots, two-release
compatibility and packaged VM acceptance are separate gates.
