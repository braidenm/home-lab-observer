# Native v2 build contract

Status: BUILD-ONLY implementation contract. Workflow activation, archive publication and target execution remain
separate gates.

## Trusted inputs and environment

`scripts/build-native-v2.sh VERSION COMMIT NEW_OUTPUT_DIRECTORY` runs only on an Ubuntu amd64 or arm64 build host.
The output parent already exists and the selected output path must be new; the builder creates that directory with
owner-only permissions. It resolves its repository root from the script location, so a caller's working directory
cannot select another Go module. Version and commit grammar are validated by the same `internal/buildidentity`
encoder used by runtime and packaging.

`COMMIT` is a provenance assertion supplied by the authorized release workflow, not discovered by the builder. The
workflow must check out that exact immutable commit and compare it with the authorized GitHub SHA before invoking
the script. The builder deliberately does not inspect a potentially dirty Git worktree and passes
`-buildvcs=false`; the release workflow and attestation bind the source checkout, while the embedded record binds
the claimed version, commit, target and Linux helper pair. Direct callers that do not establish this precondition
must not describe their output as an official release.

Every Go command runs with `GOENV=off`, `GOWORK=off`, `GOFLAGS=-buildvcs=false`, empty `GOEXPERIMENT`,
`GOTOOLCHAIN=local`, `GOAMD64=v1`, `GOARM64=v8.0` and explicit host or target `GOOS`, `GOARCH` and `CGO_ENABLED=0`.
This ignores persistent user Go configuration, workspace selection, VCS stamping defaults, architecture tuning and
ambient cross-compilation variables. The authorized workflow installs the exact `go.mod` toolchain first; module
download policy and compiler trust remain workflow/toolchain concerns.

## Exact output and pairing

The new directory contains exactly eight regular build outputs:

- `observer_linux_amd64`, `observer_linux_arm64`
- `observer_darwin_amd64`, `observer_darwin_arm64`
- `observer_windows_amd64.exe`, `observer_windows_arm64.exe`
- `observer-journal-helper_linux_amd64`, `observer-journal-helper_linux_arm64`

For each Linux architecture, the helper is built first. Its final SHA-256 is embedded into the matching main
binary's authoritative identity; helper and main identities carry the same version, commit, OS and architecture.
Non-Linux identities contain no helper digest. All builds use trimpath, an empty Go build ID, stripped symbols and
the closed `main.releaseIdentity` value. Nothing in this builder executes a target output.

`cmd/releaseidentity --scan FILE` reads one regular file (rejecting symbolic links), enforces the existing 200-MiB
scanner bound and prints its single canonical identity. This trusted build-input utility does not reject hard
links; release packaging independently enforces its stricter input policy. Formatting flags and `--scan` are
mutually exclusive. Scan validation failures expose only the fixed `BUILD_IDENTITY_INVALID` code, never a path or
binary content; command-line syntax errors use standard flag-parser diagnostics.

## Reproducibility gate

`OBSERVER_TEST_REPRODUCIBLE_BUILDS=1 bash scripts/test-build-native-v2.sh` is the single Ubuntu-only opt-in gate. It:

1. builds the eight outputs twice, with the second invocation carrying deliberately conflicting ambient Go values;
2. requires the exact regular-file allowlist and byte-for-byte equality between builds;
3. scans every binary with `buildidentity`, checks every target identity and recomputes each Linux helper digest;
4. verifies Linux main build metadata records `CGO_ENABLED=0`; and
5. checks both Linux architecture dependency graphs exclude `journalnative`, `journalreader`, `journalruntime` and
   `purego`, keeping native loading inside the separately packaged helper.

Without the explicit opt-in the test exits successfully with a skip message. Workflow integration should run this
once on an Ubuntu build job, not repeat the sixteen compilation operations in every native target smoke job.
