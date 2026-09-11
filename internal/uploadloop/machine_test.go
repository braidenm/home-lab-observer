package uploadloop

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

type sourceFunc func(context.Context, string) ([]byte, error)

func (f sourceFunc) Read(ctx context.Context, id string) ([]byte, error) { return f(ctx, id) }

type transportFunc func(context.Context, uploadstate.Request) (uploadstate.Response, error)

func (f transportFunc) Send(ctx context.Context, r uploadstate.Request) (uploadstate.Response, error) {
	return f(ctx, r)
}

type testLedger struct {
	record  uploadstate.Record
	commits int
}

func (l *testLedger) Load(context.Context) (uploadstate.Record, error) {
	return uploadstate.CloneRecord(l.record), nil
}
func (l *testLedger) Commit(_ context.Context, expected, next uploadstate.Record) error {
	if !reflect.DeepEqual(expected, l.record) {
		return uploadstate.ErrRecovery
	}
	l.record = uploadstate.CloneRecord(next)
	l.commits++
	return nil
}

var testBinding = uploadstate.Binding{ServerID: "srv_0123456789abcdef0123456789abcdef", ConnectorID: "agent_0123456789abcdef0123456789abcdef"}

func TestRealMachineRetainsPendingAcrossPacedRetries(t *testing.T) {
	clock := &testClock{time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)}
	ledger := &testLedger{record: uploadstate.Record{Binding: testBinding}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var captured []uploadstate.Request
	source := sourceFunc(func(context.Context, string) ([]byte, error) {
		return remoteprojection.Encode(observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: clock.Now()}, remoteprojection.Identity{SourceID: testBinding.ServerID, Version: "0.1.0", OS: "linux"})
	})
	transport := transportFunc(func(_ context.Context, r uploadstate.Request) (uploadstate.Response, error) {
		if ledger.record.Pending == nil || ledger.record.Pending.Sequence != r.Sequence || !bytes.Equal(ledger.record.Pending.Body, r.Body) {
			t.Fatal("request preceded durable pending")
		}
		captured = append(captured, r)
		switch len(captured) {
		case 1:
			return uploadstate.Response{Outcome: uploadstate.Retry}, nil
		case 2:
			return uploadstate.Response{Outcome: uploadstate.RateLimited}, nil
		default:
			return uploadstate.Response{Outcome: uploadstate.Acknowledged, ServerID: testBinding.ServerID, Sequence: r.Sequence}, nil
		}
	})
	machine, err := uploadstate.New(testBinding, source, clock, transport, ledger)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := New(stepFunc(func(ctx context.Context) (uploadstate.Outcome, error) {
		outcome, err := machine.Step(ctx)
		if outcome == uploadstate.Acknowledged {
			cancel()
		}
		return outcome, err
	}))
	var delays []time.Duration
	l.wait = func(_ context.Context, d time.Duration) error {
		// No transport work runs in the waiting path.
		if len(captured) != len(delays) {
			t.Fatal("unexpected send while waiting")
		}
		delays = append(delays, d)
		clock.now = clock.now.Add(d)
		return nil
	}
	l.jitter = func(cap time.Duration) time.Duration { return cap }
	result, err := l.Run(ctx)
	if result != Canceled || err != ErrCanceled || !reflect.DeepEqual(delays, []time.Duration{startupDelay, time.Second, maxRetryDelay}) {
		t.Fatal("pacing/cancellation mismatch")
	}
	if len(captured) != 3 || ledger.commits != 2 || ledger.record.Pending != nil || !ledger.record.HasAck || ledger.record.Watermark != 1 || l.Stats().Acknowledgements != 1 {
		t.Fatal("machine commit authority changed")
	}
	for _, r := range captured {
		if r.Sequence != 1 || !bytes.Equal(r.Body, captured[0].Body) {
			t.Fatal("pending identity changed across retries")
		}
	}
}

func TestRealMachineCanPollPastInvalidSnapshotWithoutSendingIt(t *testing.T) {
	clock := &testClock{time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)}
	ledger := &testLedger{record: uploadstate.Record{Binding: testBinding}}
	valid := false
	sends := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := sourceFunc(func(context.Context, string) ([]byte, error) {
		if !valid {
			return []byte("synthetic-invalid-document"), nil
		}
		return remoteprojection.Encode(observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: clock.Now()}, remoteprojection.Identity{SourceID: testBinding.ServerID, Version: "0.1.0", OS: "linux"})
	})
	machine, err := uploadstate.New(testBinding, source, clock, transportFunc(func(_ context.Context, r uploadstate.Request) (uploadstate.Response, error) {
		sends++
		return uploadstate.Response{Outcome: uploadstate.Acknowledged, ServerID: testBinding.ServerID, Sequence: r.Sequence}, nil
	}), ledger)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := New(stepFunc(func(ctx context.Context) (uploadstate.Outcome, error) {
		o, e := machine.Step(ctx)
		if o == uploadstate.Acknowledged {
			cancel()
		}
		return o, e
	}))
	waits := 0
	l.wait = func(_ context.Context, d time.Duration) error {
		waits++
		if waits == 2 {
			if d != pollDelay || sends != 0 || ledger.commits != 0 {
				t.Fatal("invalid source was sent or changed ledger")
			}
			valid = true
		}
		clock.now = clock.now.Add(d)
		return nil
	}
	if result, err := l.Run(ctx); result != Canceled || err != ErrCanceled || sends != 1 || l.Stats().InvalidSnapshots != 1 {
		t.Fatal("invalid-source recovery mismatch")
	}
}
