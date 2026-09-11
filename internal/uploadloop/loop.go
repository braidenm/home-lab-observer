// Package uploadloop paces a borrowed upload state machine without activating a worker.
package uploadloop

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

var (
	ErrConfig     = errors.New("upload_loop_invalid_config")
	ErrBusy       = errors.New("upload_loop_busy")
	ErrUsed       = errors.New("upload_loop_used")
	ErrCanceled   = errors.New("upload_loop_canceled")
	ErrRecovery   = errors.New("upload_loop_recovery_required")
	ErrDependency = errors.New("upload_loop_dependency_failed")
)

const startupDelay = 60 * time.Second
const pollDelay = 15 * time.Second
const maxRetryDelay = 60 * time.Second

type Stepper interface {
	Step(context.Context) (uploadstate.Outcome, error)
}
type Result string

type Issue string

const (
	InvalidSnapshot Issue = "INVALID_SNAPSHOT"
	RecoveryIssue   Issue = "RECOVERY_REQUIRED"
	DependencyIssue Issue = "DEPENDENCY_FAILED"
)

const (
	Canceled           Result = "CANCELED"
	RecoveryRequired   Result = "RECOVERY_REQUIRED"
	DependencyFailed   Result = "DEPENDENCY_FAILED"
	CredentialRejected Result = "CREDENTIAL_REJECTED"
	Conflict           Result = "CONFLICT"
	Rejected           Result = "REJECTED"
	Exhausted          Result = "SEQUENCE_EXHAUSTED"
)

// Stats has no identifiers, payloads, dependency errors or unbounded history.
// LastOutcome is empty or one of the closed uploadstate outcomes.
type Stats struct {
	Running, Stopped                                                  bool
	Attempts, Acknowledgements, Retries, RateLimits, InvalidSnapshots uint64
	LastOutcome                                                       uploadstate.Outcome
	LastIssue                                                         Issue
	Result                                                            Result
	NextDelay                                                         time.Duration
}

type Loop struct {
	mu      sync.Mutex
	stepper Stepper
	used    bool
	stats   Stats
	wait    func(context.Context, time.Duration) error
	jitter  func(time.Duration) time.Duration
}

func New(stepper Stepper) (*Loop, error) {
	if stepper == nil {
		return nil, ErrConfig
	}
	return &Loop{stepper: stepper, wait: waitFor, jitter: retryJitter}, nil
}

// Run blocks until a terminal result or cancellation, including completion of
// the current Step. Caller owns goroutine creation, joining and resource cleanup.
func (l *Loop) Run(ctx context.Context) (Result, error) {
	l.mu.Lock()
	if l.stats.Running {
		l.mu.Unlock()
		return "", ErrBusy
	}
	if l.used {
		l.mu.Unlock()
		return "", ErrUsed
	}
	if ctx == nil {
		l.mu.Unlock()
		return "", ErrConfig
	}
	l.used = true
	l.stats.Running = true
	l.mu.Unlock()
	return l.run(ctx)
}

