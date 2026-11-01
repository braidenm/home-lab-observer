// Package actioncore contains unused typed sequencing logic. It provides no
// credential, Docker transport, durable journal, listener or installed worker.
package actioncore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"
)

type Operation string

const (
	Start   Operation = "CONTAINER_START"
	Stop    Operation = "CONTAINER_STOP"
	Restart Operation = "CONTAINER_RESTART"
)

type State string

const (
	Created      State = "created"
	Exited       State = "exited"
	Running      State = "running"
	Paused       State = "paused"
	Restarting   State = "restarting"
	Removing     State = "removing"
	Dead         State = "dead"
	UnknownState State = "unknown"
)

// Code never includes raw adapter errors, daemon output or credential material.
type Code string

const (
	Started          Code = "STARTED"
	Stopped          Code = "STOPPED"
	Restarted        Code = "RESTARTED"
	AlreadyRunning   Code = "ALREADY_RUNNING"
	AlreadyStopped   Code = "ALREADY_STOPPED"
	Denied           Code = "DENIED"
	StateConflict    Code = "TARGET_STATE_CONFLICT"
	Unavailable      Code = "UNAVAILABLE"
	BudgetExceeded   Code = "BUDGET_EXCEEDED"
	OutcomeUnknown   Code = "OUTCOME_UNKNOWN"
	IdentityConflict Code = "IDEMPOTENCY_CONFLICT"
)

var ErrIdentityConflict = errors.New("action_identity_conflict")
var ErrUnresolvedTarget = errors.New("action_target_unresolved")

// Request is an internal authenticated receipt, NOT an HTTP body or authority.
// A future adapter must prove the provenance of every field before composition.
type Request struct {
	ActionID, ServerID, InstallationID, OwnerID, ActorID, TargetID string
	Operation                                                      Operation
	GraceSeconds                                                   uint8
	PolicyRevision, CapabilityRevision                             uint64
	CreatedAt, ExpiresAt                                           time.Time
}

// Binding comes only from protected local owner policy, never inventory aliases.
// It must not be exposed through remote responses, logs or telemetry.
type Binding struct {
	ServerID, InstallationID, OwnerID, TargetID, EngineGeneration, ContainerID string
	PolicyRevision, CapabilityRevision                                         uint64
	GraceSeconds                                                               uint8
	Operations                                                                 []Operation
}

// Delivery times are locally measured. LeaseDeadline is a trusted, conservatively
// translated authenticated deadline, not a browser timestamp. It is not identity.
type Delivery struct{ StartedAt, ReceivedAt, LeaseDeadline time.Time }

type Authority interface {
	Check(context.Context, Request, Binding) error
}
type Engine interface {
	Inspect(context.Context, string) (State, error)
	Start(context.Context, string) error
	Stop(context.Context, string, uint8) error
	Restart(context.Context, string, uint8) error
}

// Key is private journal identity, not a wire format or authentication proof.
type Key struct {
	ActionID, TargetID string
	Fingerprint        [32]byte
	Metadata           JournalMetadata
}

// Journal must atomically compare fingerprints and serialize the target. It must
// preserve unresolved intent across restart, new keys, expiry and pruning. No
// production implementation is provided here; a memory fake proves ordering only.
type Journal interface {
	Acquire(context.Context, Key) (Attempt, error)
}
type Attempt interface {
	// Empty means provably undispatched; intent without a terminal result is UNKNOWN.
	Existing() Code
	Intent(context.Context) error
	Finish(context.Context, Code) error
	// Close releases the handle, never deletes durable intent or its target fence.
	Close()
}

type Worker struct {
	authority Authority
	journal   Journal
	engine    Engine
	now       func() time.Time
}

func New(authority Authority, journal Journal, engine Engine, now func() time.Time) *Worker {
	if now == nil {
		now = time.Now
	}
	return &Worker{authority, journal, engine, now}
}

