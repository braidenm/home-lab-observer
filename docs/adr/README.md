# Architecture decision records

| ADR | Status | Decision |
| --- | --- | --- |
| [001](001-single-go-binary.md) | Accepted | One Go artifact with platform adapters and separately permissioned runtime roles |
| [002](002-api-first-local-dashboard.md) | Accepted | Same-origin loopback dashboard and API-driven Platform Demo integration |
| [003](003-bounded-observations-and-logs.md) | Accepted | Bounded SQLite observations with privacy-preserving log defaults |
| [004](004-independent-release-supply-chain.md) | Accepted | New package identity, native artifacts, attestations, and safe migration |
| [005](005-local-service-and-history.md) | Accepted | Authenticated foreground service with bounded history and embedded assets |
| [006](006-specialized-container-read-model.md) | Accepted | Dedicated container read model preserves legacy contracts and unknown metrics |
| [007](007-native-preview-installation.md) | Accepted | Versioned per-user previews without implicit service authority |
| [008](008-local-background-convenience-profile.md) | Accepted | Explicit user-session background lifecycle and bounded self-diagnostics |
| [009](009-native-log-metadata.md) | Accepted | Opt-in fixed-source metadata with bounded history and honest coverage |
| [010](010-optional-linux-journal-helper.md) | Accepted | Optional bundled Linux journal helper preserves core portability |
| [011](011-windows-log-continuation-proof.md) | Accepted | Windows continuation uses selected identity with an explicit non-forensic collision limitation |
| [012](012-explicit-remote-host-projection.md) | Accepted for pure projection | Explicit numeric-host egress profile; no upload or credential activation |
| [013](013-private-latest-snapshot-handoff.md) | Accepted for credential-free library | Private disposable handoff; installed sharing remains gated |
| [014](014-linux-connected-canary-workers.md) | Accepted for pure state machine | Isolated Linux connected canary remains proposed and gated |
| [015](015-durable-upload-ledger.md) | Accepted for unused Linux adapter | Separate bounded durable sequence ledger; never recreate lost state |
| [016](016-fixed-platform-https-transport.md) | Accepted for unused snapshot adapter | Fixed HTTPS destination and separate enrollment ambiguity boundary |
| [017](017-fail-closed-enrollment-coordination.md) | Accepted for unused coordination kernel | One-shot enrollment fails closed before credential, ledger, and READY activation |
| [018](018-shared-read-handoff.md) | Accepted for uninstalled Linux library | Exact-principal shared-read handoff, separate from owner-private storage |
| [019](019-private-enrollment-persistence.md) | Accepted for unused Linux adapter | Exclusive private enrollment commit witnesses; installed credential promotion remains separate |
| [020](020-linux-installed-worker-composition.md) | Accepted for implementation | Bounded first installed Linux worker composition; release remains gated |
| [021](021-connected-activation-evidence.md) | Accepted for implementation | Current invocation and same-process checks before connected upload |
| [022](022-compatible-connected-code-transitions.md) | Proposed | Explicit verified code selection, used-ledger compatibility and one transition journal |
| [023](023-opt-in-connected-inventory.md) | Proposed | Versioned opt-in workload egress and reviewed rich acquisition profile; numeric canary unchanged |
| [024](024-immutable-rich-installation-profiles.md) | Accepted for unused pure state machine | Immutable separately enrolled rich policy; profile-bound ledger and installed activation remain gated |
| [025](025-isolated-installed-docker-observation.md) | Accepted for engineering design only | Fixed credential-free Docker reader, immutable separately enrolled process-and-Docker profile; implementation/activation gated |
| [026](026-docker-reader-startup-barrier.md) | Accepted for dormant implementation | Reuse same-process startup rendezvous before reader Engine I/O; no second permit protocol |

Accepted ADRs are immutable except for status and supersession links. Material changes require a new ADR.
