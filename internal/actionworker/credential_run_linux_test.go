//go:build linux

package actionworker

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionipc"
)

func readyWorkerLifecycleFixture(t *testing.T) *credentialLifecycle {
	t.Helper()
	l, _, _ := lifecycleFixture(t, phaseReady)
	if l.maintain(context.Background()) != nil {
		t.Fatal("synthetic initial READY")
	}
	return l
}
func TestWorkerNextQuietPassFollowsActualConversationJoin(t *testing.T) {
	l, r, _ := lifecycleFixture(t, phaseReady)
	// First BEGIN uses an undued credential. Poison only the private store
	// fixture while converse owns the connection; the NEXT quiet pass must
	// refuse it, after that conversation has actually joined.
	if l.maintain(context.Background()) != nil {
		t.Fatal("fixture READY")
	}
	r.calls = nil
	w, _, _ := recoveryWorkerFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	joined := make(chan struct{})
	dials := 0
	err := runWorkerLoop(ctx, w, l, func(ctx context.Context) (*net.UnixConn, error) {
		dials++
		client, broker := conversationPair(t)
		go func() {
			defer broker.Close()
			defer close(joined)
			if begin, err := actionipc.Read(ctx, broker); err != nil || begin.Kind != "BEGIN" {
				return
			}
			l.store.poisoned = true
			// Clean preselection EOF finishes the ACTUAL conversation. No
			// renewal may overlap its still-owned connection.
		}()
		return client, nil
	}, func(context.Context) error {
		<-joined
		if len(r.calls) != 0 {
			t.Fatal("renewal ran while prior conversation owned work")
		}
		w.disableExecution()
		return nil
	})
	if err != ErrCredentialRecovery || dials != 1 || !w.executionDisabled() {
		t.Fatal("quiet consumer/latch integration")
	}
}

func TestWorkerDueRenewalCompletesBeforeBEGINWithoutClearingLatch(t *testing.T) {
	l, r, _ := lifecycleFixture(t, phaseReady)
	w, _, _ := recoveryWorkerFixture(t)
	w.disableExecution()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	dials := 0
	err := runWorkerLoop(ctx, w, l, func(ctx context.Context) (*net.UnixConn, error) {
		dials++
		if len(r.calls) != 4 || r.calls[3] != "activate" || !w.executionDisabled() {
			t.Fatal("BEGIN before completed renewal or cleared latch")
		}
		if _, err := l.Credential(ctx, l.binding); err != nil {
			t.Fatal("new durable credential unavailable")
		}
		client, broker := conversationPair(t)
		go func() { defer broker.Close(); actionipc.Read(ctx, broker) }()
		return client, nil
	}, func(context.Context) error { cancel(); return ErrUnsafe })
	if err != nil || dials != 1 || !w.executionDisabled() {
		t.Fatal("due quiet renewal integration")
	}
}
func TestWorkerPendingHandoffPreventsAnyDialOrClaim(t *testing.T) {
	l, _, _ := lifecycleFixture(t, phasePrepareAttempted)
	w, _, _ := recoveryWorkerFixture(t)
	dials := 0
	if runWorkerLoop(context.Background(), w, l, func(context.Context) (*net.UnixConn, error) { dials++; return nil, ErrUnsafe }, workerPace) == nil || dials != 0 {
		t.Fatal("pending handoff reached BEGIN")
	}
}
