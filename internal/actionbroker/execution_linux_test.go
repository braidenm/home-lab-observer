//go:build linux

package actionbroker

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

// A five-second overall test context would itself deny the frozen30s grace
// plus5s reserve before any mutation. This harness envelope does not change the
// production active/call/IPC limits, which remain independently enforced.
const executionHarnessLimit = 90 * time.Second

// Test-only composition, not an alternate runnable or peer-admission path.
func (c *executionConsumer) serve(ctx context.Context, conn *net.UnixConn) error {
	if c == nil || c.state == nil || conn == nil {
		return ErrRecoveryRequired
	}
	registry, err := newOwnedWorkers(ctx, &c.state.executionClosed)
	if err != nil {
		conn.Close()
		return ErrRecoveryRequired
	}
	// Cancel only this fixture registry; stopAdmission would fence the consumer.
	defer registry.cancel()
	h, err := registry.register()
	if err != nil {
		conn.Close()
		return ErrRecoveryRequired
	}
	defer h.close()
	return c.serveOwned(h, conn)
}

// Synthetic actuation only; actual Unix framing, native timing and SQLite are
// consumed. This fixture establishes no real HTTP authority or Docker effect.
// Production construction accepts only *actionengine.GuardedEngine.
type scriptedEngine struct {
	state     actioncore.State
	mutations int
	fail      bool
	onInspect func()
}

func (s *scriptedEngine) Inspect(context.Context, string) (actioncore.State, error) {
	if s.onInspect != nil {
		s.onInspect()
	}
	return s.state, nil
}
func (s *scriptedEngine) mutate(ctx context.Context, id string, op actioncore.Operation) error {
	if actionbudget.CheckMutation(ctx, id, op) != nil {
		return errors.New("synthetic missing timing")
	}
	s.mutations++
	if s.fail {
		return errors.New("synthetic-secret-canary")
	}
	return nil
}
func (s *scriptedEngine) Start(ctx context.Context, id string) error {
	return s.mutate(ctx, id, actioncore.Start)
}
func (s *scriptedEngine) Stop(ctx context.Context, id string, _ uint8) error {
	return s.mutate(ctx, id, actioncore.Stop)
}
func (s *scriptedEngine) Restart(ctx context.Context, id string, _ uint8) error {
	return s.mutate(ctx, id, actioncore.Restart)
}

func executionConsumerFixture(t *testing.T, op actioncore.Operation) (*brokerState, mappingDocument, *scriptedEngine, *executionConsumer) {
	t.Helper()
	b, m := reservationFixture(t)
	m.Operation = op
	if op == actioncore.Start {
		m.Parameters.StopGraceSeconds = actionprotocol.Grace{Null: true}
	}
	p := fixedPolicy(m)
	var err error
	b.policy = p
	b.readyHead = p.Revision
	_, b.readyEntriesDigest, err = policyEntries(p)
	if err != nil {
		t.Fatal("operation timing correlation")
	}
	engine := &scriptedEngine{state: actioncore.Running}
	if op == actioncore.Start {
		engine.state = actioncore.Exited
	}
	// Private fixture injection is NOT a production engine/permission constructor.
	return b, m, engine, &executionConsumer{state: b, engine: engine, files: &syntheticPolicySource{policy: p}, engineGeneration: m.EngineGeneration}
}

func TestExecutionConsumerProductionConstructorRejectsMissingGuard(t *testing.T) {
	b, _ := reservationFixture(t)
	if _, err := newExecutionConsumer(context.Background(), b, nil, nil); err == nil {
		t.Fatal("missing guarded engine enabled runnable consumer")
	}
}

