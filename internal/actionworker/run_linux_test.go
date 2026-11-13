//go:build linux

package actionworker

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"github.com/braidenm/home-lab-observer/internal/actionremote"
)

type interruptedSnapshotFixture struct {
	*recoveryFixture
	entered, release chan struct{}
	publications     int
}

func (f *interruptedSnapshotFixture) RefreshSnapshot(ctx context.Context, _ actionprotocol.SnapshotQuery) (actionprotocol.SnapshotReply, error) {
	f.publications++
	close(f.entered)
	select {
	case <-f.release:
		return actionprotocol.SnapshotReply{Outcome: "DENIED"}, actionremote.ErrDenied
	case <-ctx.Done():
		return actionprotocol.SnapshotReply{}, actionremote.ErrUnavailable
	}
}

func TestInterruptedSelectedSnapshotKeepsOnlyPacedRecoveryAvailable(t *testing.T) {
	w, recovery, _ := recoveryWorkerFixture(t)
	f := &interruptedSnapshotFixture{recoveryFixture: recovery, entered: make(chan struct{}), release: make(chan struct{})}
	w.remote = f
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	dials, pauses := 0, 0
	brokers := make(chan error, 2)
	err := runWorkerLoop(ctx, w, readyWorkerLifecycleFixture(t), func(ctx context.Context) (*net.UnixConn, error) {
		dials++
		if dials > 2 {
			return nil, ErrUnsafe
		}
		client, broker := conversationPair(t)
		go func(index int) {
			defer broker.Close()
			if begin, err := actionipc.Read(ctx, broker); err != nil || begin.Kind != "BEGIN" {
				brokers <- ErrUnsafe
				return
			}
			if index == 1 {
				q := actionprotocol.SnapshotQuery{ServerID: w.binding.ServerID, OwnerUserID: w.binding.OwnerUserID,
					ConnectorInstanceID: w.binding.ConnectorInstanceID, PublicationID: strings.Repeat("3", 32), Entries: []actionprotocol.Entry{}}
				if sendLocal(ctx, broker, "SNAPSHOT_QUERY", strings.Repeat("3", 32), "1", q) != nil {
					brokers <- ErrUnsafe
					return
				}
				select {
				case <-f.entered:
				case <-ctx.Done():
					brokers <- ErrUnsafe
					return
				}
				// Deterministically interrupt the selected exchange BEFORE its reply.
				// The remote call has no claim or Engine side effect.
				broker.Close()
				close(f.release)
				brokers <- nil
				return
			}
			q := queryFor(t, recovery, "STATUS", nil)
			if sendLocal(ctx, broker, "RECOVERY_QUERY", strings.Repeat("4", 32), "1", q) != nil {
				brokers <- ErrUnsafe
				return
			}
			if reply, err := actionipc.Read(ctx, broker); err != nil || reply.Kind != "RECOVERY_REPLY" {
				brokers <- ErrUnsafe
				return
			}
			brokers <- nil
		}(dials)
		return client, nil
	}, func(context.Context) error {
		pauses++
		if <-brokers != nil {
			return ErrUnsafe
		}
		if pauses == 2 {
			cancel()
			return ErrUnsafe
		}
		return nil
	})
	if err != nil || dials != 2 || pauses != 2 || !w.executionDisabled() || f.publications != 1 || f.claims != 0 || strings.Join(f.operations, ",") != "STATUS" {
		t.Fatal("interrupted snapshot stranded recovery or resumed publication/claim")
	}
}

func TestWorkerLoopLatchAndPace(t *testing.T) {
	w, f, _ := recoveryWorkerFixture(t)
	f.claimErr = actionremote.ErrClaimUnconfirmed
	claim, approval := workerFixture(t)
	w.binding = actionremote.Binding{ServerID: claim.Request.ServerID, OwnerUserID: claim.Request.OwnerUserID, ConnectorInstanceID: claim.Request.ConnectorInstanceID}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	dials, pauses := 0, 0
	brokers := make(chan error, 2)
	err := runWorkerLoop(ctx, w, readyWorkerLifecycleFixture(t), func(ctx context.Context) (*net.UnixConn, error) {
		dials++
		client, broker := conversationPair(t)
		go func(index int) {
			if _, err := actionipc.Read(ctx, broker); err != nil {
				brokers <- ErrUnsafe
				return
			}
			if index == 1 {
				if sendLocal(ctx, broker, "APPROVAL_SNAPSHOT", strings.Repeat("1", 32), "1", approval) != nil {
					brokers <- ErrUnsafe
					return
				}
				if _, err := actionipc.Read(ctx, broker); err == nil {
					brokers <- ErrUnsafe
					return
				}
				brokers <- nil
				return
			}
			q := queryFor(t, f, "STATUS", nil)
			if sendLocal(ctx, broker, "RECOVERY_QUERY", strings.Repeat("2", 32), "1", q) != nil {
				brokers <- ErrUnsafe
				return
			}
			if reply, err := actionipc.Read(ctx, broker); err != nil || reply.Kind != "RECOVERY_REPLY" {
				brokers <- ErrUnsafe
				return
			}
			brokers <- nil
		}(dials)
		return client, nil
	}, func(context.Context) error {
		pauses++
		// Wait for the fixture to observe the completed conversation before
		// canceling its read context; scheduling must not make that a false
		// malformed-reply failure after a successful synchronous write.
		if <-brokers != nil {
			return ErrUnsafe
		}
		if pauses == 2 {
			cancel()
			return ErrUnsafe
		}
		return nil
	})
	if err != nil || dials != 2 || pauses != 2 || !w.executionDisabled() || f.claims != 1 || strings.Join(f.operations, ",") != "STATUS" {
		t.Fatal("reconnect lost latch or pacing")
	}
}

