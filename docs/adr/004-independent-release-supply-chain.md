# ADR 004: Publish from an independent, verifiable supply chain

**Status:** Accepted  
**Date:** 2026-09-09

## Context

The legacy Linux image is built from a private infrastructure repository and Platform Demo assumes one private package coordinate. Making that package public would preserve confusing provenance and an unnecessarily broad trust relationship.

## Decision

Publish new native artifacts and the optional image only from `braidenm/home-lab-observer`. The image is `ghcr.io/braidenm/home-lab-observer`. Existing packages remain private rollback artifacts during migration.

Releases contain a versioned multi-platform manifest, final-byte SHA-256 checksums, SPDX SBOMs, provenance/attestations, installers, and immutable versioned assets. Public pull requests use GitHub-hosted runners with no release secrets or OIDC. Installers verify before execution and keep enrollment secrets out of URLs, arguments, and environment variables.

Platform Demo migrates additively from its legacy single-image response to the release manifest, verifies anonymous clean-machine installation and a successful snapshot, then switches the approved version/digest. Old artifacts are deprecated only after rollback is proven.

## Consequences

- Publishing observer source and artifacts exposes no private repository history or configuration.
- The old private package remains independently revocable.
- Windows/macOS preview artifacts need explicit labeling until native signing/notarization identities exist.
- Apple notarization is outside the normal pull-request CI budget and belongs to the protected release workflow.
