# Missing Linux runtime packaged-core proof

Status: test-only fixture; actual Docker execution requires a hosted Linux CI runner. No release activation implied.

`node scripts/test-missing-linux-runtime.mjs RELEASEPACK_DIRECTORY` consumes the unfinalized eight-file releasepack
output (six archives, manifest, checksums), before installers/SBOM are added. Inputs are trusted CI artifacts, not a
download authenticity mechanism. The staging utility verifies all archive/checksum contracts, requires v2 and the
host Linux architecture, then requires the single leading archive root directory derived from the verified asset
filename (without `.tar.gz`) and its exact five direct file children. Flat, alternate, duplicate or nested roots fail.
It extracts those five files into a newly owned flat package context. The runtime probe
executes the packaged primary's manifest verification mode, so identity, profile and actual helper pairing are checked.

The image is FROM scratch with only the verified package, manifest metadata and static Go test probe. No external base
image, native libraries/loader, journal, Docker socket, host mounts, network connectivity, production settings or service
registration is provided. A private writable tmpfs is the only observer state location. The process runs as UID 65532
with no capabilities, no-new-privileges, read-only root, bounded memory/PIDs/CPU and no Docker logging driver.

The probe starts the actual primary with `--log-source system`, verifies bearer protection and an authenticated current
snapshot, then requires an attempted system source with unavailable status and fixed runtime-failure reason, null counts
and no positive coverage. Initial not-yet-observed, disabled, healthy zero and permission/storage failures do not pass.
It verifies another authenticated current snapshot and readiness after this failure, then SIGTERMs and reaps the primary
with a finite deadline. Probe and child output never contain snapshot bodies, tokens, paths or raw startup errors.

All probe reads, output, polling and subprocess waits are bounded. The outer script bounds Go/Docker operations and
cleans only its random labelled container/image and owned temporary directory. Cleanup failure fails the check. No
workflow is modified by this slice. Unit tests exercise synthetic HTTP/projection/extraction seams; compiling them is
not evidence that the packaged scratch service ran. Hosted execution is a separate explicit acceptance gate.

`TestStageRealV2GoArtifacts` compiles eight synthetic Go identity-bearing binaries without executing them, builds actual
v2 releasepack archives, and stages both Linux profiles. An optional new `OBSERVER_TEST_STAGE_EXPORT` path preserves
that fixture for `TestStageExportedV2` under Linux using `OBSERVER_TEST_STAGE_INPUT`; the latter needs no Go compiler
or Docker. These synthetic binaries prove staging/layout, not actual helper runtime operation. Runtime JSON predicates
are also checked against the real local API projection and SQLite summary with synthetic failure batches.
