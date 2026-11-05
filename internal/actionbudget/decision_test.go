package actionbudget

import (
	"testing"
	"time"
)

func TestDecisionTimingCannotReplaceOriginalExecutionOrSurviveSuspend(t *testing.T) {
	for _, mode := range []string{"valid", "five-seconds", "expired", "suspend", "foreign-clock"} {
		t.Run(mode, func(t *testing.T) {
			c, now, ready := clockFixture(t)
			conversation, _ := c.Begin(ready)
			now.goTime, now.wall, now.boot = now.goTime.Add(time.Second), now.wall.Add(time.Second), now.boot+time.Second
			execution, err := conversation.AcceptClaim(claimFixture(*now), time.Second, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			if err != nil {
				t.Fatal("execution timing fixture")
			}
			send, _ := c.Capture()
			elapsed := time.Second
			if mode == "five-seconds" {
				elapsed = 5 * time.Second
			}
			if mode == "expired" {
				elapsed = 61 * time.Second
			}
			now.goTime, now.wall, now.boot = now.goTime.Add(elapsed), now.wall.Add(elapsed), now.boot+elapsed
			if mode == "suspend" {
				now.goTime = now.goTime.Add(-elapsed)
			}
			recv, _ := c.Capture()
			if mode == "foreign-clock" {
				other, _, _ := clockFixture(t)
				recv, _ = other.Capture()
			}
			err = execution.CheckDecision(send, recv, send.wall.Add(elapsed/2))
			if (mode == "valid") != (err == nil) {
				t.Fatal("fresh authority timing guard bypassed")
			}
		})
	}
}
