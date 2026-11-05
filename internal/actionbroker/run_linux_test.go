//go:build linux

package actionbroker

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionengine"
	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"golang.org/x/sys/unix"
)

func TestRunBrokerContextRefusalDoesNotOpenInstalledPaths(t *testing.T) {
	if RunBroker(nil) == nil {
		t.Fatal("nil runtime context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if RunBroker(ctx) != nil {
		t.Fatal("canceled runtime should not start")
	}
}

// Only private composition: actual Unix listeners/peer credentials/SQLite and
// empty claim framing, synthetic ownership/source and no Docker or real roles.
func runtimeFixture(t *testing.T) *brokerRuntime {
	t.Helper()
	_, _, _, consumer := executionConsumerFixture(t, actioncore.Restart)
	consumer.engine = &actionengine.GuardedEngine{} // No effects in an empty poll.
	w, _ := listenerFixture(t, workerListener)
	worker, err := w.bind(context.Background())
	if err != nil {
		t.Fatal("private worker listener")
	}
	verify := func(ctx context.Context, c *net.UnixConn) error {
		if ctx.Err() != nil {
			return ErrRecoveryRequired
		}
		raw, err := c.SyscallConn()
		if err != nil {
			return ErrRecoveryRequired
		}
		allowed := false
		if raw.Control(func(fd uintptr) {
			cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
			allowed = err == nil && cred.Uid == uint32(os.Geteuid()) && cred.Gid == uint32(os.Getegid())
		}) != nil || !allowed {
			return ErrRecoveryRequired
		}
		return nil
	}
	r := &brokerRuntime{consumer: consumer, worker: worker, verifyWorker: verify}
	r.stopBudget = consumer.state.clock
	t.Cleanup(func() { r.close() })
	return r
}

func TestBrokerRuntimeActualUnjoinedShutdownKeepsStorageOpen(t *testing.T) {
	r := runtimeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	r.verifyWorker = func(context.Context, *net.UnixConn) error { close(entered); <-release; return ErrRecoveryRequired }
	fake := &runtimeExpiryClock{checked: make(chan struct{})}
	r.stopBudget = fake // Private timing fault only; completion channels stay real.
	wait := launchBrokerRuntime(t, r, ctx, cancel)
	t.Cleanup(releaseHandler) // Release before launch's cancellation/join cleanup.
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Net: "unix", Name: r.worker.parent.path})
	if err != nil {
		t.Fatal("private dial")
	}
	defer c.Close()
	<-entered
	cancel()
	<-fake.checked
	for _, done := range []<-chan struct{}{r.acceptDone} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("owned acceptor did not stop")
		}
	}
	fake.expired.Store(true)
	if wait() == nil {
		t.Fatal("unjoined shutdown reported success")
	}
	if r.joined || r.close() == nil {
		t.Fatal("unjoined shutdown closed owned resources")
	}
	if _, err := r.consumer.state.journal.AdmissionSnapshot(context.Background()); err != nil {
		t.Fatal("actual unjoined journal was closed")
	}
	releaseHandler()
	signals, err := r.workers.all()
	if err != nil {
		t.Fatal("owned completion inventory")
	}
	for _, done := range signals {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("fixture handler not joined")
		}
	}
	// The actual acceptor and every actual handler are now complete. This is
	// explicit fixture cleanup, not a timed-out production continuation or resume.
	r.joined = true
	if r.close() != nil {
		t.Fatal("joined fixture cleanup")
	}
}

