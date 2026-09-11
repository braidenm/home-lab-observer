package uploadstate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

var testBinding = Binding{"srv_0123456789abcdef0123456789abcdef", "agent_0123456789abcdef0123456789abcdef"}
var testNow = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
var privateError = errors.New("synthetic-secret-error")

type fakeSource struct {
	body  []byte
	err   error
	reads int
	hook  func()
}

func (s *fakeSource) Read(context.Context, string) ([]byte, error) {
	s.reads++
	if s.hook != nil {
		s.hook()
	}
	return s.body, s.err
}

type fakeClock struct{ at time.Time }

func (c *fakeClock) Now() time.Time { return c.at }

type fakeTransport struct {
	requests []Request
	response Response
	err      error
	hook     func(Request)
}

func (tr *fakeTransport) Send(_ context.Context, r Request) (Response, error) {
	tr.requests = append(tr.requests, Request{r.Binding, r.Sequence, bytes.Clone(r.Body)})
	if tr.hook != nil {
		tr.hook(r)
	}
	return tr.response, tr.err
}

type fakeLedger struct {
	record          Record
	loads, commits  int
	loadErr         error
	failAt          int
	mutateOnFailure bool
	hook            func()
}

func (l *fakeLedger) Load(context.Context) (Record, error) {
	l.loads++
	return clone(l.record), l.loadErr
}
func (l *fakeLedger) Commit(_ context.Context, previous, next Record) error {
	l.commits++
	if !reflect.DeepEqual(previous, l.record) {
		return privateError
	}
	if l.hook != nil {
		l.hook()
	}
	if l.failAt == l.commits {
		if l.mutateOnFailure {
			l.record = clone(next)
		}
		return privateError
	}
	l.record = clone(next)
	return nil
}

