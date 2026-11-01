//go:build linux

package actionjournal

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"time"
)

const actionSchema = `CREATE TABLE actions (
action_id TEXT PRIMARY KEY CHECK(length(action_id)=36),
target_id TEXT NOT NULL CHECK(length(target_id)=36),
engine_generation TEXT NOT NULL CHECK(length(engine_generation)=32),
container_id TEXT NOT NULL CHECK(length(container_id)=64),
metadata BLOB NOT NULL CHECK(length(metadata) BETWEEN 1 AND 2048),
digest BLOB NOT NULL CHECK(length(digest)=32),
admitted_at TEXT NOT NULL CHECK(length(admitted_at) BETWEEN 20 AND 30)
) STRICT, WITHOUT ROWID`
const eventSchema = `CREATE TABLE events (
action_id TEXT NOT NULL REFERENCES actions(action_id),
ordinal INTEGER NOT NULL CHECK(ordinal IN (1,2)),
at TEXT NOT NULL CHECK(length(at) BETWEEN 20 AND 30),
code TEXT NOT NULL CHECK(code IN ('INTENT','STARTED','STOPPED','RESTARTED','ALREADY_RUNNING','ALREADY_STOPPED','TARGET_STATE_CONFLICT','DENIED')),
reason TEXT NOT NULL CHECK(reason IN ('','EXPIRED_UNDISPATCHED')),
PRIMARY KEY(action_id,ordinal),
CHECK((ordinal=1 AND code='INTENT' AND reason='') OR (ordinal=2 AND code!='INTENT')),
CHECK((code='DENIED' AND reason='EXPIRED_UNDISPATCHED') OR (code!='DENIED' AND reason=''))
) STRICT, WITHOUT ROWID`
const metaSchema = `CREATE TABLE metadata (
singleton INTEGER PRIMARY KEY CHECK(singleton=1),
server_id TEXT NOT NULL CHECK(length(server_id)=36),
installation_id TEXT NOT NULL CHECK(length(installation_id)=38),
owner_id TEXT NOT NULL CHECK(length(owner_id)=36),
pruned_expiry_through TEXT NOT NULL CHECK(length(pruned_expiry_through) BETWEEN 20 AND 30)
) STRICT`

var definitions = map[string]string{
	"metadata": metaSchema, "actions": actionSchema, "events": eventSchema,
	"actions_target":   `CREATE INDEX actions_target ON actions(target_id)`,
	"actions_physical": `CREATE INDEX actions_physical ON actions(engine_generation,container_id)`,
}

