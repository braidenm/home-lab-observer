package actionbudget

import (
	"context"
	"time"
)

// CheckDecision is timing-only, AFTER the caller validates the full immutable
// receipt, lease, checkpoint, fresh challenge and PERMITTED decision. It cannot
// authenticate the bridge or replace either genuine remote authority check.
func (e *Execution) CheckDecision(send, recv Stamp, checkedAt time.Time) error {
	if e == nil || e.guard == nil || e.guard.kind != executionPhase || send.clock != e.guard.clock || recv.clock != e.guard.clock ||
		!continuous(e.guard.origin, send) || !continuous(e.guard.origin, recv) || !serverBracket(checkedAt, send, recv) ||
		recv.boot-send.boot >= 5*time.Second {
		return ErrUnavailable
	}
	return CheckExecution(e.Context(context.Background()), e.guard.actionID, e.guard.digest)
}
