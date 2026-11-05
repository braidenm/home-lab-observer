//go:build linux

package actionbroker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actionjournal"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

func reservationFixture(t *testing.T) (*brokerState, mappingDocument) {
	t.Helper()
	m := policyMapping("5", "6")
	p := fixedPolicy(m)
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private test store")
	}
	j, err := actionjournal.Provision(context.Background(), dir, actionjournal.Binding{ServerID: m.ServerID, InstallationID: m.ConnectorInstanceID, OwnerID: m.OwnerUserID})
	if err != nil {
		t.Fatal("real v1 store")
	}
	t.Cleanup(func() { j.Close() })
	clock, err := actionbudget.NewClock()
	if err != nil {
		t.Fatal("native clock")
	}
	b, err := newBrokerState(context.Background(), p, j, clock)
	if err != nil {
		t.Fatal("broker state")
	}
	send, _ := clock.Capture()
	server := time.Now().UTC()
	recv, _ := clock.Capture()
	b.readiness, err = clock.ObserveServerTime(server, send, recv)
	if err != nil {
		t.Fatal("synthetic fresh timing, not remote authority")
	}
	b.readyHead = p.Revision
	_, b.readyEntriesDigest, err = policyEntries(p)
	if err != nil {
		t.Fatal("exact synthetic policy readiness")
	}
	return b, m
}

func liveClaim(t *testing.T, m mappingDocument) actionprotocol.Claim {
	t.Helper()
	r := policyReceipt(t, m)
	now := time.Now().UTC().Truncate(time.Microsecond)
	r.RequestedAt = now.Add(-time.Second).Format(actionprotocol.HTTPTimeLayout)
	r.ExpiresAt = now.Add(299 * time.Second).Format(actionprotocol.HTTPTimeLayout)
	return actionprotocol.Claim{Request: r, ExecutionLeaseID: "hle_" + strings.Repeat("7", 32),
		ClaimCredentialID: "hlac_" + strings.Repeat("8", 32), ClaimCredentialRevision: "1",
		ClaimedAt: now.Format(actionprotocol.HTTPTimeLayout), LeaseExpiresAt: now.Add(120 * time.Second).Format(actionprotocol.HTTPTimeLayout), CorrelationID: "synthetic-reservation"}
}

func TestPreclaimReservationCountsOnceAcrossDurableIntentHandoff(t *testing.T) {
	b, m := reservationFixture(t)
	one, err := b.reserve(context.Background())
	if err != nil || len(one.targets) != 1 || one.targets[0] != m.TargetID {
		t.Fatal("eligible opaque preclaim reservation")
	}
	two, err := b.reserve(context.Background())
	if err != nil {
		t.Fatal("second preclaim slot")
	}
	if _, err := b.reserve(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatal("third preclaim bypassed global two-permit bound")
	}
	if b.releaseUnclaimed(context.Background(), two) != nil {
		t.Fatal("known unclaimed slot release")
	}
	bound, execution, resolved, err := b.bindClaim(context.Background(), one, liveClaim(t, m), 0)
	if err != nil {
		t.Fatal("exact fresh claim binding")
	}
	attempt, err := bound.Acquire(execution.Context(context.Background()), resolved.key)
	if err != nil || attempt.Intent(execution.Context(context.Background())) != nil {
		t.Fatal("actual durable handoff")
	}
	if b.releaseUnclaimed(context.Background(), one) == nil {
		t.Fatal("claimed conversation released as known 204")
	}
	// This alive INTENT is the SAME slot, not a second unresolved permit.
	// Its only approved target is busy, so eligibility refuses without claiming.
	if _, err := b.reserve(context.Background()); !errors.Is(err, ErrNoEligibleTarget) {
		t.Fatal("live target was offered again or live INTENT double-counted")
	}
	attempt.Close()
	if b.finish(one) != nil {
		t.Fatal("joined conversation completion")
	}
	if _, err := b.reserve(context.Background()); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("durable UNKNOWN allowed new claims")
	}
}

func TestPreclaimRequiresFreshClockAndCannotResumeStoredAction(t *testing.T) {
	b, m := reservationFixture(t)
	ready := b.readiness
	b.readiness = nil
	if _, err := b.reserve(context.Background()); err == nil {
		t.Fatal("no validated readiness minted an execution nonce")
	}
	b.readiness = ready
	one, err := b.reserve(context.Background())
	if err != nil {
		t.Fatal("reserve")
	}
	claim := liveClaim(t, m)
	bound, execution, resolved, err := b.bindClaim(context.Background(), one, claim, 0)
	if err != nil {
		t.Fatal("claim")
	}
	a, err := bound.Acquire(execution.Context(context.Background()), resolved.key)
	if err != nil {
		t.Fatal("admission")
	}
	a.Close()
	if b.finish(one) != nil {
		t.Fatal("joined no-intent conversation")
	}
	// Simulate a new process: no nonce/stop latch is restored, but durable state
	// still forbids resuming that old action through a newly issued conversation.
	b, err = newBrokerState(context.Background(), b.policy, b.journal, b.clock)
	if err != nil {
		t.Fatal("new process state")
	}
	b.readiness = ready
	b.readyHead = b.policy.Revision
	_, b.readyEntriesDigest, err = policyEntries(b.policy)
	if err != nil {
		t.Fatal("new process exact readiness")
	}
	two, err := b.reserve(context.Background())
	if err != nil {
		t.Fatal("fresh process timing")
	}
	if _, _, _, err := b.bindClaim(context.Background(), two, claim, 0); err == nil {
		t.Fatal("new nonce resumed an already admitted action")
	}
}

func TestPreclaimReturnedJournalRequiresActualExecutionContext(t *testing.T) {
	b, m := reservationFixture(t)
	r, err := b.reserve(context.Background())
	if err != nil {
		t.Fatal("reservation")
	}
	bound, execution, resolved, err := b.bindClaim(context.Background(), r, liveClaim(t, m), 0)
	if err != nil {
		t.Fatal("new claim")
	}
	if a, err := bound.Acquire(context.Background(), resolved.key); !errors.Is(err, actionjournal.ErrRecovery) {
		if a != nil {
			a.Close()
		}
		t.Fatal("broker returned raw unguarded journal binding")
	}
	snapshot, err := b.journal.AdmissionSnapshot(context.Background())
	if err != nil || len(snapshot.Records) != 0 {
		t.Fatal("missing execution context wrote or poisoned storage")
	}
	a, err := bound.Acquire(execution.Context(context.Background()), resolved.key)
	if err != nil {
		t.Fatal("actual matching execution context refused")
	}
	a.Close()
}
