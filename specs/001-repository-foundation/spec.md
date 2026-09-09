# Spec 001: Repository foundation

**Status:** Accepted  
**Owner:** Repository owner  
**Created:** 2026-09-09

## Problem

The observer prototype is coupled to a private infrastructure repository and delivered only as a Linux container. A reusable public portfolio project needs an independent history, explicit security boundary, cross-platform architecture, governed delivery process, and clear relationship with Platform Demo before production code is moved or expanded.

## Goals

1. Establish `braidenm/home-lab-observer` as a deliberately public, MIT-licensed repository with no private infrastructure history.
2. Define spec-driven governance, engineering standards, architecture research, ADRs, threat boundary, public API direction, and supported delivery targets.
3. Keep public pull-request CI isolated from private self-hosted infrastructure and below 15 minutes.
4. Enable squash auto-merge gated by required checks.
5. Define how the old private prototype is retired without making its package or repository public.

## Non-goals

- Shipping a production observer binary.
- Making the legacy GHCR package public.
- Implementing remote actions or arbitrary command execution.
- Copying private infrastructure history, configuration, or secrets.

## Requirements

- **R1:** Public names, package coordinates, binaries, and services use `home-lab-observer`.
- **R2:** The constitution codifies read-only defaults, local ownership, portability, contract-first design, bounded data, operability, and verified delivery.
- **R3:** Architecture research compares at least a native single binary, packaged scripting runtime, and composed telemetry stack using primary sources.
- **R4:** Accepted ADRs choose the runtime/distribution and define the local UI/Platform Demo boundary.
- **R5:** The repository documents sensitive data exclusions and the public/private boundary.
- **R6:** CI uses GitHub-hosted runners for pull requests; no public pull-request workflow references private runner labels or secrets.
- **R7:** The replacement image will be `ghcr.io/braidenm/home-lab-observer`; the legacy package remains private until it can be removed after a verified migration.
- **R8:** The executable scope is tracked by a separate spec with testable cross-platform acceptance criteria.

## Acceptance criteria

- All documents linked from the README exist and cross-reference the relevant decisions.
- No tracked file contains private host identifiers, production credentials, or private infrastructure content.
- Repository settings allow auto-merge and delete merged branches.
- A pull request validates the foundation on GitHub-hosted runners and merges only after required checks pass.
- The next implementation specification clearly identifies unresolved owner decisions and privacy defaults.

## Owner decisions

Accepted on 2026-09-09 unless amended:

- Go single binary, with optional Docker packaging.
- Optional loopback-only dashboard served by the headless process.
- Platform Demo consumes versioned APIs and schemas rather than embedding the observer UI.
- Public repository and package are named `home-lab-observer` and use the MIT license.
- First release is read-only and data-rich; operational actions are a later permissioned specification.
