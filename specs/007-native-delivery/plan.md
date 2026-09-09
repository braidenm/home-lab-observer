# Implementation plan

Separate archive construction, owner installation and privileged release publication. A standard-library Go releasepack
command builds deterministic fixed-content archives from staged cross-built binaries and emits the closed release
manifest/checksum list. Archive metadata always refers to the final bytes. Per-platform installer scripts consume the
same fixed naming/layout contract but require no development runtime. Native CI tests use synthetic local archives and
temporary user roots; they never install the product as a service on the owner's workstation.

Parallel slices: releasepack/contracts; Bash/PowerShell install and lifecycle helpers; release workflow/native acceptance;
integration owner handles CLI version/direct-run behavior, docs, independent review coordination and publication.

Each archive has exactly one root directory home-lab-observer_VERSION_OS_ARCH containing observer[.exe], LICENSE,
START-HERE.md, and run-observer.sh (Unix) or Run-Observer.cmd (Windows). Archive names are the same root plus .tar.gz/.zip.
Manifest assets use os=linux|darwin|windows and arch=amd64|arm64. Version is SemVer prerelease without a leading v; tag
adds v. Installer scripts are separately uploaded versioned release assets, not embedded in the manifest's binary list.
SHA256SUMS covers all downloadable files except itself; provenance attests archives, manifest, installer scripts and SBOM.
