# Background lifecycle acceptance and review

## Requirement traceability

| Requirement | Owning boundary | Acceptance evidence required |
| --- | --- | --- |
| B1 explicit foreground/background choice | `cmd/observer` | Help/version/foreground unchanged; explicit lifecycle commands only |
| B2 session-only OS managers | `internal/background` | Native templates, least privilege, manager absence, documented login limitation |
| B3 typed configuration | CLI and manager settings | Exact registration arguments; reused Docker/listener policy; no secret or arbitrary command |
| B4 safe serialized registration | Manager and private filesystem helpers | Unknown/mismatched registration, linked path, interruption and concurrent operation tests |
| B5 honest status | Manager, readiness probe | Missing manager/registration, stopped/running/unreachable/not-ready cases |
| B6 graceful stop | `internal/lifecycle` and serve | Fresh nonce, stale/replay/oversize/path attacks; real binary shutdown and store reopen |
| B7 independent programs and data | Installers and manager | Upgrade selection, explicit restart, uninstall refusal, data preservation |
| B8 bounded private diagnostics | `internal/diagnostics` | Rotation, total bytes, oldest-record age, record allowlist and secret canaries |
| B9 nonfatal diagnostic health | Local API and optional UI source | Closed authenticated schema; unavailable is not zero; independent UI failure |
| B10 verification | Native CI, contracts and browser suite | Three native runtime OS jobs, six cross-build targets, responsive UI and independent review |

## Review process

Runtime, OS-manager and CLI/API/UI slices are authored in independent worktrees. Review is cross-slice: no author's
own implementation approval replaces another review. Root integrates only committed, tested slices and required CI
must pass before auto-merge. No developer-host startup registration or production server mutation is part of testing.

Early integration review identified these cases for explicit regression coverage:

- Validate existing ancestor links before creating missing state directories; rejection must not mutate the link target.
- Refuse manager-unavailable disable rather than remove local evidence of a potentially active registration.
- Close validation-only file handles; bound marker/template reads and reject linked files before touching them.
- Keep state outside program roots and reuse the collector's Docker endpoint rules.
- Avoid error-variable shadowing that could hide diagnostic write failures.
- Enforce age from retained record age, not continuously refreshed file modification time.
- Show unavailable diagnostic disk usage as unavailable, not a factual zero-byte reading.

These are review requirements, not claims that unfinished tests already pass. Final code hashes, tests, reviewer
decisions, hosted checks and release verification will be recorded here after integration.

## Scope safeguards

The public read-only HTTP surface has no lifecycle mutation route. User-manager control is only a local CLI operation
under the caller's existing account authority. No Docker auto-discovery, machine-wide service, remote enrollment,
raw native log ingestion, or arbitrary command executor is introduced by this specification.
