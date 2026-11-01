//go:build linux

package actionjournal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

var ctx = context.Background()
var owner = Binding{"srv_" + strings.Repeat("a", 32), "agent_" + strings.Repeat("b", 32), "usr_" + strings.Repeat("c", 32)}
var epoch = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func key(t *testing.T, n, target int) actioncore.Key {
	t.Helper()
	r := actioncore.Request{ActionID: fmt.Sprintf("hla_%032x", n), ServerID: owner.ServerID, InstallationID: owner.InstallationID, OwnerID: owner.OwnerID, ActorID: "usr_" + strings.Repeat("d", 32), TargetID: fmt.Sprintf("hlt_%032x", target), Operation: actioncore.Start, PolicyRevision: 1, CapabilityRevision: 1, CreatedAt: epoch, ExpiresAt: epoch.Add(5 * time.Minute)}
	b := actioncore.Binding{ServerID: r.ServerID, InstallationID: r.InstallationID, OwnerID: r.OwnerID, TargetID: r.TargetID, EngineGeneration: strings.Repeat("e", 32), ContainerID: fmt.Sprintf("%064x", target), PolicyRevision: 1, CapabilityRevision: 1, Operations: []actioncore.Operation{actioncore.Start}}
	k, err := actioncore.NewJournalKey(r, b)
	if err != nil {
		t.Fatal("fixture key")
	}
	return k
}
func provision(t *testing.T) (*Journal, string) {
	t.Helper()
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	j, err := Provision(ctx, dir, owner)
	if err != nil {
		t.Fatal("provision")
	}
	j.now = func() time.Time { return epoch }
	t.Cleanup(func() { j.Close() })
	return j, dir
}
func acquire(t *testing.T, j *Journal, k actioncore.Key) actioncore.Attempt {
	t.Helper()
	a, e := j.Acquire(ctx, k)
	if e != nil {
		t.Fatal("acquire")
	}
	return a
}

func TestIntentOneShotAndUnknownSurvivesCloseReopen(t *testing.T) {
	j, dir := provision(t)
	k := key(t, 1, 1)
	a := acquire(t, j, k)
	if a.Existing() != "" || a.Intent(ctx) != nil || a.Intent(ctx) == nil {
		t.Fatal("intent not one-shot")
	}
	a.Close()
	old := acquire(t, j, k)
	if old.Existing() != actioncore.OutcomeUnknown || old.Intent(ctx) == nil {
		t.Fatal("unknown replay")
	}
	old.Close()
	if _, e := j.Acquire(ctx, key(t, 2, 1)); !errors.Is(e, actioncore.ErrUnresolvedTarget) {
		t.Fatal("new key bypass")
	}
	j.Close()
	re, e := OpenExisting(ctx, dir, owner)
	if e != nil {
		t.Fatal("reopen")
	}
	defer re.Close()
	re.now = func() time.Time { return epoch.Add(100 * 24 * time.Hour) }
	if e := re.Prune(ctx); e != nil {
		t.Fatal("prune unknown")
	}
	again := acquire(t, re, k)
	if again.Existing() != actioncore.OutcomeUnknown {
		t.Fatal("unknown expired")
	}
	again.Close()
	if a.Finish(ctx, actioncore.Started) == nil {
		t.Fatal("closed stale handle finished")
	}
}

func TestTwoPermitsPhysicalFenceAndTerminalReplay(t *testing.T) {
	j, _ := provision(t)
	a := acquire(t, j, key(t, 1, 1))
	b := acquire(t, j, key(t, 2, 2))
	if _, e := j.Acquire(ctx, key(t, 3, 3)); !errors.Is(e, ErrBusy) {
		t.Fatal("third permit")
	}
	alias := key(t, 4, 4)
	alias.Metadata.ContainerID = key(t, 1, 1).Metadata.ContainerID
	raw, _ := actioncore.CanonicalJournalMetadata(key(t, 4, 4))
	_ = raw
	// Reconstruct a valid fingerprint for the same physical container under another opaque target.
	r := alias.Metadata.Request
	binding := actioncore.Binding{ServerID: r.ServerID, InstallationID: r.InstallationID, OwnerID: r.OwnerID, TargetID: r.TargetID, EngineGeneration: alias.Metadata.EngineGeneration, ContainerID: alias.Metadata.ContainerID, PolicyRevision: 1, CapabilityRevision: 1, Operations: []actioncore.Operation{actioncore.Start}}
	alias, _ = actioncore.NewJournalKey(r, binding)
	b.Close()
	if _, e := j.Acquire(ctx, alias); !errors.Is(e, ErrBusy) {
		t.Fatal("physical alias bypass")
	}
	if a.Intent(ctx) != nil || a.Finish(ctx, actioncore.Started) != nil || a.Finish(ctx, actioncore.Started) == nil {
		t.Fatal("terminal transition")
	}
	a.Close()
	replay := acquire(t, j, key(t, 1, 1))
	if replay.Existing() != actioncore.Started || replay.Intent(ctx) == nil {
		t.Fatal("terminal replay")
	}
	replay.Close()
	if _, e := j.Acquire(ctx, key(t, 5, 1)); !errors.Is(e, ErrBusy) {
		t.Fatal("cooldown")
	}
	j.now = func() time.Time { return epoch.Add(30 * time.Second) }
	a = acquire(t, j, key(t, 5, 1))
	a.Close()
}

func TestNoRecreationAndCloseInvalidatesHandles(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	if _, e := OpenExisting(ctx, dir, owner); e == nil {
		t.Fatal("missing store recreated")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("missing open wrote")
	}
	j, e := Provision(ctx, dir, owner)
	if e != nil {
		t.Fatal("provision")
	}
	j.now = func() time.Time { return epoch }
	a := acquire(t, j, key(t, 1, 1))
	if _, e := Provision(ctx, dir, owner); e == nil {
		t.Fatal("adoption")
	}
	j.Close()
	j.Close()
	if a.Intent(ctx) == nil {
		t.Fatal("close left active handle")
	}
	if os.Remove(filepath.Join(dir, "action.sqlite")) != nil {
		t.Fatal("fixture removal")
	}
	if _, e := OpenExisting(ctx, dir, owner); e == nil {
		t.Fatal("lost state recreated")
	}
}

func TestClockRollbackDoesNotTerminalizeUndispatchedAdmission(t *testing.T) {
	j, _ := provision(t)
	a := acquire(t, j, key(t, 1, 1))
	a.Close()
	j.now = func() time.Time { return epoch.Add(-time.Second) }
	if _, e := j.Acquire(ctx, key(t, 1, 1)); !errors.Is(e, actioncore.ErrIdentityConflict) {
		t.Fatal("rollback terminalized valid future admission")
	}
	r, e := j.read(ctx, key(t, 1, 1).ActionID)
	if e != nil || !r.terminal.IsZero() {
		t.Fatal("rollback changed admission")
	}
}
