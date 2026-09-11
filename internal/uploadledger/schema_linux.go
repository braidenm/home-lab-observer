//go:build linux

package uploadledger

import (
	"bytes"
	"context"
	"database/sql"
	"time"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

const schema = `CREATE TABLE upload_record (
singleton INTEGER PRIMARY KEY CHECK(singleton=1),
server_id TEXT NOT NULL CHECK(length(server_id)=36),
connector_id TEXT NOT NULL CHECK(length(connector_id)=38),
watermark INTEGER NOT NULL CHECK(watermark>=0),
has_ack INTEGER NOT NULL CHECK(has_ack IN (0,1)),
last_ack BLOB NOT NULL CHECK(length(last_ack)=32),
stopped TEXT NOT NULL CHECK(stopped IN ('','CREDENTIAL_REJECTED','CONFLICT','REJECTED','SEQUENCE_EXHAUSTED')),
pending_sequence INTEGER CHECK(pending_sequence>0),
pending_body BLOB CHECK(length(pending_body) BETWEEN 1 AND 16384),
pending_digest BLOB CHECK(length(pending_digest)=32),
pending_at TEXT CHECK(length(pending_at) BETWEEN 20 AND 30),
CHECK((pending_sequence IS NULL AND pending_body IS NULL AND pending_digest IS NULL AND pending_at IS NULL)
 OR (pending_sequence IS NOT NULL AND pending_body IS NOT NULL AND pending_digest IS NOT NULL AND pending_at IS NOT NULL))
) STRICT`

const columns = `server_id,connector_id,watermark,has_ack,last_ack,stopped,pending_sequence,pending_body,pending_digest,pending_at`
const updateRecord = `UPDATE upload_record SET server_id=?,connector_id=?,watermark=?,has_ack=?,last_ack=?,stopped=?,pending_sequence=?,pending_body=?,pending_digest=?,pending_at=? WHERE singleton=1`

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func recordArgs(r uploadstate.Record) []any {
	args := []any{r.Binding.ServerID, r.Binding.ConnectorID, r.Watermark, r.HasAck, r.LastAck[:], string(r.Stopped), nil, nil, nil, nil}
	if p := r.Pending; p != nil {
		args[6], args[7], args[8], args[9] = p.Sequence, p.Body, p.Digest[:], p.CollectedAt.UTC().Format(time.RFC3339Nano)
	}
	return args
}

func readRecord(ctx context.Context, q queryer, binding uploadstate.Binding) (uploadstate.Record, error) {
	var count, bounded int
	// Check lengths before copying a body from storage. The file itself is also bounded.
	err := q.QueryRowContext(ctx, `SELECT count(*), coalesce(min(length(server_id)=36 AND length(connector_id)=38 AND length(last_ack)=32 AND length(stopped)<=32 AND (pending_body IS NULL OR length(pending_body)<=16384) AND (pending_digest IS NULL OR length(pending_digest)=32) AND (pending_at IS NULL OR length(pending_at)<=30)),0) FROM upload_record`).Scan(&count, &bounded)
	if err != nil || count != 1 || bounded != 1 {
		return uploadstate.Record{}, ErrRecovery
	}
	var r uploadstate.Record
	var ack int
	var last, body, digest []byte
	var sequence sql.NullInt64
	var at sql.NullString
	err = q.QueryRowContext(ctx, `SELECT `+columns+` FROM upload_record WHERE singleton=1`).Scan(
		&r.Binding.ServerID, &r.Binding.ConnectorID, &r.Watermark, &ack, &last, &r.Stopped, &sequence, &body, &digest, &at)
	if err != nil || (ack != 0 && ack != 1) || len(last) != 32 {
		return uploadstate.Record{}, ErrRecovery
	}
	r.HasAck = ack == 1
	copy(r.LastAck[:], last)
	if sequence.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, at.String)
		if err != nil || !at.Valid || parsed.UTC().Format(time.RFC3339Nano) != at.String || len(digest) != 32 {
			return uploadstate.Record{}, ErrRecovery
		}
		p := &uploadstate.Pending{Sequence: sequence.Int64, Body: body, CollectedAt: parsed}
		copy(p.Digest[:], digest)
		r.Pending = p
	} else if at.Valid || body != nil || digest != nil {
		return uploadstate.Record{}, ErrRecovery
	}
	if uploadstate.ValidateRecord(r, binding) != nil {
		return uploadstate.Record{}, ErrRecovery
	}
	return r, nil
}

func samePending(a, b *uploadstate.Pending) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Sequence == b.Sequence && a.Digest == b.Digest && a.CollectedAt.Equal(b.CollectedAt) && bytes.Equal(a.Body, b.Body)
}
func sameRecord(a, b uploadstate.Record) bool {
	return a.Binding == b.Binding && a.Watermark == b.Watermark && a.HasAck == b.HasAck && a.LastAck == b.LastAck && a.Stopped == b.Stopped && samePending(a.Pending, b.Pending)
}
func validChange(expected, next uploadstate.Record, binding uploadstate.Binding) bool {
	return uploadstate.ValidateRecord(expected, binding) == nil && uploadstate.ValidateRecord(next, binding) == nil &&
		next.Watermark >= expected.Watermark &&
		(next.Pending == nil || next.Pending.Sequence > expected.Watermark || samePending(expected.Pending, next.Pending))
}
