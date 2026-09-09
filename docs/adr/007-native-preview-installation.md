# ADR 007: Native preview installation without implicit service authority

**Status:** Accepted

Deliver per-user, versioned native archives before privileged background-service or container profiles. Installation is
not authorization to elevate, register a startup task, enable Docker or delete observations. Owner-run helpers verify
fixed release assets, stage a complete version and then switch a managed launcher, retaining rollback. Data lives in the
existing private state directory independently of program versions. No-argument execution starts the foreground server.

Follow ADR 004's explicitly labeled Windows/macOS preview policy until publisher signing/notarization identities exist.
SHA-256 verifies bytes against a trusted release, not identity; GitHub build provenance supplies a separately verifiable
source/workflow claim. Neither warrants telling owners to disable platform-wide security. GA requires native signing.

Release publication requires a new explicit prerelease tag from merged main, native artifact verification, a dependency
inventory and attestations. Separate implementation specifications own service lifecycle, constrained Docker packaging,
and remote enrollment rather than mixing new privilege boundaries into a convenience installer.
