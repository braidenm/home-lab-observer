//go:build linux

package actionjournal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

type authority struct{}

func (authority) Check(context.Context, actioncore.Request, actioncore.Binding) error { return nil }

type syntheticEngine struct {
	calls, inspects       int
	failure               bool
	onInspect, onMutation func()
}

func (e *syntheticEngine) Inspect(context.Context, string) (actioncore.State, error) {
	e.inspects++
	if e.onInspect != nil {
		e.onInspect()
	}
	return actioncore.Exited, nil
}
func (e *syntheticEngine) Start(context.Context, string) error {
	e.calls++
	if e.onMutation != nil {
		e.onMutation()
	}
	if e.failure {
		return errors.New("synthetic-private-engine-error")
	}
	return nil
}
func (e *syntheticEngine) Stop(context.Context, string, uint8) error {
	return errors.New("unexpected stop")
}
func (e *syntheticEngine) Restart(context.Context, string, uint8) error {
	return errors.New("unexpected restart")
}
func execute(j *Journal, k actioncore.Key, e *syntheticEngine) actioncore.Code {
	r := k.Metadata.Request
	b := actioncore.Binding{ServerID: r.ServerID, InstallationID: r.InstallationID, OwnerID: r.OwnerID, TargetID: r.TargetID, EngineGeneration: k.Metadata.EngineGeneration, ContainerID: k.Metadata.ContainerID, PolicyRevision: r.PolicyRevision, CapabilityRevision: r.CapabilityRevision, Operations: []actioncore.Operation{actioncore.Start}}
	return actioncore.New(authority{}, j, e, func() time.Time { return epoch }).Execute(ctx, r, b, actioncore.Delivery{StartedAt: epoch, ReceivedAt: epoch, LeaseDeadline: epoch.Add(2 * time.Minute)})
}

func TestRealJournalWorkerNeverRepeatsSyntheticMutation(t *testing.T) {
	for _, phase := range []string{"ack", "engine-unknown", "intent-fault", "terminal-fault"} {
		t.Run(phase, func(t *testing.T) {
			j, dir := provision(t)
			engine := &syntheticEngine{failure: phase == "engine-unknown"}
			fault := func() error { return ErrRecovery }
			if phase == "intent-fault" {
				engine.onInspect = func() { j.beforeCommit = fault }
			}
			if phase == "terminal-fault" {
				engine.onMutation = func() { j.beforeCommit = fault }
			}
			first := execute(j, key(t, 1, 1), engine)
			want := actioncore.OutcomeUnknown
			if phase == "ack" {
				want = actioncore.Started
			}
			if first != want {
				t.Fatal("worker first result")
			}
			calls := 1
			if phase == "intent-fault" {
				calls = 0
			}
			if engine.calls != calls {
				t.Fatal("worker mutation count")
			}
			j.Close()
			re, e := OpenExisting(ctx, dir, owner)
			if e != nil {
				t.Fatal("reopen")
			}
			defer re.Close()
			re.now = func() time.Time { return epoch }
			engine.onMutation = nil
			engine.onInspect = nil
			// A failed uncommitted intent is provably undispatched; a fresh delivery may execute it.
			if phase == "intent-fault" {
				if execute(re, key(t, 1, 1), engine) != actioncore.Started || engine.calls != 1 {
					t.Fatal("undispatched retry")
				}
				return
			}
			if execute(re, key(t, 1, 1), engine) != want || engine.calls != 1 || engine.inspects != 1 {
				t.Fatal("retained delivery repeated Engine I/O")
			}
			if phase != "ack" {
				if execute(re, key(t, 2, 1), engine) != actioncore.OutcomeUnknown || engine.calls != 1 {
					t.Fatal("new key bypassed unknown target")
				}
			}
		})
	}
}
