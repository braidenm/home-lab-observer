//go:build linux

package actionbroker

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

// Only private synthetic consumers can use this source. Production construction
// requires the concrete fixed-path brokerFiles reader, not a caller callback.
type syntheticPolicySource struct {
	mu     sync.Mutex
	policy policyDocument
	err    error
}

func (s *syntheticPolicySource) Current(ctx context.Context) (policyDocument, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx == nil || ctx.Err() != nil || s.err != nil {
		return policyDocument{}, ErrPolicy
	}
	return s.policy, nil
}

func (s *syntheticPolicySource) refuse() { s.mu.Lock(); s.err = ErrPolicy; s.mu.Unlock() }

func TestConsumerSourceTripwireAndRetainedMappingsUseActualJournal(t *testing.T) {
	b, m, _, consumer := executionConsumerFixture(t, actioncore.Restart)
	if consumer.currentSource(context.Background()) != nil {
		t.Fatal("unchanged synthetic source refused")
	}
	r, err := b.reserve(context.Background())
	if err != nil {
		t.Fatal("reservation")
	}
	bound, execution, resolved, err := b.bindClaim(context.Background(), r, liveClaim(t, m), 0)
	if err != nil {
		t.Fatal("binding")
	}
	a, err := bound.Acquire(execution.Context(context.Background()), resolved.key)
	if err != nil {
		t.Fatal("actual journal admission")
	}
	a.Close()
	if consumer.currentSource(context.Background()) != nil {
		t.Fatal("retained mapping refused")
	}
	// Both local copies agree: only the actual retained row reveals the loss.
	p := b.policy
	p.Mappings = []mappingDocument{}
	p.Current = []string{}
	if validatePolicy(p) != nil {
		t.Fatal("missing-mapping fixture itself invalid")
	}
	b.policy = p
	consumer.files = &syntheticPolicySource{policy: p}
	if consumer.currentSource(context.Background()) == nil {
		t.Fatal("missing actual retained mapping admitted")
	}
}

func TestConsumerChangedOrCanceledSourceDeniesEngineAndAdmission(t *testing.T) {
	for _, mode := range []string{"changed", "canceled", "missing"} {
		t.Run(mode, func(t *testing.T) {
			b, m, engine, consumer := executionConsumerFixture(t, actioncore.Restart)
			r, err := b.reserve(context.Background())
			if err != nil {
				t.Fatal("reservation")
			}
			claim := liveClaim(t, m)
			bound, execution, resolved, err := b.bindClaim(context.Background(), r, claim, 0)
			if err != nil {
				t.Fatal("binding")
			}
			inv := &acceptedInvocation{reservation: r, claim: claim, journal: bound, execution: execution, resolved: resolved}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = execution.Context(ctx)
			adapter := &phaseEngine{engine: engine, inv: inv, consumer: consumer}
			if consumer.currentExecution(ctx, inv) != nil {
				t.Fatal("valid baseline refused before fault")
			}
			if _, err := adapter.Inspect(ctx, m.ContainerID); err != nil {
				t.Fatal("valid source baseline Inspect")
			}
			switch mode {
			case "changed":
				consumer.files.(*syntheticPolicySource).refuse()
			case "canceled":
				cancel()
			case "missing":
				consumer.files = nil
			}
			if adapter.Restart(ctx, m.ContainerID, resolved.key.Metadata.Request.GraceSeconds) == nil || engine.mutations != 0 {
				t.Fatal("source tripwire allowed Engine write")
			}
			journal := &sourceJournal{consumer: consumer, inv: inv}
			if a, err := journal.Acquire(ctx, resolved.key); err == nil || a != nil {
				t.Fatal("source tripwire admitted journal record")
			}
			snapshot, err := b.journal.AdmissionSnapshot(context.Background())
			if err != nil || len(snapshot.Records) != 0 {
				t.Fatal("refusal changed journal")
			}
		})
	}
}

