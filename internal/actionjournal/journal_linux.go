//go:build linux

package actionjournal

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	_ "modernc.org/sqlite"
)

const operationTimeout = 2 * time.Second

type Journal struct {
	mu      sync.Mutex
	dir     string
	binding Binding
	lock    *os.File
	db      *sql.DB
	conn    *sql.Conn
	failed  bool
	active  map[string]*attempt
	now     func() time.Time
	// Private fault/peak seams exercise the actual transaction, never credentials.
	beforeCommit, afterCommit func() error
}
type record struct {
	key                        actioncore.Key
	raw                        []byte
	admitted, intent, terminal time.Time
	code                       actioncore.Code
	reason                     string
}
type attempt struct {
	j       *Journal
	r       record
	closed  bool
	mutable bool
}

func Provision(ctx context.Context, dir string, b Binding) (*Journal, error) {
	return open(ctx, dir, b, true)
}
func OpenExisting(ctx context.Context, dir string, b Binding) (*Journal, error) {
	return open(ctx, dir, b, false)
}
func validID(s, prefix string) bool {
	if len(s) != len(prefix)+32 || !strings.HasPrefix(s, prefix) {
		return false
	}
	for _, c := range s[len(prefix):] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func validBinding(b Binding) bool {
	return validID(b.ServerID, "srv_") && validID(b.InstallationID, "agent_") && validID(b.OwnerID, "usr_")
}
func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func parseTime(s string) (time.Time, error) {
	t, e := time.Parse(time.RFC3339Nano, s)
	if e != nil || len(s) > 30 || stamp(t) != s {
		return time.Time{}, ErrRecovery
	}
	return t, nil
}
func dataSource(dir string) string {
	u := url.URL{Scheme: "file", Path: filepath.Join(dir, databaseName)}
	q := url.Values{"mode": {"rw"}, "cache": {"private"}, "_defensive": {"true"}}
	for _, p := range []string{"busy_timeout(250)", "synchronous(EXTRA)", "cache_size(3072)", "cache_spill(OFF)", "temp_store(MEMORY)", "trusted_schema(OFF)", "foreign_keys(ON)", "mmap_size(0)"} {
		q.Add("_pragma", p)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func open(ctx context.Context, dir string, b Binding, create bool) (_ *Journal, result error) {
	if ctx.Err() != nil || !validBinding(b) {
		return nil, ErrRecovery
	}
	dir, e := privateDirectory(dir)
	if e != nil {
		return nil, e
	}
	lock, e := acquireLock(dir, create)
	if e != nil {
		return nil, e
	}
	j := &Journal{dir: dir, binding: b, lock: lock, active: map[string]*attempt{}, now: time.Now}
	defer func() {
		if result != nil {
			j.Close()
		}
	}()
	if create {
		f, e := os.Open(dir)
		if e != nil {
			return nil, ErrUnsafe
		}
		entries, e := f.ReadDir(2)
		f.Close()
		if e != nil || len(entries) != 1 || entries[0].Name() != lockName || createDatabase(dir) != nil {
			return nil, ErrRecovery
		}
	}
	if e := checkFiles(dir, true); e != nil {
		return nil, e
	}
	if !create && checkHeader(dir) != nil {
		return nil, ErrRecovery
	}
	if !create && checkJournalHeader(dir) != nil {
		return nil, ErrRecovery
	}
	j.db, e = sql.Open("sqlite", dataSource(dir))
	if e != nil {
		return nil, ErrRecovery
	}
	j.db.SetMaxOpenConns(1)
	j.db.SetMaxIdleConns(1)
	bounded, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	j.conn, e = j.db.Conn(bounded)
	if e != nil {
		return nil, ErrRecovery
	}
	if create {
		if _, e = j.conn.ExecContext(bounded, `PRAGMA page_size=4096`); e != nil {
			return nil, ErrRecovery
		}
	}
	if e = j.configure(bounded); e != nil {
		return nil, e
	}
	if create {
		e = j.transaction(bounded, func() error {
			for _, name := range []string{"metadata", "actions", "events", "actions_target", "actions_physical"} {
				if _, e := j.conn.ExecContext(bounded, definitions[name]); e != nil {
					return ErrRecovery
				}
			}
			if _, e := j.conn.ExecContext(bounded, `PRAGMA application_id=1212957002; PRAGMA user_version=1`); e != nil {
				return ErrRecovery
			}
			_, e := j.conn.ExecContext(bounded, `INSERT INTO metadata VALUES(1,?,?,?,?)`, b.ServerID, b.InstallationID, b.OwnerID, stamp(time.Time{}))
			if e != nil {
				return ErrRecovery
			}
			return nil
		})
		if e != nil || syncDirectory(dir) != nil {
			return nil, ErrRecovery
		}
	}
	if e = j.verify(bounded); e != nil {
		return nil, e
	}
	return j, nil
}
func (j *Journal) transaction(ctx context.Context, body func() error) error {
	if _, e := j.conn.ExecContext(ctx, `BEGIN IMMEDIATE`); e != nil {
		j.failed = true
		return ErrRecovery
	}
	err := body()
	if err == nil && j.beforeCommit != nil {
		err = j.beforeCommit()
	}
	if err == nil {
		if _, e := j.conn.ExecContext(ctx, `COMMIT`); e == nil {
			if j.afterCommit == nil || j.afterCommit() == nil {
				return nil
			}
		}
		j.failed = true
		err = ErrRecovery
	}
	if !errors.Is(err, ErrBusy) && !errors.Is(err, ErrFull) && !errors.Is(err, actioncore.ErrIdentityConflict) && !errors.Is(err, actioncore.ErrUnresolvedTarget) {
		j.failed = true
		err = ErrRecovery
	}
	cleanup, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	if _, e := j.conn.ExecContext(cleanup, `ROLLBACK`); e != nil {
		j.failed = true
		return ErrRecovery
	}
	return err
}
func (j *Journal) usable() bool { return !j.failed && j.conn != nil }
func (j *Journal) Acquire(ctx context.Context, k actioncore.Key) (actioncore.Attempt, error) {
	raw, e := actioncore.CanonicalJournalMetadata(k)
	req := k.Metadata.Request
	if e != nil || req.ServerID != j.binding.ServerID || req.InstallationID != j.binding.InstallationID || req.OwnerID != j.binding.OwnerID {
		return nil, actioncore.ErrIdentityConflict
	}
	if !j.mu.TryLock() {
		return nil, ErrBusy
	}
	defer j.mu.Unlock()
	if !j.usable() {
		return nil, ErrRecovery
	}
	bounded, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	var result *attempt
	e = j.transaction(bounded, func() error {
		r, e := j.read(bounded, k.ActionID)
		exists := e == nil
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if exists {
			if !bytes.Equal(raw, r.raw) {
				return actioncore.ErrIdentityConflict
			}
			if !r.terminal.IsZero() || !r.intent.IsZero() {
				result = &attempt{j: j, r: r}
				return nil
			}
		}
		if _, ok := j.active[k.ActionID]; ok {
			return ErrBusy
		}
		now := j.now().UTC()
		if now.Before(req.CreatedAt) {
			return actioncore.ErrIdentityConflict
		}
		if !now.Before(req.ExpiresAt) {
			if exists {
				if _, e := j.conn.ExecContext(bounded, `INSERT INTO events VALUES(?,2,?,'DENIED','EXPIRED_UNDISPATCHED')`, k.ActionID, stamp(now)); e != nil {
					return ErrRecovery
				}
				r.terminal = now
				r.code = actioncore.Denied
				r.reason = "EXPIRED_UNDISPATCHED"
				result = &attempt{j: j, r: r}
				return nil
			}
			return actioncore.ErrIdentityConflict
		}
		floor, e := j.floor(bounded)
		if e != nil {
			return e
		}
		if !exists && !req.ExpiresAt.After(floor) {
			return actioncore.ErrIdentityConflict
		}
		related, e := j.ids(bounded, `SELECT action_id FROM actions WHERE target_id=? OR (engine_generation=? AND container_id=?)`, k.TargetID, k.Metadata.EngineGeneration, k.Metadata.ContainerID)
		if e != nil {
			return e
		}
		for _, id := range related {
			other, e := j.read(bounded, id)
			if e != nil {
				return e
			}
			if !other.intent.IsZero() && other.terminal.IsZero() {
				return actioncore.ErrUnresolvedTarget
			}
			if other.code == actioncore.Started || other.code == actioncore.Stopped || other.code == actioncore.Restarted {
				if now.Before(other.terminal.Add(30 * time.Second)) {
					return ErrBusy
				}
			}
		}
		permits := 0
		for _, a := range j.active {
			if a.r.terminal.IsZero() && a.r.intent.IsZero() {
				permits++
			}
			if a.r.key.TargetID == k.TargetID || (a.r.key.Metadata.EngineGeneration == k.Metadata.EngineGeneration && a.r.key.Metadata.ContainerID == k.Metadata.ContainerID) {
				return ErrBusy
			}
		}
		var unknown int
		if j.conn.QueryRowContext(bounded, `SELECT count(*) FROM events i WHERE i.ordinal=1 AND NOT EXISTS(SELECT 1 FROM events t WHERE t.action_id=i.action_id AND t.ordinal=2)`).Scan(&unknown) != nil || unknown > 2 {
			return ErrRecovery
		}
		if permits+unknown >= 2 {
			return ErrBusy
		}
		if !exists {
			var count int
			if j.conn.QueryRowContext(bounded, `SELECT count(*) FROM actions`).Scan(&count) != nil || count > maxRecords {
				return ErrRecovery
			}
			if count == maxRecords {
				return ErrFull
			}
			_, e = j.conn.ExecContext(bounded, `INSERT INTO actions VALUES(?,?,?,?,?,?,?)`, k.ActionID, k.TargetID, k.Metadata.EngineGeneration, k.Metadata.ContainerID, raw, k.Fingerprint[:], stamp(now))
			if e != nil {
				return ErrRecovery
			}
			r = record{key: k, raw: raw, admitted: now}
		}
		result = &attempt{j: j, r: r, mutable: true}
		return nil
	})
	if e != nil {
		return nil, e
	}
	if result.mutable {
		j.active[k.ActionID] = result
	}
	return result, nil
}
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.failed = true
	for _, a := range j.active {
		a.closed = true
	}
	clear(j.active)
	failed := false
	if j.conn != nil {
		failed = j.conn.Close() != nil
		j.conn = nil
	}
	if j.db != nil {
		failed = j.db.Close() != nil || failed
		j.db = nil
	}
	if j.lock != nil {
		failed = j.lock.Close() != nil || failed
		j.lock = nil
	}
	if failed {
		return ErrRecovery
	}
	return nil
}
