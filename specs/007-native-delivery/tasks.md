# Tasks

- [x] Define native preview and supply-chain boundaries.
- [x] Add deterministic archive/manifest tool and tests.
- [x] Add Bash and PowerShell installation/upgrade/rollback/uninstall helpers.
- [x] Add CLI version/direct-run behavior and archive quick-start instructions.
- [x] Add native artifact/installer smoke and least-privilege manual release workflow.
- [x] Verify dependency/security evidence and independent reviews; auto-merge through required checks.
- [x] Publish and anonymously verify the first native preview release.

Completed in [PR 10](https://github.com/braidenm/home-lab-observer/pull/10), with all 12 required checks passing.
[Release v0.1.0-preview.1](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-preview.1)
is immutable and contains six native archives, installers, manifest, SBOM and checksums.
[Release run](https://github.com/braidenm/home-lab-observer/actions/runs/34396422509) verified all six OS/architecture
combinations, attestations and anonymous downloads on 2026-09-09.
