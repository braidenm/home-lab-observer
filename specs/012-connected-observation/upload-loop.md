# C2: portable upload loop proposal

Status: Proposed, 2026-09-11. Documentation only; implementation requires review. No CLI, service, filesystem,
network adapter, enrollment, collector or worker activation. C1 retains every admission/sequence decision.

## Minimal interface

Proposed `internal/uploadloop` API:

```go
type Stepper interface { Step(context.Context) (uploadstate.Outcome, error) }
func New(stepper Stepper) (*Loop, error)
func (*Loop) Run(context.Context) (Result, error)
func (*Loop) Stats() Stats
```

`Run` is synchronous and single-use. A concurrent call fails busy; subsequent calls fail used. The caller may
start it in its own goroutine, cancel the context and join that goroutine before closing any dependencies.
There is no second Start/Stop abstraction, manual-trigger queue or goroutine per attempt. Each Step completes
before another can begin. No successful return while Step is still running. A dependency stuck in kernel I/O
can delay joining: context cancellation is not forced termination. Service shutdown escalation belongs to F1.

Reuse the existing scheduler's serialized sampling, injected timing and explicit join conventions, but do not
import `internal/scheduler`: it owns host history/projection/store closing, which this loop must not acquire.
Use standard-library timers and `math/rand/v2`; jitter is scheduling, not cryptographic identity. Keep timer and
jitter injection package-private for deterministic tests; avoid exposing production pacing overrides.

## Pacing policy proposed for review

All delays begin after the completed Step result, use monotonic timers and are interruptible. There is no ticker
backlog or catch-up burst. Saturate retry state before multiplication; the failure counter cannot overflow.

| Step result | Next operation |
| --- | --- |
| ACKNOWLEDGED or IDLE | Reset retry state; wait 15 seconds. |
| SOURCE_UNAVAILABLE, EXPIRED_DELIVERY_UNKNOWN or CLOCK_SKEW | Reset retry state; report fixed state and poll after 15 seconds. Never alter timestamps or ledger. |
| RETRY | Caps 1, 2, 4, 8, 16, 32, 60 seconds; jitter within max(1 second, cap/2)..cap. Cap remains 60 thereafter. |
| RATE_LIMITED | Wait exactly 60 seconds, with no negative jitter; raise retry cap to 60 until a non-retry result resets it. |
| CREDENTIAL_REJECTED, CONFLICT, REJECTED or SEQUENCE_EXHAUSTED | Stop with the corresponding fixed terminal result; never reopen/reset/re-enroll. |
| ErrSnapshot | Fixed invalid-source diagnostic, then 15-second poll; a later freshly valid observation may recover without repairing current bytes. |
| ErrRecovery | Stop recovery-required. No automatic machine reconstruction. |
| ErrCanceled with canceled parent | Stop canceled after Step returns. |
| ErrBusy, other errors, unknown outcomes, or ErrCanceled without canceled parent | Stop with a fixed dependency/protocol failure; never stringify the dependency error. |

Cancellation takes precedence over scheduling another call. Do not classify a send interrupted by cancellation
as acknowledged: C1 owns acknowledgement and leaves the durable pending record authoritative.

**Restart-rate-limit question:** memory-only pacing cannot remember a prior process's 429. The minimal proposed
solution is a mandatory 60-second startup wait on every Run, also canceled/joined normally. This avoids adding a
cooldown journal or changing D1 and prevents rapid restart from bypassing the normal cooldown. It delays the first
fresh enrollment upload by up to one minute. Approve that latency or separately specify durable cooldown before
implementation; do not quietly promise cross-restart pacing while starting immediately. It is not proof against
arbitrarily delayed remote processing or independently duplicated worker installations.

## Diagnostics and ownership

`Result` and `Stats` contain only closed code-owned statuses, bounded/saturating aggregate attempt/ack/retry/rate-limit
counters, running/stopped state and the next delay. No source/server/connector IDs, sequence, body, grant, credential,
URL, raw exception, unbounded history or callback receives data. Stats is a detached concurrency-safe value, not a
log sink. The later worker maps these fixed fields into existing bounded diagnostics and avoids logging each idle poll.

The loop borrows Stepper. It never closes ledger, HTTP transport or handoff. Caller shutdown order is cancel, join Run,
then close borrowed resources; `CloseIdleConnections` alone is not a join. READY validation and one installed owner remain
external prerequisites. No runtime import may pull collector, credential/filesystem adapters, HTTP or service packages.

## Acceptance before implementation completion

- [ ] Approve startup cooldown versus separately durable pacing; freeze public Result/Stats enums without duplicating C1 state.
- [ ] Deterministic fake timer/jitter tests prove all bounds, saturation, resets and exact 60-second rate-limit floor.
- [ ] Blocked Step proves one in-flight call and cancellation waits for it; canceled waits never call Step again.
- [ ] Single-use/concurrent Run, canceled-before-start, nil context and unknown/error results fail with fixed values.
- [ ] Invalid observations poll without sending or modifying state; terminal/recovery results never retry.
- [ ] Compose with real C1 and synthetic source/ledger/transport to prove identical pending bytes/sequence across retries,
  no calls during cooldown, and no post-cancellation acknowledgement. No real network or secret fixtures.
- [ ] Stats snapshots are detached and race-safe; malicious dependency errors never reach results or diagnostics.
- [ ] Full tests/vet/race and supported-target builds; existing rich scheduler behavior unchanged.

Code inspected: `internal/uploadstate/machine.go` and `internal/scheduler/scheduler.go` at main `66bbaa4`.
This is a small scheduling policy proposal, not installed isolation or production readiness evidence.
