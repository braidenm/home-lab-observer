package actionbudget

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

func clockFixture(t *testing.T) (*Clock, *reading, *Readiness) {
	t.Helper()
	base := time.Now()
	r := &reading{goTime: base, wall: base.UTC(), boot: 100 * time.Second}
	c := &Clock{read: func() (reading, error) { return *r, nil }}
	send, _ := c.Capture()
	r.goTime, r.wall, r.boot = r.goTime.Add(10*time.Millisecond), r.wall.Add(10*time.Millisecond), r.boot+10*time.Millisecond
	recv, _ := c.Capture()
	ready, err := c.ObserveServerTime(send.wall.Add(5*time.Millisecond), send, recv)
	if err != nil {
		t.Fatal("normal bounded server bracket")
	}
	return c, r, ready
}

func claimFixture(r reading) actionprotocol.Claim {
	created := r.wall.Add(-time.Second).Truncate(time.Microsecond)
	parameters := actionprotocol.Parameters{FormatVersion: 1, StopGraceSeconds: actionprotocol.Grace{Seconds: 30}}
	hash, _ := actionprotocol.ParameterDigest(actioncore.Restart, parameters)
	request := actionprotocol.RequestReceipt{
		ActionID: "hla_" + strings.Repeat("1", 32), ServerID: "srv_" + strings.Repeat("2", 32),
		OwnerUserID: "usr_" + strings.Repeat("3", 32), ActorUserID: "usr_" + strings.Repeat("3", 32),
		ConnectorInstanceID: "agent_" + strings.Repeat("4", 32), Operation: actioncore.Restart,
		TargetID: "hlt_" + strings.Repeat("5", 32), TargetKey: "ctr_0000000000000000",
		IdempotencyKeyHash: strings.Repeat("6", 64), PolicyRevision: "1", CapabilityRevision: "1",
		Parameters: parameters, ParameterSHA256: hash, RequestedAt: created.Format(actionprotocol.HTTPTimeLayout),
		ExpiresAt: created.Add(300 * time.Second).Format(actionprotocol.HTTPTimeLayout), CorrelationID: "synthetic-clock",
	}
	claimed := r.wall.Add(-time.Millisecond).Truncate(time.Microsecond)
	return actionprotocol.Claim{Request: request, ExecutionLeaseID: "hle_" + strings.Repeat("7", 32),
		ClaimCredentialID: "hlac_" + strings.Repeat("8", 32), ClaimCredentialRevision: "1",
		ClaimedAt: claimed.Format(actionprotocol.HTTPTimeLayout), LeaseExpiresAt: claimed.Add(120 * time.Second).Format(actionprotocol.HTTPTimeLayout), CorrelationID: "synthetic-claim"}
}

func TestExecutionBudgetAnchorsBeforeClaimAndCannotRestart(t *testing.T) {
	c, now, readiness := clockFixture(t)
	conversation, err := c.Begin(readiness)
	if err != nil {
		t.Fatal("fresh begin")
	}
	now.goTime, now.wall, now.boot = now.goTime.Add(time.Second), now.wall.Add(time.Second), now.boot+time.Second
	claim := claimFixture(*now)
	execution, err := conversation.AcceptClaim(claim, time.Second, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal("new claim accepted inside original delivery")
	}
	ctx := execution.Context(context.Background())
	if CheckMutation(ctx, strings.Repeat("a", 64), actioncore.Restart) != nil {
		t.Fatal("matching controlled write boundary")
	}
	if CheckMutation(ctx, strings.Repeat("b", 64), actioncore.Restart) == nil {
		t.Fatal("budget token moved to another private target")
	}
	if _, err := conversation.AcceptClaim(claim, time.Second, strings.Repeat("a", 64)); err == nil {
		t.Fatal("duplicate claim restarted budget")
	}
	// Receipt lower bound is T0+1s, so the active boundary is T0+61s,
	// not receipt+90 or a new allowance started by a transport retry.
	now.goTime, now.wall, now.boot = now.goTime.Add(60*time.Second), now.wall.Add(60*time.Second), now.boot+60*time.Second
	if CheckMutation(ctx, strings.Repeat("a", 64), actioncore.Restart) == nil {
		t.Fatal("original active equality deadline extended")
	}
}

func TestSuspendAndWallStepInvalidateBeforeControlledWrite(t *testing.T) {
	for _, suspend := range []bool{true, false} {
		c, now, readiness := clockFixture(t)
		conversation, _ := c.Begin(readiness)
		claim := claimFixture(*now)
		execution, err := conversation.AcceptClaim(claim, 0, strings.Repeat("a", 64))
		if err != nil {
			t.Fatal("claim")
		}
		if suspend {
			now.wall, now.boot = now.wall.Add(2*time.Second), now.boot+2*time.Second
		} else {
			now.wall = now.wall.Add(101 * time.Millisecond)
		}
		if CheckMutation(execution.Context(context.Background()), strings.Repeat("a", 64), actioncore.Restart) == nil {
			t.Fatal("clock discontinuity permitted a controlled write")
		}
	}
}

func TestCancellationDuringNativeSampleRefusesEveryControlledCheck(t *testing.T) {
	for _, stage := range []string{"execution", "mutation"} {
		t.Run(stage, func(t *testing.T) {
			clock, now, readiness := clockFixture(t)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			conversation, err := clock.Begin(readiness)
			if err != nil {
				t.Fatal("original execution conversation")
			}
			claim := claimFixture(*now)
			execution, err := conversation.AcceptClaim(claim, 0, strings.Repeat("a", 64))
			if err != nil {
				t.Fatal("valid original execution timing")
			}
			ctx := execution.Context(parent)
			digest, err := actionprotocol.ReceiptDigest(claim.Request)
			if err != nil {
				t.Fatal("actual receipt digest")
			}
			check := func() error { return CheckExecution(ctx, claim.Request.ActionID, digest) }
			if stage == "mutation" {
				check = func() error { return CheckMutation(ctx, strings.Repeat("a", 64), actioncore.Restart) }
			}
			if check() != nil {
				t.Fatal("positive control refused")
			}
			original := clock.read
			clock.read = func() (reading, error) { cancel(); return original() }
			if !errors.Is(check(), ErrUnavailable) {
				t.Fatal("cancellation during sample escaped controlled check")
			}
		})
	}
}

func TestAcceptClaimDeliveryBoundaryKeepsStricterClockBracket(t *testing.T) {
	// The full server bracket already refuses these near30s cases. They are
	// honest defense-in-depth coverage, not a claimed reachable equality hole
	// or proof that a full claim just before30s must be accepted.
	for _, elapsed := range []time.Duration{time.Second, 30*time.Second - time.Nanosecond, 30 * time.Second, 30*time.Second + time.Nanosecond} {
		t.Run(elapsed.String(), func(t *testing.T) {
			c, now, readiness := clockFixture(t)
			conversation, err := c.Begin(readiness)
			if err != nil {
				t.Fatal("original conversation")
			}
			now.goTime, now.wall, now.boot = now.goTime.Add(elapsed), now.wall.Add(elapsed), now.boot+elapsed
			_, err = conversation.AcceptClaim(claimFixture(*now), 0, strings.Repeat("a", 64))
			if (err == nil) != (elapsed == time.Second) {
				t.Fatal("ordinary positive or conservative delivery refusal changed")
			}
		})
	}
}
