//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type sourceUnavailable struct{}

func (sourceUnavailable) Read(context.Context, string) ([]byte, error) {
	return nil, errors.New("synthetic unavailable")
}

type testLedger struct{ binding uploadstate.Binding }

func (l testLedger) Load(context.Context) (uploadstate.Record, error) {
	return uploadstate.Record{Binding: l.binding}, nil
}
func (testLedger) Commit(context.Context, uploadstate.Record, uploadstate.Record) error {
	return errors.New("unexpected commit")
}

type unusedTransport struct{ t *testing.T }

func (t unusedTransport) Send(context.Context, uploadstate.Request) (uploadstate.Response, error) {
	t.t.Fatal("unexpected request")
	return uploadstate.Response{}, nil
}

func TestCollectionFreshnessDoesNotUseAckReceipt(t *testing.T) {
	now := time.Now().UTC()
	if freshness(now.Add(-uploadstate.MaxAge-time.Nanosecond), now) != "ACKNOWLEDGED_STALE" || freshness(time.Time{}, now) != "ACKNOWLEDGED_STALE" || freshness(now.Add(uploadstate.MaxFuture+time.Nanosecond), now) != "ACKNOWLEDGED_STALE" {
		t.Fatal("stale source presented fresh")
	}
	if freshness(now.Add(-time.Second), now) != "ACKNOWLEDGED_FRESH" {
		t.Fatal("fresh source lost")
	}
	if freshness(now.Add(-connectedstatus.FreshnessWindow), now) != "ACKNOWLEDGED_FRESH" || freshness(now.Add(-connectedstatus.FreshnessWindow-time.Nanosecond), now) != "ACKNOWLEDGED_STALE" || freshness(now.Add(-2*time.Minute), now) != "ACKNOWLEDGED_STALE" {
		t.Fatal("admission age confused with hosted freshness")
	}
}

func TestPollingIsNotCountedAsTransportAttempt(t *testing.T) {
	b := uploadstate.Binding{ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32)}
	transport := &observedTransport{delegate: unusedTransport{t}}
	machine, err := uploadstate.New(b, sourceUnavailable{}, realClock{}, transport, testLedger{b})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("chmod")
	}
	status, err := connectedstatus.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer status.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := observedStepper{machine: machine, transport: transport, status: status, cancel: cancel, record: connectedstatus.Record{Version: "observer-connected-status/v1"}}
	if outcome, err := w.Step(ctx); err != nil || outcome != uploadstate.SourceUnavailable {
		t.Fatal("poll result")
	}
	if w.record.Steps != 1 || w.record.Attempts != 0 || w.record.Acknowledgements != 0 || w.record.State != "SOURCE_UNAVAILABLE" {
		t.Fatal("poll incorrectly reported network or success")
	}
}
