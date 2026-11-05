//go:build linux

package actionengine

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

// Real native timing, synthetic authority and a private Unix HTTP fixture only.
// These tokens prove no actual remote permission and never contact Docker.
func executionTiming(t *testing.T, op actioncore.Operation) context.Context {
	t.Helper()
	c, err := actionbudget.NewClock()
	if err != nil {
		t.Fatal("native timing unavailable")
	}
	send, _ := c.Capture()
	now := time.Now().UTC().Truncate(time.Microsecond)
	recv, _ := c.Capture()
	r, err := c.ObserveServerTime(now, send, recv)
	if err != nil {
		t.Fatal("server bracket")
	}
	conversation, err := c.Begin(r)
	if err != nil {
		t.Fatal("begin")
	}
	claimed := time.Now().UTC().Truncate(time.Microsecond)
	p := actionprotocol.Parameters{FormatVersion: 1, StopGraceSeconds: actionprotocol.Grace{Seconds: 30}}
	if op == actioncore.Start {
		p.StopGraceSeconds = actionprotocol.Grace{Null: true}
	}
	hash, err := actionprotocol.ParameterDigest(op, p)
	if err != nil {
		t.Fatal("parameters")
	}
	request := actionprotocol.RequestReceipt{
		ActionID: "hla_" + strings.Repeat("1", 32), ServerID: "srv_" + strings.Repeat("2", 32),
		OwnerUserID: "usr_" + strings.Repeat("3", 32), ActorUserID: "usr_" + strings.Repeat("3", 32),
		ConnectorInstanceID: "agent_" + strings.Repeat("4", 32), Operation: op,
		TargetID: "hlt_" + strings.Repeat("5", 32), TargetKey: "ctr_0000000000000000",
		IdempotencyKeyHash: strings.Repeat("6", 64), PolicyRevision: "1", CapabilityRevision: "1",
		Parameters: p, ParameterSHA256: hash, RequestedAt: claimed.Add(-time.Second).Format(actionprotocol.HTTPTimeLayout),
		ExpiresAt: claimed.Add(299 * time.Second).Format(actionprotocol.HTTPTimeLayout), CorrelationID: "synthetic-engine-budget",
	}
	claim := actionprotocol.Claim{Request: request, ExecutionLeaseID: "hle_" + strings.Repeat("7", 32),
		ClaimCredentialID: "hlac_" + strings.Repeat("8", 32), ClaimCredentialRevision: "1",
		ClaimedAt: claimed.Format(actionprotocol.HTTPTimeLayout), LeaseExpiresAt: claimed.Add(120 * time.Second).Format(actionprotocol.HTTPTimeLayout), CorrelationID: "synthetic-claim"}
	e, err := conversation.AcceptClaim(claim, 0, fullID)
	if err != nil {
		t.Fatal("execution timing")
	}
	return e.Context(context.Background())
}

func TestGuardedEngineRequiresMatchingOperationBeforeAnyRequest(t *testing.T) {
	var calls atomic.Int32
	endpoint := unixFixture(t, versionJSON, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	a, err := openGuardedEndpoint(context.Background(), endpoint)
	if err != nil {
		t.Fatal("guarded fixture")
	}
	t.Cleanup(func() { a.Close() })
	if a.Start(context.Background(), fullID) == nil || calls.Load() != 0 {
		t.Fatal("missing budget reached transport")
	}
	ctx := executionTiming(t, actioncore.Start)
	if a.Stop(ctx, fullID, 30) == nil || a.Start(ctx, strings.Repeat("b", 64)) == nil || calls.Load() != 0 {
		t.Fatal("timing correlation moved to another operation or target")
	}
	if a.Start(ctx, fullID) != nil || calls.Load() != 1 {
		t.Fatal("matching guarded start")
	}
	if a.Stop(executionTiming(t, actioncore.Stop), fullID, 30) != nil ||
		a.Restart(executionTiming(t, actioncore.Restart), fullID, 30) != nil || calls.Load() != 3 {
		t.Fatal("independent matching stop/restart budgets")
	}
}

func TestGuardedEngineDoesNotReportCanceledAcknowledgementAsSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(executionTiming(t, actioncore.Start))
	defer cancel()
	var calls atomic.Int32
	endpoint := unixFixture(t, versionJSON, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// A real POST was received. Cancellation cannot establish that its
		// effect did not happen, even when the server sends an acknowledgement.
		cancel()
		w.WriteHeader(http.StatusNoContent)
	})
	a, err := openGuardedEndpoint(context.Background(), endpoint)
	if err != nil {
		t.Fatal("guarded fixture")
	}
	t.Cleanup(func() { a.Close() })
	if a.Start(ctx, fullID) == nil || calls.Load() != 1 {
		t.Fatal("canceled acknowledgement succeeded or mutation was retried")
	}
}

type writeProbe struct {
	net.Conn
	calls int
	after func()
}

func (p *writeProbe) Write(b []byte) (int, error) {
	p.calls++
	if p.after != nil {
		p.after()
	}
	return len(b), nil
}

func TestGuardedConnectionChecksBeforeAndAfterActualWrite(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-write", true: "after-write"}[after], func(t *testing.T) {
			ctx, cancel := context.WithCancel(executionTiming(t, actioncore.Start))
			defer cancel()
			probe := &writeProbe{}
			if after {
				probe.after = cancel
			} else {
				cancel()
			}
			conn := &guardedConnection{Conn: probe, context: ctx, target: fullID, operation: actioncore.Start}
			n, err := conn.Write([]byte("synthetic"))
			if err == nil || probe.calls != map[bool]int{false: 0, true: 1}[after] || n != map[bool]int{false: 0, true: 9}[after] {
				t.Fatal("late write reported success or precheck performed a write")
			}
		})
	}
}
