//go:build linux

package actionbroker

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionjournal"
)

func TestBrokerRestartCannotResumeUnknownEvenWithAnotherFixedTarget(t *testing.T) {
	ctx := context.Background()
	m := policyMapping("5", "6")
	other := policyMapping("7", "8")
	p := fixedPolicy(m, other)
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private journal")
	}
	binding := actionjournal.Binding{ServerID: p.ServerID, InstallationID: p.ConnectorInstanceID, OwnerID: p.OwnerUserID}
	j, err := actionjournal.Provision(ctx, dir, binding)
	if err != nil {
		t.Fatal("fixture provision")
	}
	defer j.Close()
	resolved, err := resolveReceipt(p, policyReceipt(t, m), true)
	if err != nil {
		t.Fatal("fixed receipt")
	}
	// Explicit raw component fixture only: persist a real INTENT without an effect.
	a, err := j.Acquire(ctx, resolved.key)
	if err != nil || a.Intent(ctx) != nil {
		t.Fatal("durable intent")
	}
	a.Close()
	if j.Close() != nil {
		t.Fatal("close")
	}
	reopened, err := actionjournal.OpenExisting(ctx, dir, binding)
	if err != nil {
		t.Fatal("exact existing store")
	}
	defer reopened.Close()
	clock, err := actionbudget.NewClock()
	if err != nil {
		t.Fatal("native clock")
	}
	if state, err := newBrokerState(ctx, p, reopened, clock); !errors.Is(err, ErrRecoveryRequired) || state != nil {
		t.Fatal("restart or another fixed target bypassed retained uncertainty")
	}
	s, err := reopened.AdmissionSnapshot(ctx)
	if err != nil || len(s.Records) != 1 || !s.Records[0].HasIntent || s.Records[0].Code != actioncore.OutcomeUnknown || !s.Records[0].TerminalAt.IsZero() || s.Records[0].Active {
		t.Fatal("startup changed immutable retained uncertainty")
	}
}
