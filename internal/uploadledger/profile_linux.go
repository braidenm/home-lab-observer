//go:build linux

package uploadledger

import "github.com/braidenm/home-lab-observer/internal/uploadstate"

// Fixed storage envelopes are selected by validated code-owned kind, never by
// caller limits, database contents, DSN options or a pluggable policy registry.
type storageLimits struct {
	pages, version, bodyBytes int
	databaseBytes, totalBytes int64
}

func limits(profile uploadstate.ProfileBinding) storageLimits {
	if profile.Kind == uploadstate.RichProcessesProfile {
		return storageLimits{pages: 128, version: 2, bodyBytes: 288 * 1024, databaseBytes: 4096 * 128, totalBytes: 2 * 1024 * 1024}
	}
	return storageLimits{pages: 64, version: 1, bodyBytes: 16384, databaseBytes: maxDatabaseBytes, totalBytes: maxTotalBytes}
}

const richSchema = `CREATE TABLE upload_record (
singleton INTEGER PRIMARY KEY CHECK(singleton=1),
server_id TEXT NOT NULL CHECK(length(server_id)=36),
connector_id TEXT NOT NULL CHECK(length(connector_id)=38),
watermark INTEGER NOT NULL CHECK(watermark>=0),
has_ack INTEGER NOT NULL CHECK(has_ack IN (0,1)),
last_ack BLOB NOT NULL CHECK(length(last_ack)=32),
stopped TEXT NOT NULL CHECK(stopped IN ('','CREDENTIAL_REJECTED','CONFLICT','REJECTED','SEQUENCE_EXHAUSTED')),
pending_sequence INTEGER CHECK(pending_sequence>0),
pending_body BLOB CHECK(length(pending_body) BETWEEN 1 AND 294912),
pending_digest BLOB CHECK(length(pending_digest)=32),
pending_at TEXT CHECK(length(pending_at) BETWEEN 20 AND 30),
profile_kind INTEGER NOT NULL CHECK(profile_kind=1),
policy_digest BLOB NOT NULL CHECK(length(policy_digest)=32),
CHECK((pending_sequence IS NULL AND pending_body IS NULL AND pending_digest IS NULL AND pending_at IS NULL)
 OR (pending_sequence IS NOT NULL AND pending_body IS NOT NULL AND pending_digest IS NOT NULL AND pending_at IS NOT NULL))
) STRICT`

const richColumns = columns + `,profile_kind,policy_digest`
const richUpdateRecord = `UPDATE upload_record SET server_id=?,connector_id=?,watermark=?,has_ack=?,last_ack=?,stopped=?,pending_sequence=?,pending_body=?,pending_digest=?,pending_at=?,profile_kind=?,policy_digest=? WHERE singleton=1`

func schemaFor(profile uploadstate.ProfileBinding) string {
	if profile.Kind == uploadstate.RichProcessesProfile {
		return richSchema
	}
	return schema
}

func columnsFor(profile uploadstate.ProfileBinding) string {
	if profile.Kind == uploadstate.RichProcessesProfile {
		return richColumns
	}
	return columns
}

func updateFor(profile uploadstate.ProfileBinding) string {
	if profile.Kind == uploadstate.RichProcessesProfile {
		return richUpdateRecord
	}
	return updateRecord
}

func argsFor(r uploadstate.Record) []any {
	args := recordArgs(r)
	if r.Profile.Kind == uploadstate.RichProcessesProfile {
		args = append(args, int(r.Profile.Kind), r.Profile.PolicyDigest[:])
	}
	return args
}
