package actionbudget

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

func TestRetainedReportUsesOriginalLeaseWithoutReauthorizingExecution(t *testing.T) {
	c, now, ready := clockFixture(t)
	conversation, err := c.Begin(ready)
	if err != nil {
		t.Fatal("begin")
	}
	claim := claimFixture(*now)
	e, err := conversation.AcceptClaim(claim, 0, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal("original execution")
	}
	original := e.guard.deadline
	ctx := e.Context(context.Background())
	now.goTime, now.wall, now.boot = now.goTime.Add(61*time.Second), now.wall.Add(61*time.Second), now.boot+61*time.Second
	if e.CheckReport(context.Background()) != nil {
		t.Fatal("retained report lost original lease reserve")
	}
	if e.guard.deadline != original || CheckMutation(ctx, strings.Repeat("a", 64), actioncore.Restart) == nil || CheckExecution(ctx, e.guard.actionID, e.guard.digest) == nil {
		t.Fatal("reporting extended a mutating/commit timing permission")
	}
	now.goTime, now.wall, now.boot = now.goTime.Add(58*time.Second), now.wall.Add(58*time.Second), now.boot+58*time.Second
	if e.CheckReport(context.Background()) == nil {
		t.Fatal("119s equality admitted")
	}
}

func TestReportReserveRefusesCancellationDiscontinuityAndEmptyExecution(t *testing.T) {
	for _, mode := range []string{"cancel", "suspend", "wall-rollback"} {
		t.Run(mode, func(t *testing.T) {
			c, now, ready := clockFixture(t)
			conversation, _ := c.Begin(ready)
			e, err := conversation.AcceptClaim(claimFixture(*now), 0, strings.Repeat("a", 64))
			if err != nil {
				t.Fatal("timing fixture")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "cancel":
				cancel()
			case "suspend":
				now.wall = now.wall.Add(2 * time.Second)
				now.boot += 2 * time.Second
			case "wall-rollback":
				now.wall = now.wall.Add(-time.Second)
			}
			if e.CheckReport(ctx) == nil {
				t.Fatal("invalid report timing accepted")
			}
		})
	}
	if (*Execution)(nil).CheckReport(context.Background()) == nil || (&Execution{}).CheckReport(context.Background()) == nil {
		t.Fatal("empty execution created a reporting allowance")
	}
}

func TestReportReserveKeepsOriginalDelayedReadinessInterval(t *testing.T) {
	c, now, _ := clockFixture(t)
	send, _ := c.Capture()
	now.goTime, now.wall, now.boot = now.goTime.Add(4*time.Second), now.wall.Add(4*time.Second), now.boot+4*time.Second
	recv, _ := c.Capture()
	ready, err := c.ObserveControlTime(send.wall.Add(2*time.Second), send, recv)
	if err != nil {
		t.Fatal("valid bounded startup sample")
	}
	now.goTime, now.wall, now.boot = now.goTime.Add(14*time.Second), now.wall.Add(14*time.Second), now.boot+14*time.Second
	conversation, err := c.Begin(ready)
	if err != nil {
		t.Fatal("valid delayed readiness")
	}
	e, err := conversation.AcceptClaim(claimFixture(*now), 0, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal("new original execution")
	}
	ctx := e.Context(context.Background())
	now.goTime, now.wall, now.boot = now.goTime.Add(118*time.Second), now.wall.Add(118*time.Second), now.boot+118*time.Second
	if e.CheckReport(context.Background()) != nil {
		t.Fatal("136s original interval stole valid original119s report window")
	}
	if CheckExecution(ctx, e.guard.actionID, e.guard.digest) == nil || CheckMutation(ctx, strings.Repeat("a", 64), actioncore.Restart) == nil {
		t.Fatal("report interval extended active execution")
	}
	now.goTime, now.wall, now.boot = now.goTime.Add(time.Second), now.wall.Add(time.Second), now.boot+time.Second
	if e.CheckReport(context.Background()) == nil {
		t.Fatal("119s equality accepted after delayed readiness")
	}
}

func TestReportReserveObservesCancellationDuringNativeCapture(t *testing.T) {
	c, now, ready := clockFixture(t)
	conversation, _ := c.Begin(ready)
	e, err := conversation.AcceptClaim(claimFixture(*now), 0, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal("original timing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.read = func() (reading, error) { cancel(); return *now, nil }
	if e.CheckReport(ctx) == nil {
		t.Fatal("cancellation during capture was lost")
	}
}
