//go:build linux

package actionjournal

import (
	"bytes"
	"context"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

func (a *attempt) Existing() actioncore.Code {
	j := a.j
	j.mu.Lock()
	defer j.mu.Unlock()
	if a.closed || !j.usable() {
		return actioncore.Unavailable
	}
	if !a.r.terminal.IsZero() {
		return a.r.code
	}
	if !a.r.intent.IsZero() {
		return actioncore.OutcomeUnknown
	}
	return ""
}
func (a *attempt) Intent(ctx context.Context) error { return a.transition(ctx, "", true) }
func (a *attempt) Finish(ctx context.Context, code actioncore.Code) error {
	return a.transition(ctx, code, false)
}
func (a *attempt) transition(ctx context.Context, code actioncore.Code, intent bool) error {
	j := a.j
	if !j.mu.TryLock() {
		return ErrBusy
	}
	defer j.mu.Unlock()
	if !j.usable() || !a.mutable || a.closed || j.active[a.r.key.ActionID] != a || !a.r.terminal.IsZero() || (intent && !a.r.intent.IsZero()) || (!intent && !terminalAllowed(a.r.key.Metadata.Request.Operation, !a.r.intent.IsZero(), code, "")) {
		return ErrRecovery
	}
	bounded, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	now := j.now().UTC()
	e := j.transaction(bounded, func() error {
		actual, e := j.read(bounded, a.r.key.ActionID)
		if e != nil || !bytes.Equal(actual.raw, a.r.raw) || stamp(actual.admitted) != stamp(a.r.admitted) || stamp(actual.intent) != stamp(a.r.intent) || !actual.terminal.IsZero() || now.Before(actual.admitted) || (!actual.intent.IsZero() && now.Before(actual.intent)) {
			return ErrRecovery
		}
		ordinal := 2
		stored := string(code)
		if intent {
			if !now.Before(a.r.key.Metadata.Request.ExpiresAt) {
				return ErrRecovery
			}
			ordinal = 1
			stored = "INTENT"
		}
		if _, e = j.conn.ExecContext(bounded, `INSERT INTO events VALUES(?,?,?,?,'')`, a.r.key.ActionID, ordinal, stamp(now), stored); e != nil {
			return ErrRecovery
		}
		return nil
	})
	if e != nil {
		return e
	}
	if intent {
		a.r.intent = now
	} else {
		a.r.terminal = now
		a.r.code = code
	}
	return nil
}
func (a *attempt) Close() {
	j := a.j
	j.mu.Lock()
	defer j.mu.Unlock()
	if a.closed {
		return
	}
	a.closed = true
	if j.active[a.r.key.ActionID] == a {
		delete(j.active, a.r.key.ActionID)
	}
}
