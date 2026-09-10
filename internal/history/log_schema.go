package history

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const logTimeLayout = "2006-01-02T15:04:05.000000000Z"

var ErrLogStorageUnavailable = errors.New("log storage unavailable")

// Schema versioning is deliberately separate from the core user_version. No
// log-only initialization error may invoke host database recovery/quarantine.
var logSchema = []struct{ name, statement string }{
	{"log_metadata_state", `CREATE TABLE log_metadata_state(source TEXT NOT NULL PRIMARY KEY CHECK(source IN ('system','application')), revision TEXT NOT NULL, reset_pending INTEGER NOT NULL CHECK(reset_pending IN (0,1)), opaque BLOB, attempted_at TEXT NOT NULL, coverage_through TEXT, support TEXT NOT NULL, collection TEXT NOT NULL, freshness TEXT NOT NULL, observed_at TEXT, status_coverage_through TEXT, reason TEXT) WITHOUT ROWID`},
	{"log_metadata_minutes", `CREATE TABLE log_metadata_minutes(source TEXT NOT NULL CHECK(source IN ('system','application')), minute TEXT NOT NULL, captured INTEGER NOT NULL CHECK(captured BETWEEN 0 AND 9007199254740991), discarded INTEGER NOT NULL CHECK(discarded BETWEEN 0 AND 9007199254740991), trace INTEGER NOT NULL CHECK(trace BETWEEN 0 AND 9007199254740991), debug INTEGER NOT NULL CHECK(debug BETWEEN 0 AND 9007199254740991), info INTEGER NOT NULL CHECK(info BETWEEN 0 AND 9007199254740991), warn INTEGER NOT NULL CHECK(warn BETWEEN 0 AND 9007199254740991), error INTEGER NOT NULL CHECK(error BETWEEN 0 AND 9007199254740991), critical INTEGER NOT NULL CHECK(critical BETWEEN 0 AND 9007199254740991), unknown INTEGER NOT NULL CHECK(unknown BETWEEN 0 AND 9007199254740991), CHECK(captured=trace+debug+info+warn+error+critical+unknown), PRIMARY KEY(source,minute)) WITHOUT ROWID`},
	{"log_metadata_coverage", `CREATE TABLE log_metadata_coverage(source TEXT NOT NULL CHECK(source IN ('system','application')), start_at TEXT NOT NULL, end_at TEXT NOT NULL CHECK(end_at>start_at), reason TEXT NOT NULL, PRIMARY KEY(source,start_at)) WITHOUT ROWID`},
}

type logSQLReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) ensureLogSchema(ctx context.Context) error {
	if s.logReady.Load() {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version string
	err = tx.QueryRowContext(ctx, `SELECT value FROM store_metadata WHERE key='log_metadata_schema_version'`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		for _, table := range logSchema {
			// CREATE (without IF NOT EXISTS) refuses an unversioned partial schema.
			if _, err := tx.ExecContext(ctx, table.statement); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO store_metadata(key,value) VALUES('log_metadata_schema_version','1')`); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := validateLogSchema(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.logReady.Store(true)
	return nil
}

func validateLogSchema(ctx context.Context, reader logSQLReader) error {
	var version string
	if err := reader.QueryRowContext(ctx, `SELECT value FROM store_metadata WHERE key='log_metadata_schema_version'`).Scan(&version); err != nil {
		return err
	}
	if version != "1" {
		return ErrLogStorageUnavailable
	}
	for _, table := range logSchema {
		var statement string
		if err := reader.QueryRowContext(ctx, `SELECT sql FROM sqlite_schema WHERE type='table' AND name=?`, table.name).Scan(&statement); err != nil {
			return err
		}
		if statement != table.statement {
			return ErrLogStorageUnavailable
		}
	}
	return nil
}

func encodeLogTime(at time.Time) (string, error) {
	if !validLogTime(at) {
		return "", ErrLogStorageUnavailable
	}
	return at.Format(logTimeLayout), nil
}

func decodeLogTime(text string) (time.Time, error) {
	at, err := time.Parse(logTimeLayout, text)
	if err != nil || !validLogTime(at) || at.Format(logTimeLayout) != text {
		return time.Time{}, ErrLogStorageUnavailable
	}
	return at, nil
}

func nullableLogTime(at *time.Time) (any, error) {
	if at == nil {
		return nil, nil
	}
	return encodeLogTime(*at)
}

func scanNullableLogTime(text sql.NullString) (*time.Time, error) {
	if !text.Valid {
		return nil, nil
	}
	at, err := decodeLogTime(text.String)
	if err != nil {
		return nil, err
	}
	return &at, nil
}