type harness struct {
	m  *Machine
	s  *fakeSource
	c  *fakeClock
	tr *fakeTransport
	l  *fakeLedger
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{s: &fakeSource{body: bodyAt(t, testNow)}, c: &fakeClock{testNow}, tr: &fakeTransport{}, l: &fakeLedger{record: Record{Binding: testBinding}}}
	h.tr.response = Response{Acknowledged, testBinding.ServerID, 1}
	var err error
	h.m, err = New(testBinding, h.s, h.c, h.tr, h.l)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func bodyAt(t *testing.T, at time.Time) []byte {
	t.Helper()
	body, err := remoteprojection.Encode(observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at}, remoteprojection.Identity{SourceID: testBinding.ServerID, Version: "0.1.0-preview.3", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func step(t *testing.T, h *harness, want Outcome, wantErr error) {
	t.Helper()
	got, err := h.m.Step(context.Background())
	if got != want || err != wantErr {
		t.Fatalf("outcome %q error %v", got, err)
	}
}

func TestPersistBeforeSendAckAndDuplicate(t *testing.T) {
	h := newHarness(t)
	h.tr.hook = func(r Request) {
		if h.l.record.Pending == nil || h.l.record.Watermark != r.Sequence || !bytes.Equal(h.l.record.Pending.Body, r.Body) {
			t.Fatal("send before durable admission")
		}
		r.Body[0] = 'x'
	}
	step(t, h, Acknowledged, nil)
	if h.l.commits != 2 || h.l.record.Pending != nil || !h.l.record.HasAck || h.l.record.LastAck != sha256.Sum256(h.s.body) {
		t.Fatal("ack state incorrect")
	}
	step(t, h, Idle, nil)
	if len(h.tr.requests) != 1 || h.l.record.Watermark != 1 {
		t.Fatal("duplicate latest resent")
	}
}

func TestRetryAndRestartPreserveExactBody(t *testing.T) {
	h := newHarness(t)
	h.tr.err = privateError
	step(t, h, Retry, nil)
	original := bytes.Clone(h.l.record.Pending.Body)
	h.s.body = bodyAt(t, testNow.Add(time.Second))
	step(t, h, Retry, nil)
	h.m, _ = New(testBinding, h.s, h.c, h.tr, h.l)
	h.tr.err = nil
	step(t, h, Acknowledged, nil)
	for _, r := range h.tr.requests {
		if r.Sequence != 1 || !bytes.Equal(r.Body, original) {
			t.Fatal("retry changed identity/body")
		}
	}
	if h.s.reads != 1 {
		t.Fatal("pending request replaced from source")
	}
}

func TestCommitUncertaintyLatchesRecovery(t *testing.T) {
	for _, at := range []int{1, 2} {
		for _, mutated := range []bool{false, true} {
			h := newHarness(t)
			h.l.failAt = at
			h.l.mutateOnFailure = mutated
			step(t, h, "", ErrRecovery)
			sends, commits := len(h.tr.requests), h.l.commits
			step(t, h, "", ErrRecovery)
			if len(h.tr.requests) != sends || h.l.commits != commits {
				t.Fatal("uncertain commit resumed")
			}
			if at == 1 && sends != 0 {
				t.Fatal("failed admission sent")
			}
			h.l.failAt = 0
			h.m, _ = New(testBinding, h.s, h.c, h.tr, h.l)
			want := Acknowledged
			if at == 2 && mutated {
				want = Idle
			}
			step(t, h, want, nil)
		}
	}
}

func TestExpiredPendingRetiresWithoutSequenceReuse(t *testing.T) {
	h := newHarness(t)
	h.tr.response = Response{Outcome: Retry}
	step(t, h, Retry, nil)
	h.c.at = testNow.Add(MaxAge + time.Nanosecond)
	h.s.body = bodyAt(t, h.c.at)
	h.tr.response = Response{Acknowledged, testBinding.ServerID, 2}
	step(t, h, Acknowledged, nil)
	if len(h.tr.requests) != 2 || h.tr.requests[1].Sequence != 2 || h.l.record.Watermark != 2 {
		t.Fatal("expired sequence reused")
	}
	// Failed retirement is uncertain, even if the fake applied the record.
	h = newHarness(t)
	h.tr.response = Response{Outcome: Retry}
	step(t, h, Retry, nil)
	h.c.at = testNow.Add(MaxAge + time.Second)
	h.l.failAt = 2
	h.l.mutateOnFailure = true
	step(t, h, "", ErrRecovery)
	step(t, h, "", ErrRecovery)
	if len(h.tr.requests) != 1 {
		t.Fatal("sent after failed retirement")
	}
}

func TestTranslatedReceiverCases(t *testing.T) {
	data, err := os.ReadFile("testdata/receiver-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string  `json:"name"`
		Status   int     `json:"http_status"`
		Outcome  Outcome `json:"outcome"`
		Matches  bool    `json:"ack_matches"`
		Terminal bool    `json:"terminal"`
	}
	if json.Unmarshal(data, &cases) != nil {
		t.Fatal("invalid fixture")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			h := newHarness(t)
			h.tr.response = Response{Outcome: tc.Outcome}
			if tc.Matches {
				h.tr.response.ServerID = testBinding.ServerID
				h.tr.response.Sequence = 1
			}
			want := tc.Outcome
			if want == Acknowledged && !tc.Matches {
				want = Retry
			}
			step(t, h, want, nil)
			if tc.Terminal {
				h.m, _ = New(testBinding, h.s, h.c, h.tr, h.l)
				step(t, h, want, nil)
				if len(h.tr.requests) != 1 {
					t.Fatal("terminal state not durable")
				}
			}
		})
	}
}

func TestWrongAckCorrelationAlwaysRetries(t *testing.T) {
	for _, response := range []Response{{Acknowledged, testBinding.ServerID, 0}, {Acknowledged, testBinding.ServerID, 2}, {Acknowledged, "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1}, {Acknowledged, "", 1}} {
		h := newHarness(t)
		h.tr.response = response
		step(t, h, Retry, nil)
		if h.l.record.Pending == nil || h.l.record.HasAck || h.l.commits != 1 {
			t.Fatal("wrong ACK accepted")
		}
	}
}

func TestSourceAndClockBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		offset time.Duration
		want   Outcome
	}{
		{"age boundary", -MaxAge, Acknowledged}, {"too old", -MaxAge - time.Nanosecond, Expired},
		{"future boundary", MaxFuture, Acknowledged}, {"too future", MaxFuture + time.Nanosecond, ClockSkew},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.s.body = bodyAt(t, testNow.Add(tc.offset))
			step(t, h, tc.want, nil)
		})
	}
	for _, body := range [][]byte{nil, []byte("secret-canary"), bytes.Repeat([]byte("x"), remoteprojection.MaxBytes+1)} {
		h := newHarness(t)
		h.s.body = body
		step(t, h, "", ErrSnapshot)
		if h.l.commits != 0 || len(h.tr.requests) != 0 {
			t.Fatal("invalid source admitted")
		}
	}
	h := newHarness(t)
	h.s.err = privateError
	step(t, h, Idle, nil)
	h = newHarness(t)
	h.c.at = time.Time{}
	step(t, h, ClockSkew, nil)
	h = newHarness(t)
	h.s.hook = func() { h.c.at = testNow.Add(3 * time.Minute) }
	step(t, h, Expired, nil)
	h = newHarness(t)
	h.l.hook = func() { h.c.at = testNow.Add(3 * time.Minute) }
	step(t, h, Expired, nil)
	if len(h.tr.requests) != 0 {
		t.Fatal("aged during admission still sent")
	}
}

