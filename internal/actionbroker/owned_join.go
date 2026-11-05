package actionbroker

import (
	"context"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
)

func allOwnedDone(signals []<-chan struct{}) bool {
	for _, done := range signals {
		if done == nil {
			return false
		}
		select {
		case <-done:
		default:
			return false
		}
	}
	return true
}

// At most3 workers and one accept loop. No waiter goroutine exists:
// the owning controller waits on completion or ONE original cleanup deadline.
// joined reports actual completion only; a late/error result never proves timely
// shutdown or grants effect permission, even if cleanup may now close.
func waitOwnedJoin(parent context.Context, clock *actionbudget.Clock, original actionbudget.Stamp, signals []<-chan struct{}) (joined bool, result error) {
	return waitOwnedJoinClock(parent, clock, original, signals)
}

// The production wrapper above accepts only the concrete native Clock. This
// private timing-only seam permits deterministic paused-Go-timer regressions.
type ownedJoinClock interface {
	QuiescenceContext(context.Context, actionbudget.Stamp) (context.Context, context.CancelFunc, error)
	CheckQuiescence(context.Context, actionbudget.Stamp) error
}

func waitOwnedJoinClock(parent context.Context, clock ownedJoinClock, original actionbudget.Stamp, signals []<-chan struct{}) (joined bool, result error) {
	if len(signals) > 4 {
		return false, errOwnedWorkers
	}
	for _, done := range signals {
		if done == nil {
			return false, errOwnedWorkers
		}
	}
	ctx, cancel, err := clock.QuiescenceContext(parent, original)
	if err != nil {
		return allOwnedDone(signals), errOwnedWorkers
	}
	defer cancel()
	poll := time.NewTicker(25 * time.Millisecond) // Closed cadence; no caller knob.
	defer poll.Stop()
	for _, done := range signals {
		waiting := true
		for waiting {
			if clock.CheckQuiescence(ctx, original) != nil {
				return allOwnedDone(signals), errOwnedWorkers
			}
			select {
			case <-done:
				waiting = false
			case <-ctx.Done():
				return allOwnedDone(signals), errOwnedWorkers
			case <-poll.C: // Recheck the SAME native75s stamp after resume.
			}
		}
	}
	if clock.CheckQuiescence(ctx, original) != nil {
		return true, errOwnedWorkers
	}
	return true, nil
}
