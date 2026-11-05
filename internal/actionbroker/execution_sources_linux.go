//go:build linux

package actionbroker

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionengine"
	"github.com/braidenm/home-lab-observer/internal/actionjournal"
	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

// Private to this package. The only production constructor accepts *brokerFiles;
// synthetic sources are deliberately confined to private test construction.
type currentPolicySource interface {
	Current(context.Context) (policyDocument, error)
}

func (c *executionConsumer) currentSource(ctx context.Context) error {
	if c == nil || c.state == nil || c.files == nil || ctx == nil || ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	p, err := c.files.Current(ctx)
	if err != nil || validatePolicy(p) != nil {
		return ErrRecoveryRequired
	}
	c.state.mu.Lock()
	want, e1 := json.Marshal(c.state.policy)
	c.state.mu.Unlock()
	got, e2 := json.Marshal(p)
	if e1 != nil || e2 != nil || !bytes.Equal(want, got) {
		return ErrRecoveryRequired
	}
	if validateJournalMappings(ctx, c.state.journal, p, c.engineGeneration) != nil || ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	return nil
}

func validateJournalMappings(ctx context.Context, journal *actionjournal.Journal, p policyDocument, generation string) error {
	snapshot, err := journal.AdmissionSnapshot(ctx)
	if err != nil || snapshot.Binding != (actionjournal.Binding{ServerID: p.ServerID, InstallationID: p.ConnectorInstanceID, OwnerID: p.OwnerUserID}) {
		return ErrRecoveryRequired
	}
	keys := make([]actioncore.Key, 0, len(snapshot.Records))
	for _, record := range snapshot.Records {
		if requireEngineGeneration(record.Key, generation) != nil {
			return ErrRecoveryRequired
		}
		keys = append(keys, record.Key)
	}
	if validateMappingReferences(p, keys) != nil {
		return ErrRecoveryRequired
	}
	return nil
}

func (c *executionConsumer) currentExecution(ctx context.Context, inv *acceptedInvocation) error {
	if c == nil || c.state == nil || c.state.executionClosed.Load() || inv == nil || inv.execution == nil || actionbudget.CheckExecution(ctx, inv.resolved.key.ActionID, inv.resolved.remoteDigest) != nil || c.currentSource(ctx) != nil || c.state.currentInvocation(inv) != nil ||
		actionbudget.CheckExecution(ctx, inv.resolved.key.ActionID, inv.resolved.remoteDigest) != nil {
		return ErrRecoveryRequired
	}
	return nil
}

func (c *executionConsumer) retainedReport(ctx context.Context, inv *acceptedInvocation) (actionprotocol.Report, error) {
	if inv == nil {
		return actionprotocol.Report{}, ErrRecoveryRequired
	}
	digest, err := actionprotocol.ReceiptDigest(inv.claim.Request)
	if err != nil || digest != inv.resolved.remoteDigest || inv.claim.Request.ActionID != inv.resolved.key.ActionID {
		return actionprotocol.Report{}, ErrRecoveryRequired
	}
	if inv == nil || inv.execution == nil || inv.execution.CheckReport(ctx) != nil || c.currentSource(ctx) != nil {
		return actionprotocol.Report{}, ErrRecoveryRequired
	}
	report, err := c.state.retainedReport(ctx, inv)
	if err != nil || c.currentSource(ctx) != nil || inv.execution.CheckReport(ctx) != nil {
		return actionprotocol.Report{}, ErrRecoveryRequired
	}
	return report, nil
}

// The mandatory timing-aware BindExecution still guards each actual COMMIT.
// These additional source tripwires cannot make a root-file change atomic with
// a later syscall/commit. Any failed post-effect check remains uncertain.
type sourceJournal struct {
	consumer *executionConsumer
	inv      *acceptedInvocation
}

func (j *sourceJournal) Acquire(ctx context.Context, key actioncore.Key) (actioncore.Attempt, error) {
	if j.consumer.currentExecution(ctx, j.inv) != nil {
		return nil, ErrRecoveryRequired
	}
	a, err := j.inv.journal.Acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	if a == nil || j.consumer.currentExecution(ctx, j.inv) != nil {
		if a != nil {
			a.Close()
		}
		return nil, ErrRecoveryRequired
	}
	return &sourceAttempt{Attempt: a, source: j, ctx: ctx}, nil
}

type sourceAttempt struct {
	actioncore.Attempt
	source *sourceJournal
	ctx    context.Context
}

func (a *sourceAttempt) Existing() actioncore.Code {
	if a.source.consumer.currentExecution(a.ctx, a.source.inv) != nil {
		return actioncore.Unavailable
	}
	code := a.Attempt.Existing()
	if a.source.consumer.currentExecution(a.ctx, a.source.inv) != nil {
		return actioncore.Unavailable
	}
	return code
}
func (a *sourceAttempt) Intent(ctx context.Context) error {
	if a.source.consumer.currentExecution(ctx, a.source.inv) != nil {
		return ErrRecoveryRequired
	}
	if a.Attempt.Intent(ctx) != nil || a.source.consumer.currentExecution(ctx, a.source.inv) != nil {
		return ErrRecoveryRequired
	}
	return nil
}
func (a *sourceAttempt) Finish(ctx context.Context, code actioncore.Code) error {
	if a.source.consumer.currentExecution(ctx, a.source.inv) != nil {
		return ErrRecoveryRequired
	}
	if a.Attempt.Finish(ctx, code) != nil || a.source.consumer.currentExecution(ctx, a.source.inv) != nil {
		return ErrRecoveryRequired
	}
	return nil
}

// Fixed startup only: no create/adopt/repair or caller-controlled paths. Files
// prove roles/namespace and the explicit root witness before Engine or journal.
// The returned consumer owns both handles; no listener has yet been created.
func openExecutionConsumer(ctx context.Context) (_ *executionConsumer, result error) {
	files, err := loadBrokerFiles(ctx)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	p := files.policy
	journal, err := actionjournal.OpenExisting(ctx, actionprofile.JournalPath, actionjournal.Binding{ServerID: p.ServerID, InstallationID: p.ConnectorInstanceID, OwnerID: p.OwnerUserID})
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	owned := true
	defer func() {
		if owned {
			journal.Close()
		}
	}()
	if validateJournalMappings(ctx, journal, p, files.engine.generation) != nil {
		return nil, ErrRecoveryRequired
	}
	clock, err := actionbudget.NewClock()
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	state, err := newBrokerState(ctx, p, journal, clock)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	if _, err := files.Current(ctx); err != nil {
		return nil, ErrRecoveryRequired
	}
	engine, err := actionengine.OpenGuarded(ctx, files.engine.witness)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	engineOwned := true
	defer func() {
		if engineOwned {
			engine.Close()
		}
	}()
	consumer, err := newExecutionConsumer(ctx, state, engine, files)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	owned, engineOwned = false, false
	return consumer, nil
}

// Only after all owning conversations have joined; it performs no service work.
func (c *executionConsumer) close() error {
	if c == nil || c.state == nil || c.engine == nil {
		return ErrRecoveryRequired
	}
	engine, ok := c.engine.(*actionengine.GuardedEngine)
	if !ok {
		return ErrRecoveryRequired
	}
	e1 := engine.Close()
	e2 := c.state.journal.Close()
	if e1 != nil || e2 != nil {
		return ErrRecoveryRequired
	}
	return nil
}
