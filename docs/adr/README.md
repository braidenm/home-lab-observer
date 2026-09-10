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

Accepted ADRs are immutable except for status and supersession links. Material changes require a new ADR.
