# Spec 007: Verifiable native preview delivery

**Status:** Accepted under the owner's instruction to continue the agreed delivery plan

## Outcome

Download, verify and run Home Lab Observer on Windows, macOS or Linux without installing Go, Node, Docker or a GitHub
token. Keep the local dashboard and foreground/headless operation useful independently of Platform Demo. This slice
delivers native archives and safe per-user install/upgrade/rollback helpers. Background service registration and the
constrained-proxy Linux container are subsequent focused delivery slices, not hidden side effects of installing.

## Requirements

- D1: Produce six versioned archives: linux/darwin/windows × amd64/arm64. Windows uses zip; Unix uses tar.gz. Include
  the embedded-UI observer binary, license, concise platform instructions and launch helpers. No private infrastructure,
  credentials, node_modules or source maps enter artifacts. Unix executables retain executable mode.
- D2: Publish a closed observer-release/v1 manifest, SHA256SUMS, final-byte provenance and SPDX dependency information.
  The manifest contains version (without v), tag (v-prefixed), repository, exact commit SHA, preview signing policy and
  six asset records (os, arch, filename, sha256, size_bytes, format, download_url). URLs are version-pinned GitHub releases
  for braidenm/home-lab-observer. No latest/dynamic execution URL or remote arbitrary package coordinate is accepted.
- D3: Release workflow is manual and operates on main only; validates a new explicit prerelease version, builds/tests,
  verifies artifacts on native runners, scans dependencies and attests before publishing. Public PR workflows have no
  write/OIDC privileges. Full-SHA action pins and least privilege remain mandatory. Normal PR checks remain under 15 min.
- D4: Bash (Linux/macOS) and PowerShell (Windows) helpers support explicit version/download and offline verified-archive
  installation without GitHub credentials. Use HTTPS, private staging, SHA-256 verification before extraction/execution,
  fixed archive contents and versioned per-user directories. Reject unsafe versions/paths, link/traversal entries and
  unexpected archive members. Never install dependencies, elevate, change PATH, start Docker or register services silently.
- D5: Preserve the currently selected version and data until an installation is complete. A managed launcher selects a
  validated installed version; explicit rollback switches to a retained version. Uninstall removes only known managed
  program files, preserves observation/token state by default, and refuses broad/unmanaged roots. No auto-update.
- D6: Archive launch helpers start the foreground local server; script/headless use remains documented. Add a stable
  observer version command (human/JSON) for verification. No arguments starts the foreground local server for direct-run
  convenience; --help remains explicit and no browser/network/privilege change is triggered by version/help.
- D7: Clearly label Windows/macOS previews as lacking publisher signing/notarization until owner identities exist,
  as already accepted in ADR 004. Never imply checksums provide publisher identity; document attestation verification
  and OS trust prompts without recommending disabling system-wide protections. GA native signing is still a gate.
- D8: Test archive determinism/content/bounds, native startup/version, checksum mismatch, invalid versions, unsafe
  archives, paths with spaces, installation/upgrade/rollback/uninstall and state preservation. Review installer and
  release authority boundaries independently before auto-merge. Publish the first prerelease only from merged code.

## Remaining delivery sequence

After native previews: focused background lifecycle/log-retention and constrained-proxy container packaging; additive
outbound Platform Demo enrollment/upload; richer explicitly enabled safe log sources. Future commands remain separately
permissioned and out of this observer preview. No existing private package is made public or replaced automatically.