func TestExecutionConsumerThreeOperationsRequireTwoFreshChecksAndRetainedReport(t *testing.T) {
	for _, op := range []actioncore.Operation{actioncore.Start, actioncore.Stop, actioncore.Restart} {
		t.Run(string(op), func(t *testing.T) {
			b, m, engine, consumer := executionConsumerFixture(t, op)
			server, worker := conversationPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), executionHarnessLimit)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- consumer.serve(ctx, server) }()
			sendBeginning(t, worker)
			approval, err := actionipc.Read(ctx, worker)
			if err != nil || approval.Kind != "APPROVAL_SNAPSHOT" {
				t.Fatal("execution selection")
			}
			claim := liveClaim(t, m)
			if writeLocal(ctx, worker, "CLAIMED_EXECUTE", approval.ConversationNonce, "2", actionprotocol.ClaimedExecute{Claim: claim, ClaimRTTNanos: "0"}) != nil {
				t.Fatal("delivery")
			}
			last := ""
			for index, ordinal := range []string{"3", "4"} {
				e, err := actionipc.Read(ctx, worker)
				if err != nil || e.Kind != "AUTHORITY_CHALLENGE" || e.Ordinal != ordinal {
					t.Fatal("genuine checkpoint missing")
				}
				q, err := actionprotocol.DecodeCheckRequest(e.Payload)
				checkpoint := []string{"1", "2"}[index]
				if err != nil || q.Checkpoint != checkpoint || q.Request != claim.Request || q.CheckNonce == last {
					t.Fatal("stale/substituted authority challenge")
				}
				last = q.CheckNonce
				decision := actionprotocol.ExecutionDecision{Request: q.Request, ExecutionLeaseID: q.ExecutionLeaseID, Checkpoint: q.Checkpoint, CheckNonce: q.CheckNonce,
					CredentialID: "hlac_" + strings.Repeat("8", 32), CredentialRevision: "1", CheckedAt: time.Now().UTC().Truncate(time.Microsecond).Format(actionprotocol.HTTPTimeLayout), Decision: "PERMITTED"}
				if writeLocal(ctx, worker, "AUTHORITY_REPLY", e.ConversationNonce, ordinal, decision) != nil {
					t.Fatal("fresh typed reply")
				}
			}
			e, err := actionipc.Read(ctx, worker)
			if err != nil || e.Kind != "RESULT" || e.Ordinal != "5" {
				t.Fatal("retained report handoff")
			}
			report, err := actionprotocol.DecodeReport(e.Payload)
			want := map[actioncore.Operation]actioncore.Code{actioncore.Start: actioncore.Started, actioncore.Stop: actioncore.Stopped, actioncore.Restart: actioncore.Restarted}[op]
			if err != nil || report.Request != claim.Request || report.ExecutionLeaseID != claim.ExecutionLeaseID || report.CorrelationID != claim.CorrelationID || report.ResultCode != want || report.EvidenceKind != "RETAINED_TERMINAL" || <-done != nil || engine.mutations != 1 {
				t.Fatal("incorrect operation/report provenance")
			}
			retained, err := b.journal.AdmissionSnapshot(context.Background())
			if err != nil || len(retained.Records) != 1 || retained.Records[0].Code != want || retained.Records[0].Active {
				t.Fatal("report preceded durable terminal/join")
			}
		})
	}
}

func TestExecutionConsumerDenialAndPolicyRevocationNeverFabricateResult(t *testing.T) {
	for _, mode := range []string{"first-deny", "second-deny", "wrong-lease", "policy-revoke"} {
		t.Run(mode, func(t *testing.T) {
			b, m, engine, consumer := executionConsumerFixture(t, actioncore.Restart)
			if mode == "policy-revoke" {
				engine.onInspect = func() { b.mu.Lock(); b.policy.Current = []string{}; b.mu.Unlock() }
			}
			server, worker := conversationPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), executionHarnessLimit)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- consumer.serve(ctx, server) }()
			sendBeginning(t, worker)
			approval, err := actionipc.Read(ctx, worker)
			if err != nil {
				t.Fatal("approval")
			}
			claim := liveClaim(t, m)
			if writeLocal(ctx, worker, "CLAIMED_EXECUTE", approval.ConversationNonce, "2", actionprotocol.ClaimedExecute{Claim: claim, ClaimRTTNanos: "0"}) != nil {
				t.Fatal("delivery")
			}
			for ordinal := 1; ordinal <= 2; ordinal++ {
				e, err := actionipc.Read(ctx, worker)
				if mode == "policy-revoke" && ordinal == 2 {
					if err == nil {
						t.Fatal("revoked local approval performed remote checkpoint")
					}
					break
				}
				if err != nil || e.Kind != "AUTHORITY_CHALLENGE" {
					t.Fatal("challenge")
				}
				q, _ := actionprotocol.DecodeCheckRequest(e.Payload)
				d := actionprotocol.ExecutionDecision{Request: q.Request, ExecutionLeaseID: q.ExecutionLeaseID, Checkpoint: q.Checkpoint, CheckNonce: q.CheckNonce, CredentialID: "hlac_" + strings.Repeat("8", 32), CredentialRevision: "1", CheckedAt: time.Now().UTC().Truncate(time.Microsecond).Format(actionprotocol.HTTPTimeLayout), Decision: "PERMITTED"}
				stop := mode == "first-deny" && ordinal == 1 || mode == "second-deny" && ordinal == 2 || mode == "wrong-lease" && ordinal == 1
				if stop {
					if mode == "wrong-lease" {
						d.ExecutionLeaseID = "hle_" + strings.Repeat("d", 32)
					} else {
						d.Decision = "DENIED"
					}
				}
				if writeLocal(ctx, worker, "AUTHORITY_REPLY", e.ConversationNonce, e.Ordinal, d) != nil {
					t.Fatal("decision")
				}
				if stop {
					break
				}
			}
			if <-done == nil || engine.mutations != 0 || !b.recoveryRequired {
				t.Fatal("denied/changed binding executed or silently released")
			}
			if _, err := actionipc.Read(ctx, worker); err == nil {
				t.Fatal("operational denial became domain result")
			}
		})
	}
}

