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
The opt-in real Windows manager smoke is limited to disposable GitHub-hosted CI and an installer-created temporary
root; it refuses local/self-hosted execution. Guards use the documented `GITHUB_ACTIONS` and `RUNNER_ENVIRONMENT`
[GitHub variables](https://docs.github.com/en/actions/reference/workflows-and-actions/variables), verified 2026-09-09.
If manager cleanup is uncertain, the test preserves its files instead of uninstalling beneath a possible running job.

Early integration review identified these cases for explicit regression coverage:

- Validate existing ancestor links before creating missing state directories; rejection must not mutate the link target.
- Refuse manager-unavailable disable rather than remove local evidence of a potentially active registration.
- Close validation-only file handles; bound marker/template reads and reject linked files before touching them.
- Keep state outside program roots and reuse the collector's Docker endpoint rules.
- Avoid error-variable shadowing that could hide diagnostic write failures.
- Enforce age from retained record age, not continuously refreshed file modification time.
- Show unavailable diagnostic disk usage as unavailable, not a factual zero-byte reading.

Core implementation `742cd35` (root integration `6f38498`) passes root's independent full Go tests, static analysis
and repository policy check. Its focused tests cover oldest-record age with an idle housekeeping tick, injected write
failure, hard links/ancestor links, stale/wrong/malformed stop requests and instance locking. Linux/macOS test binaries
cross-compile; hosted native execution and race detection remain pending. UI author checks and root's independent
rerun pass 35 tests and package builds. This is partial evidence, not a claim that the integrated feature or release
has passed.

### Integrated evidence, before final CI acceptance

Root reran the full Go test suite and static analysis, PowerShell 5.1 installer lifecycle/safety tests, closed API
contracts, 35 UI tests, deterministic embedded assets and packaged-style runtime smoke. The actual local binary
passed two nonce-controlled starts/stops with durable history sequence recovery. Browser checks passed at
390/768/1440 widths, including available and unavailable diagnostics; root visually reviewed mobile unavailable and
desktop available states. Tests did not register startup on the developer machine.

Independent review approved CLI/API/UI boundaries and graceful shutdown ordering, including durable-close failure
propagation and private token ownership. Review also found a shared installer/background mutation race: the common
guard and under-lock identity revalidation must both be verified before acceptance. Windows hosted saved-task
normalization remains a failing required check; the in-memory native template round trip is not a substitute for
that real manager lifecycle test. Linux/macOS native runtime and packaged tests pass on the current PR candidate.

The process-exit smoke helper now clears completed deadlines: a successful background smoke completes in roughly
two seconds locally rather than waiting for an unused 45-second timer. A regression test verifies timer/listener
cleanup. Final independent guard sign-off, all required CI checks and publication evidence are still pending.

## Scope safeguards

The public read-only HTTP surface has no lifecycle mutation route. User-manager control is only a local CLI operation
under the caller's existing account authority. No Docker auto-discovery, machine-wide service, remote enrollment,
raw native log ingestion, or arbitrary command executor is introduced by this specification.