func TestCancellationBusyAndOverflow(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.m.Step(ctx); err != ErrCanceled || h.l.loads != 0 {
		t.Fatal("canceled step did work")
	}
	ctx, cancel = context.WithCancel(context.Background())
	h.tr.hook = func(Request) { cancel() }
	if _, err := h.m.Step(ctx); err != ErrCanceled || h.l.record.Pending == nil {
		t.Fatal("canceled send lost pending")
	}
	h = newHarness(t)
	h.tr.hook = func(Request) {
		if _, err := h.m.Step(context.Background()); err != ErrBusy {
			t.Fatal("overlapping step accepted")
		}
	}
	step(t, h, Acknowledged, nil)
	h = newHarness(t)
	h.l.record.Watermark = math.MaxInt64
	step(t, h, Exhausted, nil)
	h.m, _ = New(testBinding, h.s, h.c, h.tr, h.l)
	step(t, h, Exhausted, nil)
	if len(h.tr.requests) != 0 {
		t.Fatal("overflow sent")
	}
}

func TestMalformedLedgerNeverResets(t *testing.T) {
	for _, mutate := range []func(*Record){
		func(r *Record) { r.Binding.ServerID = "other" }, func(r *Record) { r.Watermark = -1 },
		func(r *Record) { r.HasAck = true }, func(r *Record) { r.LastAck[0] = 1 },
		func(r *Record) { r.Stopped = Retry }, func(r *Record) { r.Stopped = Exhausted },
		func(r *Record) { r.Pending = &Pending{Sequence: 1, Body: []byte("private-canary")} },
	} {
		h := newHarness(t)
		mutate(&h.l.record)
		step(t, h, "", ErrRecovery)
		if h.l.commits != 0 || len(h.tr.requests) != 0 {
			t.Fatal("invalid ledger repaired")
		}
	}
	h := newHarness(t)
	h.l.loadErr = privateError
	step(t, h, "", ErrRecovery)
	step(t, h, "", ErrRecovery)
	if h.l.loads != 1 {
		t.Fatal("failed load automatically retried")
	}
}

func TestPendingValidationTerminalCommitAndExpirySourceFailure(t *testing.T) {
	for _, mutate := range []func(*Record){
		func(r *Record) { r.Pending.Sequence = 0 },
		func(r *Record) { r.Pending.Sequence++ },
		func(r *Record) { r.Pending.CollectedAt = r.Pending.CollectedAt.Add(time.Second) },
		func(r *Record) { r.Pending.Digest[0]++ },
		func(r *Record) { r.Pending.Body = bytes.Repeat([]byte("x"), remoteprojection.MaxBytes+1) },
	} {
		h := newHarness(t)
		h.tr.response = Response{Outcome: Retry}
		step(t, h, Retry, nil)
		mutate(&h.l.record)
		h.m, _ = New(testBinding, h.s, h.c, h.tr, h.l)
		step(t, h, "", ErrRecovery)
		if len(h.tr.requests) != 1 {
			t.Fatal("invalid pending sent")
		}
	}
	for _, terminal := range []Outcome{CredentialRejected, Conflict, Rejected} {
		h := newHarness(t)
		h.tr.response = Response{Outcome: terminal}
		h.l.failAt = 2
		h.l.mutateOnFailure = true
		step(t, h, "", ErrRecovery)
		step(t, h, "", ErrRecovery)
		h.l.failAt = 0
		h.m, _ = New(testBinding, h.s, h.c, h.tr, h.l)
		step(t, h, terminal, nil)
		if len(h.tr.requests) != 1 {
			t.Fatal("terminal uncertain commit resent")
		}
	}
	h := newHarness(t)
	h.tr.response = Response{Outcome: Retry}
	step(t, h, Retry, nil)
	h.c.at = testNow.Add(-MaxFuture - time.Second)
	step(t, h, ClockSkew, nil)
	h.c.at = testNow.Add(MaxAge + time.Second)
	h.s.err = privateError
	step(t, h, Expired, nil)
	if h.l.record.Pending != nil || h.l.record.Watermark != 1 {
		t.Fatal("expired pending not retired")
	}
}

func TestMaximumPendingSequenceAndCancellationBeforeCommit(t *testing.T) {
	h := newHarness(t)
	body := bytes.Clone(h.s.body)
	h.l.record.Watermark = math.MaxInt64
	h.l.record.Pending = &Pending{Sequence: math.MaxInt64, Body: body, Digest: sha256.Sum256(body), CollectedAt: testNow}
	h.tr.response = Response{Acknowledged, testBinding.ServerID, math.MaxInt64}
	step(t, h, Acknowledged, nil)
	if h.l.record.Watermark != math.MaxInt64 || h.tr.requests[0].Sequence != math.MaxInt64 {
		t.Fatal("sequence precision lost")
	}
	h = newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	h.s.hook = cancel
	if _, err := h.m.Step(ctx); err != ErrCanceled || h.l.commits != 0 {
		t.Fatal("source cancellation admitted data")
	}
	h = newHarness(t)
	h.l.record.HasAck = true
	h.l.record.Watermark = 1
	h.l.record.LastAck = sha256.Sum256(h.s.body)
	if _, err := New(Binding{}, h.s, h.c, h.tr, h.l); err != ErrConfig {
		t.Fatal("invalid binding accepted")
	}
	if _, err := New(testBinding, nil, h.c, h.tr, h.l); err != ErrConfig {
		t.Fatal("missing dependency accepted")
	}
}