func (l *Loop) run(ctx context.Context) (Result, error) {
	if err := l.pause(ctx, startupDelay); err != nil {
		return l.waitFailure(ctx)
	}
	retryCap := time.Duration(0)
	for {
		if ctx.Err() != nil {
			return l.finish(Canceled, ErrCanceled)
		}
		l.mu.Lock()
		l.stats.Attempts = increment(l.stats.Attempts)
		l.mu.Unlock()
		outcome, err := l.stepper.Step(ctx)
		// An ACK is already durably committed by C1. Retain that fact even if
		// parent cancellation raced with Step's successful return.
		if err == nil && knownOutcome(outcome) {
			l.mu.Lock()
			l.stats.LastOutcome = outcome
			l.stats.LastIssue = ""
			switch outcome {
			case uploadstate.Acknowledged:
				l.stats.Acknowledgements = increment(l.stats.Acknowledgements)
			case uploadstate.Retry:
				l.stats.Retries = increment(l.stats.Retries)
			case uploadstate.RateLimited:
				l.stats.RateLimits = increment(l.stats.RateLimits)
			}
			l.mu.Unlock()
		} else {
			l.mu.Lock()
			l.stats.LastOutcome = ""
			l.stats.LastIssue = DependencyIssue
			if errors.Is(err, uploadstate.ErrCanceled) && ctx.Err() != nil {
				l.stats.LastIssue = ""
			} else if errors.Is(err, uploadstate.ErrSnapshot) {
				l.stats.LastIssue = InvalidSnapshot
			} else if errors.Is(err, uploadstate.ErrRecovery) {
				l.stats.LastIssue = RecoveryIssue
			}
			l.mu.Unlock()
		}
		if ctx.Err() != nil {
			return l.finish(Canceled, ErrCanceled)
		}
		delay := pollDelay
		if err != nil {
			switch {
			case errors.Is(err, uploadstate.ErrRecovery):
				return l.finish(RecoveryRequired, ErrRecovery)
			case errors.Is(err, uploadstate.ErrSnapshot):
				l.mu.Lock()
				l.stats.InvalidSnapshots = increment(l.stats.InvalidSnapshots)
				l.mu.Unlock()
				retryCap = 0
			default:
				return l.finish(DependencyFailed, ErrDependency)
			}
		} else {
			switch outcome {
			case uploadstate.Acknowledged, uploadstate.Idle, uploadstate.SourceUnavailable, uploadstate.Expired, uploadstate.ClockSkew:
				retryCap = 0
			case uploadstate.Retry:
				retryCap = nextRetry(retryCap)
				delay = l.jitter(retryCap)
				// Even a bad test seam cannot silently remove the pacing bound.
				if delay < max(time.Second, retryCap/2) || delay > retryCap {
					return l.finish(DependencyFailed, ErrDependency)
				}
			case uploadstate.RateLimited:
				retryCap = maxRetryDelay
				delay = maxRetryDelay
			case uploadstate.CredentialRejected:
				return l.finish(CredentialRejected, nil)
			case uploadstate.Conflict:
				return l.finish(Conflict, nil)
			case uploadstate.Rejected:
				return l.finish(Rejected, nil)
			case uploadstate.Exhausted:
				return l.finish(Exhausted, nil)
			default:
				return l.finish(DependencyFailed, ErrDependency)
			}
		}
		if l.pause(ctx, delay) != nil {
			return l.waitFailure(ctx)
		}
	}
}

func (l *Loop) pause(ctx context.Context, d time.Duration) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	l.mu.Lock()
	l.stats.NextDelay = d
	l.mu.Unlock()
	err := l.wait(ctx, d)
	l.mu.Lock()
	l.stats.NextDelay = 0
	l.mu.Unlock()
	return err
}
func (l *Loop) waitFailure(ctx context.Context) (Result, error) {
	if ctx.Err() != nil {
		return l.finish(Canceled, ErrCanceled)
	}
	return l.finish(DependencyFailed, ErrDependency)
}
func (l *Loop) finish(result Result, err error) (Result, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stats.Running = false
	l.stats.Stopped = true
	l.stats.Result = result
	l.stats.NextDelay = 0
	return result, err
}
func (l *Loop) Stats() Stats { l.mu.Lock(); defer l.mu.Unlock(); return l.stats }
func increment(v uint64) uint64 {
	if v == math.MaxUint64 {
		return v
	}
	return v + 1
}
func nextRetry(previous time.Duration) time.Duration {
	if previous == 0 {
		return time.Second
	}
	if previous >= maxRetryDelay/2 {
		return maxRetryDelay
	}
	return previous * 2
}
func retryJitter(cap time.Duration) time.Duration {
	low := max(time.Second, cap/2)
	return low + time.Duration(rand.Int64N(int64(cap-low)+1))
}
func waitFor(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func knownOutcome(outcome uploadstate.Outcome) bool {
	switch outcome {
	case uploadstate.Idle, uploadstate.SourceUnavailable, uploadstate.Retry, uploadstate.RateLimited, uploadstate.Acknowledged, uploadstate.Expired, uploadstate.ClockSkew, uploadstate.CredentialRejected, uploadstate.Conflict, uploadstate.Rejected, uploadstate.Exhausted:
		return true
	default:
		return false
	}
}