func TestBrokerShutdownDoesNotFreeInterruptedOwnersSnapshotSlot(t *testing.T) {
	r := runtimeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan *controlConversation, 1), make(chan struct{})
	var once sync.Once
	releaseHandler := func() { once.Do(func() { close(release) }) }
	r.verifyWorker = func(ctx context.Context, _ *net.UnixConn) error {
		control, err := r.consumer.state.beginControl(ctx)
		if err != nil {
			entered <- nil
			return ErrRecoveryRequired
		}
		entered <- control
		<-release                               // Deliberately uncooperative owned actor, no real Engine effects.
		r.consumer.state.cancelControl(control) // ONLY this exact owner's cleanup.
		return ErrRecoveryRequired
	}
	fake := &runtimeExpiryClock{checked: make(chan struct{})}
	r.stopBudget = fake
	wait := launchBrokerRuntime(t, r, ctx, cancel)
	t.Cleanup(releaseHandler)
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Net: "unix", Name: r.worker.parent.path})
	if err != nil {
		t.Fatal("owned control dial")
	}
	defer c.Close()
	control := <-entered
	if control == nil {
		t.Fatal("actual snapshot reservation")
	}
	cancel()
	<-fake.checked
	fake.expired.Store(true)
	if wait() == nil {
		t.Fatal("unjoined control owner became success")
	}
	r.consumer.state.mu.Lock()
	stillOwned := r.consumer.state.control == control
	r.consumer.state.mu.Unlock()
	if !stillOwned || r.close() == nil {
		t.Fatal("shutdown freed control capacity before actual owner completion")
	}
	releaseHandler()
	signals, err := r.workers.all()
	if err != nil {
		t.Fatal("actual owner inventory")
	}
	for _, done := range signals {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("owner cleanup not joined")
		}
	}
	for _, done := range []<-chan struct{}{r.acceptDone} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("acceptor cleanup not joined")
		}
	}
	if r.consumer.state.control != nil {
		t.Fatal("exact joined owner did not clear its slot")
	}
	r.joined = true // Fixture-only cleanup AFTER actual accept/handler completion.
	if r.close() != nil {
		t.Fatal("joined fixture cleanup")
	}
}

type runtimeExpiryClock struct {
	original actionbudget.Stamp
	checked  chan struct{}
	once     sync.Once
	expired  atomic.Bool
}

func (f *runtimeExpiryClock) QuiescenceContext(parent context.Context, original actionbudget.Stamp) (context.Context, context.CancelFunc, error) {
	if f.original == (actionbudget.Stamp{}) {
		f.original = original
	}
	if f.original != original {
		return nil, nil, actionbudget.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(parent, time.Hour)
	return ctx, cancel, nil
}
func (f *runtimeExpiryClock) CheckQuiescence(parent context.Context, original actionbudget.Stamp) error {
	expired := f.expired.Load()
	f.once.Do(func() { close(f.checked) })
	if parent.Err() != nil || original != f.original || expired {
		return actionbudget.ErrUnavailable
	}
	return nil
}

func launchBrokerRuntime(t *testing.T, r *brokerRuntime, ctx context.Context, cancel context.CancelFunc) func() error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- r.run(ctx) }()
	var once sync.Once
	var result error
	wait := func() error { once.Do(func() { result = <-done }); return result }
	t.Cleanup(func() { cancel(); wait() }) // Join before fixture resources are closed.
	return wait
}

func TestBrokerRuntimeConsumesAuthenticatedEmptyPollAndJoinsShutdown(t *testing.T) {
	r := runtimeFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var verified atomic.Int32
	verify := r.verifyWorker
	r.verifyWorker = func(ctx context.Context, c *net.UnixConn) error {
		err := verify(ctx, c)
		if err == nil {
			verified.Add(1)
		}
		return err
	}
	wait := launchBrokerRuntime(t, r, ctx, cancel)
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Net: "unix", Name: r.worker.parent.path})
	if err != nil {
		t.Fatal("private worker dial")
	}
	defer c.Close()
	sendBeginning(t, c)
	approval, err := actionipc.Read(ctx, c)
	if err != nil || approval.Kind != "APPROVAL_SNAPSHOT" || verified.Load() != 1 {
		t.Fatal("bytes preceded actual fixture peer check")
	}
	if writeLocal(ctx, c, "NO_CLAIM", approval.ConversationNonce, "2", struct{}{}) != nil {
		t.Fatal("empty poll")
	}
	if c.SetReadDeadline(time.Now().Add(time.Second)) != nil {
		t.Fatal("bounded close wait")
	}
	var one [1]byte
	if _, err := c.Read(one[:]); !errors.Is(err, io.EOF) {
		t.Fatal("empty conversation not closed")
	}
	cancel()
	if wait() != nil || r.close() != nil {
		t.Fatal("shutdown failed to join/close")
	}
	for _, path := range []string{r.worker.parent.path} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("owned endpoint retained")
		}
	}
}

