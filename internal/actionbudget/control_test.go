package actionbudget

import (
	"testing"
	"time"
)

func TestControlReadinessUsesOneOriginalSuspendAwareFiveSecondBudget(t *testing.T) {
	for _, elapsed := range []time.Duration{4 * time.Second, 5 * time.Second, 5*time.Second + time.Nanosecond} {
		c, now, _ := clockFixture(t)
		send, _ := c.Capture()
		now.goTime, now.wall, now.boot = now.goTime.Add(elapsed), now.wall.Add(elapsed), now.boot+elapsed
		recv, _ := c.Capture()
		ready, err := c.ObserveControlTime(send.wall.Add(elapsed/2), send, recv)
		if (elapsed < 5*time.Second) != (err == nil && ready != nil) {
			t.Fatal("whole control deadline not enforced")
		}
	}
	c, now, _ := clockFixture(t)
	send, _ := c.Capture()
	now.wall = now.wall.Add(2 * time.Second)
	now.boot += 2 * time.Second // Suspended: Go clock did not advance.
	recv, _ := c.Capture()
	if _, err := c.ObserveControlTime(send.wall.Add(time.Second), send, recv); err == nil {
		t.Fatal("suspended control installed readiness")
	}
}
