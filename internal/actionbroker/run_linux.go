//go:build linux

package actionbroker

import (
	"context"
	"errors"
	"net"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"golang.org/x/sys/unix"
)

// RunBroker has no path, identity, Engine or permission overrides. It opens only
// the reviewed installed profile and an EXISTING journal; it never provisions,
// repairs or adopts state. Only worker snapshot/execution branches are served;
// uncertain outcomes stay locked, with no root recovery or reset ingress.
func RunBroker(ctx context.Context) error {
	if ctx == nil {
		return ErrRecoveryRequired
	}
	if ctx.Err() != nil {
		return nil
	}
	r, err := openBrokerRuntime(ctx)
	if err != nil {
		return ErrRecoveryRequired
	}
	runErr := r.run(ctx)
	closeErr := r.close()
	if runErr != nil || closeErr != nil {
		return ErrRecoveryRequired
	}
	return nil
}

type brokerRuntime struct {
	consumer                *executionConsumer
	worker                  *brokerEndpoint
	verifyWorker            func(context.Context, *net.UnixConn) error
	started, joined, closed bool // One owning run/cleanup goroutine, not handler state.
	closeErr                error
	workers                 *ownedWorkers
	stopBudget              ownedJoinClock // Production fixes the concrete native clock.
	acceptDone              <-chan struct{}
}

func (p *listenerParent) absent(ctx context.Context) error {
	if p.current(ctx) != nil {
		return ErrRecoveryRequired
	}
	var st unix.Stat_t
	if err := unix.Fstatat(p.fds[3], p.socket, &st, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
		return ErrRecoveryRequired
	}
	return nil
}

func openBrokerRuntime(ctx context.Context) (_ *brokerRuntime, result error) {
	c, err := openExecutionConsumer(ctx)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	r := &brokerRuntime{consumer: c, stopBudget: c.state.clock}
	defer func() {
		if result != nil {
			r.close()
		}
	}()
	files, ok := c.files.(*brokerFiles)
	if !ok || files == nil {
		return nil, ErrRecoveryRequired
	}
	i := files.identity
	workerParent, err := openListenerParent(ctx, workerListener, i)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	workerTransferred := false
	defer func() {
		if !workerTransferred {
			workerParent.close()
		}
	}()
	if workerParent.absent(ctx) != nil || c.currentSource(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	r.worker, err = workerParent.bind(ctx)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	workerTransferred = true
	if c.currentSource(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	r.verifyWorker = func(ctx context.Context, c *net.UnixConn) error {
		if ctx.Err() != nil {
			return ErrRecoveryRequired
		}
		raw, err := c.SyscallConn()
		if err != nil || actionprofile.VerifyWorkerPeer(raw, i) != nil || ctx.Err() != nil {
			return ErrRecoveryRequired
		}
		return nil
	}
	return r, nil
}

func (r *brokerRuntime) run(parent context.Context) error {
	if r == nil || parent == nil || r.consumer == nil || r.consumer.state == nil || r.consumer.engine == nil || r.consumer.files == nil || r.worker == nil || r.worker.listener == nil || r.verifyWorker == nil || r.stopBudget == nil || r.started || r.closed {
		return ErrRecoveryRequired
	}
	if parent.Err() != nil {
		return nil
	}
	if r.worker.current(parent) != nil {
		return ErrRecoveryRequired
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	workers, err := newOwnedWorkers(ctx, &r.consumer.state.executionClosed)
	if err != nil {
		if parent.Err() != nil {
			return nil
		}
		return ErrRecoveryRequired
	}
	r.workers = workers
	r.started = true
	workerSlots := make(chan struct{}, 3)
	failures := make(chan error, 1)
	acceptDone := make(chan struct{})
	r.acceptDone = acceptDone
	go func() {
		defer close(acceptDone)
		r.accept(ctx, r.worker, workerSlots, r.verifyWorker, failures)
	}()
	var result error
	select {
	case <-parent.Done():
	case <-failures:
		result = ErrRecoveryRequired
	}
	// ONE original cleanup stamp. Fence/cancel never waits for state.mu, and
	// no per-handler timer or WaitGroup waiter can outlive this owned controller.
	original, captureErr := r.consumer.state.clock.Capture()
	r.workers.stopAdmission()
	cancel()
	if r.consumer.state.mu.TryLock() {
		r.consumer.state.quiescing = true
		r.consumer.state.readiness = nil
		r.consumer.state.mu.Unlock()
	}
	r.worker.listener.Close()
	signals := []<-chan struct{}{acceptDone}
	acceptsJoined, joinErr := waitOwnedJoinClock(context.Background(), r.stopBudget, original, signals)
	if !acceptsJoined {
		return ErrRecoveryRequired
	}
	owned, snapshotErr := r.workers.all()
	if snapshotErr != nil {
		return ErrRecoveryRequired
	}
	signals = append(signals, owned...)
	allJoined, finishErr := waitOwnedJoinClock(context.Background(), r.stopBudget, original, signals)
	r.joined = allJoined
	if captureErr != nil || joinErr != nil || finishErr != nil {
		return ErrRecoveryRequired
	}
	return result
}

func (r *brokerRuntime) accept(ctx context.Context, e *brokerEndpoint, slots chan struct{}, verify func(context.Context, *net.UnixConn) error, failures chan<- error) {
	for ctx.Err() == nil {
		c, err := e.listener.AcceptUnix()
		if err != nil {
			if ctx.Err() == nil {
				failures <- ErrRecoveryRequired
			}
			return
		}
		if ctx.Err() != nil {
			c.Close()
			return
		}
		if e.current(ctx) != nil {
			c.Close()
			if ctx.Err() == nil {
				failures <- ErrRecoveryRequired
			}
			return
		}
		select {
		case slots <- struct{}{}:
		default:
			c.Close()
			continue
		} // No application queue.
		owned, err := r.workers.register()
		if err != nil {
			c.Close()
			<-slots
			continue
		}
		handlerCtx := owned.context
		go func() {
			defer func() { <-slots }()
			defer owned.close() // Last owned action, after socket cancellation has joined.
			defer c.Close()
			joined := make(chan struct{})
			stop := context.AfterFunc(handlerCtx, func() { c.Close(); close(joined) })
			defer func() {
				if !stop() {
					<-joined
				}
			}()
			// Kernel/profile checks precede even the first frame. A greeting,
			// pathname, PID or synthetic test seam is not production admission.
			if verify(handlerCtx, c) != nil || e.current(handlerCtx) != nil || handlerCtx.Err() != nil {
				return
			}
			r.consumer.serveOwned(owned, c)
		}()
	}
}

func (r *brokerRuntime) close() error {
	if r == nil {
		return nil
	}
	if r.closed {
		return r.closeErr
	}
	if r.started && !r.joined {
		return ErrRecoveryRequired
	} // Never close under a worker.
	r.closed = true
	failed := false
	for _, e := range []*brokerEndpoint{r.worker} {
		if e != nil {
			if e.close() != nil {
				failed = true
			}
			if e.parent.close() != nil {
				failed = true
			}
		}
	}
	if r.consumer != nil && r.consumer.close() != nil {
		failed = true
	}
	if failed {
		r.closeErr = ErrRecoveryRequired
	}
	return r.closeErr
}
