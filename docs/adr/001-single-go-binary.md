# ADR 001: Use one Go artifact with separated runtime roles

**Status:** Accepted  
**Date:** 2026-09-09

## Context

The product must be simple to run on Windows, macOS, and Linux, headless or locally interactive, while enforcing different privileges for machine collection, remote upload, and presentation.

## Decision

Build one Go executable for each supported OS/architecture. It exposes explicit collector, uploader, local web, installer/doctor, and future action modes. Installed profiles run authority-bearing roles as separate OS-managed processes and ACL-separated state; sharing an executable is not treated as a security boundary.

Native packages are primary. An optional multi-architecture Linux container supports container-oriented deployments but does not claim physical Windows/macOS host visibility through Docker Desktop.

## Consequences

- Owners install one project without a language runtime.
- Domain contracts, sanitization, configuration, and update logic remain shared.
- Platform adapters and actual-host tests are mandatory.
- Packaging/signing remains platform-specific.
- A single-process convenience mode may exist for development but cannot be described as privilege equivalent to the installed profile.

## Alternatives

Packaged Python would migrate faster but increases runtime and packaging complexity. A composed telemetry stack offers more query power but conflicts with the simple single-machine install goal. Both remain useful sources of patterns, not the core runtime.
