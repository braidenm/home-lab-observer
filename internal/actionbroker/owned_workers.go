package actionbroker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var errOwnedWorkers = errors.New("action_owned_workers_unavailable")

type workerPhase uint8

const (
	greetingPhase workerPhase = iota + 1
	nonRelayPhase
)

// Bookkeeping and cancellation only, NOT peer admission, remote permission,
// remote authority or proof of joined shutdown. Callers validate those.
type ownedWorkers struct {
	context, execution      context.Context
	cancel, cancelExecution context.CancelFunc
	fenced                  *atomic.Bool
	closing                 atomic.Bool
	mu                      sync.Mutex // No I/O or waits while held; every access is TryLock.
	handlers                map[*ownedWorker]workerPhase
}

type ownedWorker struct {
	registry                *ownedWorkers
	context, execution      context.Context
	cancel, cancelExecution context.CancelFunc // Immutable after registration.
	done                    chan struct{}
	closed                  atomic.Bool
}

func newOwnedWorkers(parent context.Context, fence *atomic.Bool) (*ownedWorkers, error) {
	if parent == nil || parent.Err() != nil || fence == nil {
		return nil, errOwnedWorkers
	}
	run, cancel := context.WithCancel(parent)
	exec, stopExec := context.WithCancel(run)
	return &ownedWorkers{context: run, execution: exec, cancel: cancel, cancelExecution: stopExec, fenced: fence, handlers: make(map[*ownedWorker]workerPhase)}, nil
}

func (r *ownedWorkers) register() (*ownedWorker, error) {
	if r == nil || r.closing.Load() || r.context.Err() != nil || !r.mu.TryLock() {
		return nil, errOwnedWorkers
	}
	defer r.mu.Unlock()
	r.pruneDone()
	if r.closing.Load() || r.context.Err() != nil || len(r.handlers) >= 3 {
		return nil, errOwnedWorkers
	}
	ctx, cancel := context.WithCancel(r.context)
	exec, stopExec := context.WithCancel(r.execution)
	h := &ownedWorker{registry: r, context: ctx, execution: exec, cancel: cancel, cancelExecution: stopExec, done: make(chan struct{})}
	r.handlers[h] = greetingPhase
	return h, nil
}

// Read stateless BEGIN before selecting the execution/control scope. Shutdown
// fences and cancels both greeting and selected scopes; no recovery branch exists.
func (h *ownedWorker) nonRelay() (context.Context, error) {
	if h == nil || h.registry == nil {
		return nil, errOwnedWorkers
	}
	r := h.registry
	if r.fenced.Load() || r.closing.Load() || !r.mu.TryLock() {
		return nil, errOwnedWorkers
	}
	defer r.mu.Unlock()
	if h.closed.Load() || r.handlers[h] != greetingPhase || r.fenced.Load() || h.execution.Err() != nil {
		return nil, errOwnedWorkers
	}
	r.handlers[h] = nonRelayPhase
	return h.execution, nil
}

func (h *ownedWorker) close() {
	if h == nil || !h.closed.CompareAndSwap(false, true) {
		return
	}
	h.cancelExecution()
	h.cancel()
	// A closing flag alone proves nothing. Keep the entry until completion is
	// actually signaled; bounded registry accesses reclaim only closed channels.
	close(h.done) // Last owned action: no journal/Engine/resource use follows.
}

// Called only while the registry lock is held. No cancellation, I/O or waiting.
func (r *ownedWorkers) pruneDone() {
	for h := range r.handlers {
		select {
		case <-h.done:
			delete(r.handlers, h)
		default:
		}
	}
}

func (r *ownedWorkers) stopAdmission() {
	if r != nil {
		r.closing.Store(true)
		r.fenced.Store(true)
		r.cancelExecution()
		r.cancel()
	}
}

func (r *ownedWorkers) all() ([]<-chan struct{}, error) {
	if r == nil || !r.mu.TryLock() {
		return nil, errOwnedWorkers
	}
	defer r.mu.Unlock()
	r.pruneDone()
	done := make([]<-chan struct{}, 0, 3)
	for h := range r.handlers {
		done = append(done, h.done)
	}
	return done, nil
}