func TestBrokerRuntimeHasNoConnectionQueueAndCancelsAcceptedHandlers(t *testing.T) {
	for _, kind := range []listenerKind{workerListener} {
		t.Run([]string{"worker3"}[int(kind)-1], func(t *testing.T) {
			r := runtimeFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			entered := make(chan struct{}, 3)
			verify := func(ctx context.Context, c *net.UnixConn) error {
				entered <- struct{}{}
				<-ctx.Done()
				return ErrRecoveryRequired
			}
			endpoint, capacity := r.worker, 3
			r.verifyWorker = verify
			wait := launchBrokerRuntime(t, r, ctx, cancel)
			for n := 0; n < capacity; n++ {
				c, err := net.DialUnix("unix", nil, &net.UnixAddr{Net: "unix", Name: endpoint.parent.path})
				if err != nil {
					t.Fatal("owned connection")
				}
				defer c.Close()
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("handler admission")
				}
			}
			fourth, err := net.DialUnix("unix", nil, &net.UnixAddr{Net: "unix", Name: endpoint.parent.path})
			if err != nil {
				t.Fatal("owned excess connection")
			}
			defer fourth.Close()
			if fourth.SetReadDeadline(time.Now().Add(time.Second)) != nil {
				t.Fatal("bounded refusal")
			}
			var one [1]byte
			if _, err := fourth.Read(one[:]); !errors.Is(err, io.EOF) {
				t.Fatal("excess connection queued instead of refused")
			}
			cancel()
			if wait() != nil || r.close() != nil {
				t.Fatal("accepted handlers not canceled/joined")
			}
		})
	}
}

func TestBrokerRuntimeShutdownCohortHasAtMostFourActualCompletionSignals(t *testing.T) {
	r := runtimeFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	entered := make(chan struct{}, 3)
	verify := func(ctx context.Context, _ *net.UnixConn) error {
		entered <- struct{}{}
		<-ctx.Done()
		return ErrRecoveryRequired
	}
	r.verifyWorker = verify
	wait := launchBrokerRuntime(t, r, ctx, cancel)
	for _, endpoint := range []*brokerEndpoint{r.worker, r.worker, r.worker} {
		c, err := net.DialUnix("unix", nil, &net.UnixAddr{Net: "unix", Name: endpoint.parent.path})
		if err != nil {
			t.Fatal("private owned cohort dial")
		}
		defer c.Close()
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("owned cohort admission")
		}
	}
	workers, err := r.workers.all()
	if err != nil || len(workers) != 3 {
		t.Fatal("actual handler cohort differs from fixed capacity")
	}
	signals := append([]<-chan struct{}{r.acceptDone}, workers...)
	if len(signals) != 4 || allOwnedDone(signals) {
		t.Fatal("completion bound or live-handler evidence incorrect")
	}
	cancel()
	if wait() != nil || !r.joined || !allOwnedDone(signals) || r.close() != nil {
		t.Fatal("actual four-signal cohort not completely joined before close")
	}
}

func TestBrokerRuntimeRequiresPeerCheckAndPreservesForeignCleanup(t *testing.T) {
	r := runtimeFixture(t)
	verify := r.verifyWorker
	r.verifyWorker = nil
	if r.run(context.Background()) == nil {
		t.Fatal("missing peer check accepted")
	}
	r.verifyWorker = verify
	path := r.worker.parent.path
	if os.Rename(path, path+".retained") != nil || os.WriteFile(path, []byte("foreign"), 0600) != nil {
		t.Fatal("private replacement")
	}
	if r.close() == nil {
		t.Fatal("foreign cleanup claimed success")
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "foreign" {
		t.Fatal("runtime removed foreign node")
	}
}
