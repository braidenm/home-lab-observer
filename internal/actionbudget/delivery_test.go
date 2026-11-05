package actionbudget

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDeliveryContextKeepsOriginalT0AndCreatesNoExecutionPermit(t *testing.T) {
	c, now, ready := clockFixture(t)
	conversation, err := c.Begin(ready)
	if err != nil {
		t.Fatal("begin")
	}
	want := conversation.start.goTime.Add(30 * time.Second)
	now.goTime, now.wall, now.boot = now.goTime.Add(3*time.Second), now.wall.Add(3*time.Second), now.boot+3*time.Second
	ctx, cancel, err := conversation.DeliveryContext(context.Background())
	if err != nil {
		t.Fatal("delivery context")
	}
	defer cancel()
	if got, ok := ctx.Deadline(); !ok || !got.Equal(want) {
		t.Fatal("delivery restarted at receive")
	}
	if _, err := checked(ctx, executionPhase); err == nil {
		t.Fatal("delivery context created execution permission")
	}
	now.goTime, now.wall, now.boot = now.goTime.Add(27*time.Second-time.Nanosecond), now.wall.Add(27*time.Second-time.Nanosecond), now.boot+27*time.Second-time.Nanosecond
	if conversation.CheckDelivery(ctx) != nil {
		t.Fatal("original horizon shortened unexpectedly")
	}
	now.goTime, now.wall, now.boot = now.goTime.Add(time.Nanosecond), now.wall.Add(time.Nanosecond), now.boot+time.Nanosecond
	if conversation.CheckDelivery(ctx) == nil {
		t.Fatal("delivery equality permitted")
	}
	if _, cancel, err := conversation.DeliveryContext(context.Background()); err == nil {
		cancel()
		t.Fatal("expired context restarted")
	}
}

func TestDeliveryChecksRejectDiscontinuityConsumedAndCancellationDuringCapture(t *testing.T) {
	for _, mode := range []string{"suspend", "wall-step", "consumed", "cancel-during-capture"} {
		t.Run(mode, func(t *testing.T) {
			c, now, ready := clockFixture(t)
			conversation, err := c.Begin(ready)
			if err != nil {
				t.Fatal("begin")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "suspend":
				now.boot += time.Second
				now.wall = now.wall.Add(time.Second)
			case "wall-step":
				now.wall = now.wall.Add(101 * time.Millisecond)
			case "consumed":
				if _, err := conversation.AcceptClaim(claimFixture(*now), 0, strings.Repeat("a", 64)); err != nil {
					t.Fatal("claim")
				}
			case "cancel-during-capture":
				c.read = func() (reading, error) { cancel(); return *now, nil }
			}
			if conversation.CheckDelivery(ctx) == nil {
				t.Fatal("unsafe delivery admitted")
			}
		})
	}
}