func TestSourceChangeAtEitherAuthorityReplyPreventsEngineWrites(t *testing.T) {
	for _, checkpoint := range []int{1, 2} {
		t.Run([]string{"first", "second"}[checkpoint-1], func(t *testing.T) {
			_, m, engine, consumer := executionConsumerFixture(t, actioncore.Restart)
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
				t.Fatal("claim")
			}
			for index := 1; index <= checkpoint; index++ {
				e, err := actionipc.Read(ctx, worker)
				if err != nil {
					t.Fatal("checkpoint")
				}
				q, err := actionprotocol.DecodeCheckRequest(e.Payload)
				if err != nil {
					t.Fatal("typed challenge")
				}
				if index == checkpoint {
					consumer.files.(*syntheticPolicySource).refuse()
				}
				d := actionprotocol.ExecutionDecision{Request: q.Request, ExecutionLeaseID: q.ExecutionLeaseID, Checkpoint: q.Checkpoint, CheckNonce: q.CheckNonce, CredentialID: "hlac_" + strings.Repeat("8", 32), CredentialRevision: "1", CheckedAt: time.Now().UTC().Truncate(time.Microsecond).Format(actionprotocol.HTTPTimeLayout), Decision: "PERMITTED"}
				if writeLocal(ctx, worker, "AUTHORITY_REPLY", e.ConversationNonce, e.Ordinal, d) != nil {
					t.Fatal("reply")
				}
			}
			if <-done == nil || engine.mutations != 0 {
				t.Fatal("changed protected source allowed write")
			}
			if _, err := actionipc.Read(ctx, worker); err == nil {
				t.Fatal("source refusal became result")
			}
		})
	}
}

func TestSourceTripwireGuardsIntentFinishAndRetainedReporting(t *testing.T) {
	for _, phase := range []string{"intent", "finish", "report"} {
		t.Run(phase, func(t *testing.T) {
			b, m, _, consumer := executionConsumerFixture(t, actioncore.Restart)
			r, err := b.reserve(context.Background())
			if err != nil {
				t.Fatal("reservation")
			}
			claim := liveClaim(t, m)
			bound, execution, resolved, err := b.bindClaim(context.Background(), r, claim, 0)
			if err != nil {
				t.Fatal("binding")
			}
			inv := &acceptedInvocation{reservation: r, claim: claim, journal: bound, execution: execution, resolved: resolved}
			ctx := execution.Context(context.Background())
			a, err := (&sourceJournal{consumer: consumer, inv: inv}).Acquire(ctx, resolved.key)
			if err != nil {
				t.Fatal("actual guarded admission")
			}
			defer a.Close()
			if phase == "report" {
				if a.Finish(ctx, actioncore.StateConflict) != nil {
					t.Fatal("actual retained terminal")
				}
				a.Close()
				if _, err := consumer.retainedReport(context.Background(), inv); err != nil {
					t.Fatal("valid report baseline")
				}
			}
			consumer.files.(*syntheticPolicySource).refuse()
			switch phase {
			case "intent":
				if a.Intent(ctx) == nil {
					t.Fatal("source change wrote intent")
				}
			case "finish":
				if a.Finish(ctx, actioncore.StateConflict) == nil {
					t.Fatal("source change wrote terminal")
				}
			case "report":
				if _, err := consumer.retainedReport(context.Background(), inv); err == nil {
					t.Fatal("source change handed off report")
				}
			}
			a.Close()
			snapshot, err := b.journal.AdmissionSnapshot(context.Background())
			if err != nil || len(snapshot.Records) != 1 {
				t.Fatal("retained row missing")
			}
			row := snapshot.Records[0]
			if row.HasIntent || (phase != "report" && (!row.TerminalAt.IsZero() || row.Code != "")) || (phase == "report" && row.Code != actioncore.StateConflict) {
				t.Fatal("refusal changed retained evidence")
			}
		})
	}
}
