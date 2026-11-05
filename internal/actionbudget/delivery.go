package actionbudget

import (
	"context"
	"time"
)

// DeliveryContext bounds only the original preclaim handoff, not execution or
// authority. Repeated calls retain the same T0; no caller duration is accepted.
func (c *Conversation) DeliveryContext(parent context.Context) (context.Context, context.CancelFunc, error) {
	if c.CheckDelivery(parent) != nil {
		return nil, nil, ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(parent, c.start.goTime.Add(30*time.Second))
	if ctx.Err() != nil {
		cancel()
		return nil, nil, ErrUnavailable
	}
	return ctx, cancel, nil
}

// CheckDelivery must surround the controlled send/receive, including verified
// empty polls. Awake timers do not replace this original BOOTTIME/continuity
// check after suspend. No context timing token or new permit is created.
func (c *Conversation) CheckDelivery(parent context.Context) error {
	if c == nil || parent == nil || parent.Err() != nil {
		return ErrUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if parent.Err() != nil || c.used || c.clock == nil || c.readiness == nil || c.readiness.clock != c.clock || c.start.clock != c.clock {
		return ErrUnavailable
	}
	now, err := c.clock.Capture()
	if err != nil || parent.Err() != nil || !continuous(c.readiness.origin, now) || !continuous(c.start, now) || now.boot-c.start.boot >= 30*time.Second {
		return ErrUnavailable
	}
	return nil
}
