//go:build linux

package uploadledger

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

const operationTimeout = 2 * time.Second

type Ledger struct {
	mu      sync.Mutex
	dir     string
	binding uploadstate.Binding
	lock    *os.File
	db      *sql.DB
	conn    *sql.Conn
	failed  bool
	// Test seams never receive credentials. Production uses the real transaction.
	beforeCommit func() error
	afterCommit  func() error
}

func Provision(ctx context.Context, directory string, binding uploadstate.Binding) (*Ledger, error) {
	return open(ctx, directory, binding, true)
}
func OpenExisting(ctx context.Context, directory string, binding uploadstate.Binding) (*Ledger, error) {
	return open(ctx, directory, binding, false)
}

func dataSource(dir string) string {
	u := url.URL{Scheme: "file", Path: filepath.Join(dir, databaseName)}
	q := url.Values{"mode": {"rw"}, "cache": {"private"}, "_txlock": {"immediate"}, "_defensive": {"true"}}
	for _, pragma := range []string{"busy_timeout(250)", "synchronous(EXTRA)", "cache_size(64)", "cache_spill(OFF)", "temp_store(MEMORY)", "trusted_schema(OFF)", "mmap_size(0)"} {
		q.Add("_pragma", pragma)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func open(ctx context.Context, directory string, binding uploadstate.Binding, create bool) (_ *Ledger, result error) {
	if ctx.Err() != nil || uploadstate.ValidateRecord(uploadstate.Record{Binding: binding}, binding) != nil {
		return nil, ErrRecovery
	}
	dir, err := privateDirectory(directory)
	if err != nil {
		return nil, err
	}
	lock, err := acquireLock(dir, create)
	if err != nil {
		return nil, err
	}
	l := &Ledger{dir: dir, binding: binding, lock: lock}
	defer func() {
		if result != nil {
			l.Close()
		}
	}()
	if create {
		// Only the proven-new lock may exist. Refuse all previously owned data.
		parent, err := os.Open(dir)
		if err != nil {
			return nil, ErrRecovery
		}
		entries, readErr := parent.ReadDir(2)
		parent.Close()
		if readErr != nil || len(entries) != 1 || entries[0].Name() != lockName {
			return nil, ErrRecovery
		}
		if createDatabase(dir) != nil {
			return nil, ErrRecovery
		}
	}
	if err := checkFiles(dir, true); err != nil {
		return nil, err
	}
	if !create && checkHeader(dir) != nil {
		return nil, ErrRecovery
	}
	db, err := sql.Open("sqlite", dataSource(dir))
	if err != nil {
		return nil, ErrRecovery
	}
	l.db = db
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	bounded, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	l.conn, err = db.Conn(bounded)
	if err != nil {
		return nil, ErrRecovery
	}
	if create {
		if _, err := l.conn.ExecContext(bounded, `PRAGMA page_size=4096`); err != nil {
			return nil, ErrRecovery
		}
	} else if err := l.verifySchema(bounded); err != nil {
		return nil, err
	}
	if err := l.configure(bounded); err != nil {
		return nil, err
	}
	if create {
		err := l.transaction(bounded, func() error {
			for _, statement := range []string{schema, `PRAGMA application_id=1213156420`, `PRAGMA user_version=1`} {
				if _, err := l.conn.ExecContext(bounded, statement); err != nil {
					return ErrRecovery
				}
			}
			if _, err := l.conn.ExecContext(bounded, `INSERT INTO upload_record(singleton,`+columns+`) VALUES(1,?,?,?,?,?,?,?,?,?,?)`, recordArgs(uploadstate.Record{Binding: binding})...); err != nil {
				return ErrRecovery
			}
			return nil
		})
		if err != nil || syncDirectory(dir) != nil {
			return nil, ErrRecovery
		}
	}
	if err := l.verifySchema(bounded); err != nil {
		return nil, err
	}
	if _, err := readRecord(bounded, l.conn, binding); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Ledger) configure(ctx context.Context) error {
	var mode string
	if l.conn.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode) != nil || mode != "delete" {
		return ErrRecovery
	}
	var size, count, max int
	if l.conn.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&size) != nil || size != 4096 ||
		l.conn.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&count) != nil || count > 64 ||
		l.conn.QueryRowContext(ctx, `PRAGMA max_page_count=64`).Scan(&max) != nil || max != 64 {
		return ErrRecovery
	}
	for name, want := range map[string]int{"synchronous": 3, "cache_size": 64, "cache_spill": 0, "temp_store": 2, "trusted_schema": 0, "mmap_size": 0, "busy_timeout": 250} {
		var got int
		if l.conn.QueryRowContext(ctx, `PRAGMA `+name).Scan(&got) != nil || got != want {
			return ErrRecovery
		}
	}
	return nil
}

