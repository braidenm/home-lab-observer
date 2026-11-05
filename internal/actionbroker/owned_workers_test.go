package actionbroker

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
)

func TestOwnedWorkersStopFencesBeforeAnyLockAndCancelsAllScopes(t *testing.T) {
	r, err := newOwnedWorkers(context.Background(), &atomic.Bool{})
	if err != nil {
		t.Fatal("registry")
	}
	defer r.cancel()
	greeting, err := r.register()
	if err != nil {
		t.Fatal("greeting")
	}
	defer greeting.close()
	active, err := r.register()
	if err != nil {
		t.Fatal("active")
	}
	defer active.close()
	execution, err := active.nonRelay()
	if err != nil {
		t.Fatal("selected execution")
	}
	r.mu.Lock()
	r.stopAdmission() // No registry lock can postpone the shutdown fence.
	r.mu.Unlock()
	if !r.fenced.Load() || execution.Err() == nil || greeting.context.Err() == nil || active.context.Err() == nil {
		t.Fatal("stop did not fence and cancel every owner")
	}
	if _, err := greeting.nonRelay(); err == nil {
		t.Fatal("greeting became execution after stop")
	}
	if _, err := r.register(); err == nil {
		t.Fatal("new owner admitted after stop")
	}
	all, err := r.all()
	if err != nil || len(all) != 2 || allOwnedDone(all) {
		t.Fatal("cancel was confused with joined completion")
	}
}

func TestOwnedWorkersFixedCapacityRequiresActualCompletion(t *testing.T) {
	r, err := newOwnedWorkers(context.Background(), &atomic.Bool{})
	if err != nil {
		t.Fatal("registry")
	}
	defer r.cancel()
	a, _ := r.register()
	b, _ := r.register()
	c, _ := r.register()
	defer b.close()
	defer c.close()
	if _, err := r.register(); err == nil {
		t.Fatal("registry queued an excess owner")
	}
	if _, err := a.nonRelay(); err != nil {
		t.Fatal("selected scope")
	}
	if _, err := a.nonRelay(); err == nil {
		t.Fatal("scope selected twice")
	}
	all, err := r.all()
	if err != nil || len(all) != 3 || allOwnedDone(all) {
		t.Fatal("unjoined owners were omitted")
	}
	a.close()
	a.close() // Idempotent completion, not a second capacity release.
	replacement, err := r.register()
	if err != nil {
		t.Fatal("completed slot not reclaimed")
	}
	defer replacement.close()
}

func TestOwnedWorkerCloseNeverWaitsForRegistryMutex(t *testing.T) {
	r, err := newOwnedWorkers(context.Background(), &atomic.Bool{})
	if err != nil {
		t.Fatal("registry")
	}
	defer r.cancel()
	h, err := r.register()
	if err != nil {
		t.Fatal("registration")
	}
	r.mu.Lock()
	h.close()
	r.mu.Unlock()
	if !allOwnedDone([]<-chan struct{}{h.done}) {
		t.Fatal("close hid a registry wait")
	}
}

func TestOwnedWorkerClosingFlagDoesNotReleaseUnjoinedSlot(t *testing.T) {
	r, err := newOwnedWorkers(context.Background(), &atomic.Bool{})
	if err != nil {
		t.Fatal("registry")
	}
	defer r.cancel()
	h, _ := r.register()
	b, _ := r.register()
	c, _ := r.register()
	defer b.close()
	defer c.close()
	h.closed.Store(true) // Pause close after CAS, before done: still owned.
	defer close(h.done)
	if _, err := r.register(); err == nil {
		t.Fatal("unjoined closing slot was reused")
	}
	all, err := r.all()
	if err != nil || len(all) != 3 || allOwnedDone(all) {
		t.Fatal("closing flag invented a join")
	}
}

// Timing-only seam: completed synthetic channels grant no execution authority.
type completedJoinClock struct{}

func (completedJoinClock) QuiescenceContext(parent context.Context, _ actionbudget.Stamp) (context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(parent)
	return ctx, cancel, nil
}
func (completedJoinClock) CheckQuiescence(ctx context.Context, _ actionbudget.Stamp) error {
	return ctx.Err()
}

func TestOwnedJoinFirstProfileHasOnlyFourPossibleOwners(t *testing.T) {
	done := make(chan struct{})
	close(done)
	for _, count := range []int{4, 5} {
		signals := make([]<-chan struct{}, count)
		for i := range signals {
			signals[i] = done
		}
		joined, err := waitOwnedJoinClock(context.Background(), completedJoinClock{}, actionbudget.Stamp{}, signals)
		if count == 4 && (err != nil || !joined) {
			t.Fatal("valid completed cohort refused")
		}
		if count == 5 && (err == nil || joined) {
			t.Fatal("first profile admitted a nonexistent fifth owner")
		}
	}
}
