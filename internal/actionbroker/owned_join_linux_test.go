//go:build linux

package actionbroker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
)

func TestOwnedJoinTimesOutWithoutInventingCompletionAndUsesSameStamp(t *testing.T) {
	clock, err := actionbudget.NewClock()
	if err != nil {
		t.Fatal("native clock")
	}
	start, err := clock.Capture()
	if err != nil {
		t.Fatal("original stop stamp")
	}
	blocked := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if joined, err := waitOwnedJoin(ctx, clock, start, []<-chan struct{}{blocked}); err == nil || joined {
		t.Fatal("unjoined handler became quiescence")
	}
	close(blocked)
	if joined, err := waitOwnedJoin(context.Background(), clock, start, []<-chan struct{}{blocked}); err != nil || !joined {
		t.Fatal("actual completion unavailable for cleanup")
	}
}

type expiringJoinClock struct {
	original actionbudget.Stamp
	checked  chan struct{}
	once     sync.Once
	expired  atomic.Bool
}

func (f *expiringJoinClock) QuiescenceContext(parent context.Context, original actionbudget.Stamp) (context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(parent, time.Hour)
	return ctx, cancel, nil // The Go timer deliberately remains unexpired.
}
func (f *expiringJoinClock) CheckQuiescence(parent context.Context, original actionbudget.Stamp) error {
	expired := f.expired.Load() // Snapshot completes BEFORE signaling the test.
	f.once.Do(func() { close(f.checked) })
	if parent.Err() != nil || original != f.original || expired {
		return actionbudget.ErrUnavailable
	}
	return nil
}

func TestOwnedJoinRechecksNativeExpiryWhileCompletionAndGoTimerRemainOpen(t *testing.T) {
	clock, err := actionbudget.NewClock()
	if err != nil {
		t.Fatal("native original clock")
	}
	start, _ := clock.Capture()
	fake := &expiringJoinClock{original: start, checked: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	blocked := make(chan struct{})
	stopped := make(chan struct{})
	var joined bool
	var result error
	go func() {
		joined, result = waitOwnedJoinClock(ctx, fake, start, []<-chan struct{}{blocked})
		close(stopped)
	}()
	defer func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Error("owned test invocation not joined")
		}
	}()
	<-fake.checked
	fake.expired.Store(true)
	select {
	case <-stopped:
		if joined || result == nil {
			t.Fatal("expired native budget accepted completion")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("native expiry still blocked on Go timer/completion")
	}
}

func TestOwnedJoinRejectsUnknownSignalsAndReportsCompletedCancellationHonestly(t *testing.T) {
	clock, err := actionbudget.NewClock()
	if err != nil {
		t.Fatal("native clock")
	}
	start, _ := clock.Capture()
	if joined, err := waitOwnedJoin(context.Background(), clock, start, []<-chan struct{}{nil}); err == nil || joined {
		t.Fatal("missing completion evidence accepted")
	}
	done := make(chan struct{})
	close(done)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if joined, err := waitOwnedJoin(ctx, clock, start, []<-chan struct{}{done}); err == nil || !joined {
		t.Fatal("completed cleanup confused with uncanceled authority")
	}
}
