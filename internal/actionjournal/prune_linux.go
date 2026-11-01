//go:build linux

package actionjournal

import (
	"context"
	"time"
)

func (j *Journal) Prune(ctx context.Context) error {
	if !j.mu.TryLock() {
		return ErrBusy
	}
	defer j.mu.Unlock()
	if !j.usable() {
		return ErrRecovery
	}
	bounded, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	return j.transaction(bounded, func() error {
		floor, e := j.floor(bounded)
		if e != nil {
			return e
		}
		now := j.now().UTC()
		cutoff := now.Add(-30 * 24 * time.Hour)
		persistedFloor := cutoff
		if persistedFloor.Before(floor) {
			persistedFloor = floor
		}
		records, e := j.records(bounded)
		if e != nil {
			return e
		}
		changed := 0
		for _, r := range records {
			if changed == 50 {
				break
			}
			if _, live := j.active[r.key.ActionID]; live {
				continue
			}
			if r.intent.IsZero() && r.terminal.IsZero() && !now.Before(r.key.Metadata.Request.ExpiresAt) {
				if _, e = j.conn.ExecContext(bounded, `INSERT INTO events VALUES(?,2,?,'DENIED','EXPIRED_UNDISPATCHED')`, r.key.ActionID, stamp(now)); e != nil {
					return ErrRecovery
				}
				changed++
			} else if !r.terminal.IsZero() && !r.terminal.After(cutoff) && !r.key.Metadata.Request.ExpiresAt.After(cutoff) {
				if _, e = j.conn.ExecContext(bounded, `DELETE FROM events WHERE action_id=?`, r.key.ActionID); e != nil {
					return ErrRecovery
				}
				if _, e = j.conn.ExecContext(bounded, `DELETE FROM actions WHERE action_id=?`, r.key.ActionID); e != nil {
					return ErrRecovery
				}
				changed++
			}
		}
		if _, e = j.conn.ExecContext(bounded, `UPDATE metadata SET pruned_expiry_through=? WHERE singleton=1`, stamp(persistedFloor)); e != nil {
			return ErrRecovery
		}
		return nil
	})
}
