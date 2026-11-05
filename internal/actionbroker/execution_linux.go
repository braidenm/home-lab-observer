//go:build linux

package actionbroker

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionengine"
	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

type executionConsumer struct {
	state            *brokerState
	engine           actioncore.Engine   // Private synthetic fixture seam; see constructor.
	files            currentPolicySource // Production constructor requires *brokerFiles.
	engineGeneration string              // Fixed protected approval, never learned from a mapping.
}

// Production can supply only the fixed, witness-checked actual-write adapter.
// No optional guard or raw Engine/transport constructor is exposed here.
func newExecutionConsumer(ctx context.Context, state *brokerState, engine *actionengine.GuardedEngine, files *brokerFiles) (*executionConsumer, error) {
	if state == nil || engine == nil || files == nil || !localHex(files.engine.generation, 32) {
		return nil, ErrRecoveryRequired
	}
	c := &executionConsumer{state: state, engine: engine, files: files, engineGeneration: files.engine.generation}
	if c.currentSource(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	return c, nil
}

func (c *executionConsumer) abandon(inv *acceptedInvocation) {
	inv.conn.Close()
	c.state.mu.Lock()
	c.state.recoveryRequired = true
	c.state.mu.Unlock()
	c.state.finish(inv.reservation)
}

func (b *brokerState) currentInvocation(inv *acceptedInvocation) error {
	if b.executionClosed.Load() {
		return ErrRecoveryRequired
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if inv == nil || inv.reservation == nil || b.live[inv.reservation.nonce] != inv.reservation || b.quiescing || b.executionClosed.Load() || b.policy.Revision != inv.reservation.policyHead {
		return ErrRecoveryRequired
	}
	resolved, err := resolveReceipt(b.policy, inv.claim.Request, true)
	if err != nil || resolved.key.Fingerprint != inv.resolved.key.Fingerprint || resolved.remoteDigest != inv.resolved.remoteDigest || resolved.mappingRevision != inv.resolved.mappingRevision {
		return ErrRecoveryRequired
	}
	return nil
}

type conversationAuthority struct {
	consumer      *executionConsumer
	state         *brokerState
	inv           *acceptedInvocation
	checkpoint    int
	lastChallenge string
}

func (a *conversationAuthority) Check(ctx context.Context, r actioncore.Request, binding actioncore.Binding) error {
	if a.checkpoint >= 2 || a.consumer.currentExecution(ctx, a.inv) != nil {
		return ErrRecoveryRequired
	}
	key, err := actioncore.NewJournalKey(r, binding)
	if err != nil || key.Fingerprint != a.inv.resolved.key.Fingerprint {
		return ErrRecoveryRequired
	}
	a.state.mu.Lock()
	challenge, err := a.state.freshNonce()
	a.state.mu.Unlock()
	if err != nil || challenge == a.lastChallenge {
		return ErrRecoveryRequired
	}
	a.lastChallenge = challenge
	a.checkpoint++
	checkpoint := strconv.Itoa(a.checkpoint)
	ordinal := strconv.Itoa(a.checkpoint + 2)
	request := actionprotocol.CheckRequest{Request: a.inv.claim.Request, ExecutionLeaseID: a.inv.claim.ExecutionLeaseID, Checkpoint: checkpoint, CheckNonce: challenge}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	send, err := a.state.clock.Capture()
	if err != nil || writeLocal(bounded, a.inv.conn, "AUTHORITY_CHALLENGE", a.inv.reservation.nonce, ordinal, request) != nil {
		return ErrRecoveryRequired
	}
	envelope, err := actionipc.Read(bounded, a.inv.conn)
	if err != nil || envelope.Version != actionprotocol.WorkerVersion || envelope.Kind != "AUTHORITY_REPLY" || envelope.ConversationNonce != a.inv.reservation.nonce || envelope.Ordinal != ordinal {
		return ErrRecoveryRequired
	}
	decision, err := actionprotocol.DecodeExecutionDecision(envelope.Payload)
	if err != nil || actionprotocol.MatchExecutionDecision(a.inv.claim, checkpoint, challenge, decision) != nil {
		return ErrRecoveryRequired
	}
	recv, err := a.state.clock.Capture()
	checked, e2 := actionprotocol.ParseHTTPTime(decision.CheckedAt)
	if err != nil || e2 != nil || bounded.Err() != nil || a.inv.execution.CheckDecision(send, recv, checked) != nil ||
		a.consumer.currentExecution(ctx, a.inv) != nil {
		return ErrRecoveryRequired
	}
	return nil
}

type phaseEngine struct {
	engine   actioncore.Engine
	inv      *acceptedInvocation
	consumer *executionConsumer
}

func (e *phaseEngine) Inspect(ctx context.Context, id string) (actioncore.State, error) {
	if id != e.inv.resolved.binding.ContainerID || e.consumer.currentExecution(ctx, e.inv) != nil {
		return actioncore.UnknownState, ErrRecoveryRequired
	}
	state, err := e.engine.Inspect(ctx, id)
	if err != nil || e.consumer.currentExecution(ctx, e.inv) != nil {
		return actioncore.UnknownState, ErrRecoveryRequired
	}
	return state, nil
}
func (e *phaseEngine) mutate(ctx context.Context, id string, op actioncore.Operation, grace uint8) error {
	if id != e.inv.resolved.binding.ContainerID || op != e.inv.claim.Request.Operation || grace != e.inv.resolved.key.Metadata.Request.GraceSeconds || e.consumer.currentExecution(ctx, e.inv) != nil || actionbudget.CheckMutation(ctx, id, op) != nil {
		return ErrRecoveryRequired
	}
	var err error
	switch op {
	case actioncore.Start:
		err = e.engine.Start(ctx, id)
	case actioncore.Stop:
		err = e.engine.Stop(ctx, id, grace)
	case actioncore.Restart:
		err = e.engine.Restart(ctx, id, grace)
	default:
		return ErrRecoveryRequired
	}
	if err != nil || e.consumer.currentExecution(ctx, e.inv) != nil || actionbudget.CheckMutation(ctx, id, op) != nil {
		return ErrRecoveryRequired
	}
	return nil
}
func (e *phaseEngine) Start(ctx context.Context, id string) error {
	return e.mutate(ctx, id, actioncore.Start, 0)
}
func (e *phaseEngine) Stop(ctx context.Context, id string, grace uint8) error {
	return e.mutate(ctx, id, actioncore.Stop, grace)
}
func (e *phaseEngine) Restart(ctx context.Context, id string, grace uint8) error {
	return e.mutate(ctx, id, actioncore.Restart, grace)
}

func (b *brokerState) retainedReport(ctx context.Context, inv *acceptedInvocation) (actionprotocol.Report, error) {
	// The generic core return code is deliberately NOT an input. In particular,
	// ErrUnresolvedTarget for ANOTHER action is not this action's durable INTENT.
	snapshot, err := b.journal.AdmissionSnapshot(ctx)
	if err != nil {
		return actionprotocol.Report{}, ErrRecoveryRequired
	}
	for _, record := range snapshot.Records {
		if record.Key.ActionID != inv.claim.Request.ActionID {
			continue
		}
		if record.Key.Fingerprint != inv.resolved.key.Fingerprint || record.Active {
			return actionprotocol.Report{}, ErrRecoveryRequired
		}
		evidence := "RETAINED_TERMINAL"
		if record.TerminalAt.IsZero() {
			if !record.HasIntent || record.Code != actioncore.OutcomeUnknown {
				return actionprotocol.Report{}, ErrRecoveryRequired
			}
			evidence = "QUIESCED_INTENT"
		}
		report := actionprotocol.Report{Request: inv.claim.Request, ExecutionLeaseID: inv.claim.ExecutionLeaseID, ResultCode: record.Code, EvidenceKind: evidence, CorrelationID: inv.claim.CorrelationID}
		raw, err := json.Marshal(report)
		if err != nil {
			return actionprotocol.Report{}, ErrRecoveryRequired
		}
		validated, err := actionprotocol.DecodeReport(raw)
		if err != nil {
			return actionprotocol.Report{}, ErrRecoveryRequired
		}
		return validated, nil
	}
	return actionprotocol.Report{}, ErrRecoveryRequired
}

func (c *executionConsumer) execute(ctx context.Context, inv *acceptedInvocation) (result error) {
	defer inv.conn.Close()
	defer func() {
		if result != nil {
			c.state.mu.Lock()
			c.state.recoveryRequired = true
			c.state.mu.Unlock()
		}
		if err := c.state.finish(inv.reservation); err != nil {
			result = ErrRecoveryRequired
		}
	}()
	if c.currentSource(ctx) != nil || c.state.currentInvocation(inv) != nil {
		return ErrRecoveryRequired
	}
	bounded := inv.execution.Context(ctx)
	if actionbudget.CheckExecution(bounded, inv.resolved.key.ActionID, inv.resolved.remoteDigest) != nil {
		return ErrRecoveryRequired
	}
	authority := &conversationAuthority{consumer: c, state: c.state, inv: inv}
	worker := actioncore.New(authority, &sourceJournal{consumer: c, inv: inv}, &phaseEngine{engine: c.engine, inv: inv, consumer: c}, nil)
	// Execute joins its actual Attempt before returning. A generic code cannot
	// grant reporting provenance or convert a transport/permission error to FAILED.
	worker.Execute(bounded, inv.resolved.key.Metadata.Request, inv.resolved.binding, inv.execution.Delivery())
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	report, err := c.retainedReport(readCtx, inv)
	if err != nil {
		return ErrRecoveryRequired
	}
	if writeLocal(ctx, inv.conn, "RESULT", inv.reservation.nonce, "5", report) != nil || c.currentSource(ctx) != nil || inv.execution.CheckReport(ctx) != nil {
		return ErrRecoveryRequired
	}
	// This proves only the local RESULT handoff. The trusted worker separately
	// reports via HTTPS; there is no invented remote-acceptance ACK on this channel.
	return nil
}
