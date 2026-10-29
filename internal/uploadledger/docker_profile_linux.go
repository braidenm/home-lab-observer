//go:build linux

package uploadledger

import "github.com/braidenm/home-lab-observer/internal/uploadstate"

// A distinct header and exact schema fence Docker approval before SQLite can
// recover a different installed kind. Existing schema literals stay unchanged.
const dockerRichSchema = `CREATE TABLE upload_record (
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
profile_kind INTEGER NOT NULL CHECK(profile_kind=2),
policy_digest BLOB NOT NULL CHECK(length(policy_digest)=32),
CHECK((pending_sequence IS NULL AND pending_body IS NULL AND pending_digest IS NULL AND pending_at IS NULL)
 OR (pending_sequence IS NOT NULL AND pending_body IS NOT NULL AND pending_digest IS NOT NULL AND pending_at IS NOT NULL))
) STRICT`

func hasPolicyColumns(profile uploadstate.ProfileBinding) bool {
	return profile.Kind == uploadstate.RichProcessesProfile || profile.Kind == uploadstate.RichProcessesDockerProfile
}