func TestWorkerLoopRetriesOnlyAbsentBroker(t *testing.T) {
	for _, unsafe := range []bool{false, true} {
		w, _, _ := recoveryWorkerFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		dials, pauses := 0, 0
		err := runWorkerLoop(ctx, w, readyWorkerLifecycleFixture(t), func(context.Context) (*net.UnixConn, error) {
			dials++
			if unsafe {
				return nil, ErrUnsafe
			}
			return nil, errBrokerAbsent
		}, func(context.Context) error { pauses++; cancel(); return ErrUnsafe })
		cancel()
		if dials != 1 || unsafe && (err == nil || pauses != 0) || !unsafe && (err != nil || pauses != 1) {
			t.Fatal("unsafe profile retried or missing broker busy-looped")
		}
	}
}

func TestWorkerLoopCancelJoinsConversation(t *testing.T) {
	w, _, _ := recoveryWorkerFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, broker := conversationPair(t)
	done := make(chan error, 1)
	lifecycle := readyWorkerLifecycleFixture(t)
	go func() {
		done <- runWorkerLoop(ctx, w, lifecycle, func(context.Context) (*net.UnixConn, error) { return client, nil }, workerPace)
	}()
	if _, err := actionipc.Read(ctx, broker); err != nil {
		t.Fatal("greeting")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("orderly cancel failed")
		}
	case <-time.After(time.Second):
		t.Fatal("active conversation not joined")
	}
	if workerPace(ctx) == nil {
		t.Fatal("canceled pace slept")
	}
}

func TestWorkerUnselectedEOFDoesNotStrandLaterRecovery(t *testing.T) {
	w, f, _ := recoveryWorkerFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	dials, pauses := 0, 0
	brokers := make(chan error, 2)
	err := runWorkerLoop(ctx, w, readyWorkerLifecycleFixture(t), func(ctx context.Context) (*net.UnixConn, error) {
		dials++
		client, broker := conversationPair(t)
		go func(index int) {
			if m, err := actionipc.Read(ctx, broker); err != nil || m.Kind != "BEGIN" {
				brokers <- ErrUnsafe
				return
			}
			if index == 1 {
				// Authenticated broker consumed only the stateless greeting. No
				// pending root query or execution selection was issued.
				broker.Close()
				brokers <- nil
				return
			}
			q := queryFor(t, f, "STATUS", nil)
			if sendLocal(ctx, broker, "RECOVERY_QUERY", strings.Repeat("2", 32), "1", q) != nil {
				brokers <- ErrUnsafe
				return
			}
			if reply, err := actionipc.Read(ctx, broker); err != nil || reply.Kind != "RECOVERY_REPLY" {
				brokers <- ErrUnsafe
				return
			}
			brokers <- nil
		}(dials)
		return client, nil
	}, func(context.Context) error {
		pauses++
		if <-brokers != nil {
			return ErrUnsafe
		}
		if pauses == 2 {
			cancel()
			return ErrUnsafe
		}
		return nil
	})
	if err != nil || dials != 2 || pauses != 2 || w.executionDisabled() || f.claims != 0 || strings.Join(f.operations, ",") != "STATUS" {
		t.Fatal("unselected greeting stranded recovery or altered execution")
	}
}

func TestWorkerPartialSelectionRemainsTerminalWithoutClaim(t *testing.T) {
	for _, latched := range []bool{false, true} {
		t.Run(map[bool]string{false: "execution-available", true: "recovery-only"}[latched], func(t *testing.T) {
			testWorkerPartialSelectionRemainsTerminalWithoutClaim(t, latched)
		})
	}
}

func testWorkerPartialSelectionRemainsTerminalWithoutClaim(t *testing.T, latched bool) {
	w, f, _ := recoveryWorkerFixture(t)
	if latched {
		w.disableExecution()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	brokerDone := make(chan error, 1)
	dials, pauses := 0, 0
	err := runWorkerLoop(ctx, w, readyWorkerLifecycleFixture(t), func(ctx context.Context) (*net.UnixConn, error) {
		dials++
		client, broker := conversationPair(t)
		go func() {
			if _, err := actionipc.Read(ctx, broker); err != nil {
				brokerDone <- ErrUnsafe
				return
			}
			_, err := broker.Write([]byte{0})
			broker.Close()
			brokerDone <- err
		}()
		return client, nil
	}, func(context.Context) error { pauses++; return ErrUnsafe })
	if err == nil || dials != 1 || pauses != 0 || w.executionDisabled() != latched || f.claims != 0 || len(f.operations) != 0 || <-brokerDone != nil {
		t.Fatal("partial selection became benign reconnect")
	}
}
