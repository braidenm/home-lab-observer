# Standalone read-only preview: requirement and evidence closeout

The published, immutable [Preview 3](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-preview.3)
is a local observer, not a Platform Demo connector. It runs in the foreground without an account or development
toolchain, offers an authenticated loopback dashboard, and has an explicit per-user background option. Installation
does not enroll, start a service, enable Docker or native logs, or upload observations.

| Standalone requirement | Evidence and limit |
| --- | --- |
| Verifiable downloads on six native OS/architecture targets | [Spec 007](../../specs/007-native-delivery/tasks.md) and [release run 34528402601](https://github.com/braidenm/home-lab-observer/actions/runs/34528402601): six native verification jobs, final-byte attestations, vulnerability scan and anonymous immutable downloads passed. The release contains six archives, two installers, manifest, SPDX SBOM and SHA256SUMS. Windows/macOS previews remain unsigned/not notarized. |
| Direct run, local dashboard and headless use | [Install guide](../install.md) and [local service guide](../local-service.md): no arguments start the foreground server; a terminal can keep it running without opening a browser. The dashboard is loopback-only and observation APIs require the local token. Packaged native service, authentication and schema smoke ran on all six release targets. |
| Safe opt-in background operation | [Spec 008](../../specs/008-background-lifecycle/tasks.md) and [review evidence](../../specs/008-background-lifecycle/review.md): native manager, graceful stop, retained history and bounded diagnostics checks passed. This is a signed-in user-session profile, not machine-wide boot-before-login support. Installation does not enable it. |
| Meaningful host, process and container views | [Spec 005](../../specs/005-local-data-plane/tasks.md) records real numeric trends, bounded history, authenticated embedded views and 390/768/1440-pixel checks. [Spec 006](../../specs/006-container-observations/tasks.md) records opt-in cached Docker inventory, fake-engine/native transport tests and real Linux engine smoke. Desktop Docker live acceptance on Windows/macOS remains explicitly unclaimed; services and hardware sensors remain unsupported. |
| Useful local history and honest optional sources | [Spec 009 traceability](../../specs/009-native-log-metadata/traceability.md): native fixtures, bounded persistence, degraded/restart smoke and synthetic responsive UI checks passed. Linux/Windows native log metadata requires explicit opt-in; macOS is unsupported. This metadata is not remote-upload eligible. |
| Documented modular portfolio project | [Constitution](../../.specify/memory/constitution.md), [specifications](../../specs/README.md), [ADRs](../adr/README.md), [data policy](../privacy/data-policy.md) and [threat model](../security/threat-model.md) describe replaceable collection, storage, transport and UI boundaries; observation does not grant control. The public repository and independent releases do not publish the private infrastructure repository. |

The independent [paired reproducibility run](https://github.com/braidenm/home-lab-observer/actions/runs/34527795041)
passed against the Preview 3 source commit `9a60a2845d716d2f252a8852f893a5825f6b7ef6`. The Spec 009 UI review used
synthetic data at 390, 768 and 1440 pixels; it is not a claim that a signed-in remote dashboard or every native source
was visually exercised on a personal machine.

Separate additive work does **not** change this release's standalone scope. Observer
[PR 35](https://github.com/braidenm/home-lab-observer/pull/35) merged a pure, unused numeric-host projection contract.
Platform Demo [PR 365](https://github.com/braidenm/platform-demo/pull/365) proved its actual snapshot parser accepts
the identical synthetic fixture, including available/unavailable host data and server-versus-connector identity;
it changed no production receiver behavior. Platform Demo
[PR 366](https://github.com/braidenm/platform-demo/pull/366) merged an opt-in, startup-validated native release
catalog and OS-aware download/setup choice. That page labels the native binary local-only and preserves the legacy
connected Linux package. These contract and catalog PRs do not install or activate an observer uploader, enroll a
machine, or establish remote connectivity. Those require their own reviewed installation and acceptance gates.