func (l *Ledger) verifySchema(ctx context.Context) error {
	var app, version int
	if l.conn.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&app) != nil || app != 1213156420 || l.conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version) != nil || version != 1 {
		return ErrRecovery
	}
	var count int
	var definition string
	if l.conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema`).Scan(&count) != nil || count != 1 ||
		l.conn.QueryRowContext(ctx, `SELECT sql FROM sqlite_schema WHERE type='table' AND name='upload_record' AND tbl_name='upload_record'`).Scan(&definition) != nil || definition != schema {
		return ErrRecovery
	}
	var integrity string
	if l.conn.QueryRowContext(ctx, `PRAGMA integrity_check(1)`).Scan(&integrity) != nil || integrity != "ok" {
		return ErrRecovery
	}
	return nil
}

func (l *Ledger) Load(ctx context.Context) (uploadstate.Record, error) {
	if !l.mu.TryLock() {
		return uploadstate.Record{}, ErrBusy
	}
	defer l.mu.Unlock()
	if l.failed || l.conn == nil {
		return uploadstate.Record{}, ErrRecovery
	}
	bounded, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	r, err := readRecord(bounded, l.conn, l.binding)
	if err != nil {
		l.failed = true
		return uploadstate.Record{}, ErrRecovery
	}
	return uploadstate.CloneRecord(r), nil
}

func (l *Ledger) Commit(ctx context.Context, expected, next uploadstate.Record) error {
	if !l.mu.TryLock() {
		return ErrBusy
	}
	defer l.mu.Unlock()
	if l.failed || l.conn == nil {
		return ErrRecovery
	}
	if !validChange(expected, next, l.binding) {
		l.failed = true
		return ErrRecovery
	}
	expected = uploadstate.CloneRecord(expected)
	next = uploadstate.CloneRecord(next)
	bounded, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	err := l.commit(bounded, expected, next)
	if err != nil {
		l.failed = true
		return ErrRecovery
	}
	return nil
}

func (l *Ledger) commit(ctx context.Context, expected, next uploadstate.Record) error {
	return l.transaction(ctx, func() error {
		actual, err := readRecord(ctx, l.conn, l.binding)
		if err != nil || !sameRecord(actual, expected) {
			return ErrRecovery
		}
		result, err := l.conn.ExecContext(ctx, updateRecord, recordArgs(next)...)
		if err != nil {
			return ErrRecovery
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return ErrRecovery
		}
		return nil
	})
}

// Explicit statements retain context cancellation during COMMIT: this driver's
// driver.Tx.Commit uses a background context. Cleanup has its own short deadline.
func (l *Ledger) transaction(ctx context.Context, body func() error) error {
	if _, err := l.conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return ErrRecovery
	}
	committed := false
	defer func() {
		if !committed {
			cleanup, cancel := context.WithTimeout(context.Background(), operationTimeout)
			defer cancel()
			_, _ = l.conn.ExecContext(cleanup, `ROLLBACK`)
		}
	}()
	if body() != nil {
		return ErrRecovery
	}
	if l.beforeCommit != nil && l.beforeCommit() != nil {
		return ErrRecovery
	}
	if _, err := l.conn.ExecContext(ctx, `COMMIT`); err != nil {
		return ErrRecovery
	}
	committed = true
	if l.afterCommit != nil && l.afterCommit() != nil {
		return ErrRecovery
	}
	return nil
}

func (l *Ledger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	failed := false
	if l.conn != nil {
		failed = l.conn.Close() != nil
		l.conn = nil
	}
	if l.db != nil {
		failed = l.db.Close() != nil || failed
		l.db = nil
	}
	if l.lock != nil {
		failed = l.lock.Close() != nil || failed
		l.lock = nil
	}
	if failed {
		return ErrRecovery
	}
	return nil
}
