package actionbudget

import (
	"context"
	"time"
)

// CheckReport bounds only the read-only retained-result handoff. The owning
// consumer must compare the immutable action/full receipt with actual joined
// journal evidence. This method creates NO context/token and changes neither
// the execution deadline nor Engine/journal permission. Use the original parent
// cancellation scope, not the core's completed/expired execution context.
func (e *Execution) CheckReport(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || e == nil || e.guard == nil || e.guard.kind != executionPhase || !actionID(e.guard.actionID) ||
		e.start.clock != e.guard.clock || !continuous(e.guard.origin, e.start) {
		return ErrUnavailable
	}
	now, err := e.guard.clock.Capture()
	if err != nil || ctx.Err() != nil || !reportContinuous(e.guard.origin, now) || now.boot < e.start.boot || now.boot-e.start.boot >= 119*time.Second {
		return ErrUnavailable
	}
	return nil
}
