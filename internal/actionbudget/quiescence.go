package actionbudget

import (
	"context"
	"time"
)

// QuiescenceContext bounds cancel/join cleanup, never an effect or query permit.
// One original stamp is shared by the entire cohort, not restarted per handler.
func (c *Clock) QuiescenceContext(parent context.Context, original Stamp) (context.Context, context.CancelFunc, error) {
	if c.CheckQuiescence(parent, original) != nil {
		return nil, nil, ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(parent, original.goTime.Add(75*time.Second))
	if ctx.Err() != nil {
		cancel()
		return nil, nil, ErrUnavailable
	}
	return ctx, cancel, nil
}

func (c *Clock) CheckQuiescence(parent context.Context, original Stamp) error {
	if c == nil || parent == nil || parent.Err() != nil || original.clock != c {
		return ErrUnavailable
	}
	now, err := c.Capture()
	// Parent/root deadlines also retain a LOCAL Go-monotonic anchor. Translate
	// that earlier limit against this ORIGINAL local stamp, never server time or
	// a new receipt. A paused Go timer must not hide native root90 expiration.
	limit := 75 * time.Second
	if deadline, ok := parent.Deadline(); ok {
		limit = min(limit, deadline.Sub(original.goTime))
	}
	if err != nil || parent.Err() != nil || !continuous(original, now) || limit <= 0 || now.boot-original.boot >= limit {
		return ErrUnavailable
	}
	return nil
}