func (j *Journal) configure(ctx context.Context) error {
	var mode string
	if j.conn.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode) != nil || mode != "delete" {
		return ErrRecovery
	}
	for name, want := range map[string]int{"page_size": 4096, "synchronous": 3, "cache_size": 3072, "cache_spill": 0, "temp_store": 2, "trusted_schema": 0, "foreign_keys": 1, "mmap_size": 0, "busy_timeout": 250} {
		var got int
		if j.conn.QueryRowContext(ctx, `PRAGMA `+name).Scan(&got) != nil || got != want {
			return ErrRecovery
		}
	}
	var pages, max int
	if j.conn.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages) != nil || pages > 3072 || j.conn.QueryRowContext(ctx, `PRAGMA max_page_count=3072`).Scan(&max) != nil || max != 3072 {
		return ErrRecovery
	}
	return nil
}
func (j *Journal) verify(ctx context.Context) error {
	var app, version int
	if j.conn.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&app) != nil || app != 0x484C414A || j.conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version) != nil || version != 1 {
		return ErrRecovery
	}
	rows, e := j.conn.QueryContext(ctx, `SELECT name,sql FROM sqlite_schema`)
	if e != nil {
		return ErrRecovery
	}
	seen := 0
	for rows.Next() {
		var name, definition string
		if rows.Scan(&name, &definition) != nil || definitions[name] != definition {
			rows.Close()
			return ErrRecovery
		}
		seen++
	}
	e = rows.Err()
	rows.Close()
	if e != nil || seen != len(definitions) {
		return ErrRecovery
	}
	var integrity string
	if j.conn.QueryRowContext(ctx, `PRAGMA integrity_check(1)`).Scan(&integrity) != nil || integrity != "ok" {
		return ErrRecovery
	}
	if _, e = j.floor(ctx); e != nil {
		return e
	}
	records, e := j.records(ctx)
	if e != nil {
		return e
	}
	unknown := 0
	targets := map[string]bool{}
	physical := map[[2]string]bool{}
	for _, r := range records {
		if !r.intent.IsZero() && r.terminal.IsZero() {
			pair := [2]string{r.key.Metadata.EngineGeneration, r.key.Metadata.ContainerID}
			if targets[r.key.TargetID] || physical[pair] {
				return ErrRecovery
			}
			targets[r.key.TargetID] = true
			physical[pair] = true
			unknown++
		}
	}
	var orphans int
	if unknown > 2 || j.conn.QueryRowContext(ctx, `SELECT count(*) FROM events e LEFT JOIN actions a ON a.action_id=e.action_id WHERE a.action_id IS NULL`).Scan(&orphans) != nil || orphans != 0 {
		return ErrRecovery
	}
	return nil
}
func (j *Journal) floor(ctx context.Context) (time.Time, error) {
	var count int
	if j.conn.QueryRowContext(ctx, `SELECT count(*) FROM metadata`).Scan(&count) != nil || count != 1 {
		return time.Time{}, ErrRecovery
	}
	var b Binding
	var at string
	if j.conn.QueryRowContext(ctx, `SELECT server_id,installation_id,owner_id,pruned_expiry_through FROM metadata WHERE singleton=1`).Scan(&b.ServerID, &b.InstallationID, &b.OwnerID, &at) != nil || b != j.binding {
		return time.Time{}, ErrRecovery
	}
	return parseTime(at)
}
func (j *Journal) ids(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, e := j.conn.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, ErrRecovery
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if len(ids) >= maxRecords || rows.Scan(&id) != nil || !validID(id, "hla_") {
			return nil, ErrRecovery
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil {
		return nil, ErrRecovery
	}
	return ids, nil
}
func (j *Journal) records(ctx context.Context) ([]record, error) {
	ids, e := j.ids(ctx, `SELECT action_id FROM actions ORDER BY action_id`)
	if e != nil {
		return nil, e
	}
	records := make([]record, 0, len(ids))
	for _, id := range ids {
		r, e := j.read(ctx, id)
		if e != nil {
			return nil, e
		}
		records = append(records, r)
	}
	return records, nil
}
func (j *Journal) read(ctx context.Context, id string) (record, error) {
	var r record
	var target, gen, container, admitted string
	var digest []byte
	var bounded int
	e := j.conn.QueryRowContext(ctx, `SELECT length(metadata) BETWEEN 1 AND 2048 AND length(digest)=32 AND length(admitted_at) BETWEEN 20 AND 30 FROM actions WHERE action_id=?`, id).Scan(&bounded)
	if errors.Is(e, sql.ErrNoRows) {
		return r, sql.ErrNoRows
	}
	if e != nil || bounded != 1 {
		return r, ErrRecovery
	}
	if j.conn.QueryRowContext(ctx, `SELECT target_id,engine_generation,container_id,metadata,digest,admitted_at FROM actions WHERE action_id=?`, id).Scan(&target, &gen, &container, &r.raw, &digest, &admitted) != nil {
		return r, ErrRecovery
	}
	k, e := actioncore.ParseJournalMetadata(r.raw)
	if e != nil || k.ActionID != id || k.TargetID != target || k.Metadata.EngineGeneration != gen || k.Metadata.ContainerID != container || !bytes.Equal(digest, k.Fingerprint[:]) {
		return r, ErrRecovery
	}
	r.key = k
	req := k.Metadata.Request
	if req.ServerID != j.binding.ServerID || req.InstallationID != j.binding.InstallationID || req.OwnerID != j.binding.OwnerID {
		return r, ErrRecovery
	}
	r.admitted, e = parseTime(admitted)
	if e != nil || r.admitted.Before(req.CreatedAt) || !r.admitted.Before(req.ExpiresAt) {
		return r, ErrRecovery
	}
	rows, e := j.conn.QueryContext(ctx, `SELECT ordinal,at,code,reason FROM events WHERE action_id=? ORDER BY ordinal`, id)
	if e != nil {
		return r, ErrRecovery
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var ordinal int
		var at, code, reason string
		if count >= 2 || rows.Scan(&ordinal, &at, &code, &reason) != nil {
			return r, ErrRecovery
		}
		when, e := parseTime(at)
		if e != nil || when.Before(r.admitted) {
			return r, ErrRecovery
		}
		count++
		if ordinal == 1 {
			if code != "INTENT" || reason != "" || !r.intent.IsZero() || !when.Before(req.ExpiresAt) {
				return r, ErrRecovery
			}
			r.intent = when
		} else if ordinal == 2 {
			if !r.terminal.IsZero() || (!r.intent.IsZero() && when.Before(r.intent)) || !terminalAllowed(req.Operation, !r.intent.IsZero(), actioncore.Code(code), reason) {
				return r, ErrRecovery
			}
			if code == string(actioncore.Denied) && when.Before(req.ExpiresAt) {
				return r, ErrRecovery
			}
			r.terminal = when
			r.code = actioncore.Code(code)
			r.reason = reason
		} else {
			return r, ErrRecovery
		}
	}
	if rows.Err() != nil {
		return r, ErrRecovery
	}
	return r, nil
}
func terminalAllowed(op actioncore.Operation, intent bool, code actioncore.Code, reason string) bool {
	if !intent && code == actioncore.Denied {
		return reason == "EXPIRED_UNDISPATCHED"
	}
	if reason != "" {
		return false
	}
	if intent {
		return op == actioncore.Start && code == actioncore.Started || op == actioncore.Stop && code == actioncore.Stopped || op == actioncore.Restart && code == actioncore.Restarted
	}
	return code == actioncore.StateConflict || op == actioncore.Start && code == actioncore.AlreadyRunning || op == actioncore.Stop && code == actioncore.AlreadyStopped
}
