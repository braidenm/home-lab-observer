package uploadloop

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type stepFunc func(context.Context) (uploadstate.Outcome, error)

func (f stepFunc) Step(ctx context.Context) (uploadstate.Outcome, error) { return f(ctx) }

type answer struct {
	outcome uploadstate.Outcome
	err     error
}

func runAnswers(t *testing.T, answers []answer) (*Loop, []time.Duration, Result, error) {
	t.Helper()
	i := 0
	l, err := New(stepFunc(func(context.Context) (uploadstate.Outcome, error) {
		if i >= len(answers) {
			t.Fatal("unexpected extra step")
		}
		a := answers[i]
		i++
		return a.outcome, a.err
	}))
	if err != nil {
		t.Fatal(err)
	}
	var delays []time.Duration
	l.wait = func(_ context.Context, d time.Duration) error { delays = append(delays, d); return nil }
	l.jitter = func(cap time.Duration) time.Duration { return cap }
	result, err := l.Run(context.Background())
	if i != len(answers) {
		t.Fatal("not all expected steps ran")
	}
	return l, delays, result, err
}

func TestPacingCapsRateFloorAndReset(t *testing.T) {
	answers := []answer{}
	for range 9 {
		answers = append(answers, answer{outcome: uploadstate.Retry})
	}
	answers = append(answers, answer{outcome: uploadstate.Acknowledged}, answer{outcome: uploadstate.Retry}, answer{outcome: uploadstate.RateLimited}, answer{outcome: uploadstate.Retry}, answer{outcome: uploadstate.Idle}, answer{outcome: uploadstate.Conflict})
	l, delays, result, err := runAnswers(t, answers)
	seconds := []int{60, 1, 2, 4, 8, 16, 32, 60, 60, 60, 15, 1, 60, 60, 15}
	want := make([]time.Duration, len(seconds))
	for i, v := range seconds {
		want[i] = time.Duration(v) * time.Second
	}
	if !reflect.DeepEqual(delays, want) || result != Conflict || err != nil {
		t.Fatal("pacing or terminal result mismatch")
	}
	s := l.Stats()
	if s.Attempts != 15 || s.Acknowledgements != 1 || s.Retries != 11 || s.RateLimits != 1 || s.Running || !s.Stopped || s.NextDelay != 0 || s.Result != Conflict {
		t.Fatal("fixed stats mismatch")
	}
	s.Attempts = 0
	if l.Stats().Attempts == 0 {
		t.Fatal("stats aliases state")
	}
}

func TestPollFailuresAndTerminals(t *testing.T) {
	for _, outcome := range []uploadstate.Outcome{uploadstate.Idle, uploadstate.SourceUnavailable, uploadstate.Expired, uploadstate.ClockSkew} {
		_, delays, _, err := runAnswers(t, []answer{{outcome: uploadstate.Retry}, {outcome: outcome}, {outcome: uploadstate.Retry}, {outcome: uploadstate.Rejected}})
		if err != nil || !reflect.DeepEqual(delays, []time.Duration{startupDelay, time.Second, pollDelay, time.Second}) {
			t.Fatal("poll/reset mismatch")
		}
	}
	for _, tc := range []struct {
		outcome uploadstate.Outcome
		result  Result
	}{{uploadstate.CredentialRejected, CredentialRejected}, {uploadstate.Conflict, Conflict}, {uploadstate.Rejected, Rejected}, {uploadstate.Exhausted, Exhausted}} {
		_, delays, result, err := runAnswers(t, []answer{{outcome: tc.outcome}})
		if result != tc.result || err != nil || len(delays) != 1 {
			t.Fatal("terminal retried")
		}
	}
	l, delays, result, err := runAnswers(t, []answer{{err: uploadstate.ErrSnapshot}, {outcome: uploadstate.Idle}, {err: uploadstate.ErrRecovery}})
	if err != ErrRecovery || result != RecoveryRequired || !reflect.DeepEqual(delays, []time.Duration{startupDelay, pollDelay, pollDelay}) || l.Stats().InvalidSnapshots != 1 {
		t.Fatal("snapshot/recovery policy mismatch")
	}
	for _, a := range []answer{{err: uploadstate.ErrBusy}, {err: uploadstate.ErrCanceled}, {err: errors.New("private-secret-error")}, {outcome: "private-secret-outcome"}, {outcome: uploadstate.Acknowledged, err: errors.New("private-secret-error")}} {
		l, delays, result, err := runAnswers(t, []answer{a})
		if result != DependencyFailed || err != ErrDependency || len(delays) != 1 || l.Stats().LastOutcome != "" || l.Stats().Acknowledgements != 0 || strings.Contains(err.Error(), "secret") {
			t.Fatal("unexpected dependency value escaped or retried")
		}
	}
}

func TestCancellationWaitsForStepAndCountsDurableAck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int32
	l, _ := New(stepFunc(func(context.Context) (uploadstate.Outcome, error) {
		calls.Add(1)
		close(entered)
		<-release
		return uploadstate.Acknowledged, nil
	}))
	l.wait = func(context.Context, time.Duration) error { return nil }
	var result Result
	var runErr error
	go func() { result, runErr = l.Run(ctx); close(done) }()
	awaitTestSignal(t, entered)
	if _, err := l.Run(context.Background()); err != ErrBusy {
		t.Fatal("concurrent run accepted")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("returned before in-flight step joined")
	default:
	}
	for range 100 {
		if !l.Stats().Running {
			t.Fatal("running state lost before step joined")
		}
	}
	unblock()
	awaitTestSignal(t, done)
	if result != Canceled || runErr != ErrCanceled || calls.Load() != 1 || l.Stats().Acknowledgements != 1 || l.Stats().LastOutcome != uploadstate.Acknowledged {
		t.Fatal("cancellation undid committed ack or started extra step")
	}
	if _, err := l.Run(context.Background()); err != ErrUsed {
		t.Fatal("restarted used loop")
	}
}

func awaitTestSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("test signal deadline exceeded")
	}
}

func TestInvalidSnapshotReplacesPriorSuccessAndRecovers(t *testing.T) {
	answers := []answer{{outcome: uploadstate.Acknowledged}, {err: uploadstate.ErrSnapshot}, {outcome: uploadstate.Idle}, {outcome: uploadstate.Rejected}}
	i := 0
	l, _ := New(stepFunc(func(context.Context) (uploadstate.Outcome, error) {
		a := answers[i]
		i++
		return a.outcome, a.err
	}))
	l.wait = func(context.Context, time.Duration) error {
		s := l.Stats()
		switch i {
		case 1:
			if s.LastOutcome != uploadstate.Acknowledged || s.LastIssue != "" {
				t.Fatal("missing success")
			}
		case 2:
			if s.LastOutcome != "" || s.LastIssue != InvalidSnapshot || s.Acknowledgements != 1 || s.InvalidSnapshots != 1 {
				t.Fatal("stale success during invalid source")
			}
		case 3:
			if s.LastOutcome != uploadstate.Idle || s.LastIssue != "" {
				t.Fatal("stale issue after recovery")
			}
		}
		return nil
	}
	if result, err := l.Run(context.Background()); result != Rejected || err != nil {
		t.Fatal("unexpected terminal result")
	}
}

func TestCanceledWaitNeverStartsAnotherStep(t *testing.T) {
	for _, afterFirst := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		waits := 0
		l, _ := New(stepFunc(func(context.Context) (uploadstate.Outcome, error) { calls++; return uploadstate.RateLimited, nil }))
		l.wait = func(ctx context.Context, d time.Duration) error {
			waits++
			if afterFirst && waits == 1 {
				return nil
			}
			if d != startupDelay {
				t.Fatal("cooldown floor changed")
			}
			cancel()
			return ctx.Err()
		}
		result, err := l.Run(ctx)
		want := 0
		if afterFirst {
			want = 1
		}
		if result != Canceled || err != ErrCanceled || calls != want {
			t.Fatal("canceled wait started a step")
		}
		cancel()
	}
}

func TestCanceledStepIsNotDependencyFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l, _ := New(stepFunc(func(context.Context) (uploadstate.Outcome, error) {
		cancel()
		return "", uploadstate.ErrCanceled
	}))
	l.wait = func(context.Context, time.Duration) error { return nil }
	if result, err := l.Run(ctx); result != Canceled || err != ErrCanceled {
		t.Fatal("canceled step not classified as shutdown")
	}
	if s := l.Stats(); s.LastIssue != "" || s.LastOutcome != "" || s.Acknowledgements != 0 || s.Result != Canceled {
		t.Fatal("normal shutdown reported as dependency failure")
	}
}

func TestConfigCancellationAndWaitFailure(t *testing.T) {
	if _, err := New(nil); err != ErrConfig {
		t.Fatal("nil stepper accepted")
	}
	l, _ := New(stepFunc(func(context.Context) (uploadstate.Outcome, error) { t.Fatal("unexpected step"); return "", nil }))
	if _, err := l.Run(nil); err != ErrConfig {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := l.Run(ctx); result != Canceled || err != ErrCanceled {
		t.Fatal("pre-canceled run not stopped")
	}
	l, _ = New(stepFunc(func(context.Context) (uploadstate.Outcome, error) { t.Fatal("unexpected step"); return "", nil }))
	l.wait = func(context.Context, time.Duration) error { return errors.New("private-error") }
	if result, err := l.Run(context.Background()); result != DependencyFailed || err != ErrDependency {
		t.Fatal("wait failure leaked")
	}
	if err := waitFor(ctx, time.Hour); err != context.Canceled {
		t.Fatal("real timer not canceled")
	}
	if err := waitFor(context.Background(), 0); err != nil {
		t.Fatal("real timer did not complete")
	}
}

func TestJitterAndCounterBounds(t *testing.T) {
	for _, cap := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, maxRetryDelay} {
		for range 100 {
			d := retryJitter(cap)
			if d < max(time.Second, cap/2) || d > cap {
				t.Fatal("jitter outside bound")
			}
		}
	}
	if nextRetry(time.Duration(math.MaxInt64)) != maxRetryDelay || increment(math.MaxUint64) != math.MaxUint64 || increment(0) != 1 {
		t.Fatal("counter overflow")
	}
	for _, bad := range []time.Duration{0, 2 * time.Second} {
		l, _ := New(stepFunc(func(context.Context) (uploadstate.Outcome, error) { return uploadstate.Retry, nil }))
		l.wait = func(context.Context, time.Duration) error { return nil }
		l.jitter = func(time.Duration) time.Duration { return bad }
		if result, err := l.Run(context.Background()); result != DependencyFailed || err != ErrDependency {
			t.Fatal("bad pacing source accepted")
		}
	}
}
