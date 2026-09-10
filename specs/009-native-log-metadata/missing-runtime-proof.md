# Missing Linux runtime packaged-core proof

Status: test-only fixture; actual Docker execution requires a hosted Linux CI runner. No release activation implied.

`node scripts/test-missing-linux-runtime.mjs RELEASEPACK_DIRECTORY` consumes the unfinalized eight-file releasepack
output (six archives, manifest, checksums), before installers/SBOM are added. Inputs are trusted CI artifacts, not a
download authenticity mechanism. The staging utility verifies all archive/checksum contracts, requires v2 and the
host Linux architecture, then extracts the exact five-file package into a newly owned context. The runtime probe
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
