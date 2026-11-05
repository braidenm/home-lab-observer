package actionbudget

import (
	"context"
	"testing"
	"time"
)

func TestQuiescenceKeepsOneOriginal75sAndCreatesNoEffectPermission(t *testing.T) {
	c, now, _ := clockFixture(t)
	start, _ := c.Capture()
	now.goTime, now.wall, now.boot = now.goTime.Add(5*time.Second), now.wall.Add(5*time.Second), now.boot+5*time.Second
	ctx, cancel, err := c.QuiescenceContext(context.Background(), start)
	if err != nil {
		t.Fatal("join budget")
	}
	defer cancel()
	if d, ok := ctx.Deadline(); !ok || !d.Equal(start.goTime.Add(75*time.Second)) {
		t.Fatal("join timer restarted")
	}
	if _, err := checked(ctx, executionPhase); err == nil {
		t.Fatal("cleanup created execution permission")
	}
	now.goTime, now.wall, now.boot = now.goTime.Add(70*time.Second-time.Nanosecond), now.wall.Add(70*time.Second-time.Nanosecond), now.boot+70*time.Second-time.Nanosecond
	if c.CheckQuiescence(ctx, start) != nil {
		t.Fatal("shared original budget refused early")
	}
	now.goTime, now.wall, now.boot = now.goTime.Add(time.Nanosecond), now.wall.Add(time.Nanosecond), now.boot+time.Nanosecond
	if c.CheckQuiescence(ctx, start) == nil {
		t.Fatal("join equality permitted")
	}
	if _, cancel, err := c.QuiescenceContext(context.Background(), start); err == nil {
		cancel()
		t.Fatal("per-handler timer restarted")
	}
}

func TestQuiescenceHonorsShorterParentBudgetAndCaptureCancellation(t *testing.T) {
	c, now, _ := clockFixture(t)
	start, _ := c.Capture()
	root, cancel := context.WithDeadline(context.Background(), start.goTime.Add(90*time.Second))
	defer cancel()
	now.goTime, now.wall, now.boot = now.goTime.Add(20*time.Second), now.wall.Add(20*time.Second), now.boot+20*time.Second
	stop, _ := c.Capture()
	join, closeJoin, err := c.QuiescenceContext(root, stop)
	if err != nil {
		t.Fatal("join")
	}
	defer closeJoin()
	want, _ := root.Deadline()
	got, _ := join.Deadline()
	if !got.Equal(want) {
		t.Fatal("cleanup extended root90 deadline")
	}
	// Fake native clocks advance together while the real Go timer remains live.
	// Root90 is earlier than stop75 (T20+75=T95), so it must refuse atT90.
	now.goTime, now.wall, now.boot = now.goTime.Add(70*time.Second), now.wall.Add(70*time.Second), now.boot+70*time.Second
	if join.Err() != nil {
		t.Fatal("fixture accidentally used the Go timer")
	}
	if c.CheckQuiescence(join, stop) == nil {
		t.Fatal("native root90 limit lost beneath stop75")
	}
	ctx, closeCtx := context.WithCancel(context.Background())
	defer closeCtx()
	c.read = func() (reading, error) { closeCtx(); return *now, nil }
	if c.CheckQuiescence(ctx, stop) == nil {
		t.Fatal("cancel during native sample ignored")
	}
}