func TestExecutionConsumerUnknownRequiresSameActionJoinedIntent(t *testing.T) {
	b, m, engine, consumer := executionConsumerFixture(t, actioncore.Restart)
	engine.fail = true
	server, worker := conversationPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), executionHarnessLimit)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- consumer.serve(ctx, server) }()
	sendBeginning(t, worker)
	approval, err := actionipc.Read(ctx, worker)
	if err != nil {
		t.Fatal("approval")
	}
	claim := liveClaim(t, m)
	if writeLocal(ctx, worker, "CLAIMED_EXECUTE", approval.ConversationNonce, "2", actionprotocol.ClaimedExecute{Claim: claim, ClaimRTTNanos: "0"}) != nil {
		t.Fatal("delivery")
	}
	for i := 0; i < 2; i++ {
		e, err := actionipc.Read(ctx, worker)
		if err != nil {
			t.Fatal("challenge")
		}
		q, _ := actionprotocol.DecodeCheckRequest(e.Payload)
		d := actionprotocol.ExecutionDecision{Request: q.Request, ExecutionLeaseID: q.ExecutionLeaseID, Checkpoint: q.Checkpoint, CheckNonce: q.CheckNonce, CredentialID: "hlac_" + strings.Repeat("8", 32), CredentialRevision: "1", CheckedAt: time.Now().UTC().Truncate(time.Microsecond).Format(actionprotocol.HTTPTimeLayout), Decision: "PERMITTED"}
		if writeLocal(ctx, worker, "AUTHORITY_REPLY", e.ConversationNonce, e.Ordinal, d) != nil {
			t.Fatal("reply")
		}
	}
	e, err := actionipc.Read(ctx, worker)
	if err != nil || e.Kind != "RESULT" {
		t.Fatal("joined INTENT report")
	}
	var report actionprotocol.Report
	if json.Unmarshal(e.Payload, &report) != nil || report.ResultCode != actioncore.OutcomeUnknown || report.EvidenceKind != "QUIESCED_INTENT" || <-done != nil || !b.recoveryRequired {
		t.Fatal("uncertainty did not retain original intent/fence")
	}
	if strings.Contains(string(e.Payload), "synthetic-secret-canary") {
		t.Fatal("raw adapter error escaped")
	}
}

func TestRetainedReportNeverBorrowsAnotherActionsUnknownOrLiveAttempt(t *testing.T) {
	b, m, _, _ := executionConsumerFixture(t, actioncore.Restart)
	r, err := b.reserve(context.Background())
	if err != nil {
		t.Fatal("reservation")
	}
	claim := liveClaim(t, m)
	bound, execution, resolved, err := b.bindClaim(context.Background(), r, claim, 0)
	if err != nil {
		t.Fatal("binding")
	}
	a, err := bound.Acquire(execution.Context(context.Background()), resolved.key)
	if err != nil {
		t.Fatal("admission")
	}
	inv := &acceptedInvocation{reservation: r, claim: claim, journal: bound, execution: execution, resolved: resolved}
	if _, err := b.retainedReport(context.Background(), inv); err == nil {
		t.Fatal("live/uncommitted invocation exposed a result")
	}
	a.Close()
	otherRequest := resolved.key.Metadata.Request
	otherRequest.ActionID = "hla_" + strings.Repeat("d", 32)
	otherKey, err := actioncore.NewJournalKey(otherRequest, resolved.binding)
	if err != nil {
		t.Fatal("other synthetic identity")
	}
	// Explicit RAW component-fixture path only: no runnable constructor can use it.
	active, err := b.journal.Acquire(context.Background(), otherKey)
	if err != nil || active.Intent(context.Background()) != nil {
		t.Fatal("other action intent")
	}
	active.Close()
	if _, err := b.retainedReport(context.Background(), inv); err == nil {
		t.Fatal("other action UNKNOWN fabricated this action's QUIESCED_INTENT")
	}
}
