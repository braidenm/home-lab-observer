//go:build linux

package actionjournal

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

// This delegates to the real selected SQLite driver; only queries are counted.
// No production instrumentation, global registration or alternate SQL engine.
type countingConnector struct {
	driver driver.Driver
	dsn    string
	count  *int
}

func (c countingConnector) Driver() driver.Driver { return c.driver }
func (c countingConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return countingConnection{Conn: conn, count: c.count}, nil
}

type countingConnection struct {
	driver.Conn
	count *int
}

func (c countingConnection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	*c.count++
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}
func (c countingConnection) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func countQueries(t *testing.T, j *Journal) *int {
	t.Helper()
	d := j.db.Driver()
	j.conn.Close()
	j.db.Close()
	n := new(int)
	j.db = sql.OpenDB(countingConnector{driver: d, dsn: dataSource(j.dir), count: n})
	j.db.SetMaxOpenConns(1)
	var err error
	j.conn, err = j.db.Conn(ctx)
	if err != nil {
		t.Fatal("counted connection")
	}
	return n
}

func maximumHistory(t *testing.T, j *Journal) {
	t.Helper()
	err := j.transaction(ctx, func() error {
		for n := 1; n <= maxRecords; n++ {
			k := maxKey(t, n)
			k.Metadata.Request.TargetID = key(t, 1, 1).TargetID
			k = rebuild(t, k)
			raw, err := actioncore.CanonicalJournalMetadata(k)
			if err != nil {
				return err
			}
			_, err = j.conn.ExecContext(ctx, `INSERT INTO actions VALUES(?,?,?,?,?,?,?)`, k.ActionID, k.TargetID, k.Metadata.EngineGeneration, k.Metadata.ContainerID, raw, k.Fingerprint[:], stamp(epoch.Add(time.Second)))
			if err != nil {
				return err
			}
			if _, err = j.conn.ExecContext(ctx, `INSERT INTO events VALUES(?,1,?,'INTENT',''),(?,2,?,'RESTARTED','')`, k.ActionID, stamp(epoch.Add(time.Second)), k.ActionID, stamp(epoch.Add(2*time.Second))); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal("maximum history fixture")
	}
}

func TestBoundedRecordLoadingStatementCount(t *testing.T) {
	j, _ := provision(t)
	maximumHistory(t, j)
	count := countQueries(t, j)
	rows, err := j.records(ctx)
	if err != nil || len(rows) != maxRecords {
		t.Fatal("maximum records")
	}
	if *count != 2 {
		t.Fatalf("record queries=%d, want two fixed streams", *count)
	}
	*count = 0
	if _, err = j.read(ctx, key(t, 1, 1).ActionID); err != nil || *count != 2 {
		t.Fatalf("single record queries=%d, want two shared-validator streams", *count)
	}
	*count = 0
	if rows, err = j.related(ctx, key(t, 1, 1)); err != nil || len(rows) != maxRecords || *count != 2 {
		t.Fatalf("related queries=%d, want two fixed streams", *count)
	}
	*count = 0
	j.now = func() time.Time { return epoch.Add(4 * time.Minute) }
	if _, err = j.Acquire(ctx, key(t, 1001, 1)); !errors.Is(err, ErrFull) || j.failed {
		t.Fatal("full related history validation")
	}
	if *count > 10 {
		t.Fatalf("related history queries=%d, must not grow per action", *count)
	}
	*count = 0
	if err = j.Prune(ctx); err != nil || *count != 4 {
		t.Fatalf("maintenance queries=%d, want floor plus two fixed streams", *count)
	}
}

func TestAllReadScopesRejectMalformedStoredRecords(t *testing.T) {
	cases := map[string]string{
		"metadata-too-large":     `UPDATE actions SET metadata=zeroblob(1048576)`,
		"digest-too-large":       `UPDATE actions SET digest=zeroblob(1048576)`,
		"target-mismatch":        `UPDATE actions SET target_id='hlt_00000000000000000000000000000000'`,
		"generation-mismatch":    `UPDATE actions SET engine_generation='00000000000000000000000000000000'`,
		"container-mismatch":     `UPDATE actions SET container_id='0000000000000000000000000000000000000000000000000000000000000000'`,
		"digest-mismatch":        `UPDATE actions SET digest=zeroblob(32)`,
		"admission-expired":      `UPDATE actions SET admitted_at='2026-11-01T12:06:00Z'`,
		"event-too-large":        `UPDATE events SET reason=hex(zeroblob(524288)) WHERE ordinal=1`,
		"ordinal":                `UPDATE events SET ordinal=3 WHERE ordinal=2`,
		"operation":              `UPDATE events SET code='STOPPED' WHERE ordinal=2`,
		"reason":                 `UPDATE events SET reason='EXPIRED_UNDISPATCHED' WHERE ordinal=2`,
		"intent-expired":         `UPDATE events SET at='2026-11-01T12:06:00Z' WHERE ordinal=1`,
		"terminal-before-intent": `UPDATE events SET at='2026-11-01T12:00:00Z' WHERE ordinal=2`,
		"alternate-UTC":          `UPDATE events SET at='2026-11-01T12:00:00+00:00'`,
	}
	for name, mutation := range cases {
		t.Run(name, func(t *testing.T) {
			j, _ := provision(t)
			k := maxKey(t, 1)
			j.now = func() time.Time { return epoch.Add(time.Second) }
			a := acquire(t, j, k)
			if a.Intent(ctx) != nil {
				t.Fatal("fixture intent")
			}
			j.now = func() time.Time { return epoch.Add(2 * time.Second) }
			if a.Finish(ctx, actioncore.Restarted) != nil {
				t.Fatal("fixture terminal")
			}
			a.Close()
			if _, err := j.conn.ExecContext(ctx, `PRAGMA ignore_check_constraints=ON`); err != nil {
				t.Fatal("corrupt fixture setup")
			}
			if _, err := j.conn.ExecContext(ctx, mutation); err != nil {
				t.Fatal("corrupt fixture mutation")
			}
			if _, err := j.records(ctx); !errors.Is(err, ErrRecovery) {
				t.Fatal("bulk accepted malformed record")
			}
			if _, err := j.read(ctx, k.ActionID); !errors.Is(err, ErrRecovery) {
				t.Fatal("single accepted malformed record")
			}
			if _, err := j.related(ctx, k); !errors.Is(err, ErrRecovery) {
				t.Fatal("related accepted malformed record")
			}
		})
	}
}

func TestBulkRejectsOrphanAndBoundsFieldsBeforeScan(t *testing.T) {
	j, _ := provision(t)
	k := key(t, 1, 1)
	a := acquire(t, j, k)
	a.Close()
	if _, err := j.conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF; PRAGMA ignore_check_constraints=ON`); err != nil {
		t.Fatal("corrupt fixture setup")
	}
	if _, err := j.conn.ExecContext(ctx, `INSERT INTO events VALUES(?,1,?,'INTENT','')`, "hla_"+strings.Repeat("f", 32), stamp(epoch)); err != nil {
		t.Fatal("orphan fixture")
	}
	if _, err := j.records(ctx); !errors.Is(err, ErrRecovery) {
		t.Fatal("orphan accepted")
	}
	if _, err := j.conn.ExecContext(ctx, `DELETE FROM events; UPDATE actions SET metadata=zeroblob(1048576),digest=zeroblob(1048576)`); err != nil {
		t.Fatal("oversized fixture")
	}
	var id, target, gen, container, admitted string
	var raw, digest []byte
	if err := j.conn.QueryRowContext(ctx, `SELECT `+actionColumns+` FROM actions a`).Scan(&id, &target, &gen, &container, &raw, &digest, &admitted); err != nil || raw != nil || digest != nil {
		t.Fatal("oversized raw fields crossed SQL projection")
	}
	if _, err := j.conn.ExecContext(ctx, `INSERT INTO events VALUES(?,1,?,'INTENT',hex(zeroblob(524288)))`, k.ActionID, stamp(epoch)); err != nil {
		t.Fatal("oversized event fixture")
	}
	var ordinal int
	var at, code string
	var reason sql.NullString
	if err := j.conn.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM events e`).Scan(&id, &ordinal, &at, &code, &reason); err != nil || reason.Valid {
		t.Fatal("oversized event field crossed SQL projection")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := j.records(cancelled); !errors.Is(err, ErrRecovery) {
		t.Fatal("cancelled load accepted")
	}
}

func TestTextProjectionsBoundBytesIncludingNULTails(t *testing.T) {
	families := []struct {
		table, alias, column, prefix, columns string
		index, limit                          int
	}{
		{"actions", "a", "action_id", "hla_" + strings.Repeat("1", 32), actionColumns, 0, 36},
		{"actions", "a", "target_id", "hlt_" + strings.Repeat("1", 32), actionColumns, 1, 36},
		{"actions", "a", "engine_generation", strings.Repeat("e", 32), actionColumns, 2, 32},
		{"actions", "a", "container_id", strings.Repeat("1", 64), actionColumns, 3, 64},
		{"actions", "a", "admitted_at", stamp(epoch), actionColumns, 6, 30},
		{"events", "e", "action_id", "hla_" + strings.Repeat("1", 32), eventColumns, 0, 36},
		{"events", "e", "at", stamp(epoch), eventColumns, 2, 30},
		{"events", "e", "code", "INTENT", eventColumns, 3, 21},
		{"events", "e", "reason", "", eventColumns, 4, 20},
	}
	for _, f := range families {
		for _, kind := range []string{"nul-tail", "multibyte"} {
			t.Run(f.table+"/"+f.column+"/"+kind, func(t *testing.T) {
				j, _ := provision(t)
				a := acquire(t, j, key(t, 1, 1))
				if a.Intent(ctx) != nil {
					t.Fatal("projection fixture")
				}
				a.Close()
				if _, err := j.conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF; PRAGMA ignore_check_constraints=ON`); err != nil {
					t.Fatal("corrupt projection setup")
				}
				value := f.prefix + "\x00" + strings.Repeat("x", 1048576)
				if kind == "multibyte" {
					value = strings.Repeat("é", f.limit)
				}
				if _, err := j.conn.ExecContext(ctx, `UPDATE `+f.table+` SET `+f.column+`=?`, value); err != nil {
					t.Fatal("corrupt projection value")
				}
				expression := strings.Split(f.columns, ",")[f.index]
				// Reproduce the original character-count predicate on the same
				// actual SQLite value: it wrongly materializes the oversized text.
				legacyExpression := strings.ReplaceAll(expression, "length(CAST("+f.alias+"."+f.column+" AS BLOB))", "length("+f.alias+"."+f.column+")")
				var legacy string
				if err := j.conn.QueryRowContext(ctx, `SELECT `+legacyExpression+` FROM `+f.table+` `+f.alias).Scan(&legacy); err != nil || len(legacy) != len(value) {
					t.Fatal("legacy character-count regression not reproduced")
				}
				var projected any
				if err := j.conn.QueryRowContext(ctx, `SELECT `+expression+` FROM `+f.table+` `+f.alias).Scan(&projected); err != nil || projected != nil {
					t.Fatal("oversized text crossed SQL projection")
				}
			})
		}
	}
}
