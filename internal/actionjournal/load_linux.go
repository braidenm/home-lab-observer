//go:build linux

package actionjournal

import (
	"bytes"
	"context"
	"database/sql"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

// CASE bounds every raw field before SQLite hands it to the driver. Invalid
// values become NULL and fail scanning/validation, not truncated valid records.
const actionColumns = `CASE WHEN length(CAST(a.action_id AS BLOB))=36 THEN a.action_id END,
CASE WHEN length(CAST(a.target_id AS BLOB))=36 THEN a.target_id END,
CASE WHEN length(CAST(a.engine_generation AS BLOB))=32 THEN a.engine_generation END,
CASE WHEN length(CAST(a.container_id AS BLOB))=64 THEN a.container_id END,
CASE WHEN length(a.metadata) BETWEEN 1 AND 2048 THEN a.metadata END,
CASE WHEN length(a.digest)=32 THEN a.digest END,
CASE WHEN length(CAST(a.admitted_at AS BLOB)) BETWEEN 20 AND 30 THEN a.admitted_at END`
const eventColumns = `CASE WHEN length(CAST(e.action_id AS BLOB))=36 THEN e.action_id END,e.ordinal,
CASE WHEN length(CAST(e.at AS BLOB)) BETWEEN 20 AND 30 THEN e.at END,
CASE WHEN length(CAST(e.code AS BLOB)) BETWEEN 1 AND 21 THEN e.code END,
CASE WHEN length(CAST(e.reason AS BLOB))<=20 THEN e.reason END`

type recordScope uint8

const (
	allRecords recordScope = iota
	oneRecord
	relatedRecords
)

func (j *Journal) records(ctx context.Context) ([]record, error) {
	return j.load(ctx, allRecords, actioncore.Key{})
}
func (j *Journal) related(ctx context.Context, k actioncore.Key) ([]record, error) {
	return j.load(ctx, relatedRecords, k)
}
func (j *Journal) read(ctx context.Context, id string) (record, error) {
	records, err := j.load(ctx, oneRecord, actioncore.Key{ActionID: id})
	if err != nil {
		return record{}, err
	}
	if len(records) == 0 {
		return record{}, sql.ErrNoRows
	}
	if len(records) != 1 {
		return record{}, ErrRecovery
	}
	return records[0], nil
}

func (j *Journal) load(ctx context.Context, scope recordScope, k actioncore.Key) ([]record, error) {
	actions := `SELECT ` + actionColumns + ` FROM actions a`
	events := `SELECT ` + eventColumns + ` FROM events e`
	var args []any
	switch scope {
	case allRecords:
	case oneRecord:
		actions += ` WHERE a.action_id=?`
		events += ` WHERE e.action_id=?`
		args = []any{k.ActionID}
	case relatedRecords:
		const where = ` WHERE a.target_id=? OR (a.engine_generation=? AND a.container_id=?)`
		actions += where
		events += ` JOIN actions a ON a.action_id=e.action_id` + where
		args = []any{k.TargetID, k.Metadata.EngineGeneration, k.Metadata.ContainerID}
	default:
		return nil, ErrRecovery
	}
	rows, err := j.conn.QueryContext(ctx, actions+` ORDER BY a.action_id`, args...)
	if err != nil {
		return nil, ErrRecovery
	}
	records := make([]record, 0)
	byID := make(map[string]int)
	for rows.Next() {
		var id, target, gen, container, admitted string
		var raw, digest []byte
		if len(records) >= maxRecords || rows.Scan(&id, &target, &gen, &container, &raw, &digest, &admitted) != nil {
			rows.Close()
			return nil, ErrRecovery
		}
		r, err := validateAction(j.binding, id, target, gen, container, raw, digest, admitted)
		if _, duplicate := byID[id]; err != nil || duplicate {
			rows.Close()
			return nil, ErrRecovery
		}
		byID[id] = len(records)
		records = append(records, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, ErrRecovery
	}
	rows, err = j.conn.QueryContext(ctx, events+` ORDER BY e.action_id,e.ordinal`, args...)
	if err != nil {
		return nil, ErrRecovery
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, at, code, reason string
		var ordinal int
		if count >= 2*maxRecords || rows.Scan(&id, &ordinal, &at, &code, &reason) != nil {
			return nil, ErrRecovery
		}
		index, exists := byID[id]
		if !exists || validateEvent(&records[index], ordinal, at, code, reason) != nil {
			return nil, ErrRecovery
		}
		count++
	}
	if rows.Err() != nil {
		return nil, ErrRecovery
	}
	return records, nil
}

func validateAction(binding Binding, id, target, gen, container string, raw, digest []byte, admitted string) (record, error) {
	r := record{raw: raw}
	k, err := actioncore.ParseJournalMetadata(raw)
	if err != nil || k.ActionID != id || k.TargetID != target || k.Metadata.EngineGeneration != gen || k.Metadata.ContainerID != container || !bytes.Equal(digest, k.Fingerprint[:]) {
		return r, ErrRecovery
	}
	r.key = k
	req := k.Metadata.Request
	if req.ServerID != binding.ServerID || req.InstallationID != binding.InstallationID || req.OwnerID != binding.OwnerID {
		return r, ErrRecovery
	}
	r.admitted, err = parseTime(admitted)
	if err != nil || r.admitted.Before(req.CreatedAt) || !r.admitted.Before(req.ExpiresAt) {
		return r, ErrRecovery
	}
	return r, nil
}

func validateEvent(r *record, ordinal int, at, code, reason string) error {
	req := r.key.Metadata.Request
	when, err := parseTime(at)
	if err != nil || when.Before(r.admitted) {
		return ErrRecovery
	}
	if ordinal == 1 {
		if code != "INTENT" || reason != "" || !r.intent.IsZero() || !r.terminal.IsZero() || !when.Before(req.ExpiresAt) {
			return ErrRecovery
		}
		r.intent = when
	} else if ordinal == 2 {
		if !r.terminal.IsZero() || (!r.intent.IsZero() && when.Before(r.intent)) || !terminalAllowed(req.Operation, !r.intent.IsZero(), actioncore.Code(code), reason) {
			return ErrRecovery
		}
		if code == string(actioncore.Denied) && when.Before(req.ExpiresAt) {
			return ErrRecovery
		}
		r.terminal = when
		r.code = actioncore.Code(code)
		r.reason = reason
	} else {
		return ErrRecovery
	}
	return nil
}