func (w *Worker) Execute(parent context.Context, request Request, binding Binding, delivery Delivery) Code {
	now := w.now()
	if !valid(request, binding, now) {
		return Denied
	}
	if w.authority == nil || w.journal == nil || w.engine == nil {
		return Unavailable
	}
	if delivery.StartedAt.IsZero() || delivery.ReceivedAt.Before(delivery.StartedAt) ||
		delivery.ReceivedAt.Sub(delivery.StartedAt) > 30*time.Second || now.Before(delivery.ReceivedAt) {
		return BudgetExceeded
	}
	end := earliest(request.ExpiresAt, delivery.StartedAt.Add(90*time.Second), delivery.ReceivedAt.Add(time.Minute), delivery.LeaseDeadline.Add(-30*time.Second))
	budget := remaining(parent, now, end)
	if budget <= 0 {
		return BudgetExceeded
	}
	ctx, cancel := context.WithTimeout(parent, budget)
	defer cancel()
	if w.authority.Check(ctx, request, binding) != nil {
		return Denied
	}
	if remaining(ctx, w.now(), end) <= 0 {
		return BudgetExceeded
	}
	key, err := NewJournalKey(request, binding)
	if err != nil {
		return Unavailable
	}
	attempt, err := w.journal.Acquire(ctx, key)
	if errors.Is(err, ErrIdentityConflict) {
		return IdentityConflict
	}
	if errors.Is(err, ErrUnresolvedTarget) {
		return OutcomeUnknown
	}
	if err != nil || attempt == nil {
		return Unavailable
	}
	defer attempt.Close()
	if remaining(ctx, w.now(), end) <= 0 {
		return BudgetExceeded
	}
	if code := attempt.Existing(); code != "" {
		if remaining(ctx, w.now(), end) <= 0 {
			return BudgetExceeded
		}
		if retained(code, request.Operation) {
			return code
		}
		return Unavailable
	}
	if remaining(ctx, w.now(), end) <= 0 {
		return BudgetExceeded
	}
	state, err := w.engine.Inspect(ctx, binding.ContainerID)
	if err != nil {
		return Unavailable
	}
	if remaining(ctx, w.now(), end) <= 0 {
		return BudgetExceeded
	}
	if w.authority.Check(ctx, request, binding) != nil {
		return Denied
	}
	if remaining(ctx, w.now(), end) <= 0 {
		return BudgetExceeded
	}
	code, mutate := classify(request.Operation, state)
	if !mutate {
		if attempt.Finish(ctx, code) != nil {
			return Unavailable
		}
		if remaining(ctx, w.now(), end) <= 0 {
			return BudgetExceeded
		}
		return code
	}
	if callBudget(ctx, w.now(), end, request.GraceSeconds) <= 0 {
		return BudgetExceeded
	}
	if attempt.Intent(ctx) != nil {
		return OutcomeUnknown
	}
	// Persistence can consume the reserved time. Intent is never silently erased.
	budget = callBudget(ctx, w.now(), end, request.GraceSeconds)
	if budget <= 0 {
		return OutcomeUnknown
	}
	callCtx, cancelCall := context.WithTimeout(ctx, budget)
	callStarted := w.now()
	switch request.Operation {
	case Start:
		err = w.engine.Start(callCtx, binding.ContainerID)
	case Stop:
		err = w.engine.Stop(callCtx, binding.ContainerID, request.GraceSeconds)
	case Restart:
		err = w.engine.Restart(callCtx, binding.ContainerID, request.GraceSeconds)
	}
	late := callCtx.Err() != nil || w.now().Sub(callStarted) >= budget || remaining(ctx, w.now(), end) <= 5*time.Second
	cancelCall()
	if err != nil || late {
		return OutcomeUnknown
	}
	if attempt.Finish(ctx, code) != nil || remaining(ctx, w.now(), end) <= 0 {
		return OutcomeUnknown
	}
	return code
}

func callBudget(ctx context.Context, now, end time.Time, grace uint8) time.Duration {
	budget := min(45*time.Second, remaining(ctx, now, end)-5*time.Second)
	if budget < time.Duration(grace)*time.Second+5*time.Second {
		return 0
	}
	return budget
}

func remaining(ctx context.Context, now, end time.Time) time.Duration {
	if ctx.Err() != nil {
		return 0
	}
	d := end.Sub(now)
	if deadline, ok := ctx.Deadline(); ok {
		d = min(d, time.Until(deadline))
	}
	return d
}

func classify(op Operation, state State) (Code, bool) {
	switch {
	case op == Start && state == Running:
		return AlreadyRunning, false
	case op == Start && (state == Created || state == Exited):
		return Started, true
	case op == Stop && (state == Created || state == Exited):
		return AlreadyStopped, false
	case op == Stop && state == Running:
		return Stopped, true
	case op == Restart && state == Running:
		return Restarted, true
	default:
		return StateConflict, false
	}
}

func valid(r Request, b Binding, now time.Time) bool {
	if !identifier(r.ActionID, "hla_") || !identifier(r.ServerID, "srv_") || !identifier(r.InstallationID, "agent_") ||
		!identifier(r.OwnerID, "usr_") || !identifier(r.ActorID, "usr_") || !identifier(r.TargetID, "hlt_") ||
		!hex(b.ContainerID, 64) || !hex(b.EngineGeneration, 32) || r.PolicyRevision == 0 || r.CapabilityRevision == 0 ||
		r.ServerID != b.ServerID || r.InstallationID != b.InstallationID || r.OwnerID != b.OwnerID || r.TargetID != b.TargetID ||
		r.PolicyRevision != b.PolicyRevision || r.CapabilityRevision != b.CapabilityRevision ||
		r.CreatedAt.IsZero() || !r.ExpiresAt.After(r.CreatedAt) || r.ExpiresAt.Sub(r.CreatedAt) > 5*time.Minute ||
		now.Before(r.CreatedAt) || !now.Before(r.ExpiresAt) || len(b.Operations) < 1 || len(b.Operations) > 3 {
		return false
	}
	seen := map[Operation]bool{}
	for _, op := range b.Operations {
		if (op != Start && op != Stop && op != Restart) || seen[op] {
			return false
		}
		seen[op] = true
	}
	if !seen[r.Operation] {
		return false
	}
	if (seen[Stop] || seen[Restart]) && (b.GraceSeconds < 1 || b.GraceSeconds > 30) {
		return false
	}
	if r.Operation == Start {
		return r.GraceSeconds == 0
	}
	return r.GraceSeconds >= 1 && r.GraceSeconds <= 30 && r.GraceSeconds == b.GraceSeconds
}

func identifier(value, prefix string) bool {
	return len(value) == len(prefix)+32 && value[:len(prefix)] == prefix && hex(value[len(prefix):], 32)
}
func hex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func earliest(values ...time.Time) time.Time {
	end := values[0]
	for _, v := range values[1:] {
		if v.Before(end) {
			end = v
		}
	}
	return end
}
func retained(code Code, op Operation) bool {
	switch code {
	case Denied, StateConflict, OutcomeUnknown:
		return true
	case Started, AlreadyRunning:
		return op == Start
	case Stopped, AlreadyStopped:
		return op == Stop
	case Restarted:
		return op == Restart
	}
	return false
}

func fingerprint(r Request, b Binding) [32]byte {
	// Fixed struct + scalar UTC values, no maps or delivery-specific state.
	value := journalRecordFor(r, b)
	encoded, _ := json.Marshal(value) // The closed scalar shape cannot fail encoding.
	return sha256.Sum256(encoded)
}
