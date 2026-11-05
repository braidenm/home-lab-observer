// Package actionbudget binds only local timing/correlation for the paired action
// profile. Tokens are NOT remote authentication, owner permission or local policy.
package actionbudget

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

var ErrUnavailable = errors.New("action_budget_unavailable")

const continuity = 100 * time.Millisecond
const skew = 4900 * time.Millisecond

type reading struct {
	goTime time.Time // Original time.Now value, retaining its monotonic component.
	wall   time.Time // UTC projection; never used as a substitute journal clock.
	boot   time.Duration
}

type Clock struct{ read func() (reading, error) }

type Stamp struct {
	clock *Clock
	reading
}

type Readiness struct {
	clock  *Clock
	origin Stamp
	recv   Stamp
}

type Conversation struct {
	mu        sync.Mutex
	clock     *Clock
	readiness *Readiness
	start     Stamp
	used      bool
}

type phase uint8

const (
	executionPhase phase = iota + 1
)

type budget struct {
	clock       *Clock
	origin      Stamp
	deadline    time.Duration
	kind        phase
	actionID    string
	digest      [32]byte
	containerID string
	operation   actioncore.Operation
}

type Execution struct {
	guard    *budget
	delivery actioncore.Delivery
	start    Stamp // Original native pre-claim T0; never serialized or restarted.
}

type contextKey struct{}

func NewClock() (*Clock, error) {
	if _, err := nativeReading(); err != nil {
		return nil, ErrUnavailable
	}
	return &Clock{read: nativeReading}, nil
}

func (c *Clock) Capture() (Stamp, error) {
	if c == nil || c.read == nil {
		return Stamp{}, ErrUnavailable
	}
	r, err := c.read()
	if err != nil || r.goTime.IsZero() || r.wall.IsZero() || r.boot < 0 || r.boot > time.Duration(1<<63-1)-134*time.Second {
		return Stamp{}, ErrUnavailable
	}
	return Stamp{clock: c, reading: r}, nil
}

func continuous(from, to Stamp) bool {
	return continuousWithin(from, to, 134*time.Second)
}

// Only the reporting window has this longer whole-origin interval. Call sites
// remain closed: no public duration/flag or restarted origin is accepted.
func reportContinuous(from, to Stamp) bool {
	return to.boot-from.boot < 5*time.Second+15*time.Second+119*time.Second && continuousWithin(from, to, 139*time.Second)
}

func continuousWithin(from, to Stamp, maximum time.Duration) bool {
	if from.clock == nil || from.clock != to.clock || to.boot < from.boot || to.wall.Before(from.wall) {
		return false
	}
	boot := to.boot - from.boot
	if boot > maximum {
		return false
	}
	mono := to.goTime.Sub(from.goTime)
	wall := to.wall.Sub(from.wall)
	return mono >= 0 && mono >= boot-continuity && mono <= boot+continuity && wall >= boot-continuity && wall <= boot+continuity
}

func serverBracket(server time.Time, send, recv Stamp) bool {
	return continuous(send, recv) && !server.IsZero() && server.Sub(recv.wall) >= -skew && server.Sub(send.wall) <= skew
}

// ObserveServerTime consumes a timestamp already authenticated by the trusted
// worker bridge. Its whole conservative bracket must fit; no offset is applied.
func (c *Clock) ObserveServerTime(server time.Time, send, recv Stamp) (*Readiness, error) {
	if send.clock != c || recv.clock != c || !serverBracket(server, send, recv) {
		return nil, ErrUnavailable
	}
	return &Readiness{clock: c, origin: send, recv: recv}, nil
}

func (c *Clock) Begin(r *Readiness) (*Conversation, error) {
	now, err := c.Capture()
	if err != nil || r == nil || r.clock != c || !continuous(r.origin, now) || now.boot-r.recv.boot > 15*time.Second || now.boot < r.recv.boot {
		return nil, ErrUnavailable
	}
	return &Conversation{clock: c, readiness: r, start: now}, nil
}

// AcceptClaim may be called once for this pre-HTTP conversation. The claim must
// be genuinely new; source freshness/remote permission are the broker's separate
// responsibility. Neither a failed attempt nor retry restarts these local clocks.
func (c *Conversation) AcceptClaim(claim actionprotocol.Claim, rtt time.Duration, containerID string) (*Execution, error) {
	if c == nil {
		return nil, ErrUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.used {
		return nil, ErrUnavailable
	}
	c.used = true
	now, err := c.clock.Capture()
	raw, encodingError := json.Marshal(claim)
	validated, validationError := actionprotocol.DecodeClaim(raw)
	claimed, timeError := actionprotocol.ParseHTTPTime(claim.ClaimedAt)
	if err != nil || encodingError != nil || validationError != nil || validated != claim || timeError != nil ||
		!continuous(c.readiness.origin, now) || now.boot-c.start.boot >= 30*time.Second || now.boot < c.start.boot ||
		rtt < 0 || rtt > 30*time.Second || rtt > now.boot-c.start.boot || !serverBracket(claimed, c.start, now) || !fullID(containerID) {
		return nil, ErrUnavailable
	}
	digest, err := actionprotocol.ReceiptDigest(claim.Request)
	if err != nil {
		return nil, ErrUnavailable
	}
	end := c.start.boot + min(89*time.Second, rtt+60*time.Second)
	if now.boot >= end {
		return nil, ErrUnavailable
	}
	return &Execution{
		guard: &budget{clock: c.clock, origin: c.readiness.origin, deadline: end, kind: executionPhase,
			actionID: claim.Request.ActionID, digest: digest, containerID: containerID, operation: claim.Request.Operation},
		delivery: actioncore.Delivery{StartedAt: c.start.goTime, ReceivedAt: c.start.goTime.Add(rtt), LeaseDeadline: c.start.goTime.Add(119 * time.Second)},
		start:    c.start,
	}, nil
}

func fullID(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func actionID(s string) bool {
	if len(s) != 36 || s[:4] != "hla_" {
		return false
	}
	for _, c := range s[4:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (e *Execution) Context(parent context.Context) context.Context {
	if parent == nil || e == nil || e.guard == nil {
		return nil
	}
	return context.WithValue(parent, contextKey{}, e.guard)
}

func (e *Execution) Delivery() actioncore.Delivery {
	if e == nil {
		return actioncore.Delivery{}
	}
	return e.delivery
}

func checked(ctx context.Context, kind phase) (*budget, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	b, ok := ctx.Value(contextKey{}).(*budget)
	if !ok || b == nil || b.kind != kind {
		return nil, ErrUnavailable
	}
	now, err := b.clock.Capture()
	if err != nil || ctx.Err() != nil || !continuous(b.origin, now) || now.boot >= b.deadline || now.boot-b.origin.boot > 134*time.Second {
		return nil, ErrUnavailable
	}
	return b, nil
}

func CheckExecution(ctx context.Context, actionID string, digest [32]byte) error {
	b, err := checked(ctx, executionPhase)
	if err != nil || b.actionID != actionID || b.digest != digest {
		return ErrUnavailable
	}
	return nil
}

// CheckMutation is rechecked at the actual transport Write, before AND after.
// It cannot make a check+syscall atomic or prevent later daemon processing.
func CheckMutation(ctx context.Context, containerID string, operation actioncore.Operation) error {
	b, err := checked(ctx, executionPhase)
	if err != nil || b.containerID != containerID || b.operation != operation {
		return ErrUnavailable
	}
	return nil
}
